package clob

import (
	"crypto/ecdsa"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/fuibox/polymarket-go/client/endpoint"
	"github.com/fuibox/polymarket-go/client/signer"
	"github.com/fuibox/polymarket-go/client/types"
)

// newTestBuilderTradesClient builds a ClobClient pointed at the given test
// server host. The endpoint is public (POC-confirmed), but GetBuilderTrades
// mirrors GetTrades and still generates L2 headers, which requires a non-nil
// signer and creds — neither value is sent anywhere real here.
func newTestBuilderTradesClient(t *testing.T, host string) *ClobClient {
	t.Helper()

	var key *ecdsa.PrivateKey
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signerHandler, err := signer.NewSigner(signer.SignerConfig{
		SignerType:       signer.PrivateKey,
		ChainID:          80002,
		PrivateKeyConfig: &signer.PrivateKeyClient{PrivateKey: key},
	})
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	return &ClobClient{
		host:          host,
		chainID:       types.ChainAmoy,
		signer:        signerHandler,
		creds:         &types.ApiKeyCreds{Key: "test-key", Secret: "test-secret", Passphrase: "test-pass"},
		useServerTime: false,
		httpClient:    &http.Client{},
	}
}

// TestGetAllBuilderTrades_Pagination verifies the pagination loop walks pages
// until next_cursor == END_CURSOR ("LTE=") and aggregates all trades, and that
// each request carries the builder_code and next_cursor query params.
func TestGetAllBuilderTrades_Pagination(t *testing.T) {
	const builderCode = "0xbuilder"

	var gotBuilderCode, gotNextCursor []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != endpoint.GetBuilderTrades {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		q := r.URL.Query()
		gotBuilderCode = append(gotBuilderCode, q.Get("builder_code"))
		gotNextCursor = append(gotNextCursor, q.Get("next_cursor"))

		w.Header().Set("Content-Type", "application/json")
		// Page 1: full of trades, hands back a cursor. Page 2: terminates.
		if q.Get("next_cursor") == types.INITIAL_CURSOR {
			w.Write([]byte(`{"data":[{"id":"1"},{"id":"2"}],"next_cursor":"abc","limit":2,"count":3}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"3"}],"next_cursor":"LTE=","limit":2,"count":3}`))
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	trades, err := c.GetAllBuilderTrades(common.Address{}, builderCode, nil)
	if err != nil {
		t.Fatalf("GetAllBuilderTrades: %v", err)
	}
	if len(trades) != 3 {
		t.Fatalf("expected 3 aggregated trades, got %d", len(trades))
	}
	if len(gotBuilderCode) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(gotBuilderCode))
	}
	for i, bc := range gotBuilderCode {
		if bc != builderCode {
			t.Errorf("request %d: builder_code = %q, want %q", i, bc, builderCode)
		}
	}
	if gotNextCursor[0] != types.INITIAL_CURSOR {
		t.Errorf("first request next_cursor = %q, want %q", gotNextCursor[0], types.INITIAL_CURSOR)
	}
	if gotNextCursor[1] != "abc" {
		t.Errorf("second request next_cursor = %q, want %q", gotNextCursor[1], "abc")
	}
}

// TestGetBuilderTrades_BuilderCodeRequired verifies the empty-builderCode guard.
func TestGetBuilderTrades_BuilderCodeRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be hit when builderCode is empty")
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	_, err := c.GetBuilderTrades(common.Address{}, "", nil, "")
	if err == nil {
		t.Fatal("expected error for empty builderCode, got nil")
	}
}

// TestGetBuilderTrades_FieldMapping verifies the wire fields deserialize into
// BuilderTrade, including the POC-confirmed TRADE_STATUS_ prefixed status and
// the newly added builderCode / builderFee fields.
func TestGetBuilderTrades_FieldMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{
			"id":"t1",
			"status":"TRADE_STATUS_CONFIRMED",
			"err_msg":"boom",
			"feeUsdc":"0.5",
			"builderFee":"0.2",
			"builderCode":"0xbuilder",
			"takerOrderHash":"0xhash",
			"transactionHash":"0xtxn"
		}],"next_cursor":"LTE="}`))
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	resp, err := c.GetBuilderTrades(common.Address{}, "0xbuilder", nil, "")
	if err != nil {
		t.Fatalf("GetBuilderTrades: %v", err)
	}
	if len(resp.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(resp.Trades))
	}
	tr := resp.Trades[0]
	if tr.Status != types.TradeStatusConfirmed {
		t.Errorf("Status = %q, want %q", tr.Status, types.TradeStatusConfirmed)
	}
	if tr.ErrMsg == nil || *tr.ErrMsg != "boom" {
		t.Errorf("ErrMsg = %v, want \"boom\"", tr.ErrMsg)
	}
	if tr.FeeUSDC != "0.5" {
		t.Errorf("FeeUSDC = %q, want \"0.5\"", tr.FeeUSDC)
	}
	if tr.BuilderFee != "0.2" {
		t.Errorf("BuilderFee = %q, want \"0.2\"", tr.BuilderFee)
	}
	if tr.BuilderCode != "0xbuilder" {
		t.Errorf("BuilderCode = %q, want \"0xbuilder\"", tr.BuilderCode)
	}
	if tr.TakerOrderHash != "0xhash" {
		t.Errorf("TakerOrderHash = %q, want \"0xhash\"", tr.TakerOrderHash)
	}
	if tr.TransactionHash != "0xtxn" {
		t.Errorf("TransactionHash = %q, want \"0xtxn\"", tr.TransactionHash)
	}
}

// TestGetBuilderTrades_DataAndTradesKeys verifies the scheme-B dual-key
// tolerance: both {"data":[...]} (the real wire key) and {"trades":[...]}
// (forward-compatible alias) parse into Trades non-empty.
func TestGetBuilderTrades_DataAndTradesKeys(t *testing.T) {
	cases := map[string]string{
		"data key":   `{"data":[{"id":"a"},{"id":"b"}],"next_cursor":"LTE="}`,
		"trades key": `{"trades":[{"id":"a"},{"id":"b"}],"next_cursor":"LTE="}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(body))
			}))
			defer srv.Close()

			c := newTestBuilderTradesClient(t, srv.URL)
			resp, err := c.GetBuilderTrades(common.Address{}, "0xbuilder", nil, "")
			if err != nil {
				t.Fatalf("GetBuilderTrades: %v", err)
			}
			if len(resp.Trades) != 2 {
				t.Fatalf("expected 2 trades, got %d", len(resp.Trades))
			}
		})
	}
}
