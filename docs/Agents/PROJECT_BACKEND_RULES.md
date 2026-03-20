# PROJECT_BACKEND_RULES.md

Project-specific rules and conventions for this repository.

These rules extend `BASE_BACKEND_RULES.md`.

---

## 1) Financial Correctness（项目专属）

### 1.1 金额字段一律用字符串传输
API 请求与响应中，所有价格、数量、金额字段（`Price`、`Size`、`MakerAmount`、`TakerAmount`、`FeeRateBps` 等）均以 **字符串**形式传输，不得使用数字类型。

### 1.2 内部计算使用 decimal
所有内部运算使用 `github.com/shopspring/decimal`，禁止使用 `float32` / `float64`。

### 1.3 价格必须对齐 TickSize
提交订单前，价格必须向下取整（floor）到该市场的 TickSize。
实现见 `client/clob/order_builder/`。
未对齐的订单会被服务端拒绝。

### 1.4 禁止在未确认订单状态前重试下单
下单（`POST /order`）失败后，**不得直接重试**。
必须先查询订单状态，确认订单不存在或已失败，再重新提交。
原因：网络超时不代表服务端未收到，盲目重试会导致重复下单。

---

## 2) 认证层级约定

本项目有三个认证层级，选错层级会导致 401 / 403：

| 层级 | 签名方式 | 适用场景 |
|---|---|---|
| L0 | 无 | 公开市场数据（订单簿、价格等） |
| L1 | EIP-712 私钥签名 | 创建 / 派生 API Key |
| L2 | HMAC-SHA256 + API Key | 下单、撤单、查持仓、查余额等 |

### 2.1 Header 生成
- L1 Header：见 `tools/headers/`，调用 `CreateL1Headers()`
- L2 Header：见 `tools/headers/`，调用 `CreateL2Headers()`
- Builder API 额外 Header：调用 `InsertBuilderHeaders()`

### 2.2 时间窗口
L1 / L2 Header 含时间戳，服务端有有效时间窗口限制。
不得缓存复用 Header，每次请求须重新生成。

---

## 3) 订单构造约定

### 3.1 NegRisk 市场路由
调用 `GetNegRisk()` 判断市场类型：
- 普通市场：使用 `Exchange` + `Collateral`
- NegRisk 市场：使用 `NegExchange` + `NegCollateral`

合约地址见 `client/config/`，路由逻辑见 `client/clob/order_builder/`。
混用合约会导致链上交易 revert，且难以排查。

### 3.2 OrderType 语义与重试行为

| OrderType | 语义 | 未完全成交时 |
|---|---|---|
| GTC | Good Till Cancel | 挂单保留 |
| GTD | Good Till Date | 到期自动撤销 |
| FOK | Fill or Kill | 立即全量成交，否则整单撤销 |
| FAK | Fill and Kill | 立即尽量成交，剩余撤销 |

FOK / FAK 未成交不是错误，不得当作失败重试。

### 3.3 链上 Safe Nonce
每笔 Relayer 交易必须使用正确的链上 Nonce（通过 `GetSafeNonceOnChain()` 获取）。
Nonce 错误会导致链上交易失败或被覆盖。
不得本地自增 Nonce，始终从链上读取最新值。

### 3.4 Token 授权前置
执行任何交易前，先调用 `CheckAllApprovals()` 确认 USDC 和 ERC-1155 授权已到位。
缺少授权会导致链上交易 revert。
授权是一次性操作，不需要每次交易前重复执行，但首次使用前必须检查。

### 3.5 签名器选择
- 本地私钥：使用 `PrivateKeySigner`
- 机构 / 云端：使用 `TurnkeySigner`

两者实现同一接口，不得在业务逻辑中硬编码签名方式。
新增功能必须同时支持两种签名器。

---

## 4) SDK 架构约定

### 4.1 纯库，无数据库
本项目无数据库、无持久化层、无事务。
`BASE_BACKEND_RULES.md` 中关于 DB 事务、guarded update、乐观锁的规则**在本项目不适用**。

### 4.2 无 main()
本项目为可复用 SDK，不含可执行入口，不得新增 `main` 包。

### 4.3 两条链并行支持
所有涉及合约地址的代码必须通过 `client/config/` 的 `ContractConfig` 获取，
不得硬编码地址，以保证主网（137）与测试网（80002）均可正常运行。

### 4.4 外部调用的幂等性
本项目无 DB 事务，但对外 HTTP / RPC 调用本身有副作用（下单、链上交易）。
重试前必须先确认操作状态，避免重复副作用。具体见 §1.4。

---

## 5) 错误处理约定

### 5.1 区分可重试与不可重试错误

| 错误类型 | 示例 | 处理方式 |
|---|---|---|
| 客户端错误（4xx） | 参数非法、签名错误、订单不存在 | 不重试，直接返回错误 |
| 服务端错误（5xx） | 服务暂时不可用 | 可退避重试 |
| 限流（429） | Too Many Requests | 退避重试，遵守 Retry-After |
| 网络超时 | 请求未到达或响应丢失 | 先查状态再决定是否重试（见 §1.4） |
| 链上 revert | 合约执行失败 | 不重试，排查参数（授权、Nonce、合约路由） |
| 链上 RPC 网络错误 | 节点超时、连接断开 | 可重试（无副作用） |

### 5.2 错误必须携带上下文
返回错误时须包含足够的定位信息：

```go
// 正确
fmt.Errorf("cancel order %s: %w", orderID, err)

// 错误 — 丢失上下文
return err
```

### 5.3 不得吞掉错误
不得用空的 `if err != nil {}` 或仅打日志而不返回错误。
调用方有权决定如何处理错误，SDK 不应替调用方决策。

---

## 6) 安全约束

### 6.1 禁止在日志 / 错误信息中输出敏感数据
以下字段**绝对不得**出现在日志、错误消息、panic 信息中：
- 私钥（`PrivateKey`）
- API Secret / Passphrase
- HMAC 签名原文
- Turnkey 凭证

违反此规则会导致密钥泄露，影响用户资产安全。

### 6.2 私钥不得序列化
包含私钥的结构体不得实现 `json.Marshal` 或以任何形式序列化到外部。

### 6.3 地址校验
接受外部传入的以太坊地址时，必须校验其格式（`common.IsHexAddress()`），
不得将未校验的字符串直接用于合约调用。

---

## 7) 并发安全

### 7.1 客户端可并发调用
`ClobClient`、`DataSDK`、`BridgeClient` 内部使用 HTTP 客户端（resty），天然支持并发调用。
不得在 SDK 层面加全局锁来串行化请求。

### 7.2 WebSocket 写操作需串行
`WebSocketClient` 的写操作（订阅、发送 Ping）不是并发安全的。
调用方若需并发操作，须在外部加锁或通过 channel 串行化。

### 7.3 Turnkey 签名并发
`TurnkeySigner` 每次签名是独立 HTTP 请求，天然支持并发。
不得对签名操作加全局锁。

---

## 8) 测试约定

### 8.1 链上操作使用测试网
涉及 `RelayClient` 的集成测试必须使用 Amoy 测试网（ChainID 80002），
禁止在主网（ChainID 137）上运行测试。

### 8.2 外部 HTTP 依赖使用 mock server
CLOB API / Data API 的单元测试使用 `net/http/httptest` mock 服务端，
不得在单元测试中发起真实网络请求。

### 8.3 签名可离线验证
EIP-712 签名和 HMAC 签名的正确性可在不连接外部服务的情况下验证，
签名相关逻辑的测试必须是纯离线测试。

---

## 9) 日志约定

### 9.1 SDK 不得使用全局 logger（目标规范，当前部分不符合）

**目标规范：**
SDK 不得依赖标准库全局 `log.Printf` / `log.Println` 或任何第三方全局 logger。
所有需要输出日志的组件，应在结构体中注入 `*log.Logger`（或更通用的 logger 接口），由调用方决定日志输出目标。
原因：调用方有自己的日志系统，全局 logger 会污染其输出，且无法被静默或重定向。

**当前现状（不符合规范的文件）：**
- `client/ws/websocket_client.go` — 结构体已有 `Logger *log.Logger` 字段，但内部大量直接调用全局 `log.Printf`，注入的 logger 未被一致使用
- `client/relayer/client.go` — 直接调用全局 `log.Printf`
- `client/clob/clob_client.go` — 直接调用全局 `log.Printf`
- `tools/hmac/hmac.go` — 直接调用全局 `log.Printf`
- `client/gamma/client.go` — 直接调用全局 `log.Printf`

**改动要求：**
修改上述任意文件时，若涉及日志相关代码，须同步将该文件内的全局 `log.Printf` 替换为结构体注入的 logger。
不要在一次 PR 中批量替换所有文件（超出三文件原则），随改随治。

### 9.2 错误优先于日志
能通过返回 `error` 传递的信息，不得改用日志输出。
日志仅用于记录无法通过错误链传递的运行时状态（如 WebSocket 重连事件、连接建立/断开）。

---

## 10) Reminder

If project-specific rules conflict with base rules (idempotency, guarded update, short transactions), stop and ask.
