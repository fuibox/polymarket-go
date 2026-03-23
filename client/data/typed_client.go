package data

import (
	"encoding/json"
	"fmt"

	sdkerrors "github.com/fuibox/polymarket-go/client/errors"
)

// TypedDataSDK 是 DataSDK 的 Wrapper，所有方法返回 *sdkerrors.SDKError 而非 error。
//
// 迁移方式：
//
//	// 原来
//	sdk, _ := data.NewDataSDK(nil)
//	positions, err := sdk.GetCurrentPositions(query)
//
//	// 迁移后
//	typed := data.NewTypedDataSDK(sdk)
//	positions, sdkErr := typed.GetCurrentPositions(query)
type TypedDataSDK struct {
	inner *DataSDK
}

// NewTypedDataSDK 用已有的 DataSDK 构造 TypedDataSDK。
func NewTypedDataSDK(sdk *DataSDK) *TypedDataSDK {
	return &TypedDataSDK{inner: sdk}
}

// Inner 返回底层 DataSDK，需要调用尚未 typed 化的方法时使用。
func (t *TypedDataSDK) Inner() *DataSDK {
	return t.inner
}

// ── typed 内部 helper ─────────────────────────────────────────────────────────

func (t *TypedDataSDK) request(method, endpoint string, query interface{}, result interface{}) *sdkerrors.SDKError {
	resp, err := t.inner.makeRequest(method, endpoint, query)
	if err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrCodeNetwork, fmt.Sprintf("request %s %s failed", method, endpoint), err)
	}

	if !resp.OK {
		raw := ""
		if resp.ErrorData != nil {
			if b, err := json.Marshal(resp.ErrorData); err == nil {
				raw = string(b)
			}
		}
		sdkErr := sdkerrors.FromHTTP(resp.Status, raw)
		// 尝试提取 error 字段作为 Message
		if errMap, ok := resp.ErrorData.(map[string]interface{}); ok {
			if msg, ok := errMap["error"].(string); ok && msg != "" {
				sdkErr.Message = msg
			}
		}
		return sdkErr
	}

	if resp.Data == nil {
		return sdkerrors.New(sdkerrors.ErrCodeNotFoundBody, fmt.Sprintf("%s returned empty response body", endpoint))
	}

	if result != nil {
		if err := json.Unmarshal(resp.Data, result); err != nil {
			return sdkerrors.Wrap(sdkerrors.ErrCodeUnmarshal, fmt.Sprintf("failed to unmarshal %s response", endpoint), err)
		}
	}
	return nil
}

// ── typed 公开方法 ─────────────────────────────────────────────────────────────

// GetCurrentPositions 获取用户当前持仓列表。
// query.User 为必填，其余为可选过滤条件。
func (t *TypedDataSDK) GetCurrentPositions(query *PositionsQuery) ([]Position, *sdkerrors.SDKError) {
	if query == nil {
		query = &PositionsQuery{}
	}
	var result []Position
	if sdkErr := t.request("GET", "/positions", query, &result); sdkErr != nil {
		return nil, sdkErr
	}
	return result, nil
}

// GetClosedPositions 获取用户已平仓持仓列表。
func (t *TypedDataSDK) GetClosedPositions(query *ClosedPositionsQuery) ([]ClosedPosition, *sdkerrors.SDKError) {
	if query == nil {
		query = &ClosedPositionsQuery{}
	}
	var result []ClosedPosition
	if sdkErr := t.request("GET", "/closed-positions", query, &result); sdkErr != nil {
		return nil, sdkErr
	}
	return result, nil
}

// GetTrades 获取用户成交记录。
func (t *TypedDataSDK) GetTrades(query *TradesQuery) ([]DataTrade, *sdkerrors.SDKError) {
	if query == nil {
		query = &TradesQuery{}
	}
	var result []DataTrade
	if sdkErr := t.request("GET", "/trades", query, &result); sdkErr != nil {
		return nil, sdkErr
	}
	return result, nil
}

// GetUserActivity 获取用户操作记录（交易、赎回等）。
func (t *TypedDataSDK) GetUserActivity(query *UserActivityQuery) ([]Activity, *sdkerrors.SDKError) {
	if query == nil {
		query = &UserActivityQuery{}
	}
	var result []Activity
	if sdkErr := t.request("GET", "/activity", query, &result); sdkErr != nil {
		return nil, sdkErr
	}
	return result, nil
}
