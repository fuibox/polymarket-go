package config

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/fuibox/polymarket-go/client/types"
)

type ContractConfig struct {
	SafeFactory          common.Address
	SafeMultisend        common.Address
	Exchange             common.Address
	NegExchange          common.Address
	Collateral           common.Address
	NegCollateral        common.Address
	ConditionalTokens    common.Address
	NegConditionalTokens common.Address

	// v2 — Polymarket CLOB v2 cutover 2026-04-28. Populated from
	// https://docs.polymarket.com/resources/contracts. Amoy v2 addresses are
	// not published in docs; callers running v2 on testnet must supply them
	// via override until Polymarket publishes them.
	ExchangeV2       common.Address
	NegExchangeV2    common.Address
	PUSD             common.Address // pUSD collateral (replaces USDC.e in v2)
	CollateralOnramp common.Address // wrap/unwrap USDC <-> pUSD
	// NegRiskAdapterV2 is a required spender for pUSD on neg-risk markets.
	// Discovered empirically: the v2 /balance-allowance response returns
	// allowances keyed by this spender and POST /order rejects with
	// "not enough balance / allowance" if not approved. Value reused from
	// v1's NEGRISK_ADAPTER (same address).
	NegRiskAdapterV2 common.Address
}

var contractConfigs = map[types.Chain]ContractConfig{
	137: {
		SafeFactory:          common.HexToAddress("0xaacFeEa03eb1561C4e67d661e40682Bd20E3541b"),
		SafeMultisend:        common.HexToAddress("0xA238CBeb142c10Ef7Ad8442C6D1f9E89e07e7761"),
		Exchange:             common.HexToAddress("0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"),
		NegExchange:          common.HexToAddress("0xC5d563A36AE78145C45a50134d48A1215220f80a"),
		Collateral:           common.HexToAddress("0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"),
		NegCollateral:        common.HexToAddress("0x2791bca1f2de4661ed88a30c99a7a9449aa84174"),
		ConditionalTokens:    common.HexToAddress("0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"),
		NegConditionalTokens: common.HexToAddress("0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"),

		ExchangeV2:       common.HexToAddress("0xE111180000d2663C0091e4f400237545B87B996B"),
		NegExchangeV2:    common.HexToAddress("0xe2222d279d744050d28e00520010520000310F59"),
		PUSD:             common.HexToAddress("0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"),
		CollateralOnramp: common.HexToAddress("0x93070a847efEf7F70739046A929D47a521F5B8ee"),
		NegRiskAdapterV2: common.HexToAddress("0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"),
	},
	80002: {
		SafeFactory:          common.HexToAddress("0xaacFeEa03eb1561C4e67d661e40682Bd20E3541b"),
		SafeMultisend:        common.HexToAddress("0xA238CBeb142c10Ef7Ad8442C6D1f9E89e07e7761"),
		Exchange:             common.HexToAddress("0xdFE02Eb6733538f8Ea35D585af8DE5958AD99E40"),
		NegExchange:          common.HexToAddress("0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"),
		Collateral:           common.HexToAddress("0x9c4e1703476e875070ee25b56a58b008cfb8fa78"),
		NegCollateral:        common.HexToAddress("0x9c4e1703476e875070ee25b56a58b008cfb8fa78"),
		ConditionalTokens:    common.HexToAddress("0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB"),
		NegConditionalTokens: common.HexToAddress("0x69308FB512518e39F9b16112fA8d994F4e2Bf8bB"),

		// Amoy v2 addresses not yet published by Polymarket. Left zero so a
		// v2-on-Amoy call fails loudly rather than silently hitting v1
		// contracts. Replace with real values once published.
	},
}

func GetContractConfig(chainID types.Chain) (ContractConfig, error) {
	cfg, ok := contractConfigs[chainID]
	if !ok {
		return ContractConfig{}, fmt.Errorf("invalid chainID: %d", chainID)
	}
	return cfg, nil
}

func GetWsPingInterval() time.Duration {
	return 10 * time.Second
}
