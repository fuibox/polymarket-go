package depositwallet

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Pinned typehashes from WalletLib.sol (verified on-chain at
// 0x58CA52ebe0DadfdF531Cde7062e76746de4Db1eB). If these drift the wallet's
// ERC-1271 verifier will reject every signature this SDK produces.
func TestBatchTypeHashes(t *testing.T) {
	if got := crypto.Keccak256Hash([]byte(BatchTypeString)); got != BatchTypeHash {
		t.Errorf("BatchTypeHash: want %s, got %s", BatchTypeHash.Hex(), got.Hex())
	}
	if got := crypto.Keccak256Hash([]byte(CallTypeString)); got != CallTypeHash {
		t.Errorf("CallTypeHash: want %s, got %s", CallTypeHash.Hex(), got.Hex())
	}
}

// TestBatchDigest_Golden — golden vector computed in Python (Crypto.Hash.keccak)
// against the same Solidity-equivalent inputs. Any digest drift indicates a
// wire-format break that would make the wallet reject the signature.
func TestBatchDigest_Golden(t *testing.T) {
	wallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")
	target := common.HexToAddress("0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB") // pUSD
	// ERC-20 approve(spender=ExchangeV2, amount=max)
	data, _ := hex.DecodeString("095ea7b3" +
		"000000000000000000000000" + "e111180000d2663c0091e4f400237545b87b996b" +
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	b := Batch{
		Wallet:   wallet,
		Nonce:    0,
		Deadline: 1760000000,
		Calls: []Call{
			{Target: target, Value: big.NewInt(0), Data: data},
		},
	}

	digest, err := BatchDigest(b, 137)
	if err != nil {
		t.Fatalf("BatchDigest: %v", err)
	}
	want := common.HexToHash("0x594af421611330195fdb86cff2f3924bf98dc6ee232b2baa1f8d8062eadb2cb7")
	if digest != want {
		t.Errorf("batch digest mismatch:\n  want %s\n  got  %s", want.Hex(), digest.Hex())
	}
}

func TestBatchDigest_RejectsBadInputs(t *testing.T) {
	target := common.HexToAddress("0x1111111111111111111111111111111111111111")
	wallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")
	goodCalls := []Call{{Target: target, Value: big.NewInt(0), Data: []byte{}}}

	cases := []struct {
		name        string
		batch       Batch
		chainID     int
		wantContain string
	}{
		{"zero wallet", Batch{Wallet: common.Address{}, Calls: goodCalls}, 137, "zero"},
		{"non-positive chain", Batch{Wallet: wallet, Calls: goodCalls}, 0, "chainID"},
		{"empty calls", Batch{Wallet: wallet, Calls: nil}, 137, "at least one call"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := BatchDigest(c.batch, c.chainID)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", c.wantContain)
			}
		})
	}
}

func TestEncodeCallsForRelayer(t *testing.T) {
	target := common.HexToAddress("0xAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAa")
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	out := EncodeCallsForRelayer([]Call{
		{Target: target, Value: big.NewInt(42), Data: data},
		{Target: target, Value: nil, Data: nil}, // nil value → "0", nil data → "0x"
	})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0]["target"] != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("target lowercased: got %s", out[0]["target"])
	}
	if out[0]["value"] != "42" || out[0]["data"] != "0xdeadbeef" {
		t.Errorf("encoded values wrong: %+v", out[0])
	}
	if out[1]["value"] != "0" || out[1]["data"] != "0x" {
		t.Errorf("nil defaults wrong: %+v", out[1])
	}
}
