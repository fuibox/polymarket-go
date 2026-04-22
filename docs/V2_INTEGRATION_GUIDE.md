# Polymarket CLOB v2 SDK 接入指南

本文档说明如何使用 `github.com/fuibox/polymarket-go` 接入 Polymarket CLOB v2
(2026-04-28 切换)。v1 代码路径保留；v2 通过 `ClientConfig.ProtocolVersion`
开启。

## 一、前置依赖

### 必备

- Go 1.25.6（见 `go.mod`）
- Polygon 主网 RPC（默认 `https://polygon-bor-rpc.publicnode.com`）
- **Builder credentials** — 三件套：`POLY_BUILDER_API_KEY` / `SECRET` /
  `PASSPHRASE`。向 Polymarket 申请。
  - 注意：Builder creds 用来认证 **Relayer 服务**（Safe 部署、wrap、approve
    等链上操作）。与下单 `/order` 用的 L2 api key 是**不同两套凭据**。L2 api
    key 由 SDK 自动 derive。

### 可选

- Turnkey 账号（走云签名，不用本地私钥）
- `POLY_BUILDER_CODE`（订单级归因，v2 直接写到 order.builder 字段）

---

## 二、构造 ClobClient

```go
import (
    "github.com/fuibox/polymarket-go/client/clob"
    "github.com/fuibox/polymarket-go/client/signer"
    "github.com/fuibox/polymarket-go/client/types"
    "github.com/fuibox/polymarket-go/tools/headers"
)

// 方式 A：本地私钥
sig, _ := signer.NewSigner(signer.SignerConfig{
    SignerType:       signer.PrivateKey,
    ChainID:          137,
    PrivateKeyConfig: &signer.PrivateKeyClient{PrivateKey: priv, Address: eoa},
})

// 方式 B：Turnkey
sig, _ := signer.NewSigner(signer.SignerConfig{
    SignerType:    signer.Turnkey,
    ChainID:       137,
    TurnkeyConfig: &turnkey.Config{...},
})

// Builder config（Relayer 必需）
builderCfg := &headers.BuilderConfig{
    APIKey:     os.Getenv("POLY_BUILDER_API_KEY"),
    Secret:     os.Getenv("POLY_BUILDER_SECRET"),
    Passphrase: os.Getenv("POLY_BUILDER_PASSPHRASE"),
}

// ClobClient（v2）
clobClient, _ := clob.NewClobClient(&clob.ClientConfig{
    Host:            "https://clob-v2.polymarket.com", // 4/28 前；切换后走 clob.polymarket.com
    ChainID:         137,
    Signer:          sig,
    BuilderConfig:   builderCfg,
    ProtocolVersion: types.ProtocolVersionV2,          // 关键；默认 v1 不改变行为
    // DefaultBuilderCode: "0x...", // 可选：订单级归因
})
```

---

## 三、Safe 钱包 + 首次授权（每个新 Safe 一次）

```go
import (
    "github.com/fuibox/polymarket-go/client/relayer"
    "github.com/fuibox/polymarket-go/client/relayer/builder"
    "github.com/fuibox/polymarket-go/client/config"
)

cc, _ := config.GetContractConfig(137)

// 1) 算 Safe 地址（CREATE2 预测，不需要上链）
safeAddr := builder.Derive(eoa, cc.SafeFactory)
// Turnkey: safeAddr := builder.Derive(turnkeyAcc, cc.SafeFactory)

// 2) RelayClient
relay, _ := relayer.NewRelayClient(
    "https://relayer-v2.polymarket.com",
    137, sig, builderCfg, nil, nil, // 最后一个 polygonRpc 为 nil 走默认
)

// 3) 部署 Safe（幂等）
if deployed, _ := relay.IsDeployed(safeAddr); !deployed {
    relay.DeployWithPrivateKey() // Turnkey: DeployWithTurnkey(turnkeyAcc)
}

// 4) USDC.e → pUSD（v2 唯一可交易抵押物）
relay.WrapCollateralWithPrivateKey(decimal.NewFromInt(10))
// Turnkey: WrapCollateralWithTurnkey(turnkeyAcc, decimal.NewFromInt(10))

// 5) pUSD 授权（一次打包授权 4 个 spender: ExchangeV2 / NegExchangeV2 /
//    NegRiskAdapterV2 / CollateralOnramp，全部 MAX）
relay.ApprovePUSDForPolymarketWithPrivateKey()
// Turnkey: ApprovePUSDForPolymarketWithTurnkey(turnkeyAcc)

// 6) CTF ERC-1155 setApprovalForAll（BUY 也需要！v2 server 少了这步会 HTTP 500）
relay.ApproveCTFForPolymarketV2WithPrivateKey()
// Turnkey: ApproveCTFForPolymarketV2WithTurnkey(turnkeyAcc)
```

每步都要**等链上落块**才做下一步。Relayer 是异步提交，客户端需要轮询
RPC 确认状态。参考 `smoke/clob_v2_smoke_test.go` 里的
`waitUntilPUSDAtLeast` / `waitUntilAllowanceNonZero`。

---

## 四、L2 API Key 派生

```go
creds, err := clobClient.CreateOrDeriveApiKey(nil, clob_types.ClobOption{
    SafeAccount: safeAddr,
})
// creds 应该持久化保存（文件或 KMS）；derive 虽然确定性，但每次都需要 EOA 在线
// 再次签名一次 L1，费时

// 带 creds 重建 ClobClient
clobClient, _ = clob.NewClobClient(&clob.ClientConfig{
    Host:            "...",
    ChainID:         137,
    Signer:          sig,
    BuilderConfig:   builderCfg,
    APIKey:          creds,                       // 新增
    ProtocolVersion: types.ProtocolVersionV2,
})
```

---

## 五、下单（4 种组合）

所有下单接口都**通过 `args.Side` 选买卖**，没有单独的 BUY/SELL 方法。

### 限价单

```go
tick, _ := clobClient.GetTickSize(tokenID)
negRisk, _ := clobClient.GetNegRisk(tokenID)

args := clob_types.OrderArgs{
    TokenID: tokenID,
    Price:   decimal.NewFromFloat(0.24),         // BUY 价上限 / SELL 价下限
    Size:    decimal.NewFromInt(5),
    Side:    types.SideBuy, // 或 types.SideSell
}
opts := clob_types.PartialCreateOrderOptions{
    OrderType:      types.OrderTypeGTC,          // GTC / GTD / FOK / FAK
    TickSize:       &tick,
    NegRisk:        &negRisk,
    SafeAccount:    safeAddr,
    TurnkeyAccount: turnkeyAcc,                   // 仅 Turnkey 时设
}

resp, err := clobClient.CreateAndPostOrder(args, opts)
// resp.OrderID / resp.Status / resp.MakingAmount / resp.TakingAmount
```

### 市价单

```go
mArgs := clob_types.MarketOrderArgs{
    TokenID:   tokenID,
    Amount:    decimal.NewFromFloat(5.0),        // BUY: pUSD 预算上限；SELL: token 数量
    Price:     decimal.NewFromFloat(0.99),       // 最差可接受价
    Side:      types.SideBuy,                    // 或 SideSell
    OrderType: types.OrderTypeFAK,                // FOK / FAK
}
resp, err := clobClient.CreateAndPostMarketOrder(mArgs, opts)
```

> **Tips**：v2 的 Market Order 在某些 tick/价格组合下会被服务器以
> `could not run the execution` 拒绝（隐含 maker/taker 比值不整除）。如果碰到，
> 用 **限价 FOK** 替代：`price = top_ask + tick`, `size` 取整数 tokens。
> 参考 smoke test 中 `--- stage 8 ---` 的实现。

### 订单状态与撤单

```go
ord, err := clobClient.GetOrder(eoa, resp.OrderID)
// ord.Status: LIVE / MATCHED / CANCELED / EXPIRED

cancelResp, err := clobClient.CancelOrder(orderID, eoa)
// Turnkey 时传 turnkeyAcc

// 注：v2 cancel 成功时响应体可能为空，SDK 当前解析为零值（"success=false" 是假象）
// 通过 GetOrder 再查一遍状态确认
```

---

## 六、余额 / 授权的服务器视角查询

```go
import "github.com/fuibox/polymarket-go/client/constants"

sigType := int(constants.POLY_GNOSIS_SAFE)

// 让服务器重新读链上状态（刚 approve 完可能要调一次）
clobClient.UpdateBalanceAllowance(eoa, &types.BalanceAllowanceParams{
    AssetType:     types.AssetTypeCollateral,
    SignatureType: &sigType,                      // 关键！少这个服务器按 EOA 查
})

// 读服务器视角
ba, _ := clobClient.GetBalanceAllowance(eoa, &types.BalanceAllowanceParams{
    AssetType:     types.AssetTypeCollateral,
    SignatureType: &sigType,
})
// ba.Balance      — pUSD 余额（6-dec 字符串）
// ba.Allowances   — map[spender address] → allowance（v2 新形状）
```

---

## 七、常见陷阱与排雷

| 症状 | 原因 | 排查 |
|---|---|---|
| `HTTP 401 Unauthorized` | L2 `POLY_ADDRESS` 传了 Safe 地址 | 必须用 **EOA**（api key 所有者） |
| `HTTP 500 could not run the execution` | 缺 CTF `setApprovalForAll` 或 POST body 缺 `postOnly`/`deferExec` | 调 `ApproveCTFForPolymarketV2*`；SDK 的 `FinalBodyV2` 已含后者 |
| `not enough balance / allowance` | pUSD 授权漏了 NegRiskAdapter | 调 `ApprovePUSDForPolymarket*`（SDK 已覆盖 4 spender） |
| `invalid signature` | 换了 token 但 tick/negRisk 还用旧的 | 每次换 token 重新 `GetTickSize` + `GetNegRisk` 再签 |
| `INVALID_ORDER_MIN_SIZE` | 订单价值 < $1 | 市价单 Amount ≥ 1；限价单 `size × price ≥ 1` |
| `could not run the execution`（授权都对） | 订单簿深度不够（fill 后剩余 < $1） | 换流动性更好的 token 或加大 Size |
| 市价单反复失败 | `GetMarketOrderAmounts` 的隐含价可能被服务器拒 | 改用限价 FOK：`price = top_ask + tick`，size 取整数 |
| `Get /deployed: EOF` | Relayer 短暂网络抖动 | 直接重试即可（幂等） |

---

## 八、合约地址（Polygon 137）

```
Exchange V2:         0xE111180000d2663C0091e4f400237545B87B996B
NegExchange V2:      0xe2222d279d744050d28e00520010520000310F59
NegRisk Adapter:     0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296
ConditionalTokens:   0x4D97DCd97eC945f40cF65F87097ACe5EA0476045
pUSD:                0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB
CollateralOnramp:    0x93070a847efEf7F70739046A929D47a521F5B8ee
USDC.e:              0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174
```

Amoy (chain 80002) 的 v2 地址 Polymarket 尚未公布，`ContractConfig` 对应
字段为零地址。在零地址上调用会主动报错（有意保护）。

---

## 九、外部验证必做项（上生产前）

1. **CollateralOnramp.unwrap ABI**：SDK 按 `unwrap(address,address,uint256)`
   猜的，**未经链上验证**。用 `cast sig` 或反编译字节码核实，然后更新
   `client/relayer/client_v2.go` 的 `unwrapFunctionSig` 常量
2. **Amoy v2 合约地址**：Polymarket 发布后填入 `client/config/config.go`
3. **v2 Cancel 响应形状**：抓一次 raw 撤单 response，若与 v1 不同，补 v2 专属 struct

---

## 十、Turnkey 完整调用示例

SDK 的签名 / 订单 / HTTP / Relayer 各层**都有对称的 Turnkey 分支**。
Turnkey 用户跑整条链路：

```go
// 0) Turnkey signer
sig, _ := signer.NewSigner(signer.SignerConfig{
    SignerType:    signer.Turnkey,
    ChainID:       137,
    TurnkeyConfig: &turnkey.Config{ /* Turnkey 云配置 */ },
})

turnkeyAcc := common.HexToAddress("0x...")      // Turnkey 账户地址
safeAddr := builder.Derive(turnkeyAcc, cc.SafeFactory)

// 1) 部署 + wrap + 授权
relay, _ := relayer.NewRelayClient(relayerURL, 137, sig, builderCfg, nil, &rpc)
relay.DeployWithTurnkey(turnkeyAcc)
relay.WrapCollateralWithTurnkey(turnkeyAcc, decimal.NewFromInt(10))
relay.ApprovePUSDForPolymarketWithTurnkey(turnkeyAcc)
relay.ApproveCTFForPolymarketV2WithTurnkey(turnkeyAcc)

// 2) ClobClient
clobClient, _ := clob.NewClobClient(&clob.ClientConfig{
    Host:            "https://clob-v2.polymarket.com",
    ChainID:         137,
    Signer:          sig,
    BuilderConfig:   builderCfg,
    ProtocolVersion: types.ProtocolVersionV2,
})

// 3) 派生 api key（Turnkey 下 L1 签名会走 Turnkey Sign API）
creds, _ := clobClient.CreateOrDeriveApiKey(nil, clob_types.ClobOption{
    TurnkeyAccount: turnkeyAcc,
    SafeAccount:    safeAddr,
})

// 4) 下单（注意 opts.TurnkeyAccount）
clobClient2, _ := clob.NewClobClient(&clob.ClientConfig{
    /* 同上 + */ APIKey: creds,
})

opts := clob_types.PartialCreateOrderOptions{
    OrderType:      types.OrderTypeGTC,
    TickSize:       &tick,
    NegRisk:        &negRisk,
    SafeAccount:    safeAddr,
    TurnkeyAccount: turnkeyAcc,      // ← 必须填，否则 signer 走不通 Turnkey 分支
}
clobClient2.CreateAndPostOrder(args, opts)
clobClient2.CreateAndPostMarketOrder(mArgs, opts)

// 5) 撤单
clobClient2.CancelOrder(orderID, turnkeyAcc)
```

---

## 十一、参考文档

- `docs/CLOB_V2_MIGRATION.md` —— v2 迁移总纲（中文）
- `docs/DAILY_2026-04-21_V2_MIGRATION.md` —— 开发日报（9 个坑的来龙去脉）
- `docs/Agents/PROJECT_BACKEND_RULES.md` §10 —— v2 期代码约定
- `smoke/clob_v2_smoke_test.go` —— 可复制粘贴的 E2E 实现（BUY + SELL 都走通）
- `PROJECT.md` —— 模块导航

---

## 十二、相关环境变量一览

| 变量 | 用途 | 必填 |
|---|---|---|
| `POLY_BUILDER_API_KEY` | Relayer 认证 | ✓ |
| `POLY_BUILDER_SECRET` | Relayer 认证 | ✓ |
| `POLY_BUILDER_PASSPHRASE` | Relayer 认证 | ✓ |
| `POLY_RELAYER_URL` | Relayer endpoint | 可选，默认 `https://relayer-v2.polymarket.com` |
| `POLY_CLOB_HOST` | CLOB endpoint | 可选，默认 `https://clob-v2.polymarket.com` |
| `POLY_POLYGON_RPC` | Polygon RPC | 可选，默认 `https://polygon-bor-rpc.publicnode.com` |

## 十三、测试工具

smoke test 在 `smoke/clob_v2_smoke_test.go`，用 build tag `smoke` 隔离：

```bash
# BUY 流程（默认）
go test -tags smoke ./smoke -run TestClobV2_FullFlow -v -timeout 45m

# SELL 当前持仓
POLY_SIDE=SELL go test -tags smoke ./smoke -run TestClobV2_FullFlow -v -timeout 45m

# 换 token
POLY_TOKEN_ID=<token_id> go test -tags smoke ./smoke -run TestClobV2_FullFlow -v -timeout 45m

# 加大金额
POLY_MARKET_ORDER_USD=10 go test -tags smoke ./smoke -run TestClobV2_FullFlow -v -timeout 45m
```

凭据在 `smoke/.env` 里（已 gitignore）。钱包和 API key 在 `smoke/.wallet.json`
（已 gitignore）。两个文件都含私钥级别的机密，**绝不能提交或分享**。
