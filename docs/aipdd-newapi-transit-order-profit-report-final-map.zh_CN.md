# AIPDD × NewAPI 中转订单与 AIPDD 利润报表最终地图

> 状态：第一版代码实施完成；2026-08-20 已按手工站点严格身份契约更新
> 确认日期：2026-08-12  
> 实施日期：2026-08-11  
> 适用范围：AIPDD 利润报表，以及为该报表必需的 NewAPI → AIPDD 订单映射和结算回传  
> 执行方案：[AIPDD × NewAPI 中转订单与利润报表执行方案](./aipdd-newapi-transit-order-profit-report-execution-plan.zh_CN.md)  
> 说明：本文既是最终产品口径，也是本轮代码实施结果地图。若旧财务/对账文档与本文冲突，以本文为准。

## 1. 目标

只为实际经过 AIPDD 渠道执行的模型请求建立一条统一、可核对的中转订单链路：

1. NewAPI 在请求 AIPDD 前创建统一订单号和本地订单记录。
2. AIPDD 接收统一订单号，为该任务创建一条中转订单记录。
3. AIPDD 记录自己的实际售价、任务成本和利润，金额统一保存积分与人民币结果。
4. AIPDD 的售价就是 NewAPI 的源头成本。
5. AIPDD 只向 NewAPI 回传售价，不回传 AIPDD 内部成本和利润。
6. 管理员在 AIPDD 的一个简单报表中查看汇总、明细、待结算订单并导出 CSV。

第一版追求单订单、单账本、单一利润口径，不建设通用财务平台。

## 2. 最终业务链路

```mermaid
flowchart LR
    Client["NewAPI 客户请求"] --> NewOrder["NewAPI 创建统一订单号和本地记录"]
    NewOrder --> AipddRequest["携带统一订单号请求 AIPDD"]
    AipddRequest --> TransitOrder["AIPDD 幂等创建中转订单"]
    TransitOrder --> Route{"实际执行路径"}
    Route -->|"自有算力或设备"| Executor["执行者完成任务并领取奖励"]
    Route -->|"超值模型"| Offering["使用命中的超值模型成本"]
    Route -->|"上游号池"| Pool["使用最终成功渠道成本"]
    Executor --> Settle["冻结售价、成本、利润和结算时间"]
    Offering --> Settle
    Pool --> Settle
    Settle --> AipddReport["AIPDD 中转订单利润报表"]
    Settle --> SourceCost["仅将 AIPDD 售价回传为 NewAPI 源头成本"]
```

## 3. 统一订单规则

### 3.1 创建范围

- 仅当 NewAPI 实际选中 AIPDD 渠道时创建统一订单。
- AIPDD 只记录携带合法统一订单号、确实来自 NewAPI 的模型请求。
- AIPDD 原生直调、充值、提现、存储等业务不进入本报表。
- Shared Task、超值模型/Seedance、OpenAI 兼容 Chat（流式与非流式）全部纳入第一版。
- AIPDD 多 Key/号池场景必须按本次请求最终实际使用的 API Key 归属订单，不能因为渠道配置了多个 Key 而跳过记账。

### 3.2 幂等与重试

- 统一订单号由 NewAPI 在第一次请求 AIPDD 前生成。
- AIPDD 以 `site_id + 统一订单号` 作为跨系统幂等标识；同一站点的同一订单号重复到达时更新原订单，不新建重复订单。
- NewAPI 的一次逻辑订单在重试时保持统一订单号不变；每次真实上游尝试使用新的 `X-AIPDD-Attempt-ID`，同一次尝试的网络重发复用原 attempt ID。
- AIPDD 内部重试、设备调度重试和上游号池故障切换仍属于同一笔订单，但可通过 attempt 记录保留实际执行与成本证据。
- NewAPI 客户重新发起的一次独立请求应生成新的统一订单号。
- NewAPI 本地镜像只保存最新 attempt；AIPDD 的统一财务账按 attempt 保存来源证据，但钱包扣款仍只能单写一次。

## 4. AIPDD 中转订单数据

每笔 AIPDD 中转订单至少保存以下字段：

| 字段 | 口径 |
|---|---|
| 统一订单号 | NewAPI 创建的跨系统订单号，唯一 |
| AIPDD 任务/请求 ID | AIPDD 内部任务或推理请求标识 |
| AIPDD 用户 ID | 实际使用 API Key 的所属用户 |
| API Key ID | 仅保存内部 ID，不保存完整 Key |
| 模型 | 实际执行的模型 |
| 订单状态 | 处理中、成本待结算、已结算、失败、已退款 |
| 售价积分 | AIPDD 对 NewAPI API Key 的最终净扣费积分 |
| 售价人民币 | 上述最终净扣费对应的人民币结果 |
| 成本积分 | 按实际执行路径得到的任务成本积分 |
| 成本人民币 | 上述任务成本对应的人民币结果 |
| 利润积分 | 售价积分减成本积分 |
| 利润人民币 | 售价人民币减成本人民币 |
| 创建时间 | AIPDD 第一次接收该统一订单的时间 |
| 结算时间 | 售价和成本都最终确定的时间 |

### 4.1 汇率字段

- 订单、接口、页面和 CSV 都不增加汇率字段。
- 结算时沿用 AIPDD 当时已有的积分/人民币换算规则，直接冻结积分和人民币结果。
- 后续汇率或价格配置变化不能改写历史订单金额。

## 5. 售价、成本和利润口径

### 5.1 售价

- AIPDD 售价等于该任务对 NewAPI 所使用 AIPDD API Key 的最终净扣费。
- 全额退款后售价为 0。
- 部分退款后售价为扣费减退款后的净额。
- 售价一旦最终确定，即可回传给 NewAPI；不必等待 AIPDD 内部成本完成结算。

### 5.2 成本来源

成本按实际执行路径确定，不再额外接入逐单上游账单系统：

| 实际执行路径 | 成本口径 |
|---|---|
| 自有算力/设备推理 | 实际确认发给执行者的奖励积分及对应人民币 |
| 超值模型/Seedance | 命中的超值模型成本配置 × 最终任务规格、时长等实际用量 |
| 上游号池 | 最终成功渠道的输入、输出、缓存成本配置 × 实际 Token 用量 |

补充规则：

- 发生上游故障切换时，只采用最终成功执行渠道的成本配置。
- 配置型成本应在订单执行时冻结对应成本快照，并在最终用量产生后计算订单成本。
- 成本配置后续被修改，不得重新计算历史订单。
- 如果模型或渠道没有合法成本配置，则不得向 NewAPI 发布或执行该模型，必须返回明确的“成本未配置”错误。
- 缺失成本不得默认为 0。
- 任务失败或退款后，如果 AIPDD 已真实发生执行成本，成本仍按实际发生值保留，允许出现负利润。

### 5.3 延迟奖励

- 部分自有算力或 Chat 设备奖励可能因低信誉等规则延迟确认。
- 客户扣费已完成但执行者奖励尚未真正发放时，订单进入“成本待结算”。
- 不把应付奖励伪装成实际成本，也不提前计算最终利润。
- 奖励实际确认发放后，再写入成本、利润和结算时间。

### 5.4 利润

```text
AIPDD 利润积分 = AIPDD 售价积分 - AIPDD 成本积分
AIPDD 利润人民币 = AIPDD 售价人民币 - AIPDD 成本人民币
```

## 6. AIPDD 返回给 NewAPI 的数据

AIPDD 只返回自己的最终售价，因为该售价就是 NewAPI 的源头成本：

```json
{
  "platform_order_id": "统一订单号",
  "settlement": {
    "status": "settled",
    "charged_points": 1000,
    "charged_rmb": "2.000000"
  }
}
```

明确不返回：

- AIPDD 成本积分或成本人民币；
- AIPDD 利润；
- 执行者奖励；
- 超值模型成本组成；
- 上游号池成本配置；
- 汇率。

### 6.1 回传方式

- Shared Task、超值模型/Seedance 等异步任务：在任务最终查询响应中附带 `settlement`。
- OpenAI 兼容 Chat（流式和非流式）：请求结束后，NewAPI 按统一订单号调用一次极简结算查询；未就绪或临时失败时只做有限次数的直接重试。
- Chat 结算查询只返回该统一订单的售价结算，不返回 AIPDD 内部成本和利润。
- 不为此建设事件游标、消息收件箱/发件箱、事件投影或通用财务同步 Worker。

### 6.2 NewAPI 本地记录

- NewAPI 在调用前先创建 `PENDING` 订单。
- AIPDD 返回最终售价后，NewAPI 将售价人民币写为该订单的源头成本；售价积分可作为核对字段保存。
- AIPDD 内部成本是否仍在待结算，不影响 NewAPI 已获得并保存 AIPDD 售价。
- 请求在 AIPDD 接收前失败时，NewAPI 应把本地订单标为失败，源头成本为 0。
- AIPDD 已产生净扣费时，以 AIPDD 返回的最终净扣费为准。

## 7. AIPDD 管理报表

### 7.1 页面范围与权限

- 仅管理员可访问。
- 直接改造现有“中转站订单记录”页面，不新增第三套财务页面。
- 页面只查询本文定义的 AIPDD 中转订单表。

### 7.2 查询条件

- 结算时间范围；
- AIPDD 用户 ID；
- 完整 API Key。

完整 API Key 查询规则：

1. 前端允许管理员粘贴完整 Key 作为查询条件。
2. 后端只在请求内对完整 Key 计算哈希，并查出对应 `api_key_id`。
3. 中转订单只保存 `api_key_id`。
4. 完整 Key 不写入订单、不写入日志、不回显、不进入 CSV。
5. 页面和 CSV 只展示 API Key 名称与脱敏前缀。

### 7.3 汇总

按结算时间范围统计已结算订单：

- 已结算订单数；
- 总售价积分、总售价人民币；
- 总成本积分、总成本人民币；
- 总利润积分、总利润人民币。

待结算订单不进入上述利润汇总。同一页面额外显示：

- 待结算订单数量；
- 待结算订单明细。

### 7.4 订单明细

明细至少展示：

- 统一订单号；
- AIPDD 任务/请求 ID；
- 用户 ID/用户名；
- API Key 名称与脱敏前缀；
- 模型；
- 状态；
- 售价积分/人民币；
- 成本积分/人民币；
- 利润积分/人民币；
- 创建时间；
- 结算时间。

### 7.5 导出

- 导出格式统一为 CSV。
- CSV 使用与当前查询相同的过滤条件和金额口径。
- CSV 不包含完整 API Key、汇率、内部财务事件、重试事件或成本修订记录。

## 8. 单一状态模型

第一版只保留业务需要的简单状态：

```mermaid
stateDiagram-v2
    [*] --> PROCESSING
    PROCESSING --> COST_PENDING: "售价已确定，实际成本未确认"
    PROCESSING --> SETTLED: "售价和成本同时确定"
    COST_PENDING --> SETTLED: "执行者奖励实际发放"
    PROCESSING --> FAILED: "失败且无净扣费"
    PROCESSING --> REFUNDED: "退款完成"
    COST_PENDING --> REFUNDED: "退款完成"
    SETTLED --> REFUNDED: "结算后退款"
```

退款后重新计算最终净售价和利润即可，不引入通用 revision/event 体系。

## 9. 明确不做

第一版不做以下内容：

- 不把 AIPDD 内部成本返回给 NewAPI；
- 不拆分“基础模型费用”和“AIPDD 模型费用”；
- 不接入火山或其他上游逐单真实账单查询；
- 不建立通用财务业务事件、成本确认、人工修订和凭证体系；
- 不建立双账、投影账或把新订单再次投影到旧报表；
- 不建立跨系统事件游标、收件箱、发件箱和毒消息处理；
- 不建立异步 XLSX 导出任务，第一版只导出 CSV；
- 不展示或保存汇率字段；
- 不保存、回显或导出完整 API Key；
- 不把 AIPDD 原生直调、充值、提现、存储等业务混入模型订单利润报表；
- 不在本阶段扩展 NewAPI 客户利润报表，NewAPI 侧只完成源头成本订单记录。

## 10. 旧系统处置

旧财务系统与逐单台账原本存在并行写入、投影和重复统计风险。本轮只替换“NewAPI 来源模型订单”链路，不破坏充值、提现、存储等独立财务业务：

1. NewAPI 已删除旧模型财务 worker、movement/inbox/outbox/cursor/export、管理员 API 和页面。
2. AIPDD 主业务路径已停止写入旧模型事件账和旧逐单账；旧逐单服务与结算接口只在显式 `legacy-aipdd-finance` Profile 下存在，默认不加载。
3. 旧模型来源已从自动恢复和新建手工回填范围中移除，不能再产生新的旧模型财务记录。
4. 充值、提现、存储等非模型财务记录继续按原业务运行，不进入本中转订单利润报表。
5. 已执行过的历史迁移和历史表不回改、不自动删除；物理归档/删表必须在生产维护窗口备份后另行执行。
6. 新利润报表只查询 `new_api_transit_order`，历史数据不会混入或重复统计。

## 11. 四象限最终地图

### 11.1 已知且已确认

- AIPDD 售价等于 NewAPI 源头成本。
- AIPDD 内部成本和利润不返回 NewAPI。
- 所有 AIPDD 模型执行路径均纳入第一版。
- 成本直接复用自有算力奖励、超值模型配置和上游号池配置。
- 订单保存积分和人民币，不保存汇率。
- 报表按结算时间统计，待结算订单单独展示。
- 管理员可以通过完整 API Key 安全查询，但系统不保存完整 Key。
- 导出使用 CSV。

### 11.2 原本未知、现已决策

- Chat 无法依赖任务最终响应：改为请求结束后按订单号做极简查询。
- 执行者奖励延迟：实际发放前保持成本待结算。
- 缺少成本配置：禁止向 NewAPI 提供该模型，不按 0 处理。
- 多 Key 渠道：按每次请求实际选中的 Key 建账，不能跳过。
- 旧复杂财务系统：先完整归档，再直接替换和清理。

### 11.3 代码中已有、应直接复用

- AIPDD API Key 已有哈希查找机制，可安全支持管理员粘贴完整 Key 查询。
- 超值模型管理已有人民币/积分成本及成本矩阵。
- 上游号池已有输入、输出、缓存三档成本配置。
- 自有算力和推理设备已有奖励结算记录。
- NewAPI 与 AIPDD 已具备跨系统统一订单头和三类执行入口的基础代码。

### 11.4 已发现并纳入防护的隐患

- 当前旧账与逐单账并行，可能重复统计。
- 当前部分 Seedance 成本被当成已确认实际成本，语义不准确。
- 当前 NewAPI 多 Key AIPDD 渠道会跳过财务记录。
- 当前 Chat 在流结束时尚未完成最终扣费，不能直接在 SSE 中可靠返回结算。
- 当前推理奖励可能延迟，任务成功不等于实际成本已经支付。
- 当前系统使用 XLSX，与本轮确认的 CSV 不一致。
- 当前跨仓协议只有静态代码对齐，缺少真正的跨仓端到端合同测试。

## 12. 第一版验收标准

至少覆盖以下验收场景：

1. NewAPI 调用 AIPDD 前成功创建统一订单，AIPDD 只生成一条对应记录。
2. 同一统一订单号重复请求不会生成重复订单。
3. Shared Task 成功后，售价正确回写 NewAPI，AIPDD 成本和利润正确。
4. 超值模型按实际规格/时长和冻结成本计算。
5. 上游号池按最终成功渠道及实际 Token 计算，不使用失败渠道成本。
6. 流式和非流式 Chat 均能通过极简查询把最终售价回写 NewAPI。
7. 自有设备奖励延迟时订单进入成本待结算，实际发放后才进入利润汇总。
8. 缺少成本配置时请求在执行前失败，且不产生虚假零成本利润。
9. 全额退款、部分退款和已发生成本的失败订单均按最终净额计算。
10. 多 Key AIPDD 渠道按实际使用 Key 记录订单，不漏单。
11. 管理员按结算时间、用户 ID、完整 API Key 均能正确查询。
12. 完整 API Key 不出现在数据库订单、响应、日志、页面和 CSV 中。
13. 待结算订单单独展示且不进入已结算利润汇总。
14. CSV 金额与页面汇总、订单明细一致。
15. 旧财务系统停止双写，旧报表不会与新订单重复统计。
16. NewAPI 与 AIPDD 增加跨仓端到端合同测试，验证字段、状态、幂等和退款行为。

## 13. 后续扩展边界

以下内容等第一版稳定后再单独立项：

- NewAPI 客户售价、源头成本和 NewAPI 利润报表；
- 上游真实账单与配置成本差异对账；
- 成本人工调整、差异审批与审计凭证；
- 更复杂的合同价、账期和月度结算；
- XLSX、多工作表或财务系统集成导出。

这些扩展不得改变第一版的核心关系：**AIPDD 售价 = NewAPI 源头成本，AIPDD 内部成本与利润只留在 AIPDD。**

## 14. 2026-08-11 实施落点

### 14.1 NewAPI

- 新增单表 `aipdd_transit_order`，只保存统一订单、本地客户归属、本地扣费、AIPDD 售价积分/人民币、创建与结算时间。
- OpenAI Chat、Seedance、Shared Task 创建请求发送 `X-AIPDD-Instance-ID`、`X-AIPDD-Order-ID`、`X-AIPDD-Attempt-ID`，并附带仅含数字的 NewAPI 用户/Token 审计头；未登记站点身份时这类头**一个都不发**，避免上游因未知 Instance 拒绝算力。
- `AIPDD_INSTANCE_ID` 固定为手工登记站点的 `externalInstanceId`；禁止再从 API Key 派生，Key 轮换时 Instance 与历史订单身份保持不变。
- 该变量未配置时**不阻断中转，也不再跳过本地账务**：NewAPI 记录一次告警，仍然建单、仍然记录客户扣费与成本预估，只是不向上游发送任何 `X-AIPDD-*` 订单头、也不读取 settlement（无站点身份时上游按设计返回 400/403）。对账页面此时有订单与预估，仅「实际源头成本」为空。详见第 18 节。
- multi-key 渠道记录本次实际选中的 Key 槽位，不再跳过订单。
- 异步任务从终态响应读取售价；Chat 通过单订单接口做有限直接重试。
- 删除旧复杂财务模型、worker、事件同步 API、XLSX 导出和管理页面，并移除 Excelize 依赖。
- NewAPI 已提供简版 AIPDD 中转订单记录页面；该页面只展示 NewAPI 客户扣费与 AIPDD 源头成本，不展示 AIPDD 内部成本和利润。

### 14.2 AIPDD

- 新增 `new_api_transit_order` 与 V200/V201 向前迁移；V201 可修复已记录 V200 但缺表的环境。
- Shared Task、Seedance、Chat 三类入口幂等创建并关联同一统一订单。
- 自有算力使用确认奖励，超值模型使用已有冻结成本，号池使用最终成功渠道三档 Token 成本。
- 缺少合法成本时在请求上游前返回 `cost_not_configured`，不生成零成本利润。
- 售价确定但奖励未确认时进入 `COST_PENDING`；成本确认后再计算利润和结算时间。
- 单订单结算接口和异步任务响应只暴露售价积分、售价人民币，不暴露 AIPDD 成本与利润。
- 原“中转站订单记录”页面已改为已结算汇总、待结算明细、完整 Key 安全查询和同步 CSV。

### 14.3 已完成验证

- NewAPI：`go test ./...` 全仓通过。
- NewAPI Web：`bun run build` 通过。
- AIPDD Java：中转订单、PostgreSQL SQL 和 Seedance BYOK 成本夹具专项回归共 16 项，16 项通过。
- AIPDD Web：`bun run build` 通过，只有既有的大分块体积警告。
- AIPDD Java 全仓实际执行 579 项；原始全仓结果为 2 项失败、5 项错误、3 项跳过。3 项 Seedance BYOK 错误来自测试夹具缺少新规则要求的成本，补齐成本后已在上述 16 项专项回归中通过。其余失败/错误属于虚拟角色响应契约、LTX 2.3 输入样例、远程测试库残留数据污染和既有调度补偿，不属于中转订单链路；本轮不扩大范围修改这些模块。

### 14.4 上线前环境验收

- 使用真实 NewAPI AIPDD 单 Key与 multi-key 渠道各完成一笔 Chat、Seedance、Shared Task 请求。
- 对照两端数据库确认统一订单一一对应，且 NewAPI 源头成本人民币等于 AIPDD 售价人民币。
- 验证生产管理员页面的完整 Key 查询、待结算转已结算和 CSV 金额。
- 上线前备份旧财务表；本轮代码不会自动删除历史数据。

## 15. 2026-08-12 本地全链路实测记录

### 15.1 测试环境

- NewAPI 本地服务：`http://127.0.0.1:6070`。
- AIPDD 本地服务：`http://127.0.0.1:1688`。
- AIPDD 管理前端：`http://127.0.0.1:16888`。
- 使用独立测试用户、两个独立 AIPDD API Key 和独立上游号池渠道；验收后已全部禁用，未删除测试订单。
- 为避免成功结果受外部供应商波动影响，除真实上游请求外，增加了一个仅在本机监听的确定性 OpenAI 兼容上游，固定返回实际 Token 用量；验收后已停止并删除。

### 15.2 成功订单金额核对

确定性上游固定返回输入 `100000` Token、输出 `20000` Token。按测试号池冻结的售价和成本配置，单笔订单结果为：

| 位置 | 售价/源头成本 | AIPDD 内部成本 | AIPDD 内部利润 |
|---|---:|---:|---:|
| AIPDD 中转订单 | 360 AWcoin / ¥3.600000 | 240 AWcoin / ¥2.400000 | 120 AWcoin / ¥1.200000 |
| NewAPI 中转订单 | 360 AWcoin / ¥3.600000 | 不接收 | 不接收 |

代表订单：`20260811160524468788300dah61qTF8lWngZoL`。两端统一订单号一致，NewAPI 源头成本人民币与 AIPDD 售价人民币精确相等。

同一订单的极简结算查询连续调用 3 次，均只返回 `status`、`charged_points`、`charged_rmb`，没有返回 AIPDD 成本或利润；AIPDD 钱包扣费流水仍只有 1 条，证明结算查询和扣费幂等。

### 15.3 multi-key 与流式

- multi-key 轮询连续两笔请求分别实际使用 API Key ID `256` 和 `257`，订单号分别为 `20260811160908269487800XiSRSULvaYjppQ1h`、`20260811160923955506700XiSRSULvXDXUvg9S`。
- 两笔订单均在 AIPDD 和 NewAPI 生成唯一记录，均结算为 360 AWcoin / ¥3.600000，未出现 multi-key 漏单。
- 流式代表订单 `20260811161107327238700XiSRSULvYbLcgxla` 返回 3 个 SSE 事件，包含正文、usage 和且仅一个 `[DONE]`；流结束后两端均结算成功，金额与非流式订单一致。

### 15.4 真实外部链路与失败退款

- 真实 Chat 请求曾成功到达外部号池并暴露“只记录号池成本、未按号池售价扣 AIPDD 用户”的缺陷；现已改为按最终实际 usage 同时计算号池售价和成本，并通过上述确定性成功链路复验。
- 修复后的真实 Chat 再测遇到两个外部渠道均 120 秒超时：AIPDD 订单为 `FAILED`，NewAPI 返回 503 且不记源头成本，未产生虚假利润。
- 真实 Seedance 标准与 VIP 请求均到达火山，但测试供应商账号返回 `ModelNotOpen`：AIPDD 记录失败，NewAPI 预扣全额退回。
- Shared Task 当前目录没有可用执行设备：请求在执行前被拒绝，NewAPI 预扣全额退回。
- 因外部账号权限和本地无在线算力设备，本轮没有获得真实 Seedance 成功单和真实 Shared Task 成功单；其成本算法、门禁、结算和退款由 Java 专项/集成测试覆盖，不能把外部失败写成成功验证。

### 15.5 管理报表与 CSV

- AIPDD “中转站订单记录”页面实测展示已结算、失败和待结算状态，显示统一订单号、用户、脱敏 Key、模型、来源、售价、成本、利润、创建时间和结算时间。
- 5 笔已结算测试单页面汇总：总售价 1440 AWcoin / ¥14.40，总成本 960 AWcoin / ¥9.60，总利润 480 AWcoin / ¥4.80。
- 使用完整 API Key 查询后输入框被清空，结果仍正确过滤；完整 Key 未出现在页面正文和响应展示中。
- CSV 实测 HTTP 200、UTF-8 BOM、16 列，包含售价/成本/利润的 AWcoin 与人民币、创建时间和结算时间；不包含汇率和完整 API Key。
- CSV 文件名按 `Asia/Shanghai` 日期生成，修复了上海时区午夜后仍使用前一天 UTC 日期的问题。

### 15.6 收尾状态

- 两个测试 API Key、测试用户和临时上游号池渠道均已禁用。
- 修复前遗留的一条测试 `PROCESSING` 订单已按实际结果关闭为 `FAILED`，不再污染待结算数量。
- NewAPI AIPDD 渠道已恢复生产地址、生产单 Key 和非 multi-key 配置。
- AIPDD 与 NewAPI 已使用最新代码重新启动；`/actuator/health` 与 `/api/status` 均返回 HTTP 200。
- 全量与专项日志统一保存在 `E:\AIPDD\runtime-e2e-logs`，便于后续追溯。

## 16. 2026-08-12 NewAPI 简版中转订单记录页面

NewAPI 已提供简版 AIPDD 中转订单记录页面；该页面只展示 NewAPI 客户扣费与 AIPDD 源头成本，不展示 AIPDD 内部成本和利润。

### 16.1 范围

- 管理员专用页面路径：`/aipdd-transit-orders`（2026-10-10 起已从侧边栏菜单移除，仅管理员直接输入 URL 可访问；路由有 `ROLE.ADMIN` 守卫）。
- 管理员查询接口：`GET /api/aipdd-transit-orders`（`middleware.AdminAuth()`）。
- 只查询本地表 `aipdd_transit_order`，不恢复旧 finance worker、movement、event、cursor、inbox/outbox、XLSX 或复杂对账系统。
- 页面支持创建时间范围、统一订单号、用户 ID、Token ID、渠道 ID、模型、状态过滤，以及分页、刷新和清空筛选。
- 空的源头成本字段显示为“待结算”，不显示为 0。（2026-10-10 起该列改为“成本预估”，见第 17 节）

### 16.2 金额口径

| 展示字段 | 来源 |
|---|---|
| NewAPI 客户扣费 quota | `customer_charge_quota` |
| NewAPI 客户扣费人民币 | `customer_charge_rmb_mic / 1_000_000` |
| AIPDD 源头成本 AWcoin | `source_charge_awcoin`（NULL = 待结算） |
| AIPDD 源头成本人民币 | `source_charge_rmb_mic / 1_000_000`（NULL = 待结算） |

不接收、不展示：AIPDD 内部成本、内部利润、执行者奖励、上游号池成本组成、汇率。

### 16.3 本地验收摘要（2026-08-12）

- `GET /api/aipdd-transit-orders` 在管理员登录后返回本地库现有 **11** 条订单（SETTLED 5、REFUNDED 3、FAILED 2、PENDING 1）。
- `source_charge_*` 为 NULL 的订单在接口中返回 `null`，前端展示为“待结算”。
- `go test ./...` 与 `web/default` 的 `bun run typecheck` / `bun run build` 均通过。

## 17. 2026-10-10 成本预估：没有站点结算也能反推源头成本

第 16 节的 `source_charge_*` 只有在 AIPDD 侧完成交付站点绑定后才回传；未绑定的部署里 settlement 请求返回 403，订单记录页的成本列永远是“待结算”，无法做任何毛利判断。为此新增**成本预估**：用 AIPDD 公开发布的价目表（catalog 售价 + 官方 `rmbPerAwcoin` 汇率）乘以本单实际计量消耗，反推 NewAPI 这一单在 AIPDD 侧的应付成本。

### 17.1 存储与口径

| 列 | 含义 | 写入时机 |
|---|---|---|
| `source_charge_awcoin` / `source_charge_rmb_mic` | AIPDD 结算回执的**实际**源头成本 | 仅 settlement 拉回后 |
| `estimated_source_awcoin` / `estimated_source_rmb_mic` | 按公开价目表**反推**的成本预估 | 本地扣费成功（`CHARGED` 且 quota>0）时 |

- 预估与实际分列存放：真实结算**永不覆盖**预估，预估也**永不覆盖**实际。
- 只有成功扣费的订单才产生预估；`NOT_CHARGED` / `REFUNDED` / 上游失败的单不写预估（无消耗证据就不猜）。
- 换渠道重试重开同一订单时，预估与实际一并置 NULL，避免留下上一次尝试的成本痕迹。
- 价目来源是 `activateAIPDDCatalog` 在 catalog 同步/快照恢复时发布的运行时价表；没有价表或模型不在价表内时预估留空，账务链路降级放行，不影响中转。

### 17.2 反推方式

- **按 token 计费模型**：`prompt/cached/completion` 分别乘公开档位单价（cached 走 cache-read 档，不重复计入 prompt 档），除以 1,000,000 得 AWcoin，再乘 `rmbPerAwcoin` 得人民币微元。
- **按量计费模型（张/秒/次）**：单价取 catalog 能力价（视频按秒价按分辨率取档，缺分辨率时取已发布最便宜档），数量优先用请求时冻结的计量数量；没有冻结数量时用「本地扣费 quota ÷（本地零售单价 × QuotaPerUnit × 分组倍率）」把计费数量反除回来——数量在分子分母同时出现，因此时长类订单同样可估。
- 模型名优先取上游模型名（映射部署下价目表按 AIPDD 模型 ID 定价），回退客户请求模型名。

### 17.3 前端

订单记录页成本列改名为**成本预估**：有实际结算时显示实际值并打「实际」标记；否则显示预估值；两者都没有时显示「无预估」（不再把 NULL 显示成 0，也不再把未结算笼统写成待结算）。

### 17.4 本机验收（2026-10-10）

- 隔离副本库 + 本地假上游跑通真实中转链路：`ap-glm-5.2`，usage `prompt=800 / cached=200 / completion=500`，catalog 单价 `prompt 80000、completion 280000、cache-read 20000` AWcoin/百万，`rmbPerAwcoin=0.0001`。
- 落库结果：`estimated_source_awcoin=192`、`estimated_source_rmb_mic=19200`（手算 600×80000 + 500×280000 + 200×20000 = 192,000,000 ÷ 1e6 = 192 AWcoin，即 ¥0.0192），`source_charge_*` 保持 NULL（该 Key 未绑站点，结算拉不回）。
- 同单 `customer_charge_quota=1300`、`customer_charge_rmb_mic=18980`（¥0.01898）→ 售价低于反推成本，说明该模型的本地倍率需要在定价页复核。
- `go test ./service/... ./model/... ./controller/... ./constant/... ./pkg/aipddcatalog/...`、`bun run typecheck`、`bun run build`、`go build -o main.exe .` 全部通过；上游返回失败时订单记为 `FAILED` 且不写预估（无消耗证据就不猜）。

## 18. 2026-10-10 长期不绑定交付站点：预估成为唯一成本口径

### 18.1 决策

本部署**不准备**在 AIPDD 侧把 API Key 绑定到交付站点，因此永远拿不到 `externalInstanceId`，`AIPDD_INSTANCE_ID` 会长期保持未配置。对 `https://api.aipdd.work` 做只读/最小消耗探测（2026-10-10，`ap-glm-5.2`，`max_tokens=1`）实测：

| 请求 | 结果 |
|---|---|
| Chat 不带头之外的账务信息 | `HTTP 200`，正常返回内容 |
| Chat 只带未登记的 `X-AIPDD-Instance-ID` | `HTTP 400`，`"X-AIPDD-Order-ID is required"` |
| Chat 带完整 `X-AIPDD-Instance-ID/Order-ID/Attempt-ID`（未登记身份） | `HTTP 403`，`"The API key is not bound to a delivery site"` —— **算力被拒**，不只是账务 |
| settlement 接口不带 `X-AIPDD-Instance-ID` | `HTTP 400`，`"X-AIPDD-Instance-ID is required for finance accounting"` |
| settlement 接口带未登记 UUID | `HTTP 403`，同上 |

结论：账务头一旦发出就必须配一个 AIPDD 认识的身份，否则连算力一起被 403 拒掉；因此未绑定站点的部署**一个 `X-AIPDD-*` 头都不能发**。同时 `source_charge_*`（实际源头成本）在本部署永久为空，成本口径只能依靠第 17 节的**反推预估**。

### 18.2 改造（解耦预估与站点身份）

改造前预估被卡在站点身份之后：建单要求 UUID 格式的 `AIPDD_INSTANCE_ID`，未配置就不建单，于是预估列在生产上永远是空的——新功能是死代码。改造后：

- `resolveAIPDDFinanceInstanceID`：未配置返回空串且**不报错**（受支持的稳态）；只有格式非法（非 UUID）才返回 error，并降级为「无站点身份」而不是「无账务」。
- `PrepareAIPDDFinanceAttempt`：无论有无站点身份都建单、都保留 `info.AIPDDFinance`（`InstanceID` 为空），告警仍只打一次。
- 两个上游出口（`relay/channel/openai/adaptor.go` 的 `setAIPDDFinanceHeaders`、`relay/channel/task/aipdd/adaptor.go` 的 `BuildRequestHeader`）：`InstanceID` 为空时**一个 `X-AIPDD-*` 头都不发**，避免未登记身份把算力一起拒掉。
- `fetchAndApplyAIPDDTransitSettlement`：无站点身份直接返回，不再消耗 4 次退避重试去撞必定 400/403 的端点。

### 18.3 该状态下的账面

| 字段 | 是否有值 | 说明 |
|---|---|---|
| `instance_id` | 空串 | 按设计，非缺陷 |
| `customer_charge_quota` / `customer_charge_rmb_mic` | 有 | 本地真实扣费 |
| `estimated_source_awcoin` / `estimated_source_rmb_mic` | 有（模型在价目表内时） | 唯一成本信号 |
| `source_charge_awcoin` / `source_charge_rmb_mic` | 永久 NULL | 无站点绑定，结算拉不回 |

利润分析只能按「本地扣费 − 反推预估」计算，且预估值依赖 catalog 公开价目表；catalog 里缺该模型时该单预估留空，页面显示「无预估」。

### 18.4 验收

- `TestPrepareAIPDDFinanceAttemptWithoutSiteIdentityStillKeepsLedger`：无站点身份 → 订单行存在、状态 `PENDING`、`instance_id` 为空、`info.AIPDDFinance` 保留。
- `TestRecordAIPDDFinanceSettlementWritesEstimateWithoutSiteIdentity`：无站点身份 → 预估落库（220 AWcoin / 22000 微元），`source_charge_*` 保持 NULL，状态仍为 `PENDING`。
- `TestSetupRequestHeaderOmitsFinanceHeadersWithoutSiteIdentity`：空 `InstanceID` → 五个 `X-AIPDD-*` 头全部不出现，`X-API-Key` 正常。
- `TestResolveAIPDDFinanceInstanceIDEmptyWhenUnconfigured` / `...RejectsMalformedOverride`：未配置＝空串无 error；非法值＝报 `must be a UUID` 并降级。
- `go test ./service/... ./model/... ./relay/channel/openai/... ./relay/channel/task/aipdd/...` 全部通过，`gofmt -l` 无输出。

## 19. 2026-10-10 默认定价：按 catalog 批发价加 10% 毛利

### 19.1 问题

在第 18 节的稳态下，预估成本是唯一成本信号，而本部署的**售价**当时是这样形成的：

- 文本模型（catalog `models`）本地没有任何价格配置——既没有 `ModelRatio` 也没有 `billing_setting.billing_expr` 行，于是命中「未配置价格的模型默认 1 倍率」（第 17 节调整后的默认值）。1 倍率约合 `¥14.6/1M`（`QuotaPerUnit=500000`、`USDExchangeRate=7.3`），而 `ap-glm-5.2` 的输出批发价是 `¥28/1M`、`ap-qwen3.8-max` 是 `¥36/1M`：**卖得越多亏得越多**。
- 任务模型（catalog `capabilities`）由导入器按批发价原样写入（倍率 1.0），毛利恒为 0。

因此需要一条把「批发价 → 默认零售价」的规则固化进目录同步的路径，而不是靠管理员手工逐个填价。

### 19.2 口径

- 唯一乘数：`model.AIPDDRetailMarkup()`，取环境变量 `AIPDD_PRICING_MARKUP`，默认 **1.10**（即加 10% 毛利）。填 `1` 为平价、`0.9` 为亏本销售——**不纠正**，因为亏本是运营决策，偷偷改回去会显示与实际扣费不符的价格。非法值（空、非数字、`<=0`、NaN/Inf）回落默认并打一条 `SysLog`。
- 换算一律走人民币：`零售USD = 批发AWcoin × rmbPerAwcoin × markup ÷ USDExchangeRate`。catalog 自带的 `usdPerAwcoin` 是过时参考价，**不使用**，这样对外人民币价正好等于批发人民币价 × 1.1，不随上游参考汇率漂移。
- 文本模型用 `tiered_expr`（分档表达式）而不是 `ModelRatio`：批发价中 cacheRead/cacheWrite 与 prompt 各不相同（例如 `ap-qwen3.8-max` 的 cacheWrite `¥15/1M` ≠ prompt `¥12/1M`，`ap-deepseek-v4-flash` 的 cacheRead 只有 `¥0.1/1M`），单一倍率无法表达，只有表达式能保留每一类 token 的独立价。系数单位是 **USD/百万 token**。
- 图片的 2K/4K 分档是相对价（`AIPDDImagePricing.RatioFor`），基础 `ModelPrice` 加了毛利后各档自动等比放大，无需单独处理。

### 19.3 落点

| 路径 | 文件 | 作用 |
|---|---|---|
| 乘数与表达式构造 | `model/aipdd_retail_pricing.go` | `AIPDDRetailMarkup`、`aipddRetailUSD`、`AIPDDLLMRetailExpr`、`IsAIPDDRetailTokenModel` |
| 文本模型导入器 | `model/aipdd_catalog_llm_pricing.go` | `syncAIPDDLLMTokenPricingTx` 写 `billing_setting.billing_mode`/`billing_expr` |
| 任务模型导入器 ×4 | `model/aipdd_catalog_task_pricing.go` | 按次固定价、共享适配器按次、同步按秒、异步按秒（含 by_resolution）全部应用乘数 |
| 同步编排 | `model/aipdd_catalog_sync.go` | 文本导入器在任务导入之后运行，避免同一目录版本给任务模型发 token 表达式；提交后一并激活 `billing_expr` |
| 后台「同步上游定价」预览 | `controller/ratio_sync.go` | 复用同一个 `AIPDDLLMRetailExpr`/`AIPDDRetailMarkup`，保证预览候选价＝已写入价，管理员点应用不会把毛利抹平 |

**上游的 `available` 标记不参与定价**（用户 2026-10-11 口径：「不要管他可用不可用，总之就是 1.1 倍」）。目录里出现的模型一律按批发 × 倍率落价，因为不可用的模型仍然挂在托管渠道和 `abilities` 里、仍可被路由，没有零售价就等于按默认倍率亏本卖。`available=false` 只影响 `/v1/models` 是否展示该模型，不影响价格。

目录不再为某个模型计价（`free=true`、`pricing.enabled=false`、改成按次计费）时，导入器会**回收**它自己写过的行：删掉表达式，且仅当模式是 `tiered_expr` 时删掉模式；管理员手工设的其它模式保留。目录里不存在的模型永不受影响。

### 19.4 默认零售价（当前 catalog revision `37cdcae4`，`rmbPerAwcoin=0.0001`）

| 模型 | 批发（¥/1M） | 零售（¥/1M） |
|---|---|---|
| `ap-chat-3.8-flash` | 入 1.2 / 出 7 | 入 1.32 / 出 7.7 |
| `ap-glm-5.3-flash` | 入 0.4 / 出 1.4 | 入 0.44 / 出 1.54 |
| `ap-deepseek-v4-flash` | 入 3 / 出 9 / 缓存读 0.1 / 缓存写 3 | 入 3.3 / 出 9.9 / 缓存读 0.11 / 缓存写 3.3 |
| `ap-deepseek-v4-pro` | 入 9 / 出 27 / 缓存读 0.3 / 缓存写 9 | 入 9.9 / 出 29.7 / 缓存读 0.33 / 缓存写 9.9 |
| `ap-glm-5.2`、`ap-glm-5.3` | 入 8 / 出 28 / 缓存读 2 / 缓存写 8 | 入 8.8 / 出 30.8 / 缓存读 2.2 / 缓存写 8.8 |
| `ap-qwen3.8-max` | 入 12 / 出 36 / 缓存读 12 / 缓存写 15 | 入 13.2 / 出 39.6 / 缓存读 13.2 / 缓存写 16.5 |
| `ap-qwen3-embedding:0.6b` | 入 0.03 | 入 0.033（无输出价，表达式 `c * 0`） |

任务侧：图片按次批发 `¥0.2 → 零售 ¥0.22`，其余各档按同样 1.1 放大（批发 ¥0.4→¥0.44、¥0.24→¥0.264、¥0.3→¥0.33、¥0.15→¥0.165、¥0.5→¥0.55）；`ap-indextts-2` `¥0.1 → ¥0.11`/次；`ap-funasr` `¥0.01 → ¥0.011`/秒；`ap-minimax-h3` 768p `¥0.12 → ¥0.132`/秒（480p ¥0.09→¥0.099、1080p ¥0.15→¥0.165）。上游标了 `available=false` 的两个也一样有价：`ap-qwen-image-edit` `¥1 → ¥1.1`/次、`ap-ltx-2.5` `¥0.4 → ¥0.44`/秒。

### 19.5 验收

- `model/aipdd_retail_pricing_test.go`：默认 1.10 / 环境覆盖 / 非法值回落；`IsAIPDDRetailTokenModel` 四种排除条件（免费、停用计价、非按 token、非 awcoin；**不含 available**）；表达式含 cache 变量、schema v1 不含 cache 变量、embedding `c * 0`；汇率或 awcoin 价不可用时报错。
- `model/aipdd_catalog_llm_pricing_test.go`：首次导入写 3 个模型（含上游标 `available=false` 的那个）且保留管理员已有价；重复导入 0 变更；批发价变动被检出；撤价/免费模型回收行（`tiered_expr` 模式删除、`ratio` 模式保留）；`markup=1` 时系数恰为平价。
- `model/aipdd_catalog_sync_test.go`、`model/aipdd_catalog_task_pricing_test.go`、`model/aipdd_production_catalog_sync_test.go`（真实目录 8 类 fixture，逐模型断言系数）、`model/channel_env_test.go`（启动即定价）、`controller/ratio_sync_aipdd_test.go`（预览候选价含毛利）全部通过。
- `go test ./model/... ./controller/... ./service/... ./setting/... ./pkg/... ./relay/... ./dto/...` 全绿。
- 生产已验（2026-10-11，本机 `one-api.db`，`rmbPerAwcoin=0.0001`、`USDExchangeRate=7.3`）：8 个文本表达式、8 个按次价、`ap-minimax-h3` 三档分辨率全部精确等于批发 ×1.1；真实中转请求 `ap-glm-5.3-flash`（29 入 / 2500 出）收客户 3869 微元、估上游 3512 微元，比值 **1.1017**（0.15% 来自 quota 取整）。注意 `customer_charge_rmb_mic` 由整数 quota 反推，1 quota ≈ 14.6 微元，几十 token 的小请求测不出毛利，必须发长输出请求。

### 19.6 注意

- 目录同步只在**价格变化**时写库；改 `AIPDD_PRICING_MARKUP` 后需要再触发一次同步（或重启）才会按新倍率重算。
- 手动在定价页改过的模型会被下一次目录同步覆盖回「批发 × 倍率」——catalog 是价格来源。要让某个人工价说了算，在「系统设置 → 定价」点该模型的钉价按钮（写入 `options` 的 `AIPDDPinnedModels`，一个模型名数组）：5 个价格导入器与上游同步预览都会跳过钉住的模型，既不覆写也不回收，取消钉价后下一次同步自动接管。整体停用同步仍可把 `AIPDD_CATALOG_SYNC_INTERVAL_MINUTES` 设为 0。
- 单个模型的价格异常（批发价为 0、缓存价缺失、分辨率档位重复等）只跳过该模型并每个 revision 打一条日志，其余模型照常定价；只有汇率基准不可用、合并后的价格表校验失败、序列化或写库失败才会让整个目录同步回滚。
- 毛利只在 catalog **列出**的模型上生效（`available` 与否都算列出了）。目录里根本没有的模型（当前 `jev`）既无批发价也无零售价，走默认 1 倍率，属于亏损区间——只能等上游发布后由同步自动接管。

