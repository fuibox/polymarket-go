// Package utils_order_builder_v2 implements EIP-712 signing for Polymarket
// CLOB v2 orders. v1 (client/clob/utils_order_builder) remains untouched so
// both pipelines can run in parallel until the 2026-04-28 cutover.
//
// v2 differences from v1:
//   - EIP-712 domain version changes from "1" to "2"
//   - Verifying contract changes (see client/config ExchangeV2 / NegExchangeV2)
//   - Signed Order drops taker / expiration / nonce / feeRateBps
//   - Signed Order gains timestamp (ms), metadata (bytes32), builder (bytes32)
package utils_order_builder_v2

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/signer"
)

type UtilsOrderBuilderV2 struct {
	ExchangeAddress common.Address
	ChainId         int
	Signer          *signer.Signer
	Option          Option
}

type Option struct {
	TurnkeyAccount common.Address
}

// OrderDataV2 is the builder-layer struct that captures everything needed to
// produce an OrderV2. All amount / token / timestamp / side fields are strings
// so callers can marshal decimals exactly.
type OrderDataV2 struct {
	Maker         common.Address    `json:"maker"`
	Signer        common.Address    `json:"signer"`
	TokenID       string            `json:"tokenId"`
	MakerAmount   string            `json:"makerAmount"`
	TakerAmount   string            `json:"takerAmount"`
	Side          int               `json:"side"`
	SignatureType constants.SigType `json:"signatureType"`
	Timestamp     string            `json:"timestamp"` // unix millis, decimal
	// Metadata and Builder are 0x-prefixed 32-byte hex; empty string → zero.
	Metadata string `json:"metadata"`
	Builder  string `json:"builder"`
}

// orderTypeStringV2 is the EIP-712 type declaration for v2 orders. The
// field order here MUST match the encoding order in OrderStructHash.
const orderTypeStringV2 = "Order(uint256 salt,address maker,address signer,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint8 side,uint8 signatureType,uint256 timestamp,bytes32 metadata,bytes32 builder)"

// OrderV2 is the fully-resolved in-memory v2 order used for hashing.
type OrderV2 struct {
	Salt          *big.Int       `json:"salt"`
	Maker         common.Address `json:"maker"`
	Signer        common.Address `json:"signer"`
	TokenID       *big.Int       `json:"tokenId"`
	MakerAmount   *big.Int       `json:"makerAmount"`
	TakerAmount   *big.Int       `json:"takerAmount"`
	Side          uint8          `json:"side"`
	SignatureType uint8          `json:"signatureType"`
	Timestamp     *big.Int       `json:"timestamp"`
	Metadata      [32]byte       `json:"metadata"`
	Builder       [32]byte       `json:"builder"`
}

func (d *OrderV2) OrderTypeHash() common.Hash {
	return crypto.Keccak256Hash([]byte(orderTypeStringV2))
}

// OrderStructHash implements keccak256(abi.encode(typeHash, encodeData(order))).
// Per EIP-712 encodeData:
//   - uintN → left-padded 32-byte big-endian
//   - address → left-padded 32-byte (20 bytes placed in the low order)
//   - bytes32 → the 32 raw bytes, no padding
func (d *OrderV2) OrderStructHash() (common.Hash, error) {
	typeHash := d.OrderTypeHash()

	padUint := func(x *big.Int) []byte {
		b := make([]byte, 32)
		x.FillBytes(b)
		return b
	}
	padAddr := func(a common.Address) []byte {
		b := make([]byte, 32)
		copy(b[12:], a.Bytes())
		return b
	}

	enc := make([]byte, 0, 32*12)
	enc = append(enc, typeHash.Bytes()...)
	enc = append(enc, padUint(d.Salt)...)
	enc = append(enc, padAddr(d.Maker)...)
	enc = append(enc, padAddr(d.Signer)...)
	enc = append(enc, padUint(d.TokenID)...)
	enc = append(enc, padUint(d.MakerAmount)...)
	enc = append(enc, padUint(d.TakerAmount)...)
	enc = append(enc, padUint(new(big.Int).SetUint64(uint64(d.Side)))...)
	enc = append(enc, padUint(new(big.Int).SetUint64(uint64(d.SignatureType)))...)
	enc = append(enc, padUint(d.Timestamp)...)
	enc = append(enc, d.Metadata[:]...)
	enc = append(enc, d.Builder[:]...)

	return crypto.Keccak256Hash(enc), nil
}

// OrderEIP712Digest = keccak256("\x19\x01" || domainSeparator || structHash).
func (d *OrderV2) OrderEIP712Digest(
	domainSeparator common.Hash,
	structHash common.Hash,
) common.Hash {
	return crypto.Keccak256Hash(
		[]byte("\x19\x01"),
		domainSeparator.Bytes(),
		structHash.Bytes(),
	)
}
