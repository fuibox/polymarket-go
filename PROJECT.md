# PROJECT.md — 项目功能全貌

## 项目定位

**polymarket-go** 是一个针对 [Polymarket](https://polymarket.com) 的 Go SDK，供外部程序调用，无 `main()` 入口。模块名：`github.com/fuibox/polymarket-go`。

支持两条链：
- Polygon 主网（ChainID 137）
- Amoy 测试网（ChainID 80002）

---

## 已实现的客户端模块

### 1. ClobClient（`client/clob/`）

CLOB（中心化限价订单簿）交易核心客户端。

**订单操作：**
- `CreateAndPostOrder()` — 创建并提交限价单
- `CreateAndPostMarketOrder()` — 创建并提交市价单
- `CancelOrder()` — 撤销单笔订单
- `CancelAllOrders()` — 撤销所有订单
- `CancelOrders()` — 批量撤销

**查询：**
- `GetOrderBook()` / `GetOrderBooks()` — 订单簿（单/批）
- `GetTickSize()` — 最小价格步长
- `GetNegRisk()` — 是否为 NegRisk 市场
- `GetFeeRateBps()` — 基础手续费（bps）
- `GetTrades()` / `GetAllTrades()` — 成交历史
- `GetBalanceAllowance()` — USDC 余额与 Token 授权量

**认证（Auth）：**
- `CreateApiKey()` — 创建 API Key（L1 私钥签名）
- `DeriveApiKey()` — 派生 API Key（L1 私钥签名）
- `GetApiKeys()` / `DeleteApiKey()` — 管理 API Key

**认证层级：**
- L0：无需认证（公开市场数据）
- L1：EIP-712 私钥签名（创建/派生 API Key）
- L2：HMAC 签名 + API Key/Passphrase（下单、查持仓等）

---

### 2. RelayClient（`client/relayer/`）

通过 Relayer 服务与链上 Safe 多签钱包交互。

**Safe 钱包管理：**
- `DeployWithPrivateKey()` / `DeployWithTurnkey()` — 部署 Safe 钱包
- `ExecuteWithPrivateKey()` / `ExecuteWithTurnkey()` — 执行 Safe 交易
- `IsDeployed()` — 检查 Safe 是否已部署
- `GetSafeNonceOnChain()` — 查询链上 Nonce

**Token 授权：**
- `ApproveForPolymarketWithPrivateKey()` / `...WithTurnkey()` — USDC 与 ERC-1155 授权
- `CheckAllApprovals()` — 验证所有必要授权是否到位

**持仓操作：**
- `RedeemPosition()` — 赎回普通持仓
- `RedeemNegRiskPosition()` — 赎回 NegRisk 持仓
- `SplitPosition()` — 拆分持仓
- `MergePosition()` — 合并持仓

**转账：**
- `TransferUsdce*()` — USDC.e 转账

---

### 3. DataSDK（`client/data/`）

从 Polymarket Data API（`https://data-api.polymarket.com`）读取用户与市场数据。

**持仓：**
- `GetCurrentPositions()` — 当前持仓
- `GetClosedPositions()` — 已平仓持仓
- `GetAllPositions()` — 全部持仓

**交易与活动：**
- `GetTrades()` — 成交记录
- `GetUserActivity()` — 用户操作记录

**市场统计：**
- `GetOpenInterest()` — 未平仓量
- `GetLiveVolume()` — 实时成交量
- `GetTopHolders()` — 持仓最多的用户

**组合查询：**
- `GetPortfolioSummary()` — 组合汇总
- `GetTotalValue()` — 组合总价值
- `GetTotalMarketsTraded()` — 参与市场数

---

### 4. BridgeClient（`client/bridge/`）

跨链资产桥接（`https://bridge.polymarket.com`）。

- `CreateDepositAddress()` — 创建充值地址
- `GetSupportedAssets()` — 获取可桥接资产列表
- 支持 EVM、SVM（Solana）、BTC 目标链

---

### 5. WebSocketClient（`client/ws/`）

实时行情订阅（`wss://ws-subscriptions-clob.polymarket.com`）。

- 订单簿变动（`BookMessage`）
- 价格变动（`PriceChangeMessage`）
- Tick Size 变动（`TickSizeChangeMessage`）
- 自动重连、Ping/Pong 保活
- 支持按 AssetID / Market 过滤

---

---

## 错误处理体系（`client/errors/`）

### 背景
SDK 原有错误均为 `fmt.Errorf` 字符串，上层无法通过类型判断错误类别，只能做脆弱的字符串匹配。

### 设计原则
- **不破坏现有接口**：原有所有方法签名不变，旧代码零改动
- **新增 Typed Wrapper**：`TypedClobClient` / `TypedDataSDK` 与原客户端并存，上层按需逐步迁移
- **迁移成本极低**：只需将原客户端包一层 `NewTypedClobClient(client)`

### 核心类型（`client/errors/errors.go`）

```go
type ErrCode int

const (
    ErrCodeBadRequest   ErrCode = 400  // 参数错误，不可重试
    ErrCodeUnauthorized ErrCode = 401  // 认证失效，需重新获取 API Key
    ErrCodeForbidden    ErrCode = 403  // 权限不足
    ErrCodeNotFound     ErrCode = 404  // 资源不存在
    ErrCodeRateLimit    ErrCode = 429  // 限流，退避后重试
    ErrCodeServerError  ErrCode = 500  // 服务端错误，可重试
    ErrCodeNetwork      ErrCode = 1001 // 网络错误，可重试
    ErrCodeUnmarshal    ErrCode = 1002 // 响应解析失败
    ErrCodeNotFoundBody ErrCode = 1003 // HTTP 200 但响应体为空（Polymarket 已知行为）
)

type SDKError struct {
    Code    ErrCode // 上层 switch 的主要依据
    Message string  // 人类可读描述
    Raw     string  // 原始响应体，调试用
    Cause   error   // 底层原始 error
}

// 快捷判断方法
func (e *SDKError) IsRetryable() bool  // 429 / 5xx / 网络错误
func (e *SDKError) IsNotFound() bool   // 404 + ErrCodeNotFoundBody
func (e *SDKError) IsAuthError() bool  // 401 / 403
```

### Typed Wrapper

| Wrapper | 文件 | 包装对象 |
|---|---|---|
| `TypedClobClient` | `client/clob/typed_client.go` | `*ClobClient` |
| `TypedDataSDK` | `client/data/typed_client.go` | `*DataSDK` |

**已 typed 化的方法：**

`TypedClobClient`：`GetOrder`、`CreateAndPostOrder`、`CreateAndPostMarketOrder`、`CancelOrder`、`CancelAllOrders`

`TypedDataSDK`：`GetCurrentPositions`、`GetClosedPositions`、`GetTrades`、`GetUserActivity`

**未 typed 化的方法**：通过 `.Inner()` 访问原客户端继续使用。

### 迁移示例

```go
// 原来（不变）
client := clob.NewClobClient(...)

// 包一层即可使用 typed 版本
typed := clob.NewTypedClobClient(client)

order, sdkErr := typed.GetOrder(addr, id)
if sdkErr != nil {
    switch {
    case sdkErr.IsNotFound():   // 订单不存在
    case sdkErr.IsAuthError():  // 重新 derive API Key
    case sdkErr.IsRetryable():  // 退避重试
    default:
        // 通过 sdkErr.Code 精确处理
    }
}

// 未迁移的方法走原客户端
markets, err := typed.Inner().GetMarkets(cursor)
```

---

## 工具包（`tools/`）

| 包 | 功能 |
|---|---|
| `tools/headers` | 生成 L1（EIP-712）和 L2（HMAC）认证 Header |
| `tools/eip712` | EIP-712 结构化签名 |
| `tools/hmac` | HMAC-SHA256 签名（L2 API Key 认证） |
| `tools/utils` | 通用工具（hex 处理、decimal 工具等） |

---

## 签名方式（`client/signer/`）

| 类型 | 说明 |
|---|---|
| `PrivateKeySigner` | 本地 ECDSA 私钥签名 |
| `TurnkeySigner` | 通过 Turnkey 云服务远程签名（机构用） |

---

## 外部依赖服务

| 服务 | 地址 | 用途 |
|---|---|---|
| Polymarket CLOB API | 初始化时传入 | 下单、订单簿、认证 |
| Polymarket Data API | `https://data-api.polymarket.com` | 持仓、成交、统计 |
| Polymarket Relayer | 初始化时传入 | 链上 Safe 交易执行 |
| Polymarket WebSocket | `wss://ws-subscriptions-clob.polymarket.com` | 实时行情 |
| Polymarket Bridge | `https://bridge.polymarket.com` | 跨链桥接 |
| Polygon RPC | `https://polygon-bor-rpc.publicnode.com` | 链上查询 |
| Turnkey | 配置注入 | 云端钱包签名 |

---

## 关键约束

1. **金额精度**：所有价格/数量使用 `shopspring/decimal`，禁止 float/double。
2. **订单金额字段**：`MakerAmount` / `TakerAmount` / `Price` / `Size` 均为字符串传输。
3. **TickSize 对齐**：订单价格必须向下取整到市场的 TickSize（见 `order_builder`）。
4. **NegRisk 路由**：NegRisk 市场使用独立合约地址（`NegExchange`、`NegCollateral`）。
5. **Safe Nonce**：每笔链上交易必须使用正确的 Safe Nonce，避免重放。
6. **认证 Header 时效**：L1/L2 Header 含时间戳，服务端有时间窗口校验。
7. **无 main()**：本项目为 SDK，不含可执行入口。
