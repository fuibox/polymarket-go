package order_builder

import (
	"errors"
	"strconv"
	"time"

	"github.com/fuibox/polymarket-go/client/clob/clob_types"
	"github.com/fuibox/polymarket-go/client/clob/utils_order_builder_v2"
	"github.com/fuibox/polymarket-go/client/config"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"

	"github.com/ethereum/go-ethereum/common"
)

// resolveExchangeV2 returns the v2 exchange address for standard or neg-risk markets.
func resolveExchangeV2(cc config.ContractConfig, negRisk bool) common.Address {
	if negRisk {
		return cc.NegExchangeV2
	}
	return cc.ExchangeV2
}

// buildSignedOrderV2 wires the v2 EIP-712 pipeline — same shape as v1's
// buildSignedOrder, but routes to utils_order_builder_v2 with the v2 exchange.
func (b *OrderBuilder) buildSignedOrderV2(
	signerHandler *signer.Signer,
	data utils_order_builder_v2.OrderDataV2,
	options clob_types.PartialCreateOrderOptions,
) (types.SignedOrderV2, error) {
	cc, err := config.GetContractConfig(types.Chain(signerHandler.ChainID()))
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	exchange := resolveExchangeV2(cc, *options.NegRisk)
	ob, err := utils_order_builder_v2.NewUtilsOrderBuilderV2(
		exchange, int(signerHandler.ChainID()), signerHandler,
		utils_order_builder_v2.Option{TurnkeyAccount: options.TurnkeyAccount},
	)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	return ob.BuildSignedOrder(data)
}

// CreateOrderV2 builds and signs a limit order for CLOB v2.
//
// Reuses GetOrderAmounts from the v1 builder — price/size/tick math is
// unchanged in v2. Populates Timestamp with time.Now().UnixMilli(); callers
// can override via args.Metadata / args.BuilderCode for non-default values.
func (b *OrderBuilder) CreateOrderV2(signerHandler *signer.Signer, args clob_types.OrderArgs, options clob_types.PartialCreateOrderOptions) (types.SignedOrderV2, error) {
	if options.NegRisk == nil {
		return types.SignedOrderV2{}, errors.New("options.NegRisk cannot be nil")
	}
	roundConfig, err := validateAndGetRoundConfig(options)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	sideInt, makerAmount, takerAmount, err := b.GetOrderAmounts(args.Side, args.Size, args.Price, *roundConfig)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	signerAddr, err := resolveSignerAddress(signerHandler, options.TurnkeyAccount)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	data := utils_order_builder_v2.OrderDataV2{
		Maker:         b.Funder,
		Signer:        signerAddr,
		TokenID:       args.TokenID,
		MakerAmount:   makerAmount,
		TakerAmount:   takerAmount,
		Side:          sideInt,
		SignatureType: b.SigType,
		Timestamp:     strconv.FormatInt(time.Now().UnixMilli(), 10),
		Metadata:      args.Metadata,
		Builder:       args.BuilderCode,
	}
	return b.buildSignedOrderV2(signerHandler, data, options)
}

// CreateMarketOrderV2 builds and signs a market order for CLOB v2.
func (b *OrderBuilder) CreateMarketOrderV2(signerHandler *signer.Signer, args clob_types.MarketOrderArgs, options clob_types.PartialCreateOrderOptions) (types.SignedOrderV2, error) {
	if options.NegRisk == nil {
		return types.SignedOrderV2{}, errors.New("options.NegRisk cannot be nil")
	}
	roundConfig, err := validateAndGetRoundConfig(options)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	sideInt, makerAmount, takerAmount, err := b.GetMarketOrderAmounts(args.Side, args.Amount, args.Price, *roundConfig)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	signerAddr, err := resolveSignerAddress(signerHandler, options.TurnkeyAccount)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	data := utils_order_builder_v2.OrderDataV2{
		Maker:         b.Funder,
		Signer:        signerAddr,
		TokenID:       args.TokenID,
		MakerAmount:   makerAmount,
		TakerAmount:   takerAmount,
		Side:          sideInt,
		SignatureType: b.SigType,
		Timestamp:     strconv.FormatInt(time.Now().UnixMilli(), 10),
		Metadata:      args.Metadata,
		Builder:       args.BuilderCode,
	}
	return b.buildSignedOrderV2(signerHandler, data, options)
}
