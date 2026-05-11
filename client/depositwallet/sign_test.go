package depositwallet

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// V2 Order EIP-712 type string. Kept in sync with
// client/clob/utils_order_builder_v2/utils_order_builder_v2_types.go.
const orderTypeStringV2 = "Order(uint256 salt,address maker,address signer,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint8 side,uint8 signatureType,uint256 timestamp,bytes32 metadata,bytes32 builder)"

// Golden values were computed in Python (Crypto.Hash.keccak) against the same
// formula Solady ERC1271._erc1271IsValidSignatureViaNestedEIP712 uses on-chain.
// Any change to typedDataSignTypeHash or digest indicates a wire-format break.
func TestWrapERC7739_Golden(t *testing.T) {
	orderStructHash := common.HexToHash("0x" + strings.Repeat("11", 32))
	appDomainSep := common.HexToHash("0x" + strings.Repeat("22", 32))
	depositWallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")
	chainID := 137

	digest, trailer, err := WrapERC7739(orderStructHash, appDomainSep, depositWallet, chainID, orderTypeStringV2)
	if err != nil {
		t.Fatalf("WrapERC7739: %v", err)
	}

	wantDigest := common.HexToHash("0x6ad2bd812255a10182cfba55134d5776a585434ee1bb0e45dd2bf84aa492d9a9")
	if digest != wantDigest {
		t.Errorf("digest mismatch:\n  want %s\n  got  %s", wantDigest.Hex(), digest.Hex())
	}

	wantLen := 32 + 32 + len(orderTypeStringV2) + 2
	if len(trailer) != wantLen {
		t.Fatalf("trailer length: want %d, got %d", wantLen, len(trailer))
	}

	// trailer[0..32]   = appDomainSep
	// trailer[32..64]  = orderStructHash
	// trailer[64..-2]  = contentsType (orderTypeStringV2)
	// trailer[-2:]     = uint16_be(len(contentsType))
	if !bytes.Equal(trailer[:32], appDomainSep.Bytes()) {
		t.Errorf("trailer[0..32] != appDomainSep")
	}
	if !bytes.Equal(trailer[32:64], orderStructHash.Bytes()) {
		t.Errorf("trailer[32..64] != orderStructHash")
	}
	if string(trailer[64:len(trailer)-2]) != orderTypeStringV2 {
		t.Errorf("trailer contentsType slice doesn't match orderTypeStringV2")
	}
	gotLen := binary.BigEndian.Uint16(trailer[len(trailer)-2:])
	if int(gotLen) != len(orderTypeStringV2) {
		t.Errorf("trailer length suffix: want %d, got %d", len(orderTypeStringV2), gotLen)
	}
}

// TestWrapERC7739_TypeHash pins the TypedDataSign type hash for the V2 Order
// contentsType. If this breaks, the on-chain ERC-1271 verifier will reject any
// signature this SDK produces.
func TestWrapERC7739_TypeHash(t *testing.T) {
	want := common.HexToHash("0x6ba028565cb324c2aa02bb714b9816d0bddd557a2f33bb36cf13272a4256bd42")
	typeString := "TypedDataSign(Order" + envelopeMid + orderTypeStringV2
	got := crypto.Keccak256Hash([]byte(typeString))
	if got != want {
		t.Errorf("typedDataSignTypeHash: want %s, got %s", want.Hex(), got.Hex())
	}
}

// TestWrapERC7739_RoundtripReconstruction matches what the wallet does on-chain
// before accepting the wrapped signature: it reconstructs
// keccak256("\x19\x01" || appDomainSep || contents) and compares to the `hash`
// argument passed to isValidSignature. The trailer carries appDomainSep and
// contents in the exact byte order the wallet reads.
func TestWrapERC7739_RoundtripReconstruction(t *testing.T) {
	orderStructHash := common.HexToHash("0xdeadbeef" + strings.Repeat("00", 28))
	appDomainSep := common.HexToHash("0xbaadc0de" + strings.Repeat("00", 28))
	depositWallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")

	_, trailer, err := WrapERC7739(orderStructHash, appDomainSep, depositWallet, 137, orderTypeStringV2)
	if err != nil {
		t.Fatalf("WrapERC7739: %v", err)
	}

	// Wallet's reconstruction: keccak256(memory[0x1e..0x60]) where memory[0x1e..0x60] =
	// 0x1901 || trailer[0..32] || trailer[32..64].
	walletInput := append([]byte{0x19, 0x01}, trailer[:64]...)
	walletReconstructed := crypto.Keccak256Hash(walletInput)

	// What the V2 Exchange passes as `hash` to isValidSignature is the unwrapped
	// EIP-712 digest = keccak256("\x19\x01" || appDomainSep || orderStructHash).
	exchangePassed := crypto.Keccak256Hash([]byte{0x19, 0x01}, appDomainSep.Bytes(), orderStructHash.Bytes())

	if walletReconstructed != exchangePassed {
		t.Errorf("wallet reconstruction != exchange-passed hash:\n  wallet=%s\n  exch  =%s",
			walletReconstructed.Hex(), exchangePassed.Hex())
	}
}

func TestWrapERC7739_RejectsBadInputs(t *testing.T) {
	zero := common.Hash{}
	addr := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")

	cases := []struct {
		name           string
		wallet         common.Address
		chainID        int
		contentsType   string
		wantSubstring  string
	}{
		{"zero wallet", common.Address{}, 137, orderTypeStringV2, "zero"},
		{"non-positive chain", addr, 0, orderTypeStringV2, "chainID"},
		{"contentsType missing paren", addr, 137, "Order", "must start with a name followed by '('"},
		{"contentsType empty name", addr, 137, "(uint256 x)", "must start with a name"},
		{"contentsType bad char in name", addr, 137, "Or,der(uint256 x)", "invalid byte"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := WrapERC7739(zero, zero, c.wallet, c.chainID, c.contentsType)
			if err == nil || !strings.Contains(err.Error(), c.wantSubstring) {
				t.Fatalf("want error containing %q, got %v", c.wantSubstring, err)
			}
		})
	}
}

func TestAssembleWrappedSignature(t *testing.T) {
	sig := make([]byte, 65)
	for i := range sig {
		sig[i] = byte(i)
	}
	trailer := []byte{0xaa, 0xbb, 0xcc}
	out, err := AssembleWrappedSignature(sig, trailer)
	if err != nil {
		t.Fatalf("AssembleWrappedSignature: %v", err)
	}
	if len(out) != 68 {
		t.Fatalf("len=%d, want 68", len(out))
	}
	if !bytes.Equal(out[:65], sig) || !bytes.Equal(out[65:], trailer) {
		t.Errorf("output not sig||trailer")
	}

	if _, err := AssembleWrappedSignature(sig[:64], trailer); err == nil {
		t.Errorf("expected error on 64-byte input")
	}
}
