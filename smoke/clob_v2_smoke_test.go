//go:build smoke
// +build smoke

// Package smoke holds end-to-end tests that touch real Polymarket endpoints
// and spend real funds. They are gated behind the `smoke` build tag so that
// plain `go test ./...` never runs them.
//
// This single test is a full bootstrap + trade loop:
//
//  1. Generate (or reload) a fresh EOA private key and derive its predicted
//     Polymarket Safe address. Save everything to ./smoke/.wallet.json
//     (gitignored). Re-runs reuse the same wallet.
//  2. Print the EOA + Safe address and wait for the user to fund the Safe
//     with USDC.e on Polygon mainnet (polls on-chain balance every 10 s,
//     30 minute cap).
//  3. Deploy the Safe via the Polymarket Relayer if not yet deployed.
//  4. If pUSD balance is short, wrap USDC.e → pUSD via CollateralOnramp.
//  5. Approve pUSD to the v2 exchanges if allowance is zero.
//  6. Derive an L2 API key (POLY_API_KEY/SECRET/PASSPHRASE) and persist it
//     back into .wallet.json so future runs skip derivation.
//  7. Place a GTC BUY at price = tick (deep OOTM → never fills), size 100,
//     ~ $1 outlay.
//  8. Cancel the order immediately.
//
// Run:
//
//	go test -tags smoke ./smoke -run TestClobV2_FullFlow -v -timeout 45m
//
// Env is loaded from ./smoke/.env (KEY=VALUE per line) if present; env vars
// already set in the shell win over the file.
//
// Required:
//
//	POLY_BUILDER_API_KEY
//	POLY_BUILDER_SECRET
//	POLY_BUILDER_PASSPHRASE
//
// Optional:
//
//	POLY_RELAYER_URL   (default https://relayer-v2.polymarket.com — official)
//	POLY_CLOB_HOST     (default https://clob-v2.polymarket.com)
//	POLY_POLYGON_RPC   (default https://polygon-bor-rpc.publicnode.com)
//	POLY_TOKEN_ID      (default: first v2-migration "movie" test-market token)
//	POLY_TARGET_PUSD   (default 5 — target pUSD on Safe after wrap)
//	POLY_MARKET_ORDER_USD (default 4.5 — USD spend cap for stage-8 market BUY)
//	POLY_WALLET_PATH   (default ./smoke/.wallet.json — same file, absolute name)
//	POLY_FUND_WAIT     (default 30m — how long to wait for USDC.e to arrive)
//
// Safety: price is pinned to the tick (deepest legal bid), so the order will
// never fill. Any unexpected error aborts before anything else executes.
// CollateralOnramp ABI is the un-verified wrap(uint256)/unwrap(uint256) shape.
package smoke

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	cryptohmac "crypto/hmac"
	"io"
	"math/big"
	nethttp "net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/shopspring/decimal"

	"github.com/fuibox/polymarket-go/client/clob"
	"github.com/fuibox/polymarket-go/client/clob/clob_types"
	"github.com/fuibox/polymarket-go/client/config"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/relayer"
	"github.com/fuibox/polymarket-go/client/relayer/builder"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
	"github.com/fuibox/polymarket-go/tools/headers"
)

const (
	defaultClobHost    = "https://clob-v2.polymarket.com"
	defaultRelayerURL  = "https://relayer-v2.polymarket.com"
	defaultPolygonRPC  = "https://polygon-bor-rpc.publicnode.com"
	defaultTestTokenID = "81662326158871781857247725348568394697379926716334270967994039975048021832777"
	// Paths are relative to the test package dir (go test's CWD is the
	// package, i.e. ./smoke).
	defaultWalletPath = ".wallet.json"
	defaultEnvPath    = ".env"
	defaultFundWait    = "30m"
	defaultTargetPUSD  = "5"
	// Market-order spend cap: $4.5 = 5 tokens at ask price 0.9 (nuclear deal
	// token's current top of book). Min server-side is $1.
	defaultMarketOrderUSD = "4.5"
)

const erc20ABI = `[
  {"constant":true,"inputs":[{"name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"},
  {"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"}
]`

// v2TestMarketTokens are the seeded-liquidity tokens from
// https://docs.polymarket.com/v2-migration#test-markets — 6 outcome tokens
// for "Highest Grossing Movie in 2026?" plus 1 for "US / Iran Nuclear Deal
// in 2027?". The test iterates these and auto-picks whichever has enough
// ask-side liquidity to satisfy the server's $1 min-order-size rule.
var v2TestMarketTokens = []string{
	// Highest Grossing Movie in 2026 (6 outcomes)
	"81662326158871781857247725348568394697379926716334270967994039975048021832777",
	"17546146554206665273662853938002443411871542020107489725107067382874986311707",
	"28161183422242370392388296744035422249088647252796713903067039294971789722479",
	"89576274136595202327975910079635847102293810595609428633134997662847357374694",
	"21556669163785052148858748369786715040594704429426952390307023288865165566607",
	"51020513216536535954567404775362000484668846352577848437115610667663875702516",
	// US / Iran Nuclear Deal in 2027
	"102936224134271070189104847090829839924697394514566827387181305960175107677216",
}

// wallet is the persisted smoke-test state. File permissions are enforced to
// 0600 on disk; this is test-only material but it still holds a private key.
type wallet struct {
	PrivateKey    string    `json:"private_key"`     // 0x-prefixed hex
	EOAAddress    string    `json:"eoa_address"`     // checksum hex
	SafeAddress   string    `json:"safe_address"`    // predicted via SafeFactory CREATE2
	APIKey        string    `json:"api_key,omitempty"`
	APISecret     string    `json:"api_secret,omitempty"`
	APIPassphrase string    `json:"api_passphrase,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func TestClobV2_FullFlow(t *testing.T) {
	env := loadEnv(t)
	cc, err := config.GetContractConfig(137)
	if err != nil {
		t.Fatalf("get contract config: %v", err)
	}

	t.Logf("======================================================================")
	t.Logf("Polymarket CLOB v2 smoke test — full bootstrap + trade loop")
	t.Logf("======================================================================")

	// --- Stage 1: wallet ---

	w, freshlyGenerated, err := loadOrCreateWallet(env.WalletPath, cc.SafeFactory)
	if err != nil {
		t.Fatalf("wallet: %v", err)
	}
	if freshlyGenerated {
		t.Logf("[NEW] generated wallet at %s", env.WalletPath)
	} else {
		t.Logf("[REUSE] loaded wallet from %s", env.WalletPath)
	}
	t.Logf("EOA  (signer):  %s", w.EOAAddress)
	t.Logf("Safe (funder):  %s", w.SafeAddress)
	t.Logf("")
	t.Logf(">>> ACTION REQUIRED:")
	t.Logf("    Send at least %s USDC.e (%s) to the Safe address above.", env.TargetPUSD.String(), cc.Collateral.Hex())
	t.Logf("    (EOA does NOT need MATIC — relayer pays gas.)")
	t.Logf("    Then leave this test running; it polls every 10s.")
	t.Logf("")

	// --- Stage 2: funding wait ---

	required := decimalToBase6(env.TargetPUSD)
	if strings.EqualFold(env.Side, "SELL") {
		// SELL doesn't need pUSD — we need CTF tokens to sell. Just check the
		// Safe is funded at all; otherwise skip the collateral wait.
		t.Logf("--- stage 2: funding wait (SELL-mode, skipping pUSD check) ---")
	} else if err := waitForCollateral(t.Context(), env.PolygonRPC, cc.Collateral, cc.PUSD, common.HexToAddress(w.SafeAddress), required, env.FundWait); err != nil {
		t.Fatalf("funding wait: %v", err)
	}

	// --- Build signer + clients ---

	priv, err := crypto.HexToECDSA(strings.TrimPrefix(w.PrivateKey, "0x"))
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	eoa := crypto.PubkeyToAddress(priv.PublicKey)
	sig, err := signer.NewSigner(signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		ChainID:          137,
		PrivateKeyConfig: &signer.PrivateKeyClient{PrivateKey: priv, Address: eoa},
	})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	relay, err := relayer.NewRelayClient(env.RelayerURL, 137, sig, env.BuilderConfig, nil, &env.PolygonRPC)
	if err != nil {
		t.Fatalf("new relay client: %v", err)
	}

	// --- Stage 3: deploy Safe if needed ---

	safeAddr := common.HexToAddress(w.SafeAddress)
	deployed, err := relay.IsDeployed(safeAddr)
	if err != nil {
		t.Fatalf("IsDeployed: %v", err)
	}
	if !deployed {
		t.Logf("--- stage 3: deploy Safe ---")
		resp, err := relay.DeployWithPrivateKey()
		if err != nil {
			t.Fatalf("DeployWithPrivateKey: %v", err)
		}
		t.Logf("deploy submitted: hash=%s id=%s", resp.TransactionHash, resp.TransactionID)
		if err := waitUntilDeployed(t, relay, safeAddr, 3*time.Minute); err != nil {
			t.Fatalf("wait for deploy: %v", err)
		}
		t.Logf("Safe deployed.")
	} else {
		t.Logf("--- stage 3: Safe already deployed, skip ---")
	}

	// --- Stage 4: wrap USDC.e → pUSD if short (BUY only) ---

	if strings.EqualFold(env.Side, "SELL") {
		t.Logf("--- stage 4: wrap (skip for SELL — no pUSD needed) ---")
	} else {
		t.Logf("--- stage 4: wrap ---")
		pusdBal, err := erc20BalanceOf(env.PolygonRPC, cc.PUSD, safeAddr)
		if err != nil {
			t.Fatalf("pUSD balanceOf: %v", err)
		}
		t.Logf("pUSD balance (6-dec) = %s  target = %s", pusdBal.String(), required.String())
		if pusdBal.Cmp(required) < 0 {
			// Wrap only the delta — avoids consuming more USDC.e than needed on
			// reruns where some pUSD is already on the Safe.
			deltaBase6 := new(big.Int).Sub(required, pusdBal)
			delta := decimal.NewFromBigInt(deltaBase6, -6) // 6 → human-readable decimals
			t.Logf("wrapping %s USDC.e → pUSD (delta, to top up to %s)", delta.String(), env.TargetPUSD.String())
			resp, err := relay.WrapCollateralWithPrivateKey(delta)
			if err != nil {
				t.Fatalf("wrap: %v", err)
			}
			t.Logf("wrap tx submitted: hash=%s id=%s", resp.TransactionHash, resp.TransactionID)
			if err := waitUntilPUSDAtLeast(t, env.PolygonRPC, cc.PUSD, safeAddr, required, 3*time.Minute); err != nil {
				t.Fatalf("wait for wrap: %v", err)
			}
		} else {
			t.Logf("pUSD already sufficient, skip wrap")
		}
	}

	// --- Stage 5: approve pUSD to v2 exchanges ---

	t.Logf("--- stage 5: approve pUSD ---")
	// The v2 orderbook checks pUSD allowance to THREE spenders; missing any
	// one → "not enough balance / allowance" on POST /order.
	v2Spenders := []struct {
		name string
		addr common.Address
	}{
		{"ExchangeV2", cc.ExchangeV2},
		{"NegExchangeV2", cc.NegExchangeV2},
		{"NegRiskAdapterV2", cc.NegRiskAdapterV2},
	}
	needApprove := false
	for _, sp := range v2Spenders {
		a, err := erc20Allowance(env.PolygonRPC, cc.PUSD, safeAddr, sp.addr)
		if err != nil {
			t.Fatalf("allowance(%s): %v", sp.name, err)
		}
		t.Logf("pUSD allowance → %-16s = %s", sp.name, a.String())
		if a.Sign() == 0 {
			needApprove = true
		}
	}
	if needApprove {
		resp, err := relay.ApprovePUSDForPolymarketWithPrivateKey()
		if err != nil {
			t.Fatalf("approve pUSD: %v", err)
		}
		t.Logf("approve tx submitted: hash=%s id=%s", resp.TransactionHash, resp.TransactionID)
		addrs := make([]common.Address, 0, len(v2Spenders))
		for _, sp := range v2Spenders {
			addrs = append(addrs, sp.addr)
		}
		if err := waitUntilAllowanceNonZero(t, env.PolygonRPC, cc.PUSD, safeAddr, addrs, 3*time.Minute); err != nil {
			t.Fatalf("wait for approve: %v", err)
		}
		t.Logf("approve confirmed on-chain")
	} else {
		t.Logf("all allowances already set, skip approve")
	}

	// --- Stage 5b: setApprovalForAll on CTF to the v2 spenders ---
	//
	// Empirical requirement: the v2 CLOB server returns HTTP 500 "could not
	// run the execution" on any order (even BUY) if the Safe hasn't granted
	// ERC-1155 setApprovalForAll on the CTF contract to the v2 exchanges +
	// NegRisk adapter. This is separate from pUSD ERC-20 approvals.

	t.Logf("--- stage 5b: CTF ERC-1155 setApprovalForAll ---")
	ctfOps := []struct {
		name string
		addr common.Address
	}{
		{"ExchangeV2", cc.ExchangeV2},
		{"NegExchangeV2", cc.NegExchangeV2},
		{"NegRiskAdapterV2", cc.NegRiskAdapterV2},
	}
	needCTFApprove := false
	for _, op := range ctfOps {
		approved, err := erc1155IsApprovedForAll(env.PolygonRPC, cc.ConditionalTokens, safeAddr, op.addr)
		if err != nil {
			t.Fatalf("isApprovedForAll(%s): %v", op.name, err)
		}
		status := "no"
		if approved {
			status = "yes"
		}
		t.Logf("CTF setApprovalForAll → %-16s = %s", op.name, status)
		if !approved {
			needCTFApprove = true
		}
	}
	if needCTFApprove {
		resp, err := relay.ApproveCTFForPolymarketV2WithPrivateKey()
		if err != nil {
			t.Fatalf("ApproveCTFForPolymarketV2: %v", err)
		}
		t.Logf("CTF approve tx submitted: hash=%s id=%s", resp.TransactionHash, resp.TransactionID)
		// Wait until all 3 approvals reflect on chain.
		deadline := time.Now().Add(3 * time.Minute)
		for time.Now().Before(deadline) {
			allOK := true
			for _, op := range ctfOps {
				ok, err := erc1155IsApprovedForAll(env.PolygonRPC, cc.ConditionalTokens, safeAddr, op.addr)
				if err != nil {
					t.Fatalf("post-approve isApprovedForAll: %v", err)
				}
				if !ok {
					allOK = false
					break
				}
			}
			if allOK {
				t.Logf("CTF approvals confirmed on-chain")
				break
			}
			time.Sleep(10 * time.Second)
		}
	} else {
		t.Logf("all CTF operator approvals already set, skip")
	}

	// --- Build CLOB client (no creds yet) ---

	noCredClient, err := clob.NewClobClient(&clob.ClientConfig{
		Host:            env.ClobHost,
		ChainID:         137,
		Signer:          sig,
		BuilderConfig:   env.BuilderConfig,
		ProtocolVersion: types.ProtocolVersionV2,
	})
	if err != nil {
		t.Fatalf("new clob client (no creds): %v", err)
	}

	// --- Stage 6: derive API key ---

	t.Logf("--- stage 6: derive API key ---")
	var creds *types.ApiKeyCreds
	if w.APIKey != "" && w.APISecret != "" && w.APIPassphrase != "" {
		t.Logf("reusing API key from wallet file")
		creds = &types.ApiKeyCreds{Key: w.APIKey, Secret: w.APISecret, Passphrase: w.APIPassphrase}
	} else {
		creds, err = noCredClient.CreateOrDeriveApiKey(nil, clob_types.ClobOption{SafeAccount: safeAddr})
		if err != nil {
			t.Fatalf("CreateOrDeriveApiKey: %v", err)
		}
		w.APIKey = creds.Key
		w.APISecret = creds.Secret
		w.APIPassphrase = creds.Passphrase
		w.UpdatedAt = time.Now().UTC()
		if err := saveWallet(env.WalletPath, w); err != nil {
			t.Fatalf("persist api key: %v", err)
		}
		t.Logf("API key derived and saved to %s", env.WalletPath)
	}

	// Full-cred client for trading.
	clobClient, err := clob.NewClobClient(&clob.ClientConfig{
		Host:            env.ClobHost,
		ChainID:         137,
		APIKey:          creds,
		Signer:          sig,
		BuilderConfig:   env.BuilderConfig,
		ProtocolVersion: types.ProtocolVersionV2,
	})
	if err != nil {
		t.Fatalf("new clob client (with creds): %v", err)
	}

	// --- Stage 7b: refresh server-side balance/allowance cache.
	//
	// Critical detail: the server needs BOTH
	//   (a) POLY_ADDRESS = EOA (api-key owner) — set via the funder arg below
	//   (b) signature_type in the query — without it the server assumes EOA
	//       wallet and returns empty balance / "not enough balance / allowance"
	// Value 2 = POLY_GNOSIS_SAFE (Polymarket proxy / Safe user).

	sigType := int(constants.POLY_GNOSIS_SAFE)
	t.Logf("--- stage 7b: refresh server-side balance/allowance cache ---")
	if err := clobClient.UpdateBalanceAllowance(eoa, &types.BalanceAllowanceParams{AssetType: types.AssetTypeCollateral, SignatureType: &sigType}); err != nil {
		t.Logf("WARNING: UpdateBalanceAllowance(collateral) failed: %v", err)
	}
	if err := clobClient.UpdateBalanceAllowance(eoa, &types.BalanceAllowanceParams{AssetType: types.AssetTypeConditional, TokenID: &env.TokenID, SignatureType: &sigType}); err != nil {
		t.Logf("WARNING: UpdateBalanceAllowance(conditional) failed: %v", err)
	}
	// Debug: log what the server thinks we have — first via the typed SDK,
	// then via a raw HTTP call to capture the full JSON shape in case v2
	// added fields or renamed them.
	if ba, err := clobClient.GetBalanceAllowance(eoa, &types.BalanceAllowanceParams{AssetType: types.AssetTypeCollateral, SignatureType: &sigType}); err != nil {
		t.Logf("WARNING: GetBalanceAllowance(collateral): %v", err)
	} else {
		t.Logf("server-side view (typed): collateral balance=%s allowance=%s", ba.Balance, ba.Allowance)
	}
	if raw, err := rawBalanceAllowance(env.ClobHost, eoa, creds, types.AssetTypeCollateral, nil, sigType); err != nil {
		t.Logf("WARNING: rawBalanceAllowance(collateral): %v", err)
	} else {
		t.Logf("server-side view (raw collateral JSON): %s", raw)
	}
	if raw, err := rawBalanceAllowance(env.ClobHost, eoa, creds, types.AssetTypeConditional, &env.TokenID, sigType); err != nil {
		t.Logf("WARNING: rawBalanceAllowance(conditional): %v", err)
	} else {
		t.Logf("server-side view (raw conditional JSON): %s", raw)
	}

	// --- Stage 7c: orderbook snapshot + liquidity probe ---
	//
	// A market BUY matches against existing SELL orders (asks). Server rule:
	// each actual fill must be >= $1 (min-order-size), so a token whose total
	// ask notional is < $1 is unplaceable for a $1 market buy. We probe all
	// v2-migration candidate tokens and pick whichever has enough ask depth
	// at or below our price cap.

	minFill, _ := decimal.NewFromString("1.0") // $1 server-side min order size
	isSell := strings.EqualFold(env.Side, "SELL")

	var bestTopPrice decimal.Decimal
	if isSell {
		// For SELL we must stick with env.TokenID — we can only sell a token we
		// actually hold. Probe its top BID; fail fast if no liquidity.
		t.Logf("--- stage 7c: orderbook snapshot (SELL-mode, bids probe) ---")
		book, err := clobClient.GetOrderBook(env.TokenID)
		if err != nil {
			t.Fatalf("GetOrderBook: %v", err)
		}
		notional, topBidPrice, topBidSize := bidNotionalAtOrAbove(book.Bids, decimal.Zero)
		t.Logf("token %s… bids=%d asks=%d  top-bid=%s @ %s  bid-notional=%s",
			env.TokenID[:12], len(book.Bids), len(book.Asks), topBidSize, topBidPrice, notional.String())
		if notional.LessThan(minFill) {
			t.Fatalf("insufficient bid-side liquidity ($%s) for a SELL — need ≥ $%s. Try again later.",
				notional.String(), minFill.String())
		}
		if p, perr := decimal.NewFromString(topBidPrice); perr == nil {
			bestTopPrice = p
		}
		t.Logf("using tokenID: %s  (bid notional: $%s, top bid at %s)",
			env.TokenID, notional.String(), bestTopPrice.String())
	} else {
		// BUY mode: probe asks across candidates, auto-switch if needed.
		t.Logf("--- stage 7c: orderbook snapshot + candidate probe (BUY-mode, asks probe) ---")
		priceCap, _ := decimal.NewFromString("0.99")
		candidates := []string{env.TokenID}
		for _, tid := range v2TestMarketTokens {
			if tid != env.TokenID {
				candidates = append(candidates, tid)
			}
		}
		bestTokenID := ""
		bestNotional := decimal.Zero
		for _, tid := range candidates {
			book, err := clobClient.GetOrderBook(tid)
			if err != nil {
				t.Logf("  token %s…: GetOrderBook err=%v", tid[:12], err)
				continue
			}
			notional, topAskPrice, topAskSize := askNotionalAtOrBelow(book.Asks, priceCap)
			marker := "  "
			if notional.GreaterThanOrEqual(minFill) && bestTokenID == "" {
				marker = "✓ "
				bestTokenID = tid
				bestNotional = notional
				if p, perr := decimal.NewFromString(topAskPrice); perr == nil {
					bestTopPrice = p
				}
			}
			t.Logf("%stoken %s… bids=%d asks=%d  top-ask=%s @ %s  ask-notional@≤%s=%s",
				marker, tid[:12], len(book.Bids), len(book.Asks),
				topAskSize, topAskPrice, priceCap.String(), notional.String())
		}
		if bestTokenID == "" {
			t.Fatalf("no candidate token has ≥ $%s ask notional at or below cap %s — orderbook too thin",
				minFill.String(), priceCap.String())
		}
		if bestTokenID != env.TokenID {
			t.Logf("switching tokenID: %s → %s (insufficient ask liquidity)", env.TokenID, bestTokenID)
			env.TokenID = bestTokenID
		}
		t.Logf("using tokenID: %s  (ask notional: $%s, top ask at %s)",
			env.TokenID, bestNotional.String(), bestTopPrice.String())
	}

	// --- Stage 7d: fetch metadata for the FINAL token ---
	//
	// Must be done AFTER 7c — the signature's EIP-712 domain depends on
	// negRisk (Exchange vs NegExchange address) and the price/size rounding
	// depends on tick. Using stale values from a different token causes
	// "invalid signature" on POST /order.

	t.Logf("--- stage 7d: market metadata (final tokenID=%s…) ---", env.TokenID[:12])
	tick, err := clobClient.GetTickSize(env.TokenID)
	if err != nil {
		t.Fatalf("GetTickSize: %v", err)
	}
	negRisk, err := clobClient.GetNegRisk(env.TokenID)
	if err != nil {
		t.Fatalf("GetNegRisk: %v", err)
	}
	t.Logf("tick=%s neg_risk=%v", tick, negRisk)

	// --- Stage 8: place LIMIT GTC order (BUY or SELL) ---
	//
	// BUY:  price = top_ask + tick   (strictly above ask → crosses)
	// SELL: price = top_bid - tick   (strictly below bid → crosses)
	// Crossing limit orders settle immediately on-chain against the existing
	// counter-side without leaving a resting order behind.

	tickDec, err := decimal.NewFromString(string(tick))
	if err != nil {
		t.Fatalf("parse tick %q: %v", tick, err)
	}
	upper := decimal.NewFromInt(1).Sub(tickDec)

	var orderPrice decimal.Decimal
	var orderSide types.Side
	var orderSize decimal.Decimal
	if isSell {
		orderSide = types.SideSell
		orderPrice = bestTopPrice.Sub(tickDec)
		if orderPrice.LessThan(tickDec) {
			orderPrice = tickDec // clamp to min
		}
		// Sell all CTF we currently hold (Polymarket CTF is 6-decimal).
		ctfBal, err := ctfBalanceOf(env.PolygonRPC, cc.ConditionalTokens, safeAddr, env.TokenID)
		if err != nil {
			t.Fatalf("pre-sell CTF balance: %v", err)
		}
		if ctfBal.Sign() == 0 {
			t.Fatalf("CTF balance for tokenID %s is 0 — nothing to sell", env.TokenID)
		}
		orderSize = decimal.NewFromBigInt(ctfBal, -6)
	} else {
		orderSide = types.SideBuy
		orderPrice = bestTopPrice.Add(tickDec)
		if orderPrice.GreaterThan(upper) {
			orderPrice = upper
		}
		orderSize = decimal.NewFromInt(5)
	}

	t.Logf("--- stage 8: place LIMIT GTC %s, size=%s @ price=%s (top %s + tick %s) ---",
		orderSide, orderSize.String(), orderPrice.String(), bestTopPrice.String(), tickDec.String())
	pUSDBefore, _ := erc20BalanceOf(env.PolygonRPC, cc.PUSD, safeAddr)
	ctfBefore, _ := ctfBalanceOf(env.PolygonRPC, cc.ConditionalTokens, safeAddr, env.TokenID)
	t.Logf("pre-trade: pUSD=%s  CTF[tokenID]=%s", pUSDBefore.String(), ctfBefore.String())

	args := clob_types.OrderArgs{
		TokenID: env.TokenID,
		Price:   orderPrice,
		Size:    orderSize,
		Side:    orderSide,
	}
	opts := clob_types.PartialCreateOrderOptions{
		OrderType:   types.OrderTypeGTC,
		TickSize:    &tick,
		NegRisk:     &negRisk,
		SafeAccount: safeAddr,
	}
	resp, err := clobClient.CreateAndPostOrder(args, opts)
	if err != nil {
		t.Fatalf("CreateAndPostOrder: %v", err)
	}
	t.Logf("order response: id=%s status=%s success=%v errorMsg=%q making=%s taking=%s",
		resp.OrderID, resp.Status, resp.Success, resp.ErrorMsg, resp.MakingAmount, resp.TakingAmount)

	// If this is a GTC and it sits as an open order, we need to cancel it
	// at end of test — otherwise it'll fill at 0.91 against a future ask.
	defer func() {
		if resp != nil && resp.OrderID != "" {
			t.Logf("--- cleanup: cancel leftover GTC order %s ---", resp.OrderID)
			if _, cerr := clobClient.CancelOrder(resp.OrderID, eoa); cerr != nil {
				t.Logf("cancel err (non-fatal): %v", cerr)
			}
		}
	}()

	// --- Stage 10: verify the fill on-chain ---
	//
	// A market FAK that successfully filled will have:
	//   - pUSD balance DROP by the filled pUSD amount
	//   - CTF balance for this tokenID INCREASE by the tokens received
	// If the orderbook was empty / all asks above our price cap, the order
	// gets killed with zero fill — no on-chain change.
	//
	// The trade itself settles via the Exchange contract and IS on-chain,
	// unlike the limit order variant that only lived in the orderbook.

	t.Logf("--- stage 10: verify on-chain fill ---")

	// Re-query the order first — helps distinguish "filled" / "matched" /
	// "unmatched" (server-side state).
	if ord, err := clobClient.GetOrder(eoa, resp.OrderID); err != nil {
		t.Logf("GetOrder(%s): %v  (empty body = order consumed / not found, expected for FAK)", resp.OrderID, err)
	} else {
		t.Logf("order %s state: status=%s size_matched=%s original_size=%s",
			ord.ID, ord.Status, ord.SizeMatched, ord.OriginalSize)
	}

	// Give the chain a few seconds to settle the fill transaction.
	time.Sleep(8 * time.Second)

	pUSDAfter, err := erc20BalanceOf(env.PolygonRPC, cc.PUSD, safeAddr)
	if err != nil {
		t.Fatalf("post-trade pUSD balanceOf: %v", err)
	}
	ctfAfter, err := ctfBalanceOf(env.PolygonRPC, cc.ConditionalTokens, safeAddr, env.TokenID)
	if err != nil {
		t.Fatalf("post-trade CTF balanceOf: %v", err)
	}

	t.Logf("post-trade: pUSD=%s  CTF[tokenID]=%s", pUSDAfter.String(), ctfAfter.String())

	if isSell {
		pUSDGained := new(big.Int).Sub(pUSDAfter, pUSDBefore)
		ctfSold := new(big.Int).Sub(ctfBefore, ctfAfter)
		t.Logf("delta     : pUSD=+%s  CTF[tokenID]=-%s", pUSDGained.String(), ctfSold.String())
		if pUSDGained.Sign() > 0 && ctfSold.Sign() > 0 {
			t.Logf("✓ SELL order FILLED on-chain — sold %s tokens for %s pUSD",
				ctfSold.String(), pUSDGained.String())
		} else if pUSDGained.Sign() == 0 && ctfSold.Sign() == 0 {
			t.Logf("✗ NO FILL — no bid crossed our sell price. Balances unchanged.")
		} else {
			t.Errorf("⚠ unexpected balance change: pUSD_delta=+%s  ctf_delta=-%s", pUSDGained.String(), ctfSold.String())
		}
	} else {
		pUSDSpent := new(big.Int).Sub(pUSDBefore, pUSDAfter)
		ctfGained := new(big.Int).Sub(ctfAfter, ctfBefore)
		t.Logf("delta     : pUSD=-%s  CTF[tokenID]=+%s", pUSDSpent.String(), ctfGained.String())
		if pUSDSpent.Sign() > 0 && ctfGained.Sign() > 0 {
			t.Logf("✓ BUY order FILLED on-chain — paid %s pUSD for %s tokens",
				pUSDSpent.String(), ctfGained.String())
		} else if pUSDSpent.Sign() == 0 && ctfGained.Sign() == 0 {
			t.Logf("✗ NO FILL — orderbook empty at our price cap; order was killed. Balances unchanged.")
		} else {
			t.Errorf("⚠ unexpected balance change: pUSD_delta=-%s  ctf_delta=+%s", pUSDSpent.String(), ctfGained.String())
		}
	}

	t.Logf("======================================================================")
	t.Logf("  ✓ smoke test COMPLETED")
	t.Logf("  credentials persisted to %s", env.WalletPath)
	t.Logf("======================================================================")
}

// --- env loading ---

type smokeEnv struct {
	BuilderConfig  *headers.BuilderConfig
	RelayerURL     string
	ClobHost       string
	PolygonRPC     string
	TokenID        string
	Side           string // "BUY" (default) or "SELL"
	TargetPUSD     decimal.Decimal
	MarketOrderUSD decimal.Decimal
	WalletPath     string
	FundWait       time.Duration
}

func loadEnv(t *testing.T) *smokeEnv {
	t.Helper()

	// Best-effort: load ./smoke/.env into the process environment. Shell vars
	// that are already set win over the file.
	if err := loadDotEnv(defaultEnvPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("load %s: %v", defaultEnvPath, err)
	}

	req := func(k string) string {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			t.Fatalf("env var %s is required (set in shell or smoke/.env)", k)
		}
		return v
	}
	opt := func(k, def string) string {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def
		}
		return v
	}

	bc := &headers.BuilderConfig{
		APIKey:     req("POLY_BUILDER_API_KEY"),
		Secret:     req("POLY_BUILDER_SECRET"),
		Passphrase: req("POLY_BUILDER_PASSPHRASE"),
	}
	if !bc.IsValid() {
		t.Fatalf("POLY_BUILDER_API_KEY/SECRET/PASSPHRASE incomplete")
	}

	target, err := decimal.NewFromString(opt("POLY_TARGET_PUSD", defaultTargetPUSD))
	if err != nil {
		t.Fatalf("POLY_TARGET_PUSD invalid: %v", err)
	}
	if target.LessThanOrEqual(decimal.Zero) {
		t.Fatalf("POLY_TARGET_PUSD must be > 0")
	}

	orderUSD, err := decimal.NewFromString(opt("POLY_MARKET_ORDER_USD", defaultMarketOrderUSD))
	if err != nil {
		t.Fatalf("POLY_MARKET_ORDER_USD invalid: %v", err)
	}
	if orderUSD.LessThan(decimal.NewFromInt(1)) {
		t.Fatalf("POLY_MARKET_ORDER_USD must be >= 1 (server min-order-size)")
	}

	waitDur, err := time.ParseDuration(opt("POLY_FUND_WAIT", defaultFundWait))
	if err != nil {
		t.Fatalf("POLY_FUND_WAIT invalid: %v", err)
	}

	side := strings.ToUpper(opt("POLY_SIDE", "BUY"))
	if side != "BUY" && side != "SELL" {
		t.Fatalf("POLY_SIDE must be BUY or SELL, got %q", side)
	}

	return &smokeEnv{
		BuilderConfig:  bc,
		RelayerURL:     opt("POLY_RELAYER_URL", defaultRelayerURL),
		ClobHost:       opt("POLY_CLOB_HOST", defaultClobHost),
		PolygonRPC:     opt("POLY_POLYGON_RPC", defaultPolygonRPC),
		TokenID:        opt("POLY_TOKEN_ID", defaultTestTokenID),
		Side:           side,
		TargetPUSD:     target,
		MarketOrderUSD: orderUSD,
		WalletPath:     opt("POLY_WALLET_PATH", defaultWalletPath),
		FundWait:       waitDur,
	}
}

// loadDotEnv parses a trivial KEY=VALUE file and sets os.Setenv for each pair
// that isn't already set in the environment. It supports:
//   - blank lines and `#` comment lines
//   - optional `export` prefix
//   - single- or double-quoted values (quotes stripped, no escape handling)
// It does NOT support multi-line values or shell expansion.
func loadDotEnv(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for lineNo, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return fmt.Errorf("%s:%d: malformed line", path, lineNo+1)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, present := os.LookupEnv(key); present {
			continue // shell wins
		}
		if err := os.Setenv(key, val); err != nil {
			return err
		}
	}
	return nil
}

// --- wallet load / create ---

func loadOrCreateWallet(path string, safeFactory common.Address) (*wallet, bool, error) {
	if b, err := os.ReadFile(path); err == nil {
		var w wallet
		if err := json.Unmarshal(b, &w); err != nil {
			return nil, false, fmt.Errorf("parse existing wallet file: %w", err)
		}
		// validate: Safe should match current SafeFactory derivation
		predicted := builder.Derive(common.HexToAddress(w.EOAAddress), safeFactory).Hex()
		if !strings.EqualFold(predicted, w.SafeAddress) {
			return nil, false, fmt.Errorf("wallet file safe %s != derived %s — SafeFactory mismatch?", w.SafeAddress, predicted)
		}
		return &w, false, nil
	} else if !os.IsNotExist(err) {
		return nil, false, err
	}

	// Fresh generation.
	priv, err := crypto.GenerateKey()
	if err != nil {
		return nil, false, fmt.Errorf("generate key: %w", err)
	}
	eoa := crypto.PubkeyToAddress(priv.PublicKey)
	safe := builder.Derive(eoa, safeFactory)

	w := &wallet{
		PrivateKey:  "0x" + hex.EncodeToString(crypto.FromECDSA(priv)),
		EOAAddress:  eoa.Hex(),
		SafeAddress: safe.Hex(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := saveWallet(path, w); err != nil {
		return nil, false, err
	}
	return w, true, nil
}

func saveWallet(path string, w *wallet) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir wallet dir: %w", err)
	}
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	// 0600: owner read/write only.
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write wallet: %w", err)
	}
	return nil
}

// --- funding + on-chain waits ---

// waitForCollateral polls until USDC.e + pUSD combined on the Safe covers
// the required amount. Both tokens are 6-dec and wrap is 1:1, so on a re-run
// where wrap already happened, pUSD alone can satisfy the condition and the
// USDC.e balance stays at 0 without blocking.
func waitForCollateral(ctx context.Context, rpcURL string, usdce, pusd, safe common.Address, required *big.Int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	start := time.Now()
	for {
		usdceBal, err := erc20BalanceOf(rpcURL, usdce, safe)
		if err != nil {
			return fmt.Errorf("check USDC.e balance: %w", err)
		}
		pusdBal, err := erc20BalanceOf(rpcURL, pusd, safe)
		if err != nil {
			return fmt.Errorf("check pUSD balance: %w", err)
		}
		combined := new(big.Int).Add(usdceBal, pusdBal)
		if combined.Cmp(required) >= 0 {
			fmt.Printf("  [FUNDED] USDC.e=%s + pUSD=%s = %s >= required %s (after %s)\n",
				usdceBal.String(), pusdBal.String(), combined.String(), required.String(),
				time.Since(start).Round(time.Second))
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for collateral: USDC.e=%s pUSD=%s, need combined %s",
				usdceBal.String(), pusdBal.String(), required.String())
		}
		fmt.Printf("  [WAITING] USDC.e=%s pUSD=%s combined=%s  (need %s)  elapsed=%s\n",
			usdceBal.String(), pusdBal.String(), combined.String(), required.String(),
			time.Since(start).Round(time.Second))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

func waitUntilDeployed(t *testing.T, relay *relayer.RelayClient, safe common.Address, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok, err := relay.IsDeployed(safe)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
	return errors.New("timed out waiting for Safe deploy")
}

func waitUntilPUSDAtLeast(t *testing.T, rpcURL string, pusd, safe common.Address, required *big.Int, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		bal, err := erc20BalanceOf(rpcURL, pusd, safe)
		if err != nil {
			return err
		}
		if bal.Cmp(required) >= 0 {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
	return errors.New("timed out waiting for pUSD to reflect wrap")
}

// waitUntilAllowanceNonZero polls until every listed spender has a non-zero
// allowance on `token` from `owner`. Returns on the first moment all are
// satisfied, or times out.
func waitUntilAllowanceNonZero(t *testing.T, rpcURL string, token, owner common.Address, spenders []common.Address, timeout time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allOK := true
		for _, sp := range spenders {
			a, err := erc20Allowance(rpcURL, token, owner, sp)
			if err != nil {
				return err
			}
			if a.Sign() == 0 {
				allOK = false
				break
			}
		}
		if allOK {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
	return errors.New("timed out waiting for allowances to land on-chain")
}

// --- inline RPC helpers ---

func erc20BalanceOf(rpcURL string, token, owner common.Address) (*big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}
	data, err := parsed.Pack("balanceOf", owner)
	if err != nil {
		return nil, err
	}
	out, err := ethCall(rpcURL, token, data)
	if err != nil {
		return nil, fmt.Errorf("balanceOf: %w", err)
	}
	unpacked, err := parsed.Unpack("balanceOf", out)
	if err != nil {
		return nil, err
	}
	v, ok := unpacked[0].(*big.Int)
	if !ok {
		return nil, errors.New("balanceOf returned non-bigint")
	}
	return v, nil
}

// erc1155IsApprovedForAll checks whether `operator` has been granted
// setApprovalForAll rights on the ERC-1155 `token` by `owner`.
func erc1155IsApprovedForAll(rpcURL string, token, owner, operator common.Address) (bool, error) {
	const fragment = `[{"inputs":[{"name":"account","type":"address"},{"name":"operator","type":"address"}],"name":"isApprovedForAll","outputs":[{"name":"","type":"bool"}],"stateMutability":"view","type":"function"}]`
	parsed, err := abi.JSON(strings.NewReader(fragment))
	if err != nil {
		return false, err
	}
	data, err := parsed.Pack("isApprovedForAll", owner, operator)
	if err != nil {
		return false, err
	}
	out, err := ethCall(rpcURL, token, data)
	if err != nil {
		return false, fmt.Errorf("isApprovedForAll: %w", err)
	}
	unpacked, err := parsed.Unpack("isApprovedForAll", out)
	if err != nil {
		return false, err
	}
	v, ok := unpacked[0].(bool)
	if !ok {
		return false, errors.New("isApprovedForAll returned non-bool")
	}
	return v, nil
}

// ctfBalanceOf reads ERC-1155 balanceOf on the ConditionalTokens contract
// for a specific token id. Polymarket CTF token IDs are decimal uint256s.
func ctfBalanceOf(rpcURL string, ctf, owner common.Address, tokenIDDec string) (*big.Int, error) {
	const ctfABIFragment = `[{"inputs":[{"name":"account","type":"address"},{"name":"id","type":"uint256"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"stateMutability":"view","type":"function"}]`
	parsed, err := abi.JSON(strings.NewReader(ctfABIFragment))
	if err != nil {
		return nil, err
	}
	id, ok := new(big.Int).SetString(tokenIDDec, 10)
	if !ok {
		return nil, fmt.Errorf("invalid decimal token id: %s", tokenIDDec)
	}
	data, err := parsed.Pack("balanceOf", owner, id)
	if err != nil {
		return nil, err
	}
	out, err := ethCall(rpcURL, ctf, data)
	if err != nil {
		return nil, fmt.Errorf("balanceOf: %w", err)
	}
	unpacked, err := parsed.Unpack("balanceOf", out)
	if err != nil {
		return nil, err
	}
	v, ok := unpacked[0].(*big.Int)
	if !ok {
		return nil, errors.New("balanceOf returned non-bigint")
	}
	return v, nil
}

func erc20Allowance(rpcURL string, token, owner, spender common.Address) (*big.Int, error) {
	parsed, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}
	data, err := parsed.Pack("allowance", owner, spender)
	if err != nil {
		return nil, err
	}
	out, err := ethCall(rpcURL, token, data)
	if err != nil {
		return nil, fmt.Errorf("allowance: %w", err)
	}
	unpacked, err := parsed.Unpack("allowance", out)
	if err != nil {
		return nil, err
	}
	v, ok := unpacked[0].(*big.Int)
	if !ok {
		return nil, errors.New("allowance returned non-bigint")
	}
	return v, nil
}

func ethCall(rpcURL string, to common.Address, data []byte) ([]byte, error) {
	cli, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial rpc: %w", err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return cli.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
}

func decimalToBase6(d decimal.Decimal) *big.Int {
	return d.Shift(6).Truncate(0).BigInt()
}

// bidNotionalAtOrAbove sums up USD value of bids whose price ≥ floor.
// Also returns the best (highest-price) bid's price + size.
// Polymarket returns bids in ascending price order, so the best bid is last.
func bidNotionalAtOrAbove(bids []types.OrderSummary, floor decimal.Decimal) (total decimal.Decimal, topPrice, topSize string) {
	total = decimal.Zero
	if len(bids) == 0 {
		return total, "-", "-"
	}
	best := bids[len(bids)-1]
	topPrice, topSize = best.Price, best.Size
	for _, b := range bids {
		p, err := decimal.NewFromString(b.Price)
		if err != nil {
			continue
		}
		if p.LessThan(floor) {
			continue
		}
		s, err := decimal.NewFromString(b.Size)
		if err != nil {
			continue
		}
		total = total.Add(p.Mul(s))
	}
	return total, topPrice, topSize
}

// askNotionalAtOrBelow sums up USD value of asks whose price ≤ cap.
// Also returns the most-aggressive (lowest-price) ask's price + size for
// pretty-printing. Polymarket returns asks in descending price order.
func askNotionalAtOrBelow(asks []types.OrderSummary, cap decimal.Decimal) (total decimal.Decimal, topPrice, topSize string) {
	total = decimal.Zero
	if len(asks) == 0 {
		return total, "-", "-"
	}
	// Lowest-price ask is at the END of the slice (descending order).
	lowest := asks[len(asks)-1]
	topPrice, topSize = lowest.Price, lowest.Size
	for _, a := range asks {
		p, err := decimal.NewFromString(a.Price)
		if err != nil {
			continue
		}
		if p.GreaterThan(cap) {
			continue
		}
		s, err := decimal.NewFromString(a.Size)
		if err != nil {
			continue
		}
		total = total.Add(p.Mul(s))
	}
	return total, topPrice, topSize
}

// rawBalanceAllowance issues a GET /balance-allowance manually so we can see
// the full JSON body without a typed unmarshaler hiding unknown fields. Mirrors
// the v2 client's flow exactly (bare-path HMAC, signature_type query param).
func rawBalanceAllowance(host string, addr common.Address, creds *types.ApiKeyCreds, assetType types.AssetType, tokenID *string, sigType int) (string, error) {
	q := neturl.Values{}
	q.Add("asset_type", string(assetType))
	if tokenID != nil {
		q.Add("token_id", *tokenID)
	}
	q.Add("signature_type", fmt.Sprintf("%d", sigType))

	bare := "/balance-allowance"
	ts := fmt.Sprintf("%d", time.Now().Unix())
	msg := ts + "GET" + bare
	secret, err := base64.URLEncoding.DecodeString(creds.Secret)
	if err != nil {
		return "", fmt.Errorf("decode api secret: %w", err)
	}
	mac := cryptohmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	req, err := nethttp.NewRequest("GET", host+bare+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("POLY_ADDRESS", addr.Hex())
	req.Header.Set("POLY_SIGNATURE", sig)
	req.Header.Set("POLY_TIMESTAMP", ts)
	req.Header.Set("POLY_API_KEY", creds.Key)
	req.Header.Set("POLY_PASSPHRASE", creds.Passphrase)

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil
}
