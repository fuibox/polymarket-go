package relayer

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/fuibox/polymarket-go/client/depositwallet"
	"github.com/fuibox/polymarket-go/client/relayer/model"
)

// Deposit wallet relayer flow:
//
//   1. CreateDepositWallet      — submit WALLET-CREATE, returns relayer txID;
//                                 poll until STATE_MINED/CONFIRMED.
//   2. GetDepositWalletNonce    — fetch the owner's WALLET nonce before
//                                 building a batch.
//   3. SubmitDepositWalletBatch — submit a signed WALLET batch; returns txID.
//   4. ExecuteDepositWalletBatch — orchestrates 2+sign+3 in one call, using
//                                 the configured Signer to produce the 65-byte
//                                 ECDSA signature over the Batch struct.
//
// All three POST methods reuse the Builder HMAC headers RelayClient already
// builds for SAFE-CREATE / SAFE submissions; the docs confirm WALLET / WALLET-
// CREATE submissions use the same auth layer.

// CreateDepositWallet submits a WALLET-CREATE transaction to the relayer. The
// payload contains no user signature — the relayer operator deploys the proxy
// using the deterministic factory salt = keccak256(abi.encode(factory,
// bytes32(owner))). Returns the relayer transaction ID; the caller can pass it
// to PollDepositWalletDeploy to wait for STATE_MINED.
func (c *RelayClient) CreateDepositWallet(owner common.Address) (*ClientRelayerTransactionResponse, error) {
	if owner == (common.Address{}) {
		return nil, errors.New("owner address is zero")
	}
	factory := c.ContractConfig.DepositWalletFactory
	if factory == (common.Address{}) {
		return nil, errors.New("deposit wallet factory not configured for this chain")
	}

	body := map[string]string{
		"type": string(model.TransactionTypeWalletCreate),
		"from": owner.Hex(),
		"to":   factory.Hex(),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.submitRaw(raw)
}

// GetDepositWalletNonce fetches the owner's current WALLET nonce — distinct
// from the SAFE nonce served by GetNonce(owner, "SAFE").
func (c *RelayClient) GetDepositWalletNonce(owner common.Address) (uint64, error) {
	return c.GetNonce(owner, "WALLET")
}

// SubmitDepositWalletBatch POSTs an already-signed Batch to the relayer.
// `signatureHex` must be a 0x-prefixed 65-byte ECDSA signature over the
// Batch EIP-712 digest from depositwallet.BatchDigest — NOT an ERC-7739
// wrapped signature.
func (c *RelayClient) SubmitDepositWalletBatch(
	owner common.Address,
	depositWallet common.Address,
	nonce uint64,
	deadline uint64,
	calls []depositwallet.Call,
	signatureHex string,
) (*ClientRelayerTransactionResponse, error) {
	if owner == (common.Address{}) || depositWallet == (common.Address{}) {
		return nil, errors.New("owner and depositWallet addresses are required")
	}
	factory := c.ContractConfig.DepositWalletFactory
	if factory == (common.Address{}) {
		return nil, errors.New("deposit wallet factory not configured for this chain")
	}
	if len(calls) == 0 {
		return nil, errors.New("batch must contain at least one call")
	}
	if !hasHexPrefix(signatureHex) {
		signatureHex = "0x" + signatureHex
	}
	// Wallet enforces 65 byte signature length (+ optional ERC-6492 session
	// signer suffix, which this path doesn't use).
	if rawLen := len(signatureHex) - 2; rawLen < 130 {
		return nil, fmt.Errorf("signature too short: %d hex chars, expected ≥130 (65 bytes)", rawLen)
	}

	body := map[string]any{
		"type":      string(model.TransactionTypeWallet),
		"from":      owner.Hex(),
		"to":        factory.Hex(),
		"nonce":     strconv.FormatUint(nonce, 10),
		"signature": signatureHex,
		"depositWalletParams": map[string]any{
			"depositWallet": depositWallet.Hex(),
			"deadline":      strconv.FormatUint(deadline, 10),
			"calls":         depositwallet.EncodeCallsForRelayer(calls),
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.submitRaw(raw)
}

// ExecuteDepositWalletBatch fetches the owner's WALLET nonce, signs the Batch
// with the configured private-key signer (Turnkey not yet supported here —
// callers using Turnkey should sign separately and call
// SubmitDepositWalletBatch), and submits.
func (c *RelayClient) ExecuteDepositWalletBatch(
	owner common.Address,
	depositWallet common.Address,
	deadline uint64,
	calls []depositwallet.Call,
) (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil {
		return nil, errors.New("signer is required")
	}
	nonce, err := c.GetDepositWalletNonce(owner)
	if err != nil {
		return nil, fmt.Errorf("get WALLET nonce: %w", err)
	}
	batch := depositwallet.Batch{
		Wallet:   depositWallet,
		Nonce:    nonce,
		Deadline: deadline,
		Calls:    calls,
	}
	sig, err := depositwallet.SignBatch(c.Signer, batch, int(c.ChainID))
	if err != nil {
		return nil, fmt.Errorf("sign batch: %w", err)
	}
	return c.SubmitDepositWalletBatch(owner, depositWallet, nonce, deadline, calls, sig)
}

// PollDepositWalletDeploy polls /transaction until the WALLET-CREATE reaches
// STATE_MINED or STATE_CONFIRMED, or the context is cancelled. Returns the
// final RelayerTransaction state.
func (c *RelayClient) PollDepositWalletDeploy(ctx context.Context, txID string, pollInterval time.Duration) (*RelayerTransaction, error) {
	return c.pollTransactionState(ctx, txID, pollInterval, map[string]struct{}{
		string(model.StateMined):     {},
		string(model.StateConfirmed): {},
	})
}

// pollTransactionState is a shared polling helper. Returns the latest
// transaction state once it enters one of `terminal`. Returns an error if the
// state enters STATE_FAILED or STATE_INVALID.
func (c *RelayClient) pollTransactionState(ctx context.Context, txID string, interval time.Duration, terminal map[string]struct{}) (*RelayerTransaction, error) {
	if txID == "" {
		return nil, errors.New("transaction id is empty")
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}
	for {
		txs, err := c.GetTransaction(txID)
		if err == nil && len(txs) > 0 {
			tx := txs[0]
			if _, ok := terminal[tx.State]; ok {
				return &tx, nil
			}
			if tx.State == string(model.StateFailed) || tx.State == string(model.StateInvalid) {
				return &tx, fmt.Errorf("relayer transaction %s ended in %s", txID, tx.State)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// submitRaw POSTs an already-serialized JSON body to /submit with the same
// Builder auth headers used by SAFE submissions. Distinct from the typed
// submit() helper because the WALLET and WALLET-CREATE bodies have fields
// (depositWalletParams) that the existing TransactionRequest struct does not
// model.
func (c *RelayClient) submitRaw(raw []byte) (*ClientRelayerTransactionResponse, error) {
	bodyStr := string(raw)
	builderHeaders, err := c.BuilderConfig.GenerateBuilderHeaders(
		"POST", SUBMIT_TRANSACTION, &bodyStr, "",
	)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", c.RelayerURL+SUBMIT_TRANSACTION, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("POLY_BUILDER_API_KEY", builderHeaders.POLYBuilderAPIKey)
	httpReq.Header.Set("POLY_BUILDER_TIMESTAMP", builderHeaders.POLYBuilderTimestamp)
	httpReq.Header.Set("POLY_BUILDER_PASSPHRASE", builderHeaders.POLYBuilderPassphrase)
	httpReq.Header.Set("POLY_BUILDER_SIGNATURE", builderHeaders.POLYBuilderSignature)

	resp, err := c.HttpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("relayer error %d: %s", resp.StatusCode, string(b))
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(respBytes) == 0 {
		return &ClientRelayerTransactionResponse{}, nil
	}
	var out struct {
		TransactionID   string `json:"transactionID"`
		TransactionHash string `json:"transactionHash"`
	}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, err
	}
	return &ClientRelayerTransactionResponse{
		TransactionID:   out.TransactionID,
		TransactionHash: out.TransactionHash,
	}, nil
}

func hasHexPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X')
}

// Compile-time assertion that hex is reachable from this package — avoids the
// "imported and not used" complaint when callers want to log raw bytes.
var _ = hex.EncodeToString
