# PROJECT_MAP.md — 目录导航

## 顶层结构

```
polymarket-go/
├── client/          # 所有 SDK 客户端
├── tools/           # 通用工具（签名、Header、工具函数）
├── turnkey/         # Turnkey 云签名服务封装
├── docs/            # 文档
├── go.mod           # 模块：github.com/fuibox/polymarket-go
└── go.sum
```

---

## client/ 模块导航

```
client/
├── clob/                    # CLOB 交易客户端（核心）
│   ├── clob_client.go       # 主客户端：下单、查询、认证
│   ├── typed_client.go      # TypedClobClient：返回 *SDKError 的 Wrapper（新）
│   ├── clob_types/          # 订单参数类型（OrderArgs、MarketOrderArgs 等）
│   └── order_builder/       # 订单签名与构造（价格取整、合约路由）
│
├── relayer/                 # 链上 Safe 钱包交互
│   ├── client.go            # 主客户端：部署、授权、赎回、转账
│   └── model/               # Safe 交易模型（SafeTransaction、TransactionRequest）
│
├── data/                    # Data API 客户端
│   ├── client.go            # 持仓、成交、统计查询
│   ├── typed_client.go      # TypedDataSDK：返回 *SDKError 的 Wrapper（新）
│   └── types.go             # 数据类型（Position、ClosedPosition、Activity 等）
│
├── errors/                  # 统一错误类型（新）
│   └── errors.go            # SDKError、ErrCode、工具方法
│
├── bridge/
│   └── bridge.go            # 跨链桥接客户端
│
├── ws/
│   └── websocket_client.go  # WebSocket 实时订阅（自动重连）
│
├── gamma/                   # Gamma 衍生品客户端
│
├── types/
│   ├── types.go             # 核心类型：Order、Trade、Auth Header、Chain 等
│   └── websocket.go         # WebSocket 消息类型
│
├── constants/               # 合约地址、签名类型、访问级别常量
├── config/                  # ContractConfig（各链合约地址映射）
├── endpoint/                # 所有 API 路径常量
└── signer/                  # 签名器（PrivateKey / Turnkey）
```

---

## tools/ 模块导航

```
tools/
├── headers/         # 生成 L1（EIP-712）和 L2（HMAC）认证 Header
├── eip712/          # EIP-712 结构化签名实现
├── hmac/            # HMAC-SHA256 签名（L2 API Key 认证）
└── utils/           # 通用工具（hex、decimal 处理）
```

---

## 关键文件速查

| 需要做什么 | 看哪个文件 |
|---|---|
| 下单 / 撤单（原有接口） | `client/clob/clob_client.go` |
| 下单 / 撤单（typed 错误） | `client/clob/typed_client.go` |
| 持仓 / 成交查询（原有接口） | `client/data/client.go` |
| 持仓 / 成交查询（typed 错误） | `client/data/typed_client.go` |
| 错误类型定义 / ErrCode | `client/errors/errors.go` |
| 订单签名逻辑 / 价格取整 | `client/clob/order_builder/` |
| 订单参数类型 | `client/clob/clob_types/` |
| 链上 Safe 交易 / Token 授权 | `client/relayer/client.go` |
| Safe 交易数据结构 | `client/relayer/model/` |
| 跨链桥接 | `client/bridge/bridge.go` |
| 实时行情订阅 | `client/ws/websocket_client.go` |
| 核心数据类型（Order、Trade） | `client/types/types.go` |
| 合约地址（主网/测试网） | `client/config/` + `client/constants/` |
| API 路径常量 | `client/endpoint/` |
| L1/L2 认证 Header 生成 | `tools/headers/` |
| EIP-712 签名 | `tools/eip712/` |
| HMAC 签名 | `tools/hmac/` |
| Turnkey 云签名 | `turnkey/` + `client/signer/` |

---

## 调试入口

### 认证问题
1. 检查 Header 生成：`tools/headers/`
2. L1 用 EIP-712：`tools/eip712/`
3. L2 用 HMAC：`tools/hmac/`
4. 确认时间戳有效（服务端有时间窗口限制）

### 下单失败
1. 检查订单构造：`client/clob/order_builder/`
2. 确认价格已对齐 TickSize（`GetTickSize()`）
3. 确认 NegRisk 市场用了正确合约（`GetNegRisk()`）
4. 检查 `MakerAmount` / `TakerAmount` 字符串格式

### 链上交易失败
1. 检查 Safe 是否已部署：`IsDeployed()`
2. 检查 Nonce：`GetSafeNonceOnChain()`
3. 检查 Token 授权：`CheckAllApprovals()`
4. Safe 交易结构：`client/relayer/model/`

### WebSocket 断连
- 重连逻辑在 `client/ws/websocket_client.go`
- 检查 Ping/Pong 超时配置

---

## 外部服务地址

| 服务 | 地址 |
|---|---|
| Polymarket Data API | `https://data-api.polymarket.com` |
| Polymarket Bridge | `https://bridge.polymarket.com` |
| WebSocket | `wss://ws-subscriptions-clob.polymarket.com` |
| Polygon RPC | `https://polygon-bor-rpc.publicnode.com` |
| CLOB API / Relayer | 初始化时由调用方传入 |
