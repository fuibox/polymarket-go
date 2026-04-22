# CLOB v2 迁移说明

本次修改在 SDK 内并行接入了 Polymarket CLOB v2 交易链路，v1 代码原封不动保留。

## 背景

Polymarket 将在 **2026-04-28 ~11:00 UTC**（约 1 小时停机）硬切到 CLOB v2：
- 切换后 v1 后端下线，v1 客户端立即失效
- 所有 v1 挂单清空，需要在 v2 重新下单
- 测试网 `clob-v2.polymarket.com` 当前已可用，可提前联调
- L1/L2 认证、WebSocket、Data API、Bridge、Relayer 流程**都不受影响**，只有交易链路改动

切换策略：v1/v2 并行，通过 `ClobClient.ProtocolVersion` 运行时派发。默认 `V1` 不改变现有行为。

---

## v1 vs v2 关键差异

| 项 | v1 | v2 |
|---|---|---|
| EIP-712 domain version | `"1"` | `"2"` |
| Exchange 地址（Polygon） | `0x4bFb41…982E` | `0xE11118…996B` |
| NegExchange 地址（Polygon） | `0xC5d563…f80a` | `0xe2222d…0F59` |
| 签名 Order 移除字段 | — | `taker` / `expiration` / `nonce` / `feeRateBps` |
| 签名 Order 新增字段 | — | `timestamp` (毫秒) / `metadata` (bytes32) / `builder` (bytes32) |
| 手续费来源 | 订单里的 `feeRateBps` | 撮合时协议设置；查询 `GET /clob-market-info/{conditionID}` |
| 抵押物 | USDC.e | **pUSD**（通过 CollateralOnramp `wrap()` 包装 USDC.e 得到） |
| Builder 归因 | `POLY_BUILDER_*` HMAC Header | 订单 `builder` 字段；v2 后端忽略 HMAC Header（本次迁移保留原有代码，4/28 后清理） |
| L1/L2 auth | — | 保持不变 |
| Cancel / 订单簿 / 查询 | — | 保持不变 |

新订单 EIP-712 类型串：

```
Order(uint256 salt,address maker,address signer,uint256 tokenId,uint256 makerAmount,
uint256 takerAmount,uint8 side,uint8 signatureType,uint256 timestamp,
bytes32 metadata,bytes32 builder)
```

---

## 本次改动清单

### 新增文件

| 文件 | 作用 |
|---|---|
| `client/clob/utils_order_builder_v2/utils_order_builder_v2_types.go` | v2 `OrderV2` 结构、EIP-712 类型串、`OrderStructHash`（11 字段，`bytes32` 原生 32 字节编码，不补零） |
| `client/clob/utils_order_builder_v2/utils_order_builder_v2.go` | v2 签名器：domain version="2"、v2 verifyingContract、`BuildSignedOrder` 返回 `types.SignedOrderV2` |
| `client/clob/utils_order_builder_v2/utils_order_v2_test.go` | 4 个离线签名测试（type-hash keccak、字节排布、ecrecover 回环、`bytes32` 严格解析） |
| `client/clob/order_builder/order_builder_v2.go` | `OrderBuilder.CreateOrderV2` / `CreateMarketOrderV2`，复用 v1 的 `GetOrderAmounts` |
| `client/clob/clob_client_v2.go` | `createOrderV2` / `createMarketOrderV2` / `postOrderV2` / `orderToBodyV2` / `GetClobMarketInfo` |
| `client/relayer/client_v2.go` | pUSD `WrapCollateral` / `UnwrapCollateral`（私钥 + Turnkey 变体）、`ApprovePUSDForPolymarket` |

### 修改文件

| 文件 | 改动 |
|---|---|
| `client/config/config.go` | `ContractConfig` 增加 `ExchangeV2`、`NegExchangeV2`、`PUSD`、`CollateralOnramp`。Polygon (137) 已填入文档地址；Amoy (80002) 保持零地址（Polymarket 尚未公布） |
| `client/types/types.go` | 新增 `ProtocolVersion` 枚举（`V1 = 1`, `V2 = 2`）与 `SignedOrderV2` 线路结构 |
| `client/clob/clob_types/clob_types.go` | `OrderArgs` / `MarketOrderArgs` 新增 `BuilderCode` 和 `Metadata` 字段；`FeeRateBps` / `Nonce` / `Expiration` / `Taker` 添加 `// v1-only` 注释 |
| `client/clob/clob_client.go` | `ClobClient` 增加 `protocolVersion` / `defaultBuilderCode`；`ClientConfig` 增加 `ProtocolVersion` / `DefaultBuilderCode`；`CreateAndPostOrder` / `CreateAndPostMarketOrder` 按 `protocolVersion` 派发到 v1 或 v2 实现 |
| `client/endpoint/endpoints.go` | 新增 `GetClobMarketInfo = "/clob-market-info/"` |
| `PROJECT.md` | 追加 v2 使用说明和待验证项 |
| `docs/Agents/PROJECT_BACKEND_RULES.md` | 追加第 10 节「CLOB v2 迁移期规则」 |
| `CLAUDE.md` | 新建仓库顶层指引文档 |

### 保持不动的代码

下列文件/能力在 v2 下**完全复用**，不需要分叉：

- `tools/headers/`（L1/L2 Header 生成）、`tools/eip712/`（L1 `ClobAuthDomain` 不变）、`tools/hmac/`
- `client/clob/order_builder/order_builder.go` 中的 `GetOrderAmounts` / `GetMarketOrderAmounts` / `resolveSignerAddress` / `validateAndGetRoundConfig`（价格、数量、Tick 取整逻辑 v2 不变）
- `client/clob/utils/`（decimal 工具）
- `client/relayer/model/polyEip712/`（Domain 结构）
- `postJSONWithHeaders` / `serializeJsonBody` / `GetServerTime` / `client/errors`
- `client/ws/`、`client/data/`、`client/bridge/`、`client/gamma/`

---

## 用法示例

### v1（默认，现状不变）

```go
client, _ := clob.NewClobClient(&clob.ClientConfig{
    Host:    "https://clob.polymarket.com",
    ChainID: 137,
    Signer:  s,
    APIKey:  creds,
})
resp, err := client.CreateAndPostOrder(args, opts)
```

### v2（切换前走 testnet；4/28 后直接走生产 host）

```go
client, _ := clob.NewClobClient(&clob.ClientConfig{
    Host:               "https://clob-v2.polymarket.com",
    ChainID:            137,
    Signer:             s,
    APIKey:             creds,
    ProtocolVersion:    types.ProtocolVersionV2,
    DefaultBuilderCode: "0x…32-byte hex…", // 可选；可按订单覆盖
})

// 下单接口签名保持不变
resp, err := client.CreateAndPostOrder(args, opts)

// 查询 v2 市场费率 / 最小单量等
info, err := client.GetClobMarketInfo(conditionID)
```

### 订单级 builder / metadata

```go
args := clob_types.OrderArgs{
    TokenID:     "...",
    Price:       decimal.NewFromFloat(0.55),
    Size:        decimal.NewFromInt(100),
    Side:        types.SideBuy,
    BuilderCode: "0x0000000000000000000000000000000000000000000000000000000000001234",
    Metadata:    "0x" + strings.Repeat("0", 64), // 可选
}
```

**注意**：`BuilderCode` / `Metadata` 必须是 **0x 开头的完整 32 字节 hex（64 个 hex 字符）**。短串或非 hex 会被签名器**主动拒绝**，而不是静默补零——这是有意的，避免归因错位。

### pUSD 资金流

```go
// 首次：USDC.e 包装成 pUSD，并批量授权 v2 合约
_, _ = relay.WrapCollateralWithPrivateKey(decimal.NewFromInt(100))
_, _ = relay.ApprovePUSDForPolymarketWithPrivateKey()

// 提现：pUSD 解包回 USDC.e
_, _ = relay.UnwrapCollateralWithPrivateKey(decimal.NewFromInt(50))

// Turnkey 变体
_, _ = relay.WrapCollateralWithTurnkey(turnkeyAccount, decimal.NewFromInt(100))
_, _ = relay.UnwrapCollateralWithTurnkey(turnkeyAccount, decimal.NewFromInt(50))
_, _ = relay.ApprovePUSDForPolymarketWithTurnkey(turnkeyAccount)
```

`WrapCollateral` 会打包两笔 Safe 交易：（1）Safe 授权 CollateralOnramp 花 USDC.e；（2）调用 `wrap(amount)`。一次 multisend 提交。

---

## 测试情况

- `go build ./...` 通过
- `go vet` 在所有本次改动的包上通过
- `go test ./client/clob/utils_order_builder_v2/...` 4/4 通过
- `go test ./tools/...` 通过

### utils_order_builder_v2 测试覆盖

1. `TestOrderTypeStringKeccak` — 断言 v2 `orderTypeString` 的 keccak 与 `OrderTypeHash` 一致，同时钉住字符串本身防止字段顺序漂移
2. `TestOrderStructHashByteLayout` — 独立实现 32 字节 × 12 格的编码，与 `OrderStructHash` 比对
3. `TestBuildSignedOrder_SignatureRoundTrip` — 固定私钥签名 → `crypto.SigToPub` 回推地址，验证整条 digest 流水线闭环
4. `TestParseBytes32_Strict` — 短 hex / 非法 hex 必须被拒绝

### 已知的预先存在问题（未在本次修复，非 v2 范围）

- `client/clob/clob_client_test.go` 调用 `GetTrades` 多传一个参数（v2 改动前就已编译失败）
- `client/relayer/client_test.go` 调用 `NewRelayClient` 少传一个参数（v2 改动前就已编译失败）
- `client/clob/utils_order_builder/utils_order_test.go:49` 使用 `big.NewInt(nil)` 路径 panic（v2 改动前就已失败）

---

## 待验证项（上线前必须完成）

| 项 | 状态 | 怎么做 |
|---|---|---|
| Amoy 的 v2 Exchange / NegExchange / pUSD / CollateralOnramp 地址 | Polymarket 未公布 | 拿到后填入 `client/config/config.go` 的 chain 80002 条目；当前是零地址，v2-on-Amoy 会因 `exchange address is zero` 主动报错（有意保护） |
| CollateralOnramp 的 `wrap` / `unwrap` ABI | 按 `wrap(uint256)` / `unwrap(uint256)` 假设实现 | 链上反编译字节码或看官方发布的 ABI 验证；不一致则更新 `client/relayer/client_v2.go` 中 `wrapFunctionSig` / `unwrapFunctionSig` |
| v2 `POST /order` 响应体是否与 v1 `OrderResponse` 同构 | 假设同构 | 在 testnet 跑一次 GTC / FOK，核对响应字段 |
| v2 Cancel 接口路径 + body | 假设与 v1 相同 | 在 testnet 跑 `CancelOrder` / `CancelAllOrders`，确认 endpoint 和 body 一致 |

### testnet 联调清单

```
1. Host = https://clob-v2.polymarket.com, ChainID = 80002, ProtocolVersion = V2
2. 调用 DeriveApiKey（L1 domain v2 下不变，应直接成功）
3. 分别下 GTC / GTD / FOK / FAK 各一单，确认 Success=true 且 /data/orders 查得到
4. 调用 CancelOrder / CancelAllOrders，确认返回正常
5. 调用 GetClobMarketInfo(conditionID),确认 {r, e, to} 字段能解析
6. 下一单 FOK 大单（必然 partial-fill-then-kill），确认返回不被 SDK 当作错误
7. 链上：ApprovePUSDForPolymarket → WrapCollateral(10 USDC) → 看 pUSD 余额
```

---

## 4/28 切换当日 / 后的清理（**不在本 PR 内做**）

切换日当晚或次日，单独起 PR 删除下列内容：

- 整个 `client/clob/utils_order_builder/` 包
- `OrderArgs` / `MarketOrderArgs` 中的 `FeeRateBps` / `Nonce` / `Expiration` / `Taker`
- `tools/headers/headers.go`：`L2WithBuilderHeader` / `BuilderConfig` / `GenerateBuilderHeaders` / `InsertBuilderHeaders` 及其调用位
- `ClobClient.ResolveFeeRateBps` / `GetFeeRateBps`；使用方改成 `GetClobMarketInfo`
- `ContractConfig.Exchange` / `NegExchange` 的 v1 地址（把 `ExchangeV2` / `NegExchangeV2` 提升为这些字段的默认值，然后删别名）
- USDC.e 的各种 Transfer 辅助改名为 pUSD
- `ClobClient.protocolVersion` / `ClientConfig.ProtocolVersion` 字段本身（只剩 v2 一条路径）

建议在项目里开一个 2026-04-28 的提醒 / issue，避免遗漏。
