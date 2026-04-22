package clob_types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
	"github.com/fuibox/polymarket-go/client/types"
)

type RequestArgs struct {
	Method         string
	RequestPath    string
	Body           []byte
	SerializedBody []byte
}

type OrderArgs struct {
	TokenID string          `json:"tokenID"`
	Price   decimal.Decimal `json:"price"`
	Size    decimal.Decimal `json:"size"`
	Side    types.Side      `json:"side"`

	// FeeRateBps is v1-only — v2 sets fees at match time.
	FeeRateBps int `json:"feeRateBps,omitempty"`
	// Nonce is v1-only — v2 uses timestamp for uniqueness.
	Nonce int `json:"nonce,omitempty"`
	// Expiration (seconds) is v1-only — v2 removed this field from the signed order.
	Expiration int `json:"expiration,omitempty"`
	// Taker is v1-only — v2 removed this field from the signed order.
	Taker common.Address `json:"taker,omitempty"`

	// BuilderCode is the v2 builder identifier written into Order.Builder.
	// Expected as a 0x-prefixed 32-byte hex string; empty string → zero bytes32.
	BuilderCode string `json:"builderCode,omitempty"`
	// Metadata is an arbitrary 32-byte tag written into Order.Metadata.
	// Expected as a 0x-prefixed 32-byte hex string; empty string → zero bytes32.
	Metadata string `json:"metadata,omitempty"`
}

type PartialCreateOrderOptions struct {
	OrderType      types.OrderType `json:"orderType"`
	TickSize       *types.TickSize `json:"tickSize"`
	NegRisk        *bool           `json:"negRisk"`
	TurnkeyAccount common.Address  `json:"turnkeyAccount"`
	SafeAccount    common.Address  `json:"safeAccount"`
}

type ClobOption struct {
	TurnkeyAccount common.Address `json:"turnkeyAccount"`
	SafeAccount    common.Address `json:"safeAccount"`
}

type MarketOrderArgs struct {
	TokenID string          `json:"token_id"`
	Amount  decimal.Decimal `json:"amount"`
	Side    types.Side      `json:"side"`
	Price   decimal.Decimal `json:"price"`

	// FeeRateBps is v1-only — v2 sets fees at match time.
	FeeRateBps int `json:"fee_rate_bps"`
	// Nonce is v1-only — v2 uses timestamp for uniqueness.
	Nonce int `json:"nonce"`
	// Taker is v1-only — v2 removed this field from the signed order.
	Taker common.Address `json:"taker"`

	OrderType types.OrderType `json:"order_type"`

	// BuilderCode is the v2 builder identifier written into Order.Builder.
	// Expected as a 0x-prefixed 32-byte hex string; empty string → zero bytes32.
	BuilderCode string `json:"builderCode,omitempty"`
	// Metadata is an arbitrary 32-byte tag written into Order.Metadata.
	// Expected as a 0x-prefixed 32-byte hex string; empty string → zero bytes32.
	Metadata string `json:"metadata,omitempty"`
}
