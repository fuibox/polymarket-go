package clob

import (
	"fmt"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fuibox/polymarket-go/client/clob/clob_types"
	"github.com/fuibox/polymarket-go/client/clob/order_builder"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/endpoint"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
	"github.com/fuibox/polymarket-go/tools/headers"
)

// createOrderV2 builds a v2 signed limit order. Mirrors createOrder (v1) but
// skips the feeRateBps resolution — v2 doesn't sign a fee into the order.
// Falls back to c.defaultBuilderCode when args.BuilderCode is empty.
func (c *ClobClient) createOrderV2(args clob_types.OrderArgs, option clob_types.PartialCreateOrderOptions) (types.SignedOrderV2, error) {
	if err := c.AssertL1Auth(); err != nil {
		return types.SignedOrderV2{}, err
	}
	if option.TickSize == nil {
		tickSize, err := c.GetTickSize(args.TokenID)
		if err != nil {
			return types.SignedOrderV2{}, err
		}
		option.TickSize = &tickSize
	}
	if err := c.priceValid(args.Price, *option.TickSize); err != nil {
		return types.SignedOrderV2{}, err
	}
	if option.NegRisk == nil {
		isNegRisk, err := c.GetNegRisk(args.TokenID)
		if err != nil {
			return types.SignedOrderV2{}, err
		}
		option.NegRisk = &isNegRisk
	}
	if args.BuilderCode == "" {
		args.BuilderCode = c.defaultBuilderCode
	}
	sigType, funder, err := resolveV2SigTypeAndFunder(option)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	ob, err := order_builder.NewOrderBuilder(c.signer, sigType, funder)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	return ob.CreateOrderV2(c.signer, args, option)
}

// resolveV2SigTypeAndFunder selects between POLY_GNOSIS_SAFE (default) and
// POLY_1271 (deposit wallet) based on the order option. Maker/funder must
// match the signature type's expected address: SafeAccount for Safe orders,
// DepositWallet for POLY_1271.
func resolveV2SigTypeAndFunder(option clob_types.PartialCreateOrderOptions) (constants.SigType, common.Address, error) {
	if option.SignatureType == nil {
		return constants.POLY_GNOSIS_SAFE, option.SafeAccount, nil
	}
	switch *option.SignatureType {
	case constants.POLY_1271:
		if option.DepositWallet == (common.Address{}) {
			return 0, common.Address{}, fmt.Errorf("POLY_1271: option.DepositWallet is required")
		}
		return constants.POLY_1271, option.DepositWallet, nil
	case constants.POLY_GNOSIS_SAFE:
		return constants.POLY_GNOSIS_SAFE, option.SafeAccount, nil
	case constants.POLY_PROXY, constants.EOA:
		return *option.SignatureType, option.SafeAccount, nil
	default:
		return 0, common.Address{}, fmt.Errorf("unsupported V2 SignatureType: %d", *option.SignatureType)
	}
}

// createMarketOrderV2 is the market-order equivalent of createOrderV2.
func (c *ClobClient) createMarketOrderV2(args clob_types.MarketOrderArgs, option clob_types.PartialCreateOrderOptions) (types.SignedOrderV2, error) {
	if err := c.AssertL1Auth(); err != nil {
		return types.SignedOrderV2{}, err
	}
	if option.TickSize == nil {
		tickSize, err := c.GetTickSize(args.TokenID)
		if err != nil {
			return types.SignedOrderV2{}, err
		}
		option.TickSize = &tickSize
	}
	if err := c.priceValid(args.Price, *option.TickSize); err != nil {
		return types.SignedOrderV2{}, err
	}
	if option.NegRisk == nil {
		isNegRisk, err := c.GetNegRisk(args.TokenID)
		if err != nil {
			return types.SignedOrderV2{}, err
		}
		option.NegRisk = &isNegRisk
	}
	if args.OrderType == "" {
		args.OrderType = types.OrderTypeFOK
	}
	if args.BuilderCode == "" {
		args.BuilderCode = c.defaultBuilderCode
	}
	sigType, funder, err := resolveV2SigTypeAndFunder(option)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	ob, err := order_builder.NewOrderBuilder(c.signer, sigType, funder)
	if err != nil {
		return types.SignedOrderV2{}, err
	}
	return ob.CreateMarketOrderV2(c.signer, args, option)
}

// FinalBodyV2 is the POST /order wrapper for v2. Matches clob-client-v2's
// NewOrderV2 struct: includes postOnly and deferExec flags alongside order
// and orderType. Omitting either caused the v2 server to HTTP 500 on match
// attempts.
type FinalBodyV2 struct {
	Order     types.SignedOrderV2 `json:"order"`
	Owner     string              `json:"owner"`
	OrderType string              `json:"orderType"`
	DeferExec bool                `json:"deferExec"`
	PostOnly  bool                `json:"postOnly"`
}

func (c *ClobClient) orderToBodyV2(order types.SignedOrderV2, creds *types.ApiKeyCreds, orderType string) (*FinalBodyV2, error) {
	if creds == nil || creds.Key == "" {
		return nil, fmt.Errorf("API credentials required")
	}
	return &FinalBodyV2{
		Order:     order,
		Owner:     creds.Key,
		OrderType: orderType,
		DeferExec: false,
		PostOnly:  false,
	}, nil
}

// postOrderV2 submits a signed v2 order. Auth, retry, and response parsing
// reuse the v1 transport — the only change is the body shape.
func (c *ClobClient) postOrderV2(order types.SignedOrderV2, option clob_types.PartialCreateOrderOptions) (*types.OrderResponse, error) {
	if err := c.AssertL2Auth(); err != nil {
		return nil, err
	}
	if c.creds == nil {
		return nil, fmt.Errorf("API credentials required")
	}
	if option.OrderType == "" {
		option.OrderType = types.OrderTypeGTC
	}

	body, err := c.orderToBodyV2(order, c.creds, string(option.OrderType))
	if err != nil {
		return nil, err
	}
	bodyStr, err := sonic.MarshalString(body)
	if err != nil {
		return nil, err
	}
	serializedBody, err := serializeJsonBody(body)
	if err != nil {
		return nil, err
	}

	requestArgs := &types.L2HeaderArgs{
		Method:         "POST",
		RequestPath:    endpoint.PostOrder,
		Body:           bodyStr,
		SerializedBody: serializedBody,
	}

	timestamp, err := c.GetServerTime()
	if err != nil {
		return nil, err
	}
	tsStr := strconv.FormatInt(timestamp, 10)

	l2headers := &types.L2PolyHeader{}
	if c.signer.SignerType() == signer.Turnkey {
		l2headers, err = headers.CreateL2Headers(option.TurnkeyAccount, c.creds, requestArgs, tsStr)
		if err != nil {
			return nil, err
		}
	} else {
		pub, err := c.signer.GetPubkeyOfPrivateKey()
		if err != nil {
			return nil, err
		}
		l2headers, err = headers.CreateL2Headers(pub, c.creds, requestArgs, tsStr)
		if err != nil {
			return nil, err
		}
	}

	// Builder HMAC headers are kept for now per migration plan — the v2
	// backend ignores them but they are harmless. The per-order builder field
	// (args.BuilderCode) is the v2-native attribution mechanism.
	if c.canBuilderAuth() {
		builderHeaders, err := c.builderConfig.GenerateBuilderHeaders(requestArgs.Method, requestArgs.RequestPath, &requestArgs.SerializedBody, tsStr)
		if err != nil {
			return nil, err
		}
		if builderHeaders == nil {
			return nil, fmt.Errorf("builder headers is nil")
		}
		enriched := headers.InsertBuilderHeaders(l2headers, builderHeaders)
		result := types.OrderResponse{}
		if err := c.postJSONWithHeaders(endpoint.PostOrder, enriched, serializedBody, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}

	result := types.OrderResponse{}
	if err := c.postJSONWithHeaders(endpoint.PostOrder, l2headers, serializedBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ClobMarketInfo is the v2 fee/market metadata response from
// GET /clob-market-info/{conditionID}. Fields match the doc: fd = {r, e, to}.
type ClobMarketInfo struct {
	ConditionID string `json:"condition_id"`
	FeeData     struct {
		Rate      string `json:"r"`
		Exponent  int    `json:"e"`
		TakerOnly bool   `json:"to"`
	} `json:"fd"`
	MinOrderSize string `json:"min_order_size"`
	TickSize     string `json:"tick_size"`
}

// GetClobMarketInfo returns v2 market/fee parameters for a conditionID. This
// replaces the v1 GET /fee-rate lookup; v2 takes fee info from here and sets
// the actual fee at match time.
func (c *ClobClient) GetClobMarketInfo(conditionID string) (*ClobMarketInfo, error) {
	var result ClobMarketInfo
	if err := c.getJSON(endpoint.GetClobMarketInfo+conditionID, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
