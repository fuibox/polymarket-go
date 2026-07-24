package clob

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"

	"github.com/fuibox/polymarket-go/client/endpoint"
	"github.com/fuibox/polymarket-go/client/types"
)

func TestOrderResponseUnmarshalsTradeIDs(t *testing.T) {
	var response types.OrderResponse
	if err := sonic.UnmarshalString(`{"success":true,"orderID":"order-1","tradeIDs":["trade-1","trade-2"]}`, &response); err != nil {
		t.Fatalf("unmarshal order response: %v", err)
	}

	if len(response.TradeIDs) != 2 || response.TradeIDs[0] != "trade-1" || response.TradeIDs[1] != "trade-2" {
		t.Fatalf("TradeIDs = %#v, want [trade-1 trade-2]", response.TradeIDs)
	}
}

func TestPostOrderResolvesTransactionHashesFromTradeIDs(t *testing.T) {
	funder := common.HexToAddress("0x1111111111111111111111111111111111111111")
	var tradeOneRequests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case endpoint.PostOrder:
			_, _ = w.Write([]byte(`{"success":true,"orderID":"order-1","status":"matched","tradeIDs":["trade-1","trade-2"]}`))
		case endpoint.GetTrades:
			switch r.URL.Query().Get("id") {
			case "trade-1":
				switch tradeOneRequests.Add(1) {
				case 1:
					http.Error(w, "temporary failure", http.StatusServiceUnavailable)
					return
				case 2:
					_, _ = w.Write([]byte(`{"data":[{"id":"trade-1","status":"MATCHED","transaction_hash":""}],"next_cursor":"LTE="}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":[{"id":"trade-1","status":"CONFIRMED","transaction_hash":"0xabc"}],"next_cursor":"LTE="}`))
			case "trade-2":
				_, _ = w.Write([]byte(`{"data":[{"id":"trade-2","status":"failed","transaction_hash":"0xmust-not-be-returned"}],"next_cursor":"LTE="}`))
			default:
				t.Errorf("unexpected trade id %q", r.URL.Query().Get("id"))
				http.Error(w, "unexpected trade id", http.StatusBadRequest)
			}
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	headers := &types.L2PolyHeader{POLYAddress: funder.Hex()}
	var response types.OrderResponse
	if err := c.postJSONWithHeaders(endpoint.PostOrder, headers, map[string]any{"order": "signed"}, &response); err != nil {
		t.Fatalf("post order: %v", err)
	}

	if len(response.TradeIDs) != 2 {
		t.Fatalf("TradeIDs = %#v, want both server trade IDs", response.TradeIDs)
	}
	if len(response.TransactionsHashes) != 1 || response.TransactionsHashes[0] != "0xabc" {
		t.Fatalf("TransactionsHashes = %#v, want [0xabc]", response.TransactionsHashes)
	}
	if tradeOneRequests.Load() < 3 {
		t.Fatalf("trade-1 requests = %d, want at least 3 polling attempts", tradeOneRequests.Load())
	}
}

func TestResolveOrderResponseTimeoutIsBestEffort(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"next_cursor":"LTE="}`))
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	response := &types.OrderResponse{Success: true, TradeIDs: []string{"trade-pending"}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	c.resolveOrderResponse(ctx, common.HexToAddress("0x1111111111111111111111111111111111111111"), response)

	if len(response.TransactionsHashes) != 0 {
		t.Fatalf("TransactionsHashes = %#v, want unchanged empty hashes", response.TransactionsHashes)
	}
	if requests.Load() == 0 {
		t.Fatal("expected at least one trade lookup before timeout")
	}
}

func TestResolveOrderResponseReturnsPartialHashesOnTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("id") == "trade-resolved" {
			_, _ = w.Write([]byte(`{"data":[{"id":"trade-resolved","status":"CONFIRMED","transaction_hash":"0xpartial"}],"next_cursor":"LTE="}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"next_cursor":"LTE="}`))
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	response := &types.OrderResponse{Success: true, TradeIDs: []string{"trade-resolved", "trade-pending"}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	c.resolveOrderResponse(ctx, common.HexToAddress("0x1111111111111111111111111111111111111111"), response)

	if len(response.TransactionsHashes) != 1 || response.TransactionsHashes[0] != "0xpartial" {
		t.Fatalf("TransactionsHashes = %#v, want partial result [0xpartial]", response.TransactionsHashes)
	}
}

func TestResolveOrderResponsePollsTradeIDsIndependently(t *testing.T) {
	var healthyRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("id") {
		case "trade-slow":
			<-r.Context().Done()
		case "trade-healthy":
			if healthyRequests.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"data":[{"id":"trade-healthy","status":"MATCHED","transaction_hash":""}],"next_cursor":"LTE="}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"trade-healthy","status":"CONFIRMED","transaction_hash":"0xhealthy"}],"next_cursor":"LTE="}`))
		default:
			http.Error(w, "unexpected trade id", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	response := &types.OrderResponse{Success: true, TradeIDs: []string{"trade-slow", "trade-healthy"}}
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	c.resolveOrderResponse(ctx, common.HexToAddress("0x1111111111111111111111111111111111111111"), response)

	if len(response.TransactionsHashes) != 1 || response.TransactionsHashes[0] != "0xhealthy" {
		t.Fatalf("TransactionsHashes = %#v, want independently resolved [0xhealthy]", response.TransactionsHashes)
	}
	if healthyRequests.Load() < 2 {
		t.Fatalf("healthy trade requests = %d, want at least 2", healthyRequests.Load())
	}
}

func TestGetTradesContextCancelsServerTimeRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != endpoint.Time {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		select {
		case <-r.Context().Done():
		case <-time.After(400 * time.Millisecond):
		}
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	c.useServerTime = true
	c.httpClient.Timeout = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := c.getTradesWithContext(ctx, common.Address{}, &types.TradeParams{}, "")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("server-time lookup ignored context; elapsed = %s", elapsed)
	}
}

func TestResolveOrderResponseSkipsAlreadyResolvedResponse(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "must not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestBuilderTradesClient(t, srv.URL)
	response := &types.OrderResponse{
		Success:            true,
		TradeIDs:           []string{"trade-1"},
		TransactionsHashes: []string{"0xexisting"},
	}
	c.resolveOrderResponse(context.Background(), common.Address{}, response)

	if requests.Load() != 0 {
		t.Fatalf("trade lookup requests = %d, want 0", requests.Load())
	}
}
