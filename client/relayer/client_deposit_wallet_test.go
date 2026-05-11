package relayer

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/fuibox/polymarket-go/client/depositwallet"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/tools/headers"
)

// Deterministic builder config so HMAC-derived headers vary only with
// timestamp; tests don't pin those.
func testBuilderConfig() *headers.BuilderConfig {
	return &headers.BuilderConfig{
		APIKey:     "test-key",
		Secret:     "dGVzdC1zZWNyZXQtbWluLTMyLWNoYXJzLWxvbmctMTIzNDU2",
		Passphrase: "test-passphrase",
	}
}

func newTestRelayClient(t *testing.T, srv *httptest.Server) *RelayClient {
	t.Helper()
	priv, err := crypto.HexToECDSA("47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a")
	if err != nil {
		t.Fatalf("priv: %v", err)
	}
	sigHandler, err := signer.NewSigner(signer.SignerConfig{
		SignerType: signer.PrivateKey,
		ChainID:    137,
		PrivateKeyConfig: &signer.PrivateKeyClient{
			PrivateKey: priv,
			Address:    crypto.PubkeyToAddress(priv.PublicKey),
		},
	})
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	dummyRPC := "http://invalid/"
	rc, err := NewRelayClient(srv.URL, 137, sigHandler, testBuilderConfig(), nil, &dummyRPC)
	if err != nil {
		t.Fatalf("new relay client: %v", err)
	}
	return rc
}

// TestCreateDepositWallet_WireFormat asserts the POST /submit body shape
// matches the docs: {type:"WALLET-CREATE", from, to=factory} with no
// signature, plus Builder HMAC headers.
func TestCreateDepositWallet_WireFormat(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submit" || r.Method != "POST" {
			t.Errorf("path/method: %s %s", r.Method, r.URL.Path)
		}
		// API_KEY, PASSPHRASE, SIGNATURE are populated from BuilderConfig +
		// HMAC; TIMESTAMP is passed as "" by the existing submit() helper
		// (relayer tolerates it). Mirror that behavior here.
		for _, h := range []string{"POLY_BUILDER_API_KEY", "POLY_BUILDER_PASSPHRASE", "POLY_BUILDER_SIGNATURE"} {
			if r.Header.Get(h) == "" {
				t.Errorf("missing header %s", h)
			}
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("body json: %v (raw=%s)", err, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"transactionID":"deploy-tx-1","transactionHash":"0xabc"}`))
	}))
	defer srv.Close()

	rc := newTestRelayClient(t, srv)
	owner := common.HexToAddress("0xc7d8944254ae16a13ec406745cd32b467979c18d")

	resp, err := rc.CreateDepositWallet(owner)
	if err != nil {
		t.Fatalf("CreateDepositWallet: %v", err)
	}
	if resp.TransactionID != "deploy-tx-1" {
		t.Errorf("txID: %q", resp.TransactionID)
	}
	if captured["type"] != "WALLET-CREATE" {
		t.Errorf("type: %v", captured["type"])
	}
	if !strings.EqualFold(captured["from"].(string), owner.Hex()) {
		t.Errorf("from: %v", captured["from"])
	}
	factory := common.HexToAddress("0x00000000000Fb5C9ADea0298D729A0CB3823Cc07")
	if !strings.EqualFold(captured["to"].(string), factory.Hex()) {
		t.Errorf("to: %v want %s", captured["to"], factory.Hex())
	}
	if _, ok := captured["signature"]; ok {
		t.Errorf("WALLET-CREATE body must not include signature, got %v", captured["signature"])
	}
}

// TestSubmitDepositWalletBatch_WireFormat asserts the POST /submit body for a
// WALLET batch matches the docs (depositWalletParams shape, target/value/data
// per call, etc.).
func TestSubmitDepositWalletBatch_WireFormat(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("body json: %v (raw=%s)", err, body)
		}
		_, _ = w.Write([]byte(`{"transactionID":"batch-tx-1"}`))
	}))
	defer srv.Close()

	rc := newTestRelayClient(t, srv)
	owner := common.HexToAddress("0xc7d8944254ae16a13ec406745cd32b467979c18d")
	wallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")
	target := common.HexToAddress("0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB")
	calls := []depositwallet.Call{
		{Target: target, Value: big.NewInt(0), Data: []byte{0xde, 0xad}},
	}
	signature := "0x" + strings.Repeat("ab", 65)

	resp, err := rc.SubmitDepositWalletBatch(owner, wallet, 7, 1760000000, calls, signature)
	if err != nil {
		t.Fatalf("SubmitDepositWalletBatch: %v", err)
	}
	if resp.TransactionID != "batch-tx-1" {
		t.Errorf("txID: %q", resp.TransactionID)
	}
	if captured["type"] != "WALLET" {
		t.Errorf("type: %v", captured["type"])
	}
	if !strings.EqualFold(captured["from"].(string), owner.Hex()) {
		t.Errorf("from: %v", captured["from"])
	}
	if captured["nonce"] != "7" {
		t.Errorf("nonce: %v", captured["nonce"])
	}
	if captured["signature"] != signature {
		t.Errorf("signature: %v", captured["signature"])
	}
	params, ok := captured["depositWalletParams"].(map[string]any)
	if !ok {
		t.Fatalf("depositWalletParams missing/invalid: %v", captured["depositWalletParams"])
	}
	if !strings.EqualFold(params["depositWallet"].(string), wallet.Hex()) {
		t.Errorf("depositWallet: %v", params["depositWallet"])
	}
	if params["deadline"] != strconv.FormatInt(1760000000, 10) {
		t.Errorf("deadline: %v", params["deadline"])
	}
	encodedCalls, ok := params["calls"].([]any)
	if !ok || len(encodedCalls) != 1 {
		t.Fatalf("calls: %v", params["calls"])
	}
	call0 := encodedCalls[0].(map[string]any)
	if !strings.EqualFold(call0["target"].(string), target.Hex()) || call0["value"] != "0" || call0["data"] != "0xdead" {
		t.Errorf("call[0] wrong: %+v", call0)
	}
}

func TestSubmitDepositWalletBatch_RejectsShortSig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit for short sig")
	}))
	defer srv.Close()
	rc := newTestRelayClient(t, srv)

	wallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")
	target := common.HexToAddress("0x1111111111111111111111111111111111111111")
	owner := common.HexToAddress("0xc7d8944254ae16a13ec406745cd32b467979c18d")
	_, err := rc.SubmitDepositWalletBatch(owner, wallet, 0, 1, []depositwallet.Call{
		{Target: target, Value: big.NewInt(0)},
	}, "0xabcd")
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("want short-sig error, got %v", err)
	}
}

// TestExecuteDepositWalletBatch_Integration walks the full flow against a
// fake relayer: GetNonce → sign → SubmitDepositWalletBatch. The signature
// posted to /submit must ecrecover to the test signer over the Batch digest.
func TestExecuteDepositWalletBatch_Integration(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/nonce":
			_, _ = w.Write([]byte(`{"nonce":"3"}`))
		case "/submit":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &capturedBody)
			_, _ = w.Write([]byte(`{"transactionID":"ok"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	rc := newTestRelayClient(t, srv)
	priv, _ := crypto.HexToECDSA("47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a")
	owner := crypto.PubkeyToAddress(priv.PublicKey)
	wallet := common.HexToAddress("0xC493511524780Be2B6A26b357187524E5deE6013")
	target := common.HexToAddress("0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB")

	calls := []depositwallet.Call{
		{Target: target, Value: big.NewInt(0), Data: []byte{0x09, 0x5e, 0xa7, 0xb3}},
	}
	deadline := uint64(1760000000)
	if _, err := rc.ExecuteDepositWalletBatch(owner, wallet, deadline, calls); err != nil {
		t.Fatalf("ExecuteDepositWalletBatch: %v", err)
	}

	expectedDigest, err := depositwallet.BatchDigest(depositwallet.Batch{
		Wallet: wallet, Nonce: 3, Deadline: deadline, Calls: calls,
	}, 137)
	if err != nil {
		t.Fatalf("BatchDigest: %v", err)
	}
	sigHex, ok := capturedBody["signature"].(string)
	if !ok {
		t.Fatalf("submitted body missing signature: %+v", capturedBody)
	}
	sig := common.FromHex(sigHex)
	if len(sig) != 65 {
		t.Fatalf("signature length %d, want 65", len(sig))
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(expectedDigest.Bytes(), sig)
	if err != nil {
		t.Fatalf("SigToPub: %v", err)
	}
	if got := crypto.PubkeyToAddress(*pub); got != owner {
		t.Fatalf("ecrecover %s != owner %s", got.Hex(), owner.Hex())
	}
}

// silence unused-import if signer test helper changes later.
var _ = fmt.Sprintf
