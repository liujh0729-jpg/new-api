# AIPDD × NewAPI 财务台账第一阶段技术契约

状态：实施基线（2026-08-08）

## 1. 边界与事实源

- 普通请求日志、`new_api_usage_event`、任务展示表只用于诊断或来源关联，不作为财务事实源。
- AIPDD 的 `finance_order_ledger`、`finance_order_movement` 与事务 outbox 是上游成本事实源。
- NewAPI 的 `aipdd_finance_order`、movement、inbox/outbox 是客户扣费事实及 AIPDD 结算镜像。
- 原始 API Key 永不进入订单、事件、报表或导出。只保存 AIPDD `api_key_id`、NewAPI `token_id` 和实例 ID。
- 金额跨系统一律使用十进制定点字符串；AIPDD 数据库使用 `NUMERIC(24,6)`，NewAPI 使用人民币微元 `int64`（1 元 = 1,000,000 微元）。

## 2. 统一订单身份

NewAPI 调用 AIPDD 时发送：

| Header | 含义 | 约束 |
|---|---|---|
| `X-AIPDD-Instance-ID` | NewAPI 实例 UUID | 必填且必须归属当前 AIPDD API Key |
| `X-AIPDD-Order-ID` | 平台订单 ID | 一次客户请求全生命周期稳定；当前采用 NewAPI request ID |
| `X-AIPDD-Attempt-ID` | 上游尝试 ID | `order_id:retry_index:channel_id`，重试可区分 |
| `X-AIPDD-NewAPI-User-ID` | NewAPI 本地用户 ID | 仅数字字符串 |
| `X-AIPDD-NewAPI-Token-ID` | NewAPI 本地 token ID | 仅数字字符串，不是 token 内容 |

AIPDD 唯一键为 `(api_key_id, instance_id, platform_order_id)`。同一订单的后续尝试更新 `latest_attempt_id`，不得新建重复财务订单。

## 3. 状态机

订单状态：

`RECEIVED -> PROCESSING -> SUCCEEDED | FAILED | CANCELLED`

NewAPI 本地扣费状态与 AIPDD 上游订单状态分列，不能互相覆盖：

`PENDING -> SETTLEMENT_PENDING -> CHARGED | SETTLEMENT_REVIEW_REQUIRED`；失败退款走 `REFUND_PENDING -> REFUNDED | REFUND_REVIEW_REQUIRED`，无需扣费则为 `NOT_CHARGED`。pending 字段保存预期 quota/人民币但不冒充已扣金额。

成本状态：

`PENDING -> PARTIAL -> CONFIRMED`，异常进入 `RECONCILIATION_REQUIRED`；可重试，不能靠删除记录恢复。

同步状态：

`PENDING -> PROCESSING -> DONE`；失败回 `PENDING` 并递增 attempts、设置 `next_attempt_at`。inbox 以 `event_id` 唯一去重；同订单只接受更大的 `settlement_revision`，旧事件记为已处理但不回滚镜像。

失败退款采用净额口径：客户扣费归零或扣除已退金额；已实际发生的上游成本保留，因此利润可以为负数。

## 4. AIPDD 数据字段

`finance_order_ledger`：订单身份、AIPDD user/api-key/instance、NewAPI user/token、业务来源与来源 ID、模型快照、订单/成本状态、结算修订号、客户扣费 AWCoin/人民币、实际支出 AWCoin/人民币、基础模型费、AIPDD 模型费、利润、定价证据、发生/结算时间。

`finance_order_attempt`：同一 platform order 下每次上游尝试的 attempt ID、独立 source/task/usage、状态及金额快照。订单总额按 attempt 增量聚合，避免“第一次已产生上游成本但响应丢失，第二次重试成功”时漏掉第一次成本。

`finance_order_movement`：追加式金额变动；`idempotency_key` 唯一，保存 component、AWCoin/RMB delta、证据与修订号。

`finance_settlement_outbox`：和订单更新处于同一数据库事务；全局递增 `sequence`、事件 UUID、订单修订号、不可变 JSON 快照。按已认证 api-key + instance 拉取。

## 5. NewAPI 数据字段

`aipdd_finance_order`：本地 order/attempt/request、user/token/channel、模型、NewAPI 客户实际 quota/人民币、AIPDD 实际向该中转站扣除的 AWCoin/人民币、AIPDD 内部基础模型费/AIPDD 模型费/实际支出镜像、利润、状态、上游修订号与证据快照。唯一范围为 `(channel_id, instance_id, platform_order_id)`，以保留跨 AIPDD 账号重试产生的独立成本。NewAPI 利润按“NewAPI 客户扣费人民币 − AIPDD 向中转站实际扣费人民币”计算；不得误用 AIPDD 内部支出代替中转站源头成本。

`aipdd_finance_movement`：本地扣费/退款与上游镜像变动，幂等键唯一。

`aipdd_finance_inbox`：AIPDD `event_id` 唯一，保存 sequence/revision/payload/处理结果。

`aipdd_finance_outbox`：本地订单生命周期与刷新命令；可租约重试。

`aipdd_finance_cursor`：每个 channel + instance 的最后连续上游 sequence。游标与 inbox、镜像应用在同一事务中提交。

## 6. 跨系统 API v1

认证沿用 AIPDD 渠道的 `X-API-Key`，并要求 `X-AIPDD-Instance-ID`。

- `GET /api/finance/v1/settlements/{platformOrderId}`：实时查询单笔最新快照；只能访问当前 key + instance。
- `GET /api/finance/v1/settlement-events?after_sequence=N&limit=200`：按 sequence 升序恢复事件，limit 1..500。

NewAPI 第一阶段另提供管理员只读核查接口 `GET /api/aipdd-finance/orders`，支持时间、user、token、channel（AIPDD API Key 的本地非敏感身份）和 platform order ID 筛选；完整利润报表与 XLSX 下载属于后续阶段。

响应 envelope：`schema_version=1`、`event_id`、`sequence`、`event_type`、`settlement_revision`、`occurred_at`、`order`。金额字段均为十进制字符串或 null。

示例（单笔查询不返回 `event_id/sequence`，增量事件返回）：

```json
{
  "schema_version": 1,
  "event_id": "b238b8be-8fc3-4e54-a837-a6828640fcd4",
  "sequence": 1024,
  "event_type": "ORDER_SETTLED",
  "settlement_revision": 3,
  "occurred_at": "2026-08-09T00:00:00.000000",
  "order": {
    "platform_order_id": "req-...",
    "latest_attempt_id": "req-...:1:8",
    "instance_id": "...",
    "order_status": "SUCCEEDED",
    "cost_status": "CONFIRMED",
    "customer_charge_awcoin": "1250",
    "customer_charge_rmb": "2.500000",
    "base_model_cost_rmb": "1.100000",
    "aipdd_model_cost_rmb": "0.300000",
    "actual_spend_rmb": "1.400000",
    "profit_rmb": "1.100000"
  }
}
```

## 7. 迁移与发布顺序

1. 先发布 AIPDD 表、幂等服务与只读 query/pull API；不改变旧日志和旧报表。
2. 发布 NewAPI 镜像、inbox/outbox/cursor 和后台恢复 worker，此时尚可不发送订单头。
3. 为每个 NewAPI 实例配置 `AIPDD_INSTANCE_ID`，校验其归属后开启订单头。
4. 开启同步/异步逐单写入与实时刷新；观察 pending/retry/reconciliation 指标。
5. 后续阶段执行 180 天回填，标记 `EXACT/RULE_DERIVED/UNVERIFIABLE`；不把回填猜测伪装成精确事实。

回滚时只关闭订单头与 worker，不删除任何 ledger、movement、inbox/outbox 数据。

## 8. 合同补充约束（后续月结阶段）

- 预充值结算，单次最低充值人民币 5000 元，余额不足需预警。
- 每月 10 个中国法定工作日内出具上月完整模型消耗报表，包含品类金额、折扣和实际扣减。
- 以 `Asia/Shanghai` 和国务院公布的调休工作日历计算；以“实际收到报表”的人工确认及证据开始 3 个工作日核对期。
- 月结同时保留可重算实时视图与不可变关账快照；迟到数据进入当期调整并关联原订单、原账期。

## 9. 第一阶段验收

- 同步和异步 AIPDD 调用均带统一订单/尝试身份且无原始 key。
- AIPDD 逐单 ledger 更新时事务性写 outbox；重复请求/重复事件不重复计费。
- NewAPI 结算后冻结客户扣费人民币换算快照，并能接收更高 revision 的 AIPDD 成本。
- 实时失败后可由 pull worker 从 cursor 恢复；重复拉取由 inbox 幂等吸收。
- 所有迁移满足各自数据库支持范围；NewAPI 同时兼容 SQLite、MySQL、PostgreSQL。
- 第一阶段要求一个 AIPDD channel 只配置一个上游 API Key；multi-key channel 因无法在长期恢复时稳定还原所属 key 而被 worker 明确拒绝，不会把事件错误归到另一把 key。

## 10. 实施偏离记录

实施过程中只在这里追加偏离，不静默改变契约。

- `2026-08-08 / AIPDD channel multi-key`：第一阶段恢复游标按 channel + instance 建立，而 AIPDD 事件所有权按 API Key 隔离。为避免 key 轮换后把历史事件归错账号，worker 对 multi-key AIPDD channel 明确报错并跳过；部署时必须一把 AIPDD Key 对应一个 channel。后续若需要 multi-key，必须新增不可逆 key identity/fingerprint 维度并迁移游标，不能保存原始 key。
- `2026-08-08 / 推理响应后的进程崩溃窗口`：现有推理 usage 由虚拟线程异步落库。现在会在派发前先持久化 finance order/outbox，并把订单 ID 写入 usage 以供 30 秒恢复任务重连；但若进程在响应完成后、usage 尚未落库前崩溃，已不存在可精确重建的 token 来源。15 分钟后该订单会显式进入 `RECONCILIATION_REQUIRED`，后续回填必须标记 `UNVERIFIABLE`，不会猜测成本。
- `2026-08-08 / Seedance 外部提交原子性`：无法让火山外部任务创建、AIPDD task 表和本地 finance ledger 共用一个数据库事务。实现选择“先写 finance order/outbox，再调用上游；task 持久化时保存订单身份；30 秒恢复任务重连”。若进程恰好在上游接受任务后、task 行写入前崩溃，只能依据上游侧证据人工/后续账单回填，原因是外部接口没有参与本地两阶段提交。
- `2026-08-08 / 第一阶段成本覆盖`：本阶段完整建立金额字段、状态、修订与传输链路；Seedance 可使用现有冻结成本快照，推理自有模型和其它合成模型的最终实际支出仍保持 `PENDING`，待第二阶段接计算成本与火山账单差异分摊。pending 不计入“已确认利润”。
- `2026-08-08 / 报表交付格式`：第一阶段只增加管理员逐单核查 API，不提前实现合同月报与 XLSX。完整 XLSX、关账快照、3 个工作日异议窗口和 180 天回填按既定后续阶段实施；不会以 CSV 代替。
- `2026-08-09 / NewAPI 钱包扣费/退款的模糊失败`：现有钱包额度变更是非幂等的 `quota +=/-= N`，没有 request ID 级唯一业务键；数据库返回错误时无法判断写入是否已生效，盲目自动重试可能造成多扣或多退。实现会在资金变更前先落 `SETTLEMENT_PENDING` 及预期金额，确认成功后才写 `CHARGED`；退款先写 `REFUND_PENDING`，成功后写 `REFUNDED`。15 分钟仍未确认分别转 `SETTLEMENT_REVIEW_REQUIRED` / `REFUND_REVIEW_REQUIRED`，不伪报实际金额。订阅退款沿用现有 request ID 幂等重试。后续要实现钱包自动恢复，必须先把钱包变更改造成带唯一业务键的账本操作。
