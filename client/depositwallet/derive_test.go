package depositwallet

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestDeriveDepositWalletAddress_OnChainVector cross-checks the Go derivation
// against a real Polygon-mainnet deployment: factory.predict(...) for owner
// 0xc7d8...c18d returns 0xC493...6013, and inspecting that wallet on-chain
// shows id() == bytes32(owner) and factory() == 0x00...Cc07. Code length is
// 125 bytes (= 0x7d), which is what the PUSH2 immediate in initCodePrefix
// encodes for n=0x40.
func TestDeriveDepositWalletAddress_OnChainVector(t *testing.T) {
	owner := common.HexToAddress("0xc7d8944254ae16a13ec406745cd32b467979c18d")
	factory := common.HexToAddress("0x00000000000Fb5C9ADea0298D729A0CB3823Cc07")
	impl := common.HexToAddress("0x58CA52ebe0DadfdF531Cde7062e76746de4Db1eB")
	want := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")

	got, err := DeriveDepositWalletAddress(owner, factory, impl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("derived address mismatch:\n  want %s\n  got  %s", want.Hex(), got.Hex())
	}
}

func TestDeriveDepositWalletForChain_Polygon(t *testing.T) {
	owner := common.HexToAddress("0xc7d8944254ae16a13ec406745cd32b467979c18d")
	want := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")

	got, err := DeriveDepositWalletForChain(owner, 137)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("chain-resolved address mismatch: want %s got %s", want.Hex(), got.Hex())
	}
}

func TestDeriveDepositWalletForChain_AmoyUnconfigured(t *testing.T) {
	owner := common.HexToAddress("0xc7d8944254ae16a13ec406745cd32b467979c18d")
	_, err := DeriveDepositWalletForChain(owner, 80002)
	if err != ErrChainNotConfigured {
		t.Fatalf("want ErrChainNotConfigured, got %v", err)
	}
}

func TestDeriveDepositWalletAddress_ZeroInputs(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000001")
	cases := []struct {
		name                       string
		owner, factory, impl       common.Address
		wantErrSubstring           string
	}{
		{"zero owner", common.Address{}, addr, addr, "owner"},
		{"zero factory", addr, common.Address{}, addr, "factory"},
		{"zero impl", addr, addr, common.Address{}, "implementation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := DeriveDepositWalletAddress(c.owner, c.factory, c.impl)
			if err == nil || !strings.Contains(err.Error(), c.wantErrSubstring) {
				t.Fatalf("want error containing %q, got %v", c.wantErrSubstring, err)
			}
		})
	}
}
