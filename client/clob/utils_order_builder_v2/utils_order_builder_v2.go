package utils_order_builder_v2

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/fuibox/polymarket-go/client/clob/utils"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/depositwallet"
	"github.com/fuibox/polymarket-go/client/relayer/model/polyEip712"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
)

// NewUtilsOrderBuilderV2 constructs a v2 signer harness. exchange is the v2
// CTF Exchange address (either ExchangeV2 or NegExchangeV2 from config).
func NewUtilsOrderBuilderV2(exchange common.Address, chainId int, signerHandler *signer.Signer, option Option) (*UtilsOrderBuilderV2, error) {
	if signerHandler.SignerType() == signer.Turnkey && option.TurnkeyAccount == constants.ZERO_ADDRESS {
		return nil, errors.New("turnkeyAccount is empty")
	}
	if exchange == constants.ZERO_ADDRESS {
		return nil, errors.New("v2 exchange address is zero — ContractConfig.ExchangeV2/NegExchangeV2 not set for this chain")
	}
	return &UtilsOrderBuilderV2{
		ExchangeAddress: exchange,
		ChainId:         chainId,
		Signer:          signerHandler,
		Option:          option,
	}, nil
}

// GenerateSalt mirrors v1 salt generation. Salt plus timestamp gives the
// replay-uniqueness v2 needs (v1 used nonce + salt for this).
func (b *UtilsOrderBuilderV2) GenerateSalt() *big.Int {
	now := time.Now().UTC().UnixNano()
	r := rand.Float64()
	salt := float64(now) * r
	return big.NewInt(int64(math.Round(salt)))
}

func (b *UtilsOrderBuilderV2) BuildSignedOrder(data OrderDataV2) (types.SignedOrderV2, error) {
	order, err := b.buildOrder(data)
	if err != nil {
		return types.SignedOrderV2{}, err
	}

	structHash, err := order.OrderStructHash()
	if err != nil {
		return types.SignedOrderV2{}, err
	}

	name := "Polymarket CTF Exchange"
	version := "2"
	chainId := b.ChainId
	contract := b.ExchangeAddress.Hex()
	domain := polyEip712.MakeDomain(&name, &version, &chainId, &contract, nil)

	domainSep, err := domain.HashStruct()
	if err != nil {
		return types.SignedOrderV2{}, fmt.Errorf("v2 domain hash failed: %w", err)
	}
	domainSepHash := common.BytesToHash(domainSep[:])

	// For POLY_1271 the EOA signs an ERC-7739-wrapped digest, not the raw V2
	// order digest. The wallet's on-chain ERC-1271 verifier reconstructs the
	// unwrapped digest from the trailer the wire signature carries.
	var (
		digest       common.Hash
		erc7739Trail []byte
	)
	if order.SignatureType == uint8(constants.POLY_1271) {
		digest, erc7739Trail, err = depositwallet.WrapERC7739(
			structHash, domainSepHash, order.Maker, b.ChainId, orderTypeStringV2,
		)
		if err != nil {
			return types.SignedOrderV2{}, fmt.Errorf("erc7739 wrap: %w", err)
		}
	} else {
		digest = order.OrderEIP712Digest(domainSepHash, structHash)
	}

	var sig string
	if b.Signer.SignerType() == signer.Turnkey {
		sig, err = b.Signer.SignHashWithTurnkey(digest.Hex(), b.Option.TurnkeyAccount)
	} else {
		sig, err = b.Signer.SignHash(digest.Hex())
	}
	if err != nil {
		return types.SignedOrderV2{}, err
	}

	if erc7739Trail != nil {
		// SignHash returns a 0x-prefixed 65-byte ECDSA signature. Concatenate
		// the ERC-7739 trailer so the wallet's ERC-1271 verifier can validate.
		ecdsaBytes, decodeErr := hex.DecodeString(strings.TrimPrefix(sig, "0x"))
		if decodeErr != nil {
			return types.SignedOrderV2{}, fmt.Errorf("decode ecdsa sig: %w", decodeErr)
		}
		wrapped, wrapErr := depositwallet.AssembleWrappedSignature(ecdsaBytes, erc7739Trail)
		if wrapErr != nil {
			return types.SignedOrderV2{}, wrapErr
		}
		sig = "0x" + hex.EncodeToString(wrapped)
	}

	side := "BUY"
	if order.Side == 1 {
		side = "SELL"
	}

	return types.SignedOrderV2{
		Salt:  order.Salt.Int64(),
		Maker: order.Maker.Hex(),
		// Taker is always zero in v2 (removed from EIP-712 hash) but the
		// REST body still carries it; server rejects null with HTTP 500.
		Taker:         constants.ZERO_ADDRESS.Hex(),
		Signer:        order.Signer.Hex(),
		TokenID:       order.TokenID.String(),
		MakerAmount:   order.MakerAmount.String(),
		TakerAmount:   order.TakerAmount.String(),
		Side:          side,
		SignatureType: types.SignatureType(order.SignatureType),
		Timestamp:     order.Timestamp.String(),
		// Expiration is similarly wire-only in v2 — "0" means never expires.
		Expiration: "0",
		Metadata:   "0x" + hex.EncodeToString(order.Metadata[:]),
		Builder:    "0x" + hex.EncodeToString(order.Builder[:]),
		Signature:  sig,
	}, nil
}

func (b *UtilsOrderBuilderV2) buildOrder(data OrderDataV2) (OrderV2, error) {
	if err := b.validateInputs(data); err != nil {
		return OrderV2{}, err
	}

	if data.Signer == constants.ZERO_ADDRESS {
		data.Signer = data.Maker
	}
	// For POLY_1271, data.Signer is the deposit wallet (not the EOA), so the
	// usual pubkey-equality check does not apply. The deposit wallet's on-chain
	// ERC-1271 verifier authenticates the actual EOA via ecrecover. We only
	// enforce the docs' rule that maker == signer == deposit wallet.
	if data.SignatureType == constants.POLY_1271 {
		if data.Maker != data.Signer {
			return OrderV2{}, errors.New("POLY_1271: maker and signer must both equal the deposit wallet address")
		}
		if b.Signer.SignerType() != signer.PrivateKey && b.Signer.SignerType() != signer.Turnkey {
			return OrderV2{}, errors.New("signer type is invalid")
		}
	} else {
		switch b.Signer.SignerType() {
		case signer.PrivateKey:
			signerAddr, err := b.Signer.GetPubkeyOfPrivateKey()
			if err != nil {
				return OrderV2{}, err
			}
			if data.Signer != signerAddr {
				return OrderV2{}, errors.New("signer does not match data.Signer")
			}
		case signer.Turnkey:
			if data.Signer != b.Option.TurnkeyAccount {
				return OrderV2{}, errors.New("turnkeyAccount does not match data.Signer")
			}
		default:
			return OrderV2{}, errors.New("signer type is invalid")
		}
	}

	tokenId, err := utils.MustBigInt(data.TokenID)
	if err != nil {
		return OrderV2{}, err
	}
	makerAmt, err := utils.MustBigInt(data.MakerAmount)
	if err != nil {
		return OrderV2{}, fmt.Errorf("invalid makerAmount: %w", err)
	}
	takerAmt, err := utils.MustBigInt(data.TakerAmount)
	if err != nil {
		return OrderV2{}, fmt.Errorf("invalid takerAmount: %w", err)
	}

	tsUint, err := strconv.ParseUint(data.Timestamp, 10, 64)
	if err != nil {
		return OrderV2{}, fmt.Errorf("invalid timestamp (expect unix millis as decimal string): %w", err)
	}
	tsBig := new(big.Int).SetUint64(tsUint)

	if data.Side != 0 && data.Side != 1 {
		return OrderV2{}, errors.New("side must be 0(BUY) or 1(SELL)")
	}

	metaBytes, err := parseBytes32(data.Metadata)
	if err != nil {
		return OrderV2{}, fmt.Errorf("invalid metadata: %w", err)
	}
	builderBytes, err := parseBytes32(data.Builder)
	if err != nil {
		return OrderV2{}, fmt.Errorf("invalid builder: %w", err)
	}

	return OrderV2{
		Salt:          b.GenerateSalt(),
		Maker:         data.Maker,
		Signer:        data.Signer,
		TokenID:       tokenId,
		MakerAmount:   makerAmt,
		TakerAmount:   takerAmt,
		Side:          uint8(data.Side),
		SignatureType: uint8(data.SignatureType),
		Timestamp:     tsBig,
		Metadata:      metaBytes,
		Builder:       builderBytes,
	}, nil
}

func (b *UtilsOrderBuilderV2) validateInputs(data OrderDataV2) error {
	if data.Maker == constants.ZERO_ADDRESS {
		return fmt.Errorf("maker is required")
	}
	if data.TokenID == "" {
		return fmt.Errorf("tokenId is required")
	}
	if data.MakerAmount == "" {
		return fmt.Errorf("makerAmount is required")
	}
	if data.TakerAmount == "" {
		return fmt.Errorf("takerAmount is required")
	}
	if data.Side != types.SideBuy.Int() && data.Side != types.SideSell.Int() {
		return fmt.Errorf("side must be 0(BUY) or 1(SELL)")
	}
	if data.Timestamp == "" {
		return fmt.Errorf("timestamp is required (unix millis as decimal string)")
	}
	switch data.SignatureType {
	case constants.EOA, constants.POLY_GNOSIS_SAFE, constants.POLY_PROXY, constants.POLY_1271:
	default:
		return fmt.Errorf("invalid signatureType")
	}
	return nil
}

// parseBytes32 accepts "" (→ zeros), or a 0x-prefixed / bare hex string that
// decodes to exactly 32 bytes. Shorter inputs are NOT padded — callers must
// pass full bytes32 values so zero-vs-padded ambiguity never hits the signer.
func parseBytes32(s string) ([32]byte, error) {
	var out [32]byte
	if s == "" {
		return out, nil
	}
	clean := strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(clean) != 64 {
		return out, fmt.Errorf("bytes32 value must be 32 bytes (64 hex chars), got %d", len(clean))
	}
	decoded, err := hex.DecodeString(clean)
	if err != nil {
		return out, err
	}
	copy(out[:], decoded)
	return out, nil
}
