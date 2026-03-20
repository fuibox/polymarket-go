# Task Template

Use this structure when assigning a coding task.

---

Read AGENTS.md, PROJECT_MAP.md, and CRITICAL_RULES.md.
Summarize the relevant constraints before editing code.

Task:
[describe the issue clearly]

Target files:
[list the most likely relevant files]

Problem:
[describe the bug or expected behavior]

Constraints:
- keep changes minimal
- avoid modifying unrelated modules
- keep transactions short
- preserve idempotency
- preserve concurrency safety

Expected result:
[describe the desired outcome]

Task:
trade 上链需要，taker价格需要取trades.price,不能用taker quote中的price
Target files:
internal/syncer/trade_sync.go

Task:
合约event TradeMatch 升级，新增trade id参数：tid
1.修改model，生成新增字段sql
2.修改合约abi
3.修改TradeMatch event监控入库逻辑，赋值tid

Task:
event 链上 resolve需要新增一个结算模式：UMA OOV3
Target files:
合约已经实现两种模式：平台直接resolve结果、UMA裁决结果，可以查看C:\Project\smartcontract\PredictionMarket\Surf\premarket\docs\current_contract_requirements_baseline.md 合约功能说明
Problem:
先分析当前实现，结合智能合约当前设计，拆解实现方案，先不着急写代码，有任何疑问及时提出

Task:
分析订单取消失败原因
Problem:
用户在前端取消订单提示成功，订单并没有实际取消
针对当前任务有任何需求不清楚及时提出