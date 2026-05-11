// Package depositwallet derives Polymarket deposit wallet addresses and builds
// ERC-7739-wrapped signatures for V2 CLOB orders signed by a deposit wallet
// (POLY_1271 signature type).
//
// A deposit wallet is a per-user ERC-1967 minimal proxy deployed by the
// DepositWalletFactory using Solady's LibClone.deployDeterministicERC1967
// (immutable-args variant). The factory and implementation contracts live on
// Polygon mainnet at the addresses recorded in client/config.
package depositwallet

import (
	"errors"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fuibox/polymarket-go/client/config"
	"github.com/fuibox/polymarket-go/client/types"
)

// ErrChainNotConfigured is returned when deposit wallet contracts are not
// configured for the requested chain (e.g. Amoy testnet).
var ErrChainNotConfigured = errors.New("deposit wallet contracts not configured for this chain")

// init code template fragments for Solady ERC-1967 minimal proxy with immutable
// args, length n = 0x40 (= abi.encode(address,bytes32)). The PUSH2 immediate
// 0x007d encodes runtime size = 0x3d + n; changing args length would require
// recomputing this byte. Verified against on-chain wallet 0xC493...6013 whose
// deployed code length is exactly 0x7d = 125 bytes.
var (
	initCodePrefix = []byte{0x61, 0x00, 0x7d, 0x3d, 0x81, 0x60, 0x23, 0x3d, 0x39, 0x73}
	initCodeMid    = []byte{0x60, 0x09}
	initCodeConst1 = common.FromHex("0x5155f3363d3d373d3d363d7f360894a13ba1a3210667c828492db98dca3e2076")
	initCodeConst2 = common.FromHex("0xcc3735a920a3ca505d382bbc545af43d6000803e6038573d6000fd5b3d6000f3")
)

// DeriveDepositWalletAddress reproduces the factory's predictWalletAddress
// view: walletId = bytes32(owner); args = abi.encode(factory, walletId);
// salt = keccak256(args); the wallet is CREATE2-deployed from `factory` with
// initCode being Solady's ERC-1967 immutable-args proxy template parameterised
// by `implementation` and `args`.
func DeriveDepositWalletAddress(owner, factory, implementation common.Address) (common.Address, error) {
	if owner == (common.Address{}) {
		return common.Address{}, errors.New("owner address is zero")
	}
	if factory == (common.Address{}) {
		return common.Address{}, errors.New("factory address is zero")
	}
	if implementation == (common.Address{}) {
		return common.Address{}, errors.New("implementation address is zero")
	}

	// walletId = bytes32(owner), left-padded.
	var walletId [32]byte
	copy(walletId[12:], owner.Bytes())

	args, err := abiEncodeFactoryAndId(factory, walletId)
	if err != nil {
		return common.Address{}, err
	}
	salt := crypto.Keccak256(args)

	initCode := make([]byte, 0, 160)
	initCode = append(initCode, initCodePrefix...)
	initCode = append(initCode, implementation.Bytes()...)
	initCode = append(initCode, initCodeMid...)
	initCode = append(initCode, initCodeConst1...)
	initCode = append(initCode, initCodeConst2...)
	initCode = append(initCode, args...)
	initCodeHash := crypto.Keccak256(initCode)

	buf := make([]byte, 0, 1+20+32+32)
	buf = append(buf, 0xff)
	buf = append(buf, factory.Bytes()...)
	buf = append(buf, salt...)
	buf = append(buf, initCodeHash...)

	return common.BytesToAddress(crypto.Keccak256(buf)[12:]), nil
}

// DeriveDepositWalletForChain looks up the factory and implementation from
// client/config for the given chain. Returns ErrChainNotConfigured if either
// address is zero on that chain.
func DeriveDepositWalletForChain(owner common.Address, chainID types.Chain) (common.Address, error) {
	cfg, err := config.GetContractConfig(chainID)
	if err != nil {
		return common.Address{}, err
	}
	if cfg.DepositWalletFactory == (common.Address{}) || cfg.DepositWalletImplementation == (common.Address{}) {
		return common.Address{}, ErrChainNotConfigured
	}
	return DeriveDepositWalletAddress(owner, cfg.DepositWalletFactory, cfg.DepositWalletImplementation)
}

func abiEncodeFactoryAndId(factory common.Address, walletId [32]byte) ([]byte, error) {
	addrType, err := abi.NewType("address", "", nil)
	if err != nil {
		return nil, err
	}
	bytes32Type, err := abi.NewType("bytes32", "", nil)
	if err != nil {
		return nil, err
	}
	args := abi.Arguments{{Type: addrType}, {Type: bytes32Type}}
	return args.Pack(factory, walletId)
}
