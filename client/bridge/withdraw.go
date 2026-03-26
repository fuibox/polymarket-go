package bridge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/bytedance/sonic"
)

// 错误定义
var (
	// ErrInvalidRequest 请求参数错误 (HTTP 400)
	ErrInvalidRequest = errors.New("invalid request parameters")

	// ErrUnsupportedAsset 不支持的链或代币 (HTTP 404)
	ErrUnsupportedAsset = errors.New("unsupported chain or token")

	// ErrInvalidAddress 收款地址格式无效 (HTTP 422)
	ErrInvalidAddress = errors.New("invalid recipient address format")

	// ErrRateLimited 请求过于频繁 (HTTP 429)
	ErrRateLimited = errors.New("rate limited, please retry later")

	// ErrBridgeService Bridge 服务内部错误 (HTTP 500/502/503)
	ErrBridgeService = errors.New("bridge service internal error")

	// ErrTimeout 请求超时
	ErrTimeout = errors.New("request timeout")
)

// CreateWithdrawAddressRequest POST /withdraw 请求参数
type CreateWithdrawAddressRequest struct {
	// Address 用户 Polymarket Safe 地址（Polygon 上的 0x 地址）
	Address string `json:"address"`

	// ToChainID 目标链 ID
	// 示例: "1"=Ethereum, "8453"=Base, "1151111081099710"=Solana, "8253038"=Bitcoin
	ToChainID string `json:"toChainId"`

	// ToTokenAddress 目标代币合约地址
	// EVM链: 0x 地址; Solana: Base58 地址; Bitcoin: 留空或特殊标识
	ToTokenAddress string `json:"toTokenAddress"`

	// RecipientAddr 最终收款地址（格式须匹配目标链类型）
	RecipientAddr string `json:"recipientAddr"`
}

// CreateWithdrawAddressResponse POST /withdraw 响应
type CreateWithdrawAddressResponse struct {
	// Address 根据链类型返回对应的提现地址
	Address struct {
		EVM string `json:"evm"` // EVM 链提现地址
		SVM string `json:"svm"` // Solana 提现地址
		BTC string `json:"btc"` // Bitcoin 提现地址
	} `json:"address"`

	// Note 附加说明（如有）
	Note string `json:"note,omitempty"`
}

// CreateWithdrawAddress 创建跨链提现地址
//
// 调用方（后端服务）需将 USDC.e 转入返回的地址以触发跨链提现。
// 该地址是一次性的，每次调用返回新地址。
//
// 参数:
//   - ctx: 上下文，用于超时控制（建议 20s 超时）
//   - req: 创建提现地址请求参数
//
// 返回:
//   - *CreateWithdrawAddressResponse: 包含提现地址的响应
//   - error: 调用失败时返回错误信息
//
// 错误码:
//   - ErrInvalidRequest: 请求参数错误 (HTTP 400)
//   - ErrUnsupportedAsset: 不支持的链或代币 (HTTP 404)
//   - ErrInvalidAddress: 收款地址格式无效 (HTTP 422)
//   - ErrRateLimited: 请求过于频繁 (HTTP 429)
//   - ErrBridgeService: Bridge 服务内部错误 (HTTP 500/502/503)
//   - ErrTimeout: 请求超时
//
// 示例:
//
//	resp, err := client.CreateWithdrawAddress(ctx, bridge.CreateWithdrawAddressRequest{
//	    Address:        "0xUserSafeAddress...",
//	    ToChainID:      "1",
//	    ToTokenAddress: "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
//	    RecipientAddr:  "0xRecipientAddress...",
//	})
//	if err != nil {
//	    // 处理错误
//	}
//
//	// 根据目标链类型选择对应地址
//	var withdrawAddr string
//	switch req.ToChainID {
//	case "1", "8453", "137": // EVM 链
//	    withdrawAddr = resp.Address.EVM
//	case "1151111081099710": // Solana
//	    withdrawAddr = resp.Address.SVM
//	case "8253038": // Bitcoin
//	    withdrawAddr = resp.Address.BTC
//	}
func (c *BridgeClient) CreateWithdrawAddress(
	ctx context.Context,
	req CreateWithdrawAddressRequest,
) (*CreateWithdrawAddressResponse, error) {
	// 参数校验
	if req.Address == "" {
		return nil, fmt.Errorf("%w: address is required", ErrInvalidRequest)
	}
	if req.ToChainID == "" {
		return nil, fmt.Errorf("%w: toChainId is required", ErrInvalidRequest)
	}
	if req.RecipientAddr == "" {
		return nil, fmt.Errorf("%w: recipientAddr is required", ErrInvalidRequest)
	}
	// Bitcoin 链的 toTokenAddress 可以为空，其他链需要验证
	if req.ToTokenAddress == "" && req.ToChainID != "8253038" {
		return nil, fmt.Errorf("%w: toTokenAddress is required", ErrInvalidRequest)
	}

	// 序列化请求体
	body, err := sonic.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建请求
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/withdraw", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		if ctx.Err() == context.Canceled {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 处理 HTTP 状态码
	if resp.StatusCode != http.StatusCreated {
		return nil, c.mapError(resp.StatusCode, respBody)
	}

	// 解析响应
	var result CreateWithdrawAddressResponse
	if err := sonic.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(respBody))
	}

	return &result, nil
}

// mapError 将 HTTP 状态码映射为对应的错误
func (c *BridgeClient) mapError(statusCode int, body []byte) error {
	// 尝试解析错误响应
	var errResp ErrResp
	if e := sonic.Unmarshal(body, &errResp); e == nil {
		if errResp.Error != "" {
			return c.createErrorFromStatus(statusCode, errResp.Error)
		}
		if errResp.Message != "" {
			return c.createErrorFromStatus(statusCode, errResp.Message)
		}
	}

	// 使用默认错误消息
	return c.createErrorFromStatus(statusCode, string(body))
}

// createErrorFromStatus 根据状态码创建对应的错误
func (c *BridgeClient) createErrorFromStatus(statusCode int, message string) error {
	switch statusCode {
	case http.StatusBadRequest:
		return fmt.Errorf("%w: %s", ErrInvalidRequest, message)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrUnsupportedAsset, message)
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %s", ErrInvalidAddress, message)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrRateLimited, message)
	default:
		if statusCode >= 500 {
			return fmt.Errorf("%w: HTTP %d - %s", ErrBridgeService, statusCode, message)
		}
		return fmt.Errorf("unexpected HTTP %d: %s", statusCode, message)
	}
}
