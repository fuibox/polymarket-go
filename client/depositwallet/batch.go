package depositwallet

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fuibox/polymarket-go/client/signer"
)

// EIP-712 type strings used by the deposit wallet's Batch authorisation flow
// (source: WalletLib.sol on the deployed wallet implementation
// 0x58CA52ebe0DadfdF531Cde7062e76746de4Db1eB). The Batch type string contains
// the Call type appended at the end per EIP-712 nested-type encoding rules.
const (
	BatchTypeString = "Batch(address wallet,uint256 nonce,uint256 deadline,Call[] calls)Call(address target,uint256 value,bytes data)"
	CallTypeString  = "Call(address target,uint256 value,bytes data)"
)

// Pre-computed EIP-712 typehashes. Pinned as constants so a typo in the type
// string never silently produces a different on-chain digest.
var (
	BatchTypeHash  = common.HexToHash("0x712ef66e8362c387e862cabf0923c209db0fa24cfc97d25eccba7c86f3ee1dd3")
	CallTypeHash   = common.HexToHash("0x84fa2cf05cd88e992eae77e851af68a4ee278dcff6ef504e487a55b3baadfbe5")
	domainTypeHash = common.HexToHash("0x8b73c3c69bb8fe3d512ecc4cf759cc79239f7b179b0ffacaa9a75d522b39400f")
)

// Call is one step of a deposit wallet Batch — a single CALL that the wallet
// will execute against `Target` with `Value` wei of native asset and `Data`
// as calldata.
type Call struct {
	Target common.Address
	Value  *big.Int
	Data   []byte
}

// Batch is the EIP-712 payload the deposit wallet owner / session signer signs
// to authorise a sequence of on-chain calls executed by the wallet.
type Batch struct {
	Wallet   common.Address
	Nonce    uint64
	Deadline uint64
	Calls    []Call
}

// HashCall returns the EIP-712 struct hash of a single Call.
func HashCall(c Call) common.Hash {
	value := c.Value
	if value == nil {
		value = new(big.Int)
	}
	buf := make([]byte, 0, 4*32)
	buf = append(buf, CallTypeHash.Bytes()...)
	buf = append(buf, padAddress(c.Target)...)
	buf = append(buf, padUint(value)...)
	buf = append(buf, crypto.Keccak256(c.Data)...)
	return crypto.Keccak256Hash(buf)
}

// HashCalls returns keccak256(call0Hash || call1Hash || ...) per EIP-712's
// dynamic-array hashing rule.
func HashCalls(calls []Call) common.Hash {
	buf := make([]byte, 0, 32*len(calls))
	for _, c := range calls {
		buf = append(buf, HashCall(c).Bytes()...)
	}
	return crypto.Keccak256Hash(buf)
}

// HashBatch returns the EIP-712 struct hash of a Batch.
func HashBatch(b Batch) common.Hash {
	buf := make([]byte, 0, 5*32)
	buf = append(buf, BatchTypeHash.Bytes()...)
	buf = append(buf, padAddress(b.Wallet)...)
	buf = append(buf, padUint(new(big.Int).SetUint64(b.Nonce))...)
	buf = append(buf, padUint(new(big.Int).SetUint64(b.Deadline))...)
	buf = append(buf, HashCalls(b.Calls).Bytes()...)
	return crypto.Keccak256Hash(buf)
}

// BatchDomainSeparator returns the EIP-712 domain separator the deposit
// wallet uses for Batch signatures: name="DepositWallet", version="1",
// chainId, verifyingContract=wallet. Salt is intentionally omitted (Solady's
// EIP712 base class does not include it in the domain).
func BatchDomainSeparator(wallet common.Address, chainID int) common.Hash {
	buf := make([]byte, 0, 5*32)
	buf = append(buf, domainTypeHash.Bytes()...)
	buf = append(buf, crypto.Keccak256([]byte(depositWalletDomainName))...)
	buf = append(buf, crypto.Keccak256([]byte(depositWalletDomainVersion))...)
	buf = append(buf, padUint(new(big.Int).SetInt64(int64(chainID)))...)
	buf = append(buf, padAddress(wallet)...)
	return crypto.Keccak256Hash(buf)
}

// BatchDigest returns the final 32-byte hash the owner / session signer must
// sign: keccak256("\x19\x01" || domainSeparator || hashStruct(Batch)).
func BatchDigest(b Batch, chainID int) (common.Hash, error) {
	if b.Wallet == (common.Address{}) {
		return common.Hash{}, errors.New("batch wallet address is zero")
	}
	if chainID <= 0 {
		return common.Hash{}, fmt.Errorf("chainID must be positive, got %d", chainID)
	}
	if len(b.Calls) == 0 {
		return common.Hash{}, errors.New("batch must contain at least one call (wallet rejects empty batches)")
	}
	domain := BatchDomainSeparator(b.Wallet, chainID)
	structHash := HashBatch(b)
	digestBytes := crypto.Keccak256(
		[]byte{0x19, 0x01},
		domain.Bytes(),
		structHash.Bytes(),
	)
	return common.BytesToHash(digestBytes), nil
}

// SignBatch returns a 0x-prefixed 65-byte ECDSA signature suitable for the
// relayer's WALLET endpoint. The deposit wallet's on-chain ERC-1271 verifier
// short-circuits to direct ECDSA when the caller is the factory, which is
// exactly the relayer's execution path — so no ERC-7739 wrapping is needed.
//
// For Turnkey-managed keys, callers should compute BatchDigest themselves
// and pass it to signer.SignHashWithTurnkey directly.
func SignBatch(s *signer.Signer, b Batch, chainID int) (string, error) {
	digest, err := BatchDigest(b, chainID)
	if err != nil {
		return "", err
	}
	return s.SignHash(digest.Hex())
}

// EncodeCallsForRelayer renders the Calls slice into the JSON shape the
// relayer's depositWalletParams.calls expects: target as hex address,
// value/data as 0x-prefixed strings.
func EncodeCallsForRelayer(calls []Call) []map[string]string {
	out := make([]map[string]string, 0, len(calls))
	for _, c := range calls {
		value := c.Value
		if value == nil {
			value = new(big.Int)
		}
		out = append(out, map[string]string{
			"target": strings.ToLower(c.Target.Hex()),
			"value":  value.String(),
			"data":   "0x" + hex.EncodeToString(c.Data),
		})
	}
	return out
}

func padAddress(a common.Address) []byte {
	b := make([]byte, 32)
	copy(b[12:], a.Bytes())
	return b
}

func padUint(x *big.Int) []byte {
	b := make([]byte, 32)
	if x.Sign() < 0 {
		// EIP-712 doesn't define negative uints; clamp defensively.
		x = new(big.Int)
	}
	x.FillBytes(b)
	return b
}
