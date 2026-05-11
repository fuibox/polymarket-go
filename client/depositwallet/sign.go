package depositwallet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// EIP-712 domain values baked into every Polymarket deposit wallet (matches
// Solady ERC1271 + EIP712 contracts at impl 0x58CA52ebe0DadfdF531Cde7062e76746de4Db1eB).
const (
	depositWalletDomainName    = "DepositWallet"
	depositWalletDomainVersion = "1"
)

// ERC-7739 nested EIP-712 envelope, per Solady ERC1271._erc1271IsValidSignatureViaNestedEIP712.
// The full type string is "TypedDataSign(" || contentsName || envelopeMid || contentsType.
const envelopeMid = " contents,string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)"

// WrapERC7739 produces:
//   - digest: 32-byte hash the EOA / session signer must sign with ECDSA
//   - trailer: bytes appended after the 65-byte ECDSA signature to form the wire signature
//
// Inputs:
//   - orderStructHash: keccak256(abi.encode(orderTypeHash, ...orderFields)) — already produced
//     by client/clob/utils_order_builder_v2.OrderV2.OrderStructHash().
//   - appDomainSeparator: V2 CTF Exchange EIP-712 domain separator. The same value the V2
//     builder uses to form the unwrapped EIP-712 digest.
//   - depositWallet: the deposit wallet contract address (which is also the order's
//     maker and signer fields for POLY_1271).
//   - chainID: current chain.
//   - contentsType: the full inner EIP-712 type string, e.g.
//     "Order(uint256 salt,address maker,...)". Must start with the contentsName followed by '('.
//
// Wire format of the final signature (after the caller concatenates ECDSA(digest) || trailer):
//
//	r(32) || s(32) || v(1) || appDomainSeparator(32) || orderStructHash(32) || contentsType || uint16_be(len(contentsType))
//
// Implicit mode only (contentsName is the leading identifier of contentsType, so no separate
// contentsName needs to be appended). Session-signer ERC-6492 wrapping is intentionally
// not produced here.
func WrapERC7739(
	orderStructHash common.Hash,
	appDomainSeparator common.Hash,
	depositWallet common.Address,
	chainID int,
	contentsType string,
) (digest common.Hash, trailer []byte, err error) {
	if depositWallet == (common.Address{}) {
		return common.Hash{}, nil, errors.New("deposit wallet address is zero")
	}
	if chainID <= 0 {
		return common.Hash{}, nil, fmt.Errorf("chainID must be positive, got %d", chainID)
	}
	contentsName, err := extractContentsName(contentsType)
	if err != nil {
		return common.Hash{}, nil, err
	}

	// typedDataSignTypeHash = keccak256("TypedDataSign(<Name> contents,string name,...)" || contentsType)
	typeString := "TypedDataSign(" + contentsName + envelopeMid + contentsType
	typedDataSignTypeHash := crypto.Keccak256Hash([]byte(typeString))

	nameHash := crypto.Keccak256Hash([]byte(depositWalletDomainName))
	versionHash := crypto.Keccak256Hash([]byte(depositWalletDomainVersion))

	chainIDPadded := make([]byte, 32)
	new(big.Int).SetInt64(int64(chainID)).FillBytes(chainIDPadded)

	walletPadded := make([]byte, 32)
	copy(walletPadded[12:], depositWallet.Bytes())

	saltZero := make([]byte, 32)

	// hashStruct(TypedDataSign) =
	//   keccak256(typeHash || contents || keccak(name) || keccak(version) || chainId || verifyingContract || salt)
	structHashInput := make([]byte, 0, 32*7)
	structHashInput = append(structHashInput, typedDataSignTypeHash.Bytes()...)
	structHashInput = append(structHashInput, orderStructHash.Bytes()...)
	structHashInput = append(structHashInput, nameHash.Bytes()...)
	structHashInput = append(structHashInput, versionHash.Bytes()...)
	structHashInput = append(structHashInput, chainIDPadded...)
	structHashInput = append(structHashInput, walletPadded...)
	structHashInput = append(structHashInput, saltZero...)
	typedDataSignStructHash := crypto.Keccak256(structHashInput)

	// digest = keccak256("\x19\x01" || appDomainSep || hashStruct(typedDataSign))
	digestBytes := crypto.Keccak256(
		[]byte{0x19, 0x01},
		appDomainSeparator.Bytes(),
		typedDataSignStructHash,
	)
	digest = common.BytesToHash(digestBytes)

	// trailer = appDomainSep || contents || contentsType || uint16_be(len(contentsType))
	contentsTypeBytes := []byte(contentsType)
	if len(contentsTypeBytes) > 0xFFFF {
		return common.Hash{}, nil, fmt.Errorf("contentsType too long: %d bytes (max 65535)", len(contentsTypeBytes))
	}
	lenBE := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBE, uint16(len(contentsTypeBytes)))

	trailer = make([]byte, 0, 32+32+len(contentsTypeBytes)+2)
	trailer = append(trailer, appDomainSeparator.Bytes()...)
	trailer = append(trailer, orderStructHash.Bytes()...)
	trailer = append(trailer, contentsTypeBytes...)
	trailer = append(trailer, lenBE...)

	return digest, trailer, nil
}

// AssembleWrappedSignature returns the full wire signature: a 65-byte ECDSA signature
// (r||s||v) concatenated with the trailer from WrapERC7739. Callers usually have an
// `eth_sign`-style 65-byte signature already; this just stitches the trailer on.
func AssembleWrappedSignature(ecdsa65 []byte, trailer []byte) ([]byte, error) {
	if len(ecdsa65) != 65 {
		return nil, fmt.Errorf("ECDSA signature must be 65 bytes, got %d", len(ecdsa65))
	}
	out := make([]byte, 0, 65+len(trailer))
	out = append(out, ecdsa65...)
	out = append(out, trailer...)
	return out, nil
}

func extractContentsName(contentsType string) (string, error) {
	i := strings.IndexByte(contentsType, '(')
	if i <= 0 {
		return "", fmt.Errorf("contentsType must start with a name followed by '(', got %q", contentsType)
	}
	name := contentsType[:i]
	// Solady rejects names that have any of `\0`, ' ', ')', ',' before the '(' —
	// catching all four here matches the on-chain corruption guard.
	for j := 0; j < len(name); j++ {
		switch name[j] {
		case 0, ' ', ')', ',':
			return "", fmt.Errorf("contentsType name contains invalid byte 0x%02x at position %d", name[j], j)
		}
	}
	return name, nil
}
