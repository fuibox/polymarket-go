package bridge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"
)

func TestCreateWithdrawAddress_Success_EVM(t *testing.T) {
	// Mock 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/withdraw" {
			t.Errorf("expected /withdraw, got %s", r.URL.Path)
		}

		// 验证 Content-Type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", contentType)
		}

		// 返回成功响应
		resp := CreateWithdrawAddressResponse{}
		resp.Address.EVM = "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb"
		resp.Address.SVM = ""
		resp.Address.BTC = ""
		resp.Note = "Test withdraw address"

		w.WriteHeader(http.StatusCreated)
		respBody, _ := sonic.Marshal(resp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, err := NewBridgeClient(&ClientConfig{
		Host:    server.URL,
		Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress123456789012345678901234567890",
		ToChainID:      "1",
		ToTokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		RecipientAddr:  "0xRecipientAddress1234567890123456789012345678",
	}

	resp, err := client.CreateWithdrawAddress(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证响应
	if resp.Address.EVM == "" {
		t.Error("expected non-empty EVM address")
	}
	if !common.IsHexAddress(resp.Address.EVM) {
		t.Errorf("expected valid EVM address, got %s", resp.Address.EVM)
	}
	if resp.Address.SVM != "" {
		t.Errorf("expected empty SVM address for EVM chain, got %s", resp.Address.SVM)
	}
	if resp.Address.BTC != "" {
		t.Errorf("expected empty BTC address for EVM chain, got %s", resp.Address.BTC)
	}
	if resp.Note != "Test withdraw address" {
		t.Errorf("expected note 'Test withdraw address', got %s", resp.Note)
	}
}

func TestCreateWithdrawAddress_Success_Solana(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CreateWithdrawAddressResponse{}
		resp.Address.EVM = ""
		resp.Address.SVM = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"
		resp.Address.BTC = ""

		w.WriteHeader(http.StatusCreated)
		respBody, _ := sonic.Marshal(resp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress123456789012345678901234567890",
		ToChainID:      "1151111081099710", // Solana
		ToTokenAddress: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		RecipientAddr:  "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU",
	}

	resp, err := client.CreateWithdrawAddress(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Address.EVM != "" {
		t.Errorf("expected empty EVM address for Solana chain, got %s", resp.Address.EVM)
	}
	if resp.Address.SVM == "" {
		t.Error("expected non-empty SVM address")
	}
	if resp.Address.BTC != "" {
		t.Errorf("expected empty BTC address for Solana chain, got %s", resp.Address.BTC)
	}
}

func TestCreateWithdrawAddress_Success_Bitcoin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := CreateWithdrawAddressResponse{}
		resp.Address.EVM = ""
		resp.Address.SVM = ""
		resp.Address.BTC = "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh"

		w.WriteHeader(http.StatusCreated)
		respBody, _ := sonic.Marshal(resp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress123456789012345678901234567890",
		ToChainID:      "8253038", // Bitcoin
		ToTokenAddress: "",
		RecipientAddr:  "bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh",
	}

	resp, err := client.CreateWithdrawAddress(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Address.EVM != "" {
		t.Errorf("expected empty EVM address for Bitcoin chain, got %s", resp.Address.EVM)
	}
	if resp.Address.SVM != "" {
		t.Errorf("expected empty SVM address for Bitcoin chain, got %s", resp.Address.SVM)
	}
	if resp.Address.BTC == "" {
		t.Error("expected non-empty BTC address")
	}
}

func TestCreateWithdrawAddress_Error_BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errResp := ErrResp{
			Error:   "Bad Request",
			Message: "missing required field: address",
		}
		w.WriteHeader(http.StatusBadRequest)
		respBody, _ := sonic.Marshal(errResp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}

	if !strings.Contains(err.Error(), "Bad Request") {
		t.Errorf("expected error message to contain 'Bad Request', got %s", err.Error())
	}
}

func TestCreateWithdrawAddress_Error_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errResp := ErrResp{
			Error: "Not Found",
		}
		w.WriteHeader(http.StatusNotFound)
		respBody, _ := sonic.Marshal(errResp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "999999", // 不支持的链
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrUnsupportedAsset) {
		t.Errorf("expected ErrUnsupportedAsset, got %v", err)
	}
}

func TestCreateWithdrawAddress_Error_UnprocessableEntity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errResp := ErrResp{
			Message: "Invalid recipient address format",
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		respBody, _ := sonic.Marshal(errResp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "invalid-address",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("expected ErrInvalidAddress, got %v", err)
	}
}

func TestCreateWithdrawAddress_Error_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errResp := ErrResp{
			Error: "Too Many Requests",
		}
		w.WriteHeader(http.StatusTooManyRequests)
		respBody, _ := sonic.Marshal(errResp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestCreateWithdrawAddress_Error_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errResp := ErrResp{
			Error: "Internal Server Error",
		}
		w.WriteHeader(http.StatusInternalServerError)
		respBody, _ := sonic.Marshal(errResp)
		w.Write(respBody)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrBridgeService) {
		t.Errorf("expected ErrBridgeService, got %v", err)
	}
}

func TestCreateWithdrawAddress_Error_ServerError_502(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Bad Gateway"))
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrBridgeService) {
		t.Errorf("expected ErrBridgeService, got %v", err)
	}
}

func TestCreateWithdrawAddress_Error_ServerError_503(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service Unavailable"))
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrBridgeService) {
		t.Errorf("expected ErrBridgeService, got %v", err)
	}
}

func TestCreateWithdrawAddress_Error_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟慢响应
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	// 创建带有超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}

func TestCreateWithdrawAddress_Error_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟慢响应
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client, _ := NewBridgeClient(&ClientConfig{Host: server.URL, Timeout: 20 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消上下文
	cancel()

	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCreateWithdrawAddress_Validation_MissingAddress(t *testing.T) {
	client, _ := NewBridgeClient(&ClientConfig{Host: "http://localhost", Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "", // 空地址
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}

	if !strings.Contains(err.Error(), "address is required") {
		t.Errorf("expected error message to contain 'address is required', got %s", err.Error())
	}
}

func TestCreateWithdrawAddress_Validation_MissingToChainID(t *testing.T) {
	client, _ := NewBridgeClient(&ClientConfig{Host: "http://localhost", Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "", // 空链 ID
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestCreateWithdrawAddress_Validation_MissingToTokenAddress(t *testing.T) {
	client, _ := NewBridgeClient(&ClientConfig{Host: "http://localhost", Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "", // 空代币地址
		RecipientAddr:  "0xRecipientAddr",
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestCreateWithdrawAddress_Validation_MissingRecipientAddr(t *testing.T) {
	client, _ := NewBridgeClient(&ClientConfig{Host: "http://localhost", Timeout: 20 * time.Second})

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xUserSafeAddress",
		ToChainID:      "1",
		ToTokenAddress: "0xTokenAddress",
		RecipientAddr:  "", // 空收款地址
	}

	_, err := client.CreateWithdrawAddress(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
}

// TestCreateWithdrawAddress_Integration 集成测试示例（跳过实际执行）
func TestCreateWithdrawAddress_Integration(t *testing.T) {
	t.Skip("Skipping integration test - requires real Bridge API access")

	client, err := NewBridgeClient(&ClientConfig{
		Host:    "https://bridge.polymarket.com",
		Timeout: 20 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	req := CreateWithdrawAddressRequest{
		Address:        "0xYourSafeAddress",
		ToChainID:      "1", // Ethereum
		ToTokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC
		RecipientAddr:  "0xYourRecipientAddress",
	}

	resp, err := client.CreateWithdrawAddress(ctx, req)
	if err != nil {
		t.Fatalf("create withdraw address failed: %v", err)
	}

	t.Logf("EVM Address: %s", resp.Address.EVM)
	t.Logf("SVM Address: %s", resp.Address.SVM)
	t.Logf("BTC Address: %s", resp.Address.BTC)
	t.Logf("Note: %s", resp.Note)

	if resp.Address.EVM == "" {
		t.Error("expected non-empty EVM address")
	}
	if !common.IsHexAddress(resp.Address.EVM) {
		t.Errorf("expected valid EVM address, got %s", resp.Address.EVM)
	}
}
