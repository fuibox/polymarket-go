package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"
)

const (
	testBuilderCodeHex  = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	testBuilderCodeNorm = "0x" + testBuilderCodeHex
)

func newTestClient(t *testing.T, host, builderCode string) *BridgeClient {
	t.Helper()
	client, err := NewBridgeClient(&ClientConfig{
		Host:        host,
		Timeout:     5 * time.Second,
		BuilderCode: builderCode,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

func depositHandler(t *testing.T, wantBuilderCode string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deposit" {
			t.Errorf("expected /deposit, got %s", r.URL.Path)
		}
		if got := r.Header.Get(headerBuilderCode); got != wantBuilderCode {
			t.Errorf("expected %s header %q, got %q", headerBuilderCode, wantBuilderCode, got)
		}
		resp := CreateDepositAddressResponse{}
		resp.Address.EVM = "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb"
		resp.Address.Tron = "TXYZa1b2c3d4e5f6g7h8i9j0kLmNoPqRsT"
		w.WriteHeader(http.StatusCreated)
		respBody, _ := sonic.Marshal(resp)
		w.Write(respBody)
	}
}

func withdrawHandler(t *testing.T, wantBuilderCode string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/withdraw" {
			t.Errorf("expected /withdraw, got %s", r.URL.Path)
		}
		if got := r.Header.Get(headerBuilderCode); got != wantBuilderCode {
			t.Errorf("expected %s header %q, got %q", headerBuilderCode, wantBuilderCode, got)
		}
		resp := CreateWithdrawAddressResponse{}
		resp.Address.EVM = "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb"
		resp.Address.Tron = "TXYZa1b2c3d4e5f6g7h8i9j0kLmNoPqRsT"
		w.WriteHeader(http.StatusCreated)
		respBody, _ := sonic.Marshal(resp)
		w.Write(respBody)
	}
}

// 配置了 BuilderCode 时,POST /deposit 必须带 X-Builder-Code,
// 且各种输入形式(带/不带 0x 前缀、大小写)都归一化为 0x + 小写 64 hex。
func TestBuilderCode_DepositSendsHeader(t *testing.T) {
	inputs := []string{
		testBuilderCodeNorm,                        // 标准形式
		testBuilderCodeHex,                         // 无前缀
		"0X" + strings.ToUpper(testBuilderCodeHex), // 0X + 大写
		"0x" + strings.ToUpper(testBuilderCodeHex), // 0x + 大写
	}
	for _, in := range inputs {
		server := httptest.NewServer(depositHandler(t, testBuilderCodeNorm))
		client := newTestClient(t, server.URL, in)
		if _, err := client.CreateDepositAddress(common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb")); err != nil {
			t.Errorf("input %q: unexpected error: %v", in, err)
		}
		server.Close()
	}
}

// 配置了 BuilderCode 时,POST /withdraw 必须带 X-Builder-Code。
func TestBuilderCode_WithdrawSendsHeader(t *testing.T) {
	server := httptest.NewServer(withdrawHandler(t, testBuilderCodeNorm))
	defer server.Close()

	client := newTestClient(t, server.URL, testBuilderCodeNorm)
	_, err := client.CreateWithdrawAddress(context.Background(), CreateWithdrawAddressRequest{
		Address:        "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb",
		ToChainID:      "1",
		ToTokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		RecipientAddr:  "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// 未配置 BuilderCode 时安全降级:不发送 header,请求照常成功。
func TestBuilderCode_OmittedDegradesSafely(t *testing.T) {
	depositServer := httptest.NewServer(depositHandler(t, ""))
	defer depositServer.Close()

	client := newTestClient(t, depositServer.URL, "")
	if _, err := client.CreateDepositAddress(common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb")); err != nil {
		t.Errorf("deposit without builder code: unexpected error: %v", err)
	}

	withdrawServer := httptest.NewServer(withdrawHandler(t, ""))
	defer withdrawServer.Close()

	client = newTestClient(t, withdrawServer.URL, "")
	_, err := client.CreateWithdrawAddress(context.Background(), CreateWithdrawAddressRequest{
		Address:        "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb",
		ToChainID:      "1",
		ToTokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		RecipientAddr:  "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb",
	})
	if err != nil {
		t.Errorf("withdraw without builder code: unexpected error: %v", err)
	}
}

// 格式非法的 BuilderCode 在构造 client 时 fail-fast(避免每次请求吃服务端 400)。
func TestBuilderCode_MalformedFailsFast(t *testing.T) {
	invalid := []string{
		"0x1234",                       // 太短
		testBuilderCodeHex + "00",      // 太长
		"0x" + strings.Repeat("g", 64), // 非 hex 字符
		"0x " + testBuilderCodeHex[1:], // 含空格
	}
	for _, code := range invalid {
		if _, err := NewBridgeClient(&ClientConfig{BuilderCode: code}); err == nil {
			t.Errorf("expected error for builder code %q, got nil", code)
		}
	}

	valid := []string{"", testBuilderCodeNorm, testBuilderCodeHex, "0X" + strings.ToUpper(testBuilderCodeHex)}
	for _, code := range valid {
		if _, err := NewBridgeClient(&ClientConfig{BuilderCode: code}); err != nil {
			t.Errorf("expected success for builder code %q, got %v", code, err)
		}
	}
}

// 官方规范只允许 /deposit、/withdraw 收此 header:
// 即使配置了 BuilderCode,POST /quote 也不能带。
func TestBuilderCode_QuoteNeverSendsHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(headerBuilderCode); got != "" {
			t.Errorf("expected no %s header on /quote, got %q", headerBuilderCode, got)
		}
		w.WriteHeader(http.StatusOK)
		respBody, _ := sonic.Marshal(QuoteResponse{QuoteID: "q-1"})
		w.Write(respBody)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, testBuilderCodeNorm)
	_, err := client.GetAQuote(QuoteRequest{
		FromAmountBaseUnit: "1000000",
		FromChainID:        "1",
		FromTokenAddress:   "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		RecipientAddress:   "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb",
		ToChainID:          "137",
		ToTokenAddress:     "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// 官方响应 address 对象含 tron 字段,deposit / withdraw 两个响应结构都要解析出来。
func TestBuilderCode_TronAddressParsed(t *testing.T) {
	const wantTron = "TXYZa1b2c3d4e5f6g7h8i9j0kLmNoPqRsT"

	depositServer := httptest.NewServer(depositHandler(t, ""))
	defer depositServer.Close()

	client := newTestClient(t, depositServer.URL, "")
	depResp, err := client.CreateDepositAddress(common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if depResp.Address.Tron != wantTron {
		t.Errorf("deposit: expected tron address %q, got %q", wantTron, depResp.Address.Tron)
	}

	withdrawServer := httptest.NewServer(withdrawHandler(t, ""))
	defer withdrawServer.Close()

	client = newTestClient(t, withdrawServer.URL, "")
	wdResp, err := client.CreateWithdrawAddress(context.Background(), CreateWithdrawAddressRequest{
		Address:        "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb",
		ToChainID:      "1",
		ToTokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
		RecipientAddr:  "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEBb",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wdResp.Address.Tron != wantTron {
		t.Errorf("withdraw: expected tron address %q, got %q", wantTron, wdResp.Address.Tron)
	}
}
