package clob

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/bytedance/sonic"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fuibox/polymarket-go/client/clob/clob_types"
	clob_endpoint "github.com/fuibox/polymarket-go/client/endpoint"
	sdkerrors "github.com/fuibox/polymarket-go/client/errors"
	"github.com/fuibox/polymarket-go/client/types"
)

// TypedClobClient 是 ClobClient 的 Wrapper，所有方法返回 *sdkerrors.SDKError 而非 error。
// 上层可通过 sdkErr.Code / sdkErr.IsRetryable() / sdkErr.IsNotFound() 精确处理异常。
//
// 迁移方式：
//
//	// 原来
//	client := clob.NewClobClient(...)
//	order, err := client.GetOrder(addr, id)
//
//	// 迁移后（只需包一层，其他代码不动）
//	typed := clob.NewTypedClobClient(client)
//	order, sdkErr := typed.GetOrder(addr, id)
//	if sdkErr != nil {
//	    switch {
//	    case sdkErr.IsNotFound():   // 订单不存在
//	    case sdkErr.IsAuthError():  // 认证失效，重新 derive API Key
//	    case sdkErr.IsRetryable():  // 退避重试
//	    default:                    // 其他错误
//	    }
//	}
type TypedClobClient struct {
	inner *ClobClient
}

// NewTypedClobClient 用已有的 ClobClient 构造 TypedClobClient，不创建新连接。
func NewTypedClobClient(c *ClobClient) *TypedClobClient {
	return &TypedClobClient{inner: c}
}

// Inner 返回底层 ClobClient，需要调用尚未 typed 化的方法时使用。
func (t *TypedClobClient) Inner() *ClobClient {
	return t.inner
}

// ── typed HTTP helpers ────────────────────────────────────────────────────────

func (t *TypedClobClient) getJSON(ep string, headers interface{}, params url.Values, result interface{}) *sdkerrors.SDKError {
	fullURL := t.inner.host + ep
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to create request", err)
	}

	t.inner.addHeadersToRequest(req, headers)

	if t.inner.geoBlockToken != "" {
		q := req.URL.Query()
		q.Add("geo_block_token", t.inner.geoBlockToken)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := t.inner.httpClient.Do(req)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to make request", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to read response body", err)
	}

	if resp.StatusCode >= 400 {
		return sdkerrors.FromHTTP(resp.StatusCode, string(body))
	}

	if err := sonic.Unmarshal(body, result); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeUnmarshal, "failed to unmarshal response", err)
	}
	return nil
}

func (t *TypedClobClient) postJSON(ep string, headers interface{}, data interface{}, result interface{}) *sdkerrors.SDKError {
	var bodyReader io.Reader
	if data != nil {
		switch v := data.(type) {
		case string:
			bodyReader = bytes.NewReader([]byte(v))
		case []byte:
			bodyReader = bytes.NewReader(v)
		case json.RawMessage:
			bodyReader = bytes.NewReader(v)
		default:
			jsonData, err := sonic.Marshal(v)
			if err != nil {
				return sdkerrors.Wrap(sdkerrors.ErrCodeUnmarshal, "failed to marshal request data", err)
			}
			bodyReader = bytes.NewReader(jsonData)
		}
	}

	req, err := http.NewRequest("POST", t.inner.host+ep, bodyReader)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to create request", err)
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	t.inner.addHeadersToRequest(req, headers)

	if t.inner.geoBlockToken != "" {
		q := req.URL.Query()
		q.Add("geo_block_token", t.inner.geoBlockToken)
		req.URL.RawQuery = q.Encode()
	}

	resp, err := t.inner.httpClient.Do(req)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to make request", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to read response body", err)
	}

	if resp.StatusCode >= 400 {
		sdkErr := sdkerrors.FromHTTP(resp.StatusCode, string(body))
		// 尝试提取 API 返回的 error 字段作为 Message
		var apiErr struct {
			Error string `json:"error"`
		}
		if sonic.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
			sdkErr.Message = apiErr.Error
		}
		return sdkErr
	}

	if result != nil {
		if err := sonic.Unmarshal(body, result); err != nil {
			return sdkerrors.Wrap(sdkerrors.ErrCodeUnmarshal, fmt.Sprintf("failed to unmarshal response: %s", string(body)), err)
		}
	}
	return nil
}

func (t *TypedClobClient) deleteJSON(ep string, headers interface{}, data interface{}, result interface{}) *sdkerrors.SDKError {
	var bodyReader io.Reader
	if data != nil {
		switch v := data.(type) {
		case string:
			bodyReader = bytes.NewReader([]byte(v))
		case []byte:
			bodyReader = bytes.NewReader(v)
		case json.RawMessage:
			bodyReader = bytes.NewReader(v)
		default:
			j, err := sonic.Marshal(v)
			if err != nil {
				return sdkerrors.Wrap(sdkerrors.ErrCodeUnmarshal, "failed to marshal delete body", err)
			}
			bodyReader = bytes.NewReader(j)
		}
	}

	req, err := http.NewRequest("DELETE", t.inner.host+ep, bodyReader)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to create request", err)
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	t.inner.addHeadersToRequest(req, headers)

	resp, err := t.inner.httpClient.Do(req)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to make request", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, "failed to read response body", err)
	}

	if resp.StatusCode >= 400 {
		return sdkerrors.FromHTTP(resp.StatusCode, string(body))
	}

	if result != nil {
		if err := sonic.Unmarshal(body, result); err != nil {
			return sdkerrors.Wrap(sdkerrors.ErrCodeUnmarshal, fmt.Sprintf("failed to unmarshal response: %s", string(body)), err)
		}
	}
	return nil
}

// ── typed 公开方法 ─────────────────────────────────────────────────────────────

// GetOrder 获取单个订单，订单不存在时 sdkErr.IsNotFound() == true。
func (t *TypedClobClient) GetOrder(funder common.Address, orderID string) (*types.OpenOrder, *sdkerrors.SDKError) {
	if t.inner.creds == nil {
		return nil, sdkerrors.New(sdkerrors.ErrCodeUnauthorized, "API credentials are required")
	}

	ep := clob_endpoint.GetOrder + orderID
	headerArgs := &types.L2HeaderArgs{Method: "GET", RequestPath: ep}
	headers, err := t.inner.createL2Headers(funder, headerArgs)
	if err != nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrCodeUnauthorized, "failed to create L2 headers", err)
	}

	var result types.OpenOrder
	if sdkErr := t.getJSON(ep, headers, url.Values{}, &result); sdkErr != nil {
		return nil, sdkErr
	}
	if result.ID == "" {
		return nil, sdkerrors.New(sdkerrors.ErrCodeNotFoundBody, fmt.Sprintf("order not found: %s", orderID))
	}
	return &result, nil
}

// CreateAndPostOrder 创建并提交限价单。
func (t *TypedClobClient) CreateAndPostOrder(args clob_types.OrderArgs, option clob_types.PartialCreateOrderOptions) (*types.OrderResponse, *sdkerrors.SDKError) {
	order, err := t.inner.CreateAndPostOrder(args, option)
	if err != nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrCodeUnknown, err.Error(), err)
	}
	return order, nil
}

// CreateAndPostMarketOrder 创建并提交市价单。
func (t *TypedClobClient) CreateAndPostMarketOrder(args clob_types.MarketOrderArgs, option clob_types.PartialCreateOrderOptions) (*types.OrderResponse, *sdkerrors.SDKError) {
	order, err := t.inner.CreateAndPostMarketOrder(args, option)
	if err != nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrCodeUnknown, err.Error(), err)
	}
	return order, nil
}

// CancelOrder 撤销单笔订单。参数顺序与 ClobClient.CancelOrder 一致。
func (t *TypedClobClient) CancelOrder(orderID string, signerAddr common.Address) (*types.OrderResponse, *sdkerrors.SDKError) {
	result, err := t.inner.CancelOrder(orderID, signerAddr)
	if err != nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrCodeUnknown, err.Error(), err)
	}
	return result, nil
}

// CancelAllOrders 撤销当前用户全部挂单。
func (t *TypedClobClient) CancelAllOrders(signerAddr common.Address) (*types.OrderResponse, *sdkerrors.SDKError) {
	result, err := t.inner.CancelAllOrders(signerAddr)
	if err != nil {
		return nil, sdkerrors.Wrap(sdkerrors.ErrCodeUnknown, err.Error(), err)
	}
	return result, nil
}
