package relayer

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/relayer/builder"
	"github.com/fuibox/polymarket-go/client/relayer/model"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/shopspring/decimal"
)

// --- v2 collateral onramp (USDC.e ↔ pUSD) ---
//
// Real ABI (verified against on-chain source at the CollateralOnramp + pUSD
// addresses on Polygon):
//
//   CollateralOnramp.wrap(address asset, address to, uint256 amount)
//       — pulls `amount` of `asset` from msg.sender (must be approved) and
//         mints an equal amount of pUSD to `to`. Emits Wrapped(caller, asset, to, amount).
//
//   pUSD.unwrap(address asset, address to, uint256 amount, address callbackReceiver, bytes data)
//       — WRAPPER_ROLE only; ordinary users must unwrap via the Onramp's
//         public path (sig below is the likely public wrapper — re-verify
//         before first mainnet use).
//
// The original guess `wrap(uint256)` reverted on-chain because of selector
// mismatch; flagged in docs but kept here in the commit log for reference.

const (
	wrapFunctionSig = "wrap(address,address,uint256)"
	// Best-current-guess for Onramp's public unwrap — still unverified.
	// If unwrap reverts, re-read the Onramp ABI and update this.
	unwrapFunctionSig = "unwrap(address,address,uint256)"
)

func (c *RelayClient) encodeWrap(asset, to common.Address, amount *big.Int) (string, error) {
	return c.encodeAssetToAmountCall(wrapFunctionSig, asset, to, amount)
}

func (c *RelayClient) encodeUnwrap(asset, to common.Address, amount *big.Int) (string, error) {
	return c.encodeAssetToAmountCall(unwrapFunctionSig, asset, to, amount)
}

func (c *RelayClient) encodeAssetToAmountCall(signature string, asset, to common.Address, amount *big.Int) (string, error) {
	selector := c.functionSelector(signature)
	addressType, _ := abi.NewType("address", "", nil)
	uintType, _ := abi.NewType("uint256", "", nil)
	args := abi.Arguments{
		{Type: addressType},
		{Type: addressType},
		{Type: uintType},
	}
	encoded, err := args.Pack(asset, to, amount)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(append(selector, encoded...)), nil
}

// scaleUSDC converts a human-readable amount (e.g. 10.5) to 6-decimal integer
// (USDC & pUSD are both 6 decimals). Returns an error if the scaled integer
// would be zero.
func scaleUSDC(amount decimal.Decimal) (*big.Int, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("amount must be > 0, got %s", amount.String())
	}
	scaled := amount.Shift(6).Truncate(0).BigInt()
	if scaled.Sign() <= 0 {
		return nil, fmt.Errorf("amount too small after 6-decimal truncation: %s", amount.String())
	}
	return scaled, nil
}

// WrapCollateralWithPrivateKey converts USDC.e → pUSD via CollateralOnramp.
// Bundles two Safe ops: (1) approve onramp to spend USDC.e for `amount`,
// (2) call wrap(USDC.e, Safe, amount) on the onramp. Executes as a single
// multisend.
func (c *RelayClient) WrapCollateralWithPrivateKey(amount decimal.Decimal) (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.PrivateKey {
		return nil, errors.New("private key signer required")
	}
	pub, err := c.Signer.GetPubkeyOfPrivateKey()
	if err != nil {
		return nil, err
	}
	safe := builder.Derive(pub, c.ContractConfig.SafeFactory)
	return c.wrapTxns(safe, amount, func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithPrivateKey(txs, md)
	})
}

// WrapCollateralWithTurnkey is the Turnkey-signed variant of WrapCollateralWithPrivateKey.
func (c *RelayClient) WrapCollateralWithTurnkey(turnkeyAccount common.Address, amount decimal.Decimal) (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.Turnkey {
		return nil, errors.New("turnkey signer required")
	}
	if turnkeyAccount == constants.ZERO_ADDRESS {
		return nil, errors.New("turnkey account is required")
	}
	safe := builder.Derive(turnkeyAccount, c.ContractConfig.SafeFactory)
	return c.wrapTxns(safe, amount, func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithTurnkey(txs, md, turnkeyAccount)
	})
}

// UnwrapCollateralWithPrivateKey converts pUSD → USDC.e via CollateralOnramp.
// NOTE: the onramp's public unwrap signature is still assumed to be
// unwrap(address,address,uint256) — re-verify on first use.
func (c *RelayClient) UnwrapCollateralWithPrivateKey(amount decimal.Decimal) (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.PrivateKey {
		return nil, errors.New("private key signer required")
	}
	pub, err := c.Signer.GetPubkeyOfPrivateKey()
	if err != nil {
		return nil, err
	}
	safe := builder.Derive(pub, c.ContractConfig.SafeFactory)
	return c.unwrapTxns(safe, amount, func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithPrivateKey(txs, md)
	})
}

// UnwrapCollateralWithTurnkey is the Turnkey-signed variant of UnwrapCollateralWithPrivateKey.
func (c *RelayClient) UnwrapCollateralWithTurnkey(turnkeyAccount common.Address, amount decimal.Decimal) (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.Turnkey {
		return nil, errors.New("turnkey signer required")
	}
	if turnkeyAccount == constants.ZERO_ADDRESS {
		return nil, errors.New("turnkey account is required")
	}
	safe := builder.Derive(turnkeyAccount, c.ContractConfig.SafeFactory)
	return c.unwrapTxns(safe, amount, func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithTurnkey(txs, md, turnkeyAccount)
	})
}

type execFunc func(txs []model.SafeTransaction, metadata string) (*ClientRelayerTransactionResponse, error)

func (c *RelayClient) wrapTxns(safe common.Address, amount decimal.Decimal, exec execFunc) (*ClientRelayerTransactionResponse, error) {
	onramp := c.ContractConfig.CollateralOnramp
	if onramp == constants.ZERO_ADDRESS {
		return nil, fmt.Errorf("CollateralOnramp not configured for chain %d", c.ChainID)
	}
	scaled, err := scaleUSDC(amount)
	if err != nil {
		return nil, err
	}

	// Step 1: approve the onramp to pull USDC.e from the Safe.
	approveData, err := c.encodeApprove(onramp, scaled)
	if err != nil {
		return nil, fmt.Errorf("encode approve: %w", err)
	}
	// Step 2: call wrap(USDC.e, Safe, amount) — mints pUSD to the Safe.
	wrapData, err := c.encodeWrap(c.ContractConfig.Collateral, safe, scaled)
	if err != nil {
		return nil, fmt.Errorf("encode wrap: %w", err)
	}

	txs := []model.SafeTransaction{
		{To: c.ContractConfig.Collateral, Operation: model.Call, Data: approveData, Value: "0"},
		{To: onramp, Operation: model.Call, Data: wrapData, Value: "0"},
	}
	metadata := fmt.Sprintf("wrap %s USDC.e -> pUSD via onramp", amount.String())
	return exec(txs, metadata)
}

func (c *RelayClient) unwrapTxns(safe common.Address, amount decimal.Decimal, exec execFunc) (*ClientRelayerTransactionResponse, error) {
	onramp := c.ContractConfig.CollateralOnramp
	if onramp == constants.ZERO_ADDRESS {
		return nil, fmt.Errorf("CollateralOnramp not configured for chain %d", c.ChainID)
	}
	pusd := c.ContractConfig.PUSD
	if pusd == constants.ZERO_ADDRESS {
		return nil, fmt.Errorf("pUSD not configured for chain %d", c.ChainID)
	}
	scaled, err := scaleUSDC(amount)
	if err != nil {
		return nil, err
	}

	// Approve the onramp to pull pUSD from the Safe, then call unwrap.
	approveData, err := c.encodeApprove(onramp, scaled)
	if err != nil {
		return nil, fmt.Errorf("encode approve: %w", err)
	}
	// unwrap(USDC.e-asset-to-receive, Safe, amount) — returns USDC.e to Safe.
	unwrapData, err := c.encodeUnwrap(c.ContractConfig.Collateral, safe, scaled)
	if err != nil {
		return nil, fmt.Errorf("encode unwrap: %w", err)
	}

	txs := []model.SafeTransaction{
		{To: pusd, Operation: model.Call, Data: approveData, Value: "0"},
		{To: onramp, Operation: model.Call, Data: unwrapData, Value: "0"},
	}
	metadata := fmt.Sprintf("unwrap %s pUSD -> USDC.e via onramp", amount.String())
	return exec(txs, metadata)
}

// --- pUSD approvals for v2 exchanges ---

// pusdSpendersV2 returns the addresses that must be approved to spend pUSD
// from the Safe for trading on CLOB v2. Empty addresses (not yet configured
// for the target chain) are skipped.
//
// Set discovered empirically by calling GET /balance-allowance on the v2
// preprod server: the response's `allowances` map is keyed by these spenders,
// and POST /order rejects with "not enough balance / allowance" unless every
// one has a non-zero allowance.
func (c *RelayClient) pusdSpendersV2() []common.Address {
	out := make([]common.Address, 0, 4)
	if c.ContractConfig.ExchangeV2 != constants.ZERO_ADDRESS {
		out = append(out, c.ContractConfig.ExchangeV2)
	}
	if c.ContractConfig.NegExchangeV2 != constants.ZERO_ADDRESS {
		out = append(out, c.ContractConfig.NegExchangeV2)
	}
	if c.ContractConfig.NegRiskAdapterV2 != constants.ZERO_ADDRESS {
		out = append(out, c.ContractConfig.NegRiskAdapterV2)
	}
	// CollateralOnramp needs pUSD allowance for unwrap()s — not required by
	// the orderbook but part of the overall v2 funding flow.
	if c.ContractConfig.CollateralOnramp != constants.ZERO_ADDRESS {
		out = append(out, c.ContractConfig.CollateralOnramp)
	}
	return out
}

// ApprovePUSDForPolymarketWithPrivateKey grants unlimited pUSD allowance to
// the v2 exchanges + onramp from the Safe. Mirrors ApproveForPolymarketWithPrivateKey.
func (c *RelayClient) ApprovePUSDForPolymarketWithPrivateKey() (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.PrivateKey {
		return nil, errors.New("private key signer required")
	}
	return c.approvePUSD(func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithPrivateKey(txs, md)
	})
}

// ApprovePUSDForPolymarketWithTurnkey is the Turnkey-signed variant.
func (c *RelayClient) ApprovePUSDForPolymarketWithTurnkey(turnkeyAccount common.Address) (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.Turnkey {
		return nil, errors.New("turnkey signer required")
	}
	if turnkeyAccount == constants.ZERO_ADDRESS {
		return nil, errors.New("turnkey account is required")
	}
	return c.approvePUSD(func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithTurnkey(txs, md, turnkeyAccount)
	})
}

// ctfOperatorsV2 returns the v2 addresses that need `setApprovalForAll` on
// the CTF ERC-1155 contract. Empirically required by the v2 CLOB server — it
// returns HTTP 500 "could not run the execution" on any order (even BUY) if
// these approvals are absent.
func (c *RelayClient) ctfOperatorsV2() []common.Address {
	out := make([]common.Address, 0, 3)
	if c.ContractConfig.ExchangeV2 != constants.ZERO_ADDRESS {
		out = append(out, c.ContractConfig.ExchangeV2)
	}
	if c.ContractConfig.NegExchangeV2 != constants.ZERO_ADDRESS {
		out = append(out, c.ContractConfig.NegExchangeV2)
	}
	if c.ContractConfig.NegRiskAdapterV2 != constants.ZERO_ADDRESS {
		out = append(out, c.ContractConfig.NegRiskAdapterV2)
	}
	return out
}

// ApproveCTFForPolymarketV2WithPrivateKey grants ERC-1155 operator rights on
// the CTF contract to the v2 exchanges + NegRisk adapter. This must be
// called once per Safe before the v2 orderbook will accept ANY order from
// that Safe — including pure BUYs.
func (c *RelayClient) ApproveCTFForPolymarketV2WithPrivateKey() (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.PrivateKey {
		return nil, errors.New("private key signer required")
	}
	return c.approveCTFV2(func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithPrivateKey(txs, md)
	})
}

// ApproveCTFForPolymarketV2WithTurnkey is the Turnkey-signed variant.
func (c *RelayClient) ApproveCTFForPolymarketV2WithTurnkey(turnkeyAccount common.Address) (*ClientRelayerTransactionResponse, error) {
	if c.Signer == nil || c.Signer.SignerType() != signer.Turnkey {
		return nil, errors.New("turnkey signer required")
	}
	if turnkeyAccount == constants.ZERO_ADDRESS {
		return nil, errors.New("turnkey account is required")
	}
	return c.approveCTFV2(func(txs []model.SafeTransaction, md string) (*ClientRelayerTransactionResponse, error) {
		return c.ExecuteWithTurnkey(txs, md, turnkeyAccount)
	})
}

func (c *RelayClient) approveCTFV2(exec execFunc) (*ClientRelayerTransactionResponse, error) {
	ctf := c.ContractConfig.ConditionalTokens
	if ctf == constants.ZERO_ADDRESS {
		return nil, fmt.Errorf("ConditionalTokens not configured for chain %d", c.ChainID)
	}
	ops := c.ctfOperatorsV2()
	if len(ops) == 0 {
		return nil, fmt.Errorf("no v2 CTF operators configured for chain %d", c.ChainID)
	}
	txs := make([]model.SafeTransaction, 0, len(ops))
	for _, op := range ops {
		data, err := c.encodeSetApprovalForAll(op, true)
		if err != nil {
			return nil, err
		}
		txs = append(txs, model.SafeTransaction{
			To:        ctf,
			Operation: model.Call,
			Data:      data,
			Value:     "0",
		})
	}
	return exec(txs, "setApprovalForAll CTF to v2 polymarket contracts")
}

func (c *RelayClient) approvePUSD(exec execFunc) (*ClientRelayerTransactionResponse, error) {
	pusd := c.ContractConfig.PUSD
	if pusd == constants.ZERO_ADDRESS {
		return nil, fmt.Errorf("pUSD not configured for chain %d", c.ChainID)
	}
	spenders := c.pusdSpendersV2()
	if len(spenders) == 0 {
		return nil, fmt.Errorf("no v2 spenders configured for chain %d", c.ChainID)
	}

	maxUint := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	txs := make([]model.SafeTransaction, 0, len(spenders))
	for _, sp := range spenders {
		data, err := c.encodeApprove(sp, maxUint)
		if err != nil {
			return nil, err
		}
		txs = append(txs, model.SafeTransaction{
			To:        pusd,
			Operation: model.Call,
			Data:      data,
			Value:     "0",
		})
	}
	return exec(txs, "approve pUSD to v2 polymarket contracts")
}
