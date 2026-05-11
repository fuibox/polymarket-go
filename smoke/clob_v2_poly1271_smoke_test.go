//go:build smoke
// +build smoke

// POLY_1271 (deposit wallet) smoke test — produces a real ERC-7739-wrapped
// signed V2 order from a Polymarket deposit wallet and (optionally) posts it
// to the CLOB. Unlike clob_v2_smoke_test.go this test does not deploy or fund
// the wallet — the wallet must already be live (created via the relayer's
// WALLET-CREATE flow, with pUSD balance + V2 exchange allowance set).
//
// Run:
//
//	go test -tags smoke ./smoke -run TestClobV2_POLY1271 -v -timeout 5m
//
// Required env (shell or smoke/.env):
//
//	POLY_DEPOSIT_OWNER_PK         owner EOA private key (0x… or bare hex)
//
// Optional:
//
//	POLY_DEPOSIT_WALLET           expected wallet address; if set, must match
//	                              the address derived from the owner key
//	POLY_DEPOSIT_TOKEN_ID         token ID for the test order
//	POLY_DEPOSIT_POST_TO_SERVER   "1" to POST a real order. Default off —
//	                              the test only builds + structurally
//	                              verifies the signed order.
//	POLY_API_KEY, POLY_API_SECRET, POLY_API_PASSPHRASE
//	                              L2 api creds (required only when POSTing).
//	POLY_BUILDER_API_KEY/SECRET/PASSPHRASE
//	                              builder creds (required only when POSTing).
//	POLY_CLOB_HOST                default https://clob-v2.polymarket.com
//
// Server-side preconditions for a successful POST:
//   - wallet deployed (poll wallet.factory() on Polygon)
//   - pUSD balance funded
//   - pUSD allowance to ExchangeV2 (and NegRiskAdapterV2 for neg-risk markets)
//   - CLOB balance-allowance cache refreshed with signature_type=3
package smoke

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/shopspring/decimal"

	"github.com/fuibox/polymarket-go/client/clob"
	"github.com/fuibox/polymarket-go/client/clob/clob_types"
	"github.com/fuibox/polymarket-go/client/clob/order_builder"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/depositwallet"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
	"github.com/fuibox/polymarket-go/tools/headers"
)

func TestClobV2_POLY1271(t *testing.T) {
	if err := loadDotEnv(defaultEnvPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("load %s: %v", defaultEnvPath, err)
	}

	ownerPKHex := strings.TrimSpace(os.Getenv("POLY_DEPOSIT_OWNER_PK"))
	if ownerPKHex == "" {
		t.Skip("POLY_DEPOSIT_OWNER_PK not set — POLY_1271 smoke is opt-in")
	}
	ownerPKHex = strings.TrimPrefix(ownerPKHex, "0x")
	priv, err := crypto.HexToECDSA(ownerPKHex)
	if err != nil {
		t.Fatalf("POLY_DEPOSIT_OWNER_PK: %v", err)
	}
	ownerAddr := crypto.PubkeyToAddress(priv.PublicKey)

	derived, err := depositwallet.DeriveDepositWalletForChain(ownerAddr, 137)
	if err != nil {
		t.Fatalf("DeriveDepositWalletForChain: %v", err)
	}
	if expected := strings.TrimSpace(os.Getenv("POLY_DEPOSIT_WALLET")); expected != "" {
		if !strings.EqualFold(expected, derived.Hex()) {
			t.Fatalf("derived wallet %s != POLY_DEPOSIT_WALLET %s — owner key likely wrong",
				derived.Hex(), expected)
		}
	}
	t.Logf("owner EOA:      %s", ownerAddr.Hex())
	t.Logf("deposit wallet: %s", derived.Hex())

	sig, err := signer.NewSigner(signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		ChainID:          137,
		PrivateKeyConfig: &signer.PrivateKeyClient{PrivateKey: priv, Address: ownerAddr},
	})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	tokenID := strings.TrimSpace(os.Getenv("POLY_DEPOSIT_TOKEN_ID"))
	if tokenID == "" {
		tokenID = defaultTestTokenID
	}
	negRisk := false
	ts := types.TickSize001
	price, _ := decimal.NewFromString("0.01") // deepest legal bid — never fills
	size, _ := decimal.NewFromString("100")
	sigType := constants.POLY_1271

	options := clob_types.PartialCreateOrderOptions{
		TickSize:      &ts,
		NegRisk:       &negRisk,
		SignatureType: &sigType,
		DepositWallet: derived,
	}
	args := clob_types.OrderArgs{
		TokenID: tokenID,
		Price:   price,
		Size:    size,
		Side:    types.SideBuy,
	}

	postLive := strings.TrimSpace(os.Getenv("POLY_DEPOSIT_POST_TO_SERVER")) == "1"

	if !postLive {
		// Structural-only path: build via the order_builder directly so we
		// don't trigger ClobClient's network calls (tick / negRisk lookups).
		ob, err := order_builder.NewOrderBuilder(sig, constants.POLY_1271, derived)
		if err != nil {
			t.Fatalf("NewOrderBuilder: %v", err)
		}
		signedOrder, err := ob.CreateOrderV2(sig, args, options)
		if err != nil {
			t.Fatalf("CreateOrderV2: %v", err)
		}
		assertPOLY1271OrderShape(t, signedOrder, derived)
		t.Log("POLY_DEPOSIT_POST_TO_SERVER != 1 — structural checks done, skipping live POST")
		return
	}

	// Live POST path — exercises the full CreateAndPostOrder pipeline,
	// including the server's ERC-1271 verification of the wrapped signature.
	bc := &headers.BuilderConfig{
		APIKey:     os.Getenv("POLY_BUILDER_API_KEY"),
		Secret:     os.Getenv("POLY_BUILDER_SECRET"),
		Passphrase: os.Getenv("POLY_BUILDER_PASSPHRASE"),
	}
	if !bc.IsValid() {
		t.Fatalf("POLY_BUILDER_API_KEY/SECRET/PASSPHRASE incomplete (needed for live POST)")
	}
	apiKey := strings.TrimSpace(os.Getenv("POLY_API_KEY"))
	apiSecret := strings.TrimSpace(os.Getenv("POLY_API_SECRET"))
	apiPassphrase := strings.TrimSpace(os.Getenv("POLY_API_PASSPHRASE"))
	if apiKey == "" || apiSecret == "" || apiPassphrase == "" {
		t.Fatalf("POLY_API_KEY/SECRET/PASSPHRASE required when POLY_DEPOSIT_POST_TO_SERVER=1")
	}
	creds := &types.ApiKeyCreds{
		Key:        apiKey,
		Secret:     apiSecret,
		Passphrase: apiPassphrase,
	}

	clobHost := strings.TrimSpace(os.Getenv("POLY_CLOB_HOST"))
	if clobHost == "" {
		clobHost = defaultClobHost
	}

	clobClient, err := clob.NewClobClient(&clob.ClientConfig{
		Host:            clobHost,
		ChainID:         137,
		APIKey:          creds,
		Signer:          sig,
		BuilderConfig:   bc,
		ProtocolVersion: types.ProtocolVersionV2,
	})
	if err != nil {
		t.Fatalf("new clob client: %v", err)
	}

	sigTypeInt := int(constants.POLY_1271)
	if err := clobClient.UpdateBalanceAllowance(derived, &types.BalanceAllowanceParams{
		AssetType:     types.AssetTypeCollateral,
		SignatureType: &sigTypeInt,
	}); err != nil {
		t.Logf("WARNING: UpdateBalanceAllowance(collateral, sigtype=3): %v", err)
	}

	resp, err := clobClient.CreateAndPostOrder(args, options)
	if err != nil {
		t.Fatalf("CreateAndPostOrder: %v — wallet/allowance/balance preconditions likely missing", err)
	}
	t.Logf("POST /order: id=%s success=%v errMsg=%q", resp.OrderID, resp.Success, resp.ErrorMsg)
	if !resp.Success {
		t.Fatalf("server rejected POLY_1271 order: %s", resp.ErrorMsg)
	}

	if resp.OrderID != "" {
		if _, cancelErr := clobClient.CancelOrder(resp.OrderID, derived); cancelErr != nil {
			t.Logf("WARNING: cancel %s failed: %v", resp.OrderID, cancelErr)
		} else {
			t.Logf("cancelled %s", resp.OrderID)
		}
	}
}

func assertPOLY1271OrderShape(t *testing.T, signed types.SignedOrderV2, depositWallet common.Address) {
	t.Helper()
	if signed.SignatureType != types.SignatureType(constants.POLY_1271) {
		t.Errorf("SignatureType: want 3, got %d", signed.SignatureType)
	}
	if !strings.EqualFold(signed.Maker, depositWallet.Hex()) || !strings.EqualFold(signed.Signer, depositWallet.Hex()) {
		t.Errorf("maker/signer must both equal deposit wallet (maker=%s signer=%s)", signed.Maker, signed.Signer)
	}
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signed.Signature, "0x"))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if len(sigBytes) <= 65 {
		t.Fatalf("wrapped sig must exceed 65 bytes, got %d", len(sigBytes))
	}
	t.Logf("wrapped signature length: %d bytes (= 65 ECDSA + %d trailer)", len(sigBytes), len(sigBytes)-65)
}
