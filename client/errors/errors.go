package sdkerrors

import "fmt"

// ErrCode 是 SDK 统一错误码，上层通过 Code 判断错误类型并决定处理策略。
type ErrCode int

const (
	// HTTP 层错误（与 HTTP 状态码对应）
	ErrCodeBadRequest   ErrCode = 400 // 参数错误，不可重试
	ErrCodeUnauthorized ErrCode = 401 // 认证失效，需重新获取 API Key
	ErrCodeForbidden    ErrCode = 403 // 权限不足，不可重试
	ErrCodeNotFound     ErrCode = 404 // 资源不存在
	ErrCodeRateLimit    ErrCode = 429 // 限流，退避后重试
	ErrCodeServerError  ErrCode = 500 // 服务端错误，可重试

	// SDK 扩展错误码（1xxx = 网络/传输层，2xxx = 业务层）
	ErrCodeNetwork      ErrCode = 1001 // 网络连接失败，可重试
	ErrCodeUnmarshal    ErrCode = 1002 // 响应解析失败
	ErrCodeNotFoundBody ErrCode = 1003 // HTTP 200 但响应体为空（Polymarket 已知行为）

	ErrCodeUnknown ErrCode = 0
)

// SDKError 是 SDK 统一错误类型，实现 error 接口，可直接替换原有 error 使用。
// 上层通过 errors.As 提取，或直接使用新 Typed 方法获取 *SDKError。
type SDKError struct {
	// Code 错误码，上层 switch 的主要依据。
	Code ErrCode

	// Message 人类可读的错误描述。
	Message string

	// Raw 原始响应体，调试用，可能为空。
	Raw string

	// Cause 原始底层错误，供 errors.Is / errors.As 链式查找。
	Cause error
}

func (e *SDKError) Error() string {
	if e.Raw != "" {
		return fmt.Sprintf("[%d] %s (raw: %s)", e.Code, e.Message, e.Raw)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *SDKError) Unwrap() error { return e.Cause }

// IsRetryable 判断该错误是否值得退避重试。
// 限流（429）、服务端错误（5xx）、网络错误均可重试。
func (e *SDKError) IsRetryable() bool {
	switch e.Code {
	case ErrCodeRateLimit, ErrCodeServerError, ErrCodeNetwork:
		return true
	}
	return false
}

// IsNotFound 判断资源是否不存在（含 Polymarket 200 空响应的已知行为）。
func (e *SDKError) IsNotFound() bool {
	return e.Code == ErrCodeNotFound || e.Code == ErrCodeNotFoundBody
}

// IsAuthError 判断是否为认证/权限错误，需要刷新凭证。
func (e *SDKError) IsAuthError() bool {
	return e.Code == ErrCodeUnauthorized || e.Code == ErrCodeForbidden
}

// FromHTTPStatus 将 HTTP 状态码映射为 ErrCode。
func FromHTTPStatus(status int) ErrCode {
	switch status {
	case 400:
		return ErrCodeBadRequest
	case 401:
		return ErrCodeUnauthorized
	case 403:
		return ErrCodeForbidden
	case 404:
		return ErrCodeNotFound
	case 429:
		return ErrCodeRateLimit
	default:
		if status >= 500 {
			return ErrCodeServerError
		}
		return ErrCodeUnknown
	}
}

// New 构造一个 SDKError。
func New(code ErrCode, message string) *SDKError {
	return &SDKError{Code: code, Message: message}
}

// Wrap 构造一个携带底层 cause 的 SDKError。
func Wrap(code ErrCode, message string, cause error) *SDKError {
	return &SDKError{Code: code, Message: message, Cause: cause}
}

// FromHTTP 从 HTTP 状态码和响应体构造 SDKError。
func FromHTTP(status int, body string) *SDKError {
	return &SDKError{
		Code:    FromHTTPStatus(status),
		Message: fmt.Sprintf("HTTP %d", status),
		Raw:     body,
	}
}
