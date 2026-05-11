package utils_order_builder_v2

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fuibox/polymarket-go/client/constants"
	"github.com/fuibox/polymarket-go/client/relayer/model/polyEip712"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
)

// A fixed, well-known test private key. Test-only; this wallet must NEVER hold funds.
const testPrivHex = "47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a"

// Mainnet v2 CTF Exchange — from docs.polymarket.com/resources/contracts.
const testV2ExchangeHex = "0xE111180000d2663C0091e4f400237545B87B996B"

// TestOrderTypeStringKeccak independently confirms the keccak256 of the v2
// orderTypeString matches what OrderTypeHash computes.
func TestOrderTypeStringKeccak(t *testing.T) {
	expected := crypto.Keccak256Hash([]byte(orderTypeStringV2))
	got := (&OrderV2{}).OrderTypeHash()
	if expected != got {
		t.Fatalf("type-hash mismatch:\n  want %s\n  got  %s", expected.Hex(), got.Hex())
	}
	// Sanity: if any field ordering drifts, the string itself changes. Pin it.
	want := "Order(uint256 salt,address maker,address signer,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint8 side,uint8 signatureType,uint256 timestamp,bytes32 metadata,bytes32 builder)"
	if orderTypeStringV2 != want {
		t.Fatalf("orderTypeStringV2 drifted:\n  want %q\n  got  %q", want, orderTypeStringV2)
	}
}

// TestOrderStructHashByteLayout verifies the exact 32-byte-per-field encoding
// (11 fields + type hash = 12 slots = 384 bytes before the final keccak).
func TestOrderStructHashByteLayout(t *testing.T) {
	order := fixedTestOrder()
	// Recompute by hand using the same padding rules and confirm both match.
	var buf bytes.Buffer
	buf.Write(order.OrderTypeHash().Bytes())
	buf.Write(pad32Uint(order.Salt))
	buf.Write(pad32Addr(order.Maker))
	buf.Write(pad32Addr(order.Signer))
	buf.Write(pad32Uint(order.TokenID))
	buf.Write(pad32Uint(order.MakerAmount))
	buf.Write(pad32Uint(order.TakerAmount))
	buf.Write(pad32Uint(new(big.Int).SetUint64(uint64(order.Side))))
	buf.Write(pad32Uint(new(big.Int).SetUint64(uint64(order.SignatureType))))
	buf.Write(pad32Uint(order.Timestamp))
	buf.Write(order.Metadata[:])
	buf.Write(order.Builder[:])
	if buf.Len() != 32*12 {
		t.Fatalf("encoded length = %d, want %d", buf.Len(), 32*12)
	}
	expected := crypto.Keccak256Hash(buf.Bytes())
	got, err := order.OrderStructHash()
	if err != nil {
		t.Fatalf("OrderStructHash err: %v", err)
	}
	if expected != got {
		t.Fatalf("struct-hash mismatch:\n  want %s\n  got  %s", expected.Hex(), got.Hex())
	}
}

// TestBuildSignedOrder_SignatureRoundTrip signs a deterministic OrderV2 with a
// known private key and ecrecovers the signer — proves the digest flow is
// internally consistent and the signature fits the expected 65-byte layout.
func TestBuildSignedOrder_SignatureRoundTrip(t *testing.T) {
	priv, err := crypto.HexToECDSA(testPrivHex)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	addr := crypto.PubkeyToAddress(priv.PublicKey)

	signerHandler, err := signer.NewSigner(signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		ChainID:          137,
		PrivateKeyConfig: &signer.PrivateKeyClient{PrivateKey: priv, Address: addr},
	})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	b, err := NewUtilsOrderBuilderV2(common.HexToAddress(testV2ExchangeHex), 137, signerHandler, Option{})
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	data := OrderDataV2{
		Maker:         addr,
		Signer:        addr,
		TokenID:       "123456789012345678901234567890",
		MakerAmount:   "1146000",
		TakerAmount:   "6000000",
		Side:          0, // BUY
		SignatureType: constants.EOA,
		Timestamp:     "1713398400000",
		Metadata:      "0x0000000000000000000000000000000000000000000000000000000000000000",
		Builder:       "0x0000000000000000000000000000000000000000000000000000000000000000",
	}

	signed, err := b.BuildSignedOrder(data)
	if err != nil {
		t.Fatalf("BuildSignedOrder: %v", err)
	}

	// Signature must be 0x + 130 hex chars (65 bytes: r||s||v).
	sigHex := strings.TrimPrefix(signed.Signature, "0x")
	if len(sigHex) != 130 {
		t.Fatalf("signature hex length = %d, want 130", len(sigHex))
	}
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	// ecrecover expects v=0 or v=1 in the last byte (go-ethereum convention).
	// SigToPub handles both 0/1 and 27/28.
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}

	// Reproduce the digest independently.
	order, err := b.buildOrder(data)
	if err != nil {
		t.Fatalf("buildOrder: %v", err)
	}
	// The builder generates a random salt; override with what got signed.
	order.Salt = big.NewInt(signed.Salt)
	structHash, err := order.OrderStructHash()
	if err != nil {
		t.Fatalf("struct hash: %v", err)
	}
	name := "Polymarket CTF Exchange"
	version := "2"
	chainId := 137
	contract := testV2ExchangeHex
	dom := polyEip712.MakeDomain(&name, &version, &chainId, &contract, nil)
	dsep, err := dom.HashStruct()
	if err != nil {
		t.Fatalf("domain hash: %v", err)
	}
	digest := order.OrderEIP712Digest(common.BytesToHash(dsep[:]), structHash)

	pub, err := crypto.SigToPub(digest.Bytes(), sigBytes)
	if err != nil {
		t.Fatalf("ecrecover: %v", err)
	}
	recovered := crypto.PubkeyToAddress(*pub)
	if recovered != addr {
		t.Fatalf("recovered signer = %s, want %s", recovered.Hex(), addr.Hex())
	}
}

// TestBuildSignedOrder_POLY1271 covers the deposit-wallet path: signature
// length should exceed 65 bytes (ECDSA + ERC-7739 trailer), and the trailer's
// reconstructed digest must equal the unwrapped V2 EIP-712 digest — which is
// what the wallet's on-chain isValidSignature checks before unwrapping.
func TestBuildSignedOrder_POLY1271(t *testing.T) {
	priv, err := crypto.HexToECDSA(testPrivHex)
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	eoa := crypto.PubkeyToAddress(priv.PublicKey)
	depositWallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")

	signerHandler, err := signer.NewSigner(signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		ChainID:          137,
		PrivateKeyConfig: &signer.PrivateKeyClient{PrivateKey: priv, Address: eoa},
	})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	b, err := NewUtilsOrderBuilderV2(common.HexToAddress(testV2ExchangeHex), 137, signerHandler, Option{})
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	data := OrderDataV2{
		Maker:         depositWallet,
		Signer:        depositWallet,
		TokenID:       "123456789012345678901234567890",
		MakerAmount:   "1146000",
		TakerAmount:   "6000000",
		Side:          0,
		SignatureType: constants.POLY_1271,
		Timestamp:     "1713398400000",
		Metadata:      "0x0000000000000000000000000000000000000000000000000000000000000000",
		Builder:       "0x0000000000000000000000000000000000000000000000000000000000000000",
	}

	signed, err := b.BuildSignedOrder(data)
	if err != nil {
		t.Fatalf("BuildSignedOrder: %v", err)
	}

	if signed.SignatureType != types.SignatureType(constants.POLY_1271) {
		t.Errorf("SignatureType: want %d, got %d", constants.POLY_1271, signed.SignatureType)
	}
	if signed.Maker != depositWallet.Hex() || signed.Signer != depositWallet.Hex() {
		t.Errorf("maker/signer must both equal deposit wallet")
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signed.Signature, "0x"))
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	// Expected layout: 65 (ECDSA) + 32 (appDomainSep) + 32 (contents) + len(contentsType) + 2 (uint16)
	wantLen := 65 + 32 + 32 + len(orderTypeStringV2) + 2
	if len(sigBytes) != wantLen {
		t.Fatalf("wrapped sig length = %d, want %d", len(sigBytes), wantLen)
	}

	// Trailer bytes start at offset 65. Reconstruct the unwrapped V2 digest and
	// confirm it matches keccak256("\x19\x01" || trailer[0..64]).
	trailer := sigBytes[65:]
	walletReconstructed := crypto.Keccak256Hash(append([]byte{0x19, 0x01}, trailer[:64]...))

	order, err := b.buildOrder(data)
	if err != nil {
		t.Fatalf("buildOrder: %v", err)
	}
	order.Salt = big.NewInt(signed.Salt)
	structHash, err := order.OrderStructHash()
	if err != nil {
		t.Fatalf("struct hash: %v", err)
	}
	name := "Polymarket CTF Exchange"
	version := "2"
	chainId := 137
	contract := testV2ExchangeHex
	dom := polyEip712.MakeDomain(&name, &version, &chainId, &contract, nil)
	dsep, err := dom.HashStruct()
	if err != nil {
		t.Fatalf("domain hash: %v", err)
	}
	exchangeDigest := order.OrderEIP712Digest(common.BytesToHash(dsep[:]), structHash)

	if walletReconstructed != exchangeDigest {
		t.Fatalf("trailer reconstruction != unwrapped exchange digest:\n  wallet=%s\n  exch  =%s",
			walletReconstructed.Hex(), exchangeDigest.Hex())
	}
}

// TestBuildSignedOrder_POLY1271_RejectsMismatchedMakerSigner enforces the
// docs' constraint at the SDK boundary.
func TestBuildSignedOrder_POLY1271_RejectsMismatchedMakerSigner(t *testing.T) {
	priv, _ := crypto.HexToECDSA(testPrivHex)
	signerHandler, _ := signer.NewSigner(signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		ChainID:          137,
		PrivateKeyConfig: &signer.PrivateKeyClient{PrivateKey: priv, Address: crypto.PubkeyToAddress(priv.PublicKey)},
	})
	b, _ := NewUtilsOrderBuilderV2(common.HexToAddress(testV2ExchangeHex), 137, signerHandler, Option{})

	data := OrderDataV2{
		Maker:         common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013"),
		Signer:        common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
		TokenID:       "1",
		MakerAmount:   "1",
		TakerAmount:   "1",
		Side:          0,
		SignatureType: constants.POLY_1271,
		Timestamp:     "1713398400000",
	}
	_, err := b.BuildSignedOrder(data)
	if err == nil || !strings.Contains(err.Error(), "maker and signer must both equal") {
		t.Fatalf("want maker/signer mismatch error, got %v", err)
	}
}

// TestParseBytes32_Strict confirms short hex is rejected (no silent padding).
func TestParseBytes32_Strict(t *testing.T) {
	if _, err := parseBytes32(""); err != nil {
		t.Fatalf("empty string should yield zero without error, got %v", err)
	}
	if _, err := parseBytes32("0xdead"); err == nil {
		t.Fatalf("short hex must be rejected")
	}
	if _, err := parseBytes32("0x" + strings.Repeat("ab", 32)); err != nil {
		t.Fatalf("full 32-byte hex failed: %v", err)
	}
}

// --- helpers ---

func fixedTestOrder() *OrderV2 {
	priv, _ := crypto.HexToECDSA(testPrivHex)
	addr := crypto.PubkeyToAddress(priv.PublicKey)
	tokenId, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	var meta, builder [32]byte
	meta[31] = 0x42
	copy(builder[:], bytes.Repeat([]byte{0xab}, 32))
	return &OrderV2{
		Salt:          big.NewInt(987654321),
		Maker:         addr,
		Signer:        addr,
		TokenID:       tokenId,
		MakerAmount:   big.NewInt(1146000),
		TakerAmount:   big.NewInt(6000000),
		Side:          0,
		SignatureType: uint8(constants.EOA),
		Timestamp:     big.NewInt(1713398400000),
		Metadata:      meta,
		Builder:       builder,
	}
}

func pad32Uint(x *big.Int) []byte {
	b := make([]byte, 32)
	x.FillBytes(b)
	return b
}

func pad32Addr(a common.Address) []byte {
	b := make([]byte, 32)
	copy(b[12:], a.Bytes())
	return b
}
