# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**polymarket-go** is a Go SDK (module `github.com/fuibox/polymarket-go`) for [Polymarket](https://polymarket.com). It is a **library only** — there is no `main()` and no executable entry point. Do not add one.

Supports Polygon mainnet (ChainID 137) and Amoy testnet (ChainID 80002).

Required reading before non-trivial work (already present in repo):
- `PROJECT.md` — implemented modules and key constraints
- `PROJECT_MAP.md` — directory map and debugging entry points
- `docs/Agents/BASE_BACKEND_RULES.md` — generic backend rules
- `docs/Agents/PROJECT_BACKEND_RULES.md` — project-specific rules (authoritative for conflicts)

## Commands

```bash
# Build everything
go build ./...

# Run all tests
go test ./...

# Run tests in a single package
go test ./client/clob/...
go test ./tools/hmac

# Run a single test
go test ./client/clob -run TestCreateAndPostOrder -v

# Vet / tidy
go vet ./...
go mod tidy
```

Integration tests that touch chain must use Amoy (ChainID 80002), never mainnet. HTTP-dependent unit tests use `net/http/httptest` mocks — do not hit real Polymarket endpoints from unit tests.

## Architecture

The SDK is organized as independent clients under `client/`, all sharing types, config, and signing utilities.

### Clients (`client/`)

- **`clob/`** — CLOB trading (orders, books, trades, API-key management). `clob_client.go` is the main client; `order_builder/` handles order signing and **TickSize floor-rounding** (prices not aligned to `GetTickSize()` are rejected server-side); `clob_types/` holds `OrderArgs` / `MarketOrderArgs`.
- **`relayer/`** — on-chain Safe multisig operations via Polymarket Relayer: deploy, approve (USDC + ERC-1155), execute, redeem/split/merge positions, USDC.e transfer. Each tx needs the current on-chain Safe nonce from `GetSafeNonceOnChain()` — never increment locally.
- **`data/`** — reads from `data-api.polymarket.com` (positions, trades, user activity, market stats, portfolio).
- **`bridge/`** — cross-chain deposits/withdrawals via `bridge.polymarket.com` (EVM/SVM/BTC).
- **`ws/`** — WebSocket subscriptions (`wss://ws-subscriptions-clob.polymarket.com`) with auto-reconnect + ping/pong. **Writes are not concurrency-safe** — caller must serialize.
- **`gamma/`** — Gamma derivatives client.
- **`errors/`** — `SDKError` + `ErrCode` typed error system (see below).
- **`types/`**, **`constants/`**, **`config/`**, **`endpoint/`**, **`signer/`** — shared types, chain/contract constants, `ContractConfig` per chain, API path constants, signer interface.

### Tools (`tools/`)

`headers/` builds L1 (EIP-712) / L2 (HMAC) auth headers and builder headers; `eip712/` and `hmac/` implement the underlying signatures; `utils/` has hex + decimal helpers.

### Turnkey (`turnkey/`)

`TurnkeyService` wraps Turnkey's cloud signing API; `client/signer/` exposes both `PrivateKeySigner` and `TurnkeySigner` implementing the same interface. Never hardcode a signer choice — new features must work with both.

### Auth levels (critical)

- **L0** — none (public market data)
- **L1** — EIP-712 private-key signature (create/derive API key only)
- **L2** — HMAC-SHA256 + API key/passphrase (orders, positions, balances, cancels)

Headers contain timestamps and have a server-side validity window — regenerate per request, never cache.

### Typed error wrappers (`client/errors/`)

New code must use `TypedClobClient` (`clob/typed_client.go`) and `TypedDataSDK` (`data/typed_client.go`) — they wrap the original clients and return `*SDKError` with an `ErrCode` (400/401/403/404/429/500/1001/1002/1003). Branch with `IsRetryable()` / `IsNotFound()` / `IsAuthError()`, never on `strings.Contains(err.Error(), "...")`. Methods not yet typed are reachable via `.Inner()`.

When adding a method to `ClobClient` or `DataSDK`, add the matching typed method at the same time.

Known Polymarket quirk: `GET /data/order/{id}` returns **200 + empty body** for non-existent orders (code `ErrCodeNotFoundBody` = 1003); check `result.ID == ""`, not HTTP status.

## Project-specific rules (from `PROJECT_BACKEND_RULES.md`)

- **Decimal only**: `shopspring/decimal` for all money/price/size math. No `float32`/`float64`.
- **Strings on the wire**: `Price`, `Size`, `MakerAmount`, `TakerAmount`, `FeeRateBps` are strings in API payloads.
- **Tick-align prices** (floor) to `GetTickSize()` before submitting orders.
- **NegRisk routing**: call `GetNegRisk()`; NegRisk markets must use `NegExchange`+`NegCollateral`, others `Exchange`+`Collateral`. Addresses live in `client/config/` — never hardcode.
- **Don't blind-retry orders**: on POST /order failure, query order state first (network timeout ≠ server didn't receive). Applies to any external call with side effects.
- **Check approvals once**: call `CheckAllApprovals()` before first use on an address; token approval is one-shot, not per-trade.
- **FOK/FAK not filling is not a failure** — don't retry.
- **No secrets in logs/errors**: private keys, API secrets, passphrases, HMAC inputs, Turnkey creds must not appear in log output, error messages, or panic strings. Structs containing private keys must not be JSON-marshaled.
- **Validate addresses** at the boundary with `common.IsHexAddress()`.
- **No DB** in this project — transaction / guarded-update rules from `BASE_BACKEND_RULES.md` do not apply here. Idempotency still matters at the HTTP/RPC boundary.
- **Three-file rule**: if a fix touches more than three files, stop and re-check the root cause before continuing.
- **Logger drift**: the SDK is transitioning from global `log.Printf` to injected `*log.Logger`. Files still using globals: `client/ws/websocket_client.go`, `client/relayer/client.go`, `client/clob/clob_client.go`, `tools/hmac/hmac.go`, `client/gamma/client.go`. When editing logging in any of these, migrate that file — but don't do sweeping multi-file cleanups.
