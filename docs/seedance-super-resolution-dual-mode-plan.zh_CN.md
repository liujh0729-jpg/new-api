# Seedance 超分双模式补全计划

> 状态：代码已实施，真实账号灰度待专用测试凭证  
> 日期：2026-09-04  
> 目标项目：`new-api`  
> 行为参考：`aipdd-java` 已运行的火山 AI MediaKit 对接

> 本次验证：`go test ./...`、前端 `typecheck`、生产构建及 Seedance 定向 ESLint 已通过。仓库全量 ESLint 仍被其他既有功能文件的 12 个错误阻塞；未使用真实 MediaKit API Key 产生付费任务。

## 1. 背景与结论

当前 Seedance 管理只实现了“自定义远端超分服务”，没有实现原需求中的“本站直连火山 AI MediaKit”。这不是单纯的前端入口缺失：后端保存校验、Provider 工厂、请求 DTO、响应解析和取消语义都只支持当前通用协议。

本次应补齐两种管理员可选的执行模式：

1. **火山 AI MediaKit 直连**：NewAPI 服务端使用独立的 MediaKit API Key，直接调用火山官方画质增强 API。
2. **自定义远端服务**：NewAPI 服务端调用管理员配置的第三方或自建超分 API。

未来的 **AIPDD 自有超分** 是第三种执行位置，不等于火山直连，本计划不实现它。

“本站直连火山”只表示请求由 NewAPI 服务端发出，不表示 NewAPI 本机启动 GPU、模型或超分进程。

## 2. 范围

### 2.1 本期包含

- 管理页提供“火山 AI MediaKit / 自定义远端服务”两种接入方式。
- 火山直连模式提供明确的“AI MediaKit API Key”输入框，由管理员直接填写。
- 新增火山 AI MediaKit 专用协议适配器。
- 保留现有 `EnhancementProvider` 主工作流和历史快照机制。
- 独立加密保存 MediaKit API Key，不复用 Ark API Key。
- 火山端点、请求、查询、状态和结果字段按官方协议处理。
- 对幂等、提交结果未知、取消不支持等情况采用不会重复扣费的安全语义。
- 管理端显示真实的超分术语；普通用户接口、模型列表、日志和账单展示继续隐藏内部超分实现细节。
- 覆盖 Go 单元测试、工作流测试、前端类型检查和生产构建。

### 2.2 本期不包含

- AIPDD 自有 GPU/SeedVR2 超分执行。
- `AIPDD_INTERNAL` Provider 的任务 API。
- 自动跨 Provider 降级。
- 通用工作流 DSL 或任意供应商脚本化适配。
- 重新设计 Seedance 的公共任务协议或定价体系。
- 在保存配置时提交可能产生费用的真实超分任务。

## 3. 四象限确认地图

### 3.1 已知事实

| 事实 | 证据 | 影响 |
| --- | --- | --- |
| 数据模型已预留 Provider 类型、端点、加密凭证、服务代码和策略字段 | `model/seedance.go` 的 `MediaEnhancementProvider` | 可以演进现有模型，无需另建一套超分系统 |
| 后端目前只允许 `DIRECT_EXTERNAL` | `controller/seedance_admin.go` 的 `SaveSeedanceProvider` | 第二种模式不是只改前端就能恢复 |
| Provider 工厂目前只构造 `DirectEnhancementProvider` | `service/seedance_workflow.go` 的 `newEnhancementProvider` | 必须增加专用适配器分派 |
| 通用外部协议提交 `input/specification/idempotency_key`，查询和删除使用同一端点拼接任务 ID | `service/seedance_workflow.go` | 无法直接兼容火山 MediaKit 协议 |
| 管理页把 Provider 类型硬编码为 `DIRECT_EXTERNAL` | `web/default/src/features/seedance-admin/index.tsx` 与 `types.ts` | 需要条件表单和类型扩展 |
| Java 基线已实现 MediaKit 固定端点、Bearer API Key、官方请求字段和查询结果解析 | `aipdd-java/.../OpenPlatformMediaKitConfigService.java` 与 `ModelOfferingEnhanceOrchestrator.java` | 优先等价迁移，避免重新猜协议 |
| 火山官方服务域名已切换为 `mediakit.cn-beijing.volces.com` | 火山 AI MediaKit 官方更新记录 | 新适配器不能沿用旧 `amk` 域名 |
| AIPDD 当前财务协议把外部供应商统一记为 `DIRECT_EXTERNAL` | `aipdd-java/docs/service-usage-postpaid-internal-api.md` | 不能直接把结算字段改成新的第三类值 |

### 3.2 已知但尚需确认的问题

| 问题 | 当前建议 | 状态/解锁条件 |
| --- | --- | --- |
| 火山接口是否保证相同幂等键不会重复创建任务 | 使用稳定的 `client_token` 安全重试 | **已确认（2026-09-04）**：官方基础概念文档明确同一 `client_token` 返回原任务，不重复执行或计费；实现同时按官方 24 小时幂等窗口设定重试上限，窗口外转人工核对，绝不再次 POST |
| 火山新任务 API 是否支持取消 | 不伪造取消成功；已受理任务返回明确冲突并继续查询 | **已确认（2026-09-04）**：当前 AI MediaKit 异步任务 API 未提供取消端点；文档中的旧 `KillJob` 属于另一套媒体处理 API，不适用于本适配器 |
| 首期开放哪些版本 | 先开放 Java 已验证的 `standard`、`professional`；`fast` 独立验证后再开 | **OPEN**：确认当前产品开通与成本表 |
| MediaKit 是否返回可用于精确成本核算的时长、分辨率、帧率 | 保存 `duration/resolution/fps/tool_version/expires_at` 非敏感证据，首期仍使用已冻结的 Provider 成本快照 | **部分确认**：官方响应契约已确认字段；精确成本仍待真实账单样本核对 |
| 是否提供“测试连接”按钮 | 没有无副作用的官方鉴权接口时不提供伪验证 | **OPEN**：找到官方无费用验证方式后再启用 |

以上 OPEN 项不阻止完成代码结构，但会阻止 Provider 从 `DISABLED` 切换到生产 `ACTIVE`。

### 3.3 已从现有交互中确认的隐含要求

- 页面只给管理员使用，因此可以明确展示“超分”“火山 AI MediaKit”等运维术语。
- 普通用户不能从公共模型名、任务响应、消费记录或错误信息判断内部是否做了超分。
- 所有选择控件使用 NewAPI 现有 `SeedanceSelect`/Base UI 风格，不使用浏览器原生下拉框。
- 管理端状态和模式显示中文，不直接把内部英文枚举当作界面文案。
- 火山直连模式应尽量少填：管理员不需要输入固定官方 URL，也不应手写请求 JSON。
- 火山直连模式必须允许管理员填写独立的 AI MediaKit API Key，不能要求从 Ark API Key 自动推导或复用。
- 自定义远端模式保留高级 JSON 配置能力，避免削弱现有可扩展性。

### 3.4 风险与盲点

#### 风险 A：`provider_type` 同时承担成本归属和调用协议

如果直接增加 `provider_type=VOLCENGINE_MEDIAKIT`，AIPDD 第一阶段财务接口会拒绝该账单事件。火山 MediaKit 从成本归属看仍然是外部供应商。

**处理：** 保留 `provider_type=DIRECT_EXTERNAL` 表示成本和执行归属；新增 `adapter_type` 表示实际线协议。

#### 风险 B：通用外部协议不能通过“填火山 URL”兼容

火山提交端点与查询端点不同，请求字段为 `video_url/scene/resolution/tool_version`，结果地址位于 `result.video_url`。继续复用当前通用 DTO 会得到错误请求或错误查询地址。

**处理：** 实现专用适配器，不在通用适配器中堆供应商条件分支。

#### 风险 C：未知提交结果可能造成重复任务和重复成本

当前工作流会复用相同 attempt 和幂等键重试。火山官方异步任务契约现已明确支持请求体 `client_token` 幂等控制；仅发送自定义 Header 仍不能证明安全。

**处理：** MediaKit 将稳定 attempt ID 写入 `client_token`，允许相同 attempt 安全重试；其他未证明幂等的适配器仍在网络超时后进入人工核对状态，不自动再次 POST。

#### 风险 D：取消接口是当前工作流的硬依赖

当前订单取消在已有 `execution_task_id` 时要求 Provider 的 `Cancel` 成功。若火山不支持取消，直接返回成功会造成“本站已退款、火山仍执行并产生成本”的账实不一致。

**处理：** 增加“取消能力”语义；不支持时拒绝把已受理的远端任务标为已取消。

#### 风险 E：历史任务必须按创建时的适配器继续查询

Provider 配置可编辑，历史订单又保存了加密 Provider 快照。如果适配器类型只存在当前配置表，修改配置后可能用错误协议查询旧任务。

**处理：** `adapter_type` 必须进入 Provider 快照和快照哈希输入；执行始终从订单快照恢复。

#### 风险 F：火山生成结果 URL 的有效期

Ark 输出和 MediaKit 输出都可能是短期签名 URL。提交延迟、重试和用户下载时间过长都可能导致 URL 失效。

**处理：** 端到端测试必须覆盖无文件扩展名的签名 URL；记录 URL 到期信息（若响应提供），但不把完整签名 URL写入普通日志。官方当前契约说明结果默认保留 24 小时，剩余不足 2 小时时再次查询会自动续期，任务只支持查询创建后 30 天内的数据。首期灰度必须在 24 小时内验证下载；若业务要求长期下载，必须先增加持久对象存储，不能把查询续期当成永久保存。

## 4. 目标模型

### 4.1 两个维度，不再混用

| 字段 | 含义 | 本期值 |
| --- | --- | --- |
| `provider_type` | 执行和成本归属 | `DIRECT_EXTERNAL`；预留 `AIPDD_INTERNAL` |
| `adapter_type` | 实际调用协议 | `GENERIC_HTTP`、`VOLCENGINE_MEDIAKIT`；预留 `AIPDD_SUPER_RESOLUTION` |

合法组合：

| `provider_type` | `adapter_type` | 是否启用 |
| --- | --- | --- |
| `DIRECT_EXTERNAL` | `GENERIC_HTTP` | 是，兼容现有自定义远端节点 |
| `DIRECT_EXTERNAL` | `VOLCENGINE_MEDIAKIT` | 是，本次新增 |
| `AIPDD_INTERNAL` | `AIPDD_SUPER_RESOLUTION` | 否，后续阶段 |
| 其他组合 | 任意 | 后端拒绝 |

### 4.2 数据库与快照

在 `MediaEnhancementProvider` 增加：

```text
adapter_type varchar(32) not null default 'GENERIC_HTTP'
```

迁移要求：

- 既有 `DIRECT_EXTERNAL` 记录回填为 `GENERIC_HTTP`。
- SQLite、MySQL、PostgreSQL 都通过 GORM/兼容迁移完成。
- `adapter_type` 写入订单的 Provider 快照。
- 旧快照没有该字段时，按 `provider_type=DIRECT_EXTERNAL -> GENERIC_HTTP` 兼容读取。
- Provider 列表改为专用响应 DTO，增加 `credential_configured`，绝不序列化密文或明文。
- 空凭证继续表示“保留旧值”；清除凭证使用显式 `clear_credential`，避免误清空。

### 4.3 各适配器配置语义

#### `GENERIC_HTTP`

- `service_endpoint`：管理员填写的任务集合 URL。
- `credential`：可选 Bearer Token。
- `service_code`：管理员填写。
- `capabilities/timeout/retry/fallback`：保留现有规则。

#### `VOLCENGINE_MEDIAKIT`

- `service_endpoint`：后端强制保存/返回官方基地址 `https://mediakit.cn-beijing.volces.com`，前端只读展示。
- `mediakit_api_key`：管理员可在火山直连表单中直接填写；提交后映射到 Provider 的加密凭证字段保存，不复用 Ark API Key，也不向前端回传原文。
- `service_code`：后端标准化为 `volcengine_ai_mediakit_quality_enhance`，前端无需手填。
- `capabilities`：由内建适配器决定，不允许管理员伪造取消或幂等能力。
- `timeout/retry/fallback`：界面提供结构化字段；高级 JSON 只作为管理员折叠项。
- 新建记录默认 `DISABLED`，只有通过配置检查和显式启用后才可绑定新模型。

## 5. 火山 AI MediaKit 适配器

新增 `VolcengineMediaKitEnhancementProvider`，继续实现现有最小接口：

```text
Submit(input, specification, idempotency_key)
Query(execution_task_id)
Cancel(execution_task_id)
```

### 5.1 提交

```http
POST https://mediakit.cn-beijing.volces.com/api/v1/tools/enhance-video
Authorization: Bearer {MediaKit API Key}
Content-Type: application/json
```

建议的内部 specification：

```json
{
  "scene": "aigc",
  "resolution": "1080p",
  "tool_version": "standard"
}
```

转换后的火山请求：

```json
{
  "video_url": "https://signed.example/source",
  "scene": "aigc",
  "resolution": "1080p",
  "tool_version": "standard"
}
```

要求：

- `video_url` 必须来自工作流生成结果，不能被管理员表单覆盖。
- `scene/resolution/tool_version` 使用白名单验证。
- API Key 只放在 Authorization Header，不进入请求证据、日志或错误文本。
- 尝试 ID 作为 `client_token` 写入请求体；超长或非 ASCII ID 先稳定哈希到 64 字符以内。
- 解析 `task_id` 并持久化为 `execution_task_id`。

### 5.2 查询

```http
GET https://mediakit.cn-beijing.volces.com/api/v1/tasks/{url-escaped-task-id}
Authorization: Bearer {MediaKit API Key}
```

至少映射：

| 火山字段 | NewAPI 字段 |
| --- | --- |
| `task_id` | `execution_task_id` |
| `status=completed/succeeded/success` | `SUCCEEDED` |
| `status=failed/error/cancelled/canceled` | `FAILED` |
| 其他非终态 | `RUNNING` |
| `result.video_url` | `result_url` |
| `result` 中非敏感的时长/分辨率/帧率信息 | `usage_evidence` |

HTTP 4xx/5xx、业务 `success=false`、错误码和查询超时分别分类，不能把所有异常都变成“处理中”。

### 5.3 取消

当前 AI MediaKit 异步任务 API 未提供取消端点，因此：

- 尚未提交到 MediaKit 的本地任务可以取消；
- 已获得 `execution_task_id` 的任务返回明确的“上游不支持取消”冲突；
- 工作流继续查询远端终态，不提前退款或删除成本证据；
- 管理页面提示实际状态，不显示虚假的“取消成功”。

### 5.4 提交结果未知

为 Provider 增加静态执行能力描述，至少包含：

```text
submit_retry_safe
cancel_supported
```

- `GENERIC_HTTP` 继续要求远端服务遵守现有幂等契约。
- `VOLCENGINE_MEDIAKIT` 已依据官方 `client_token` 契约设置 `submit_retry_safe=true`，真实账号灰度仍需验证返回同一 `task_id`。
- MediaKit 的安全重试能力带 24 小时有效窗口；窗口从增强 usage 的 `started_at`（旧数据回退 `created_at`）计算。结果未知且窗口已过时进入人工核对，不因全局任务超时被配置得更长而重新创建付费任务。
- `submit_retry_safe=false` 且提交响应未知时，不再次 POST；记录 `SUBMISSION_OUTCOME_UNKNOWN`，在管理员页面进入人工核对队列。
- 未确认原任务不存在前，禁止自动切换到另一个 Provider。

## 6. 管理页面调整

### 6.1 Provider 表单

把“外部超分节点”改为“超分服务”，增加 NewAPI 风格的“接入方式”下拉框：

- `火山 AI MediaKit（本站直连）`
- `自定义远端超分服务`

选择火山模式时显示：

- 显示名称（默认“火山 AI MediaKit”）
- 官方服务地址（只读）
- AI MediaKit API Key（管理员可填写的密码输入框，新建直连配置时必填）
- 凭证状态（已配置/未配置）
- 状态（停用/启用）
- 超时设置
- 高级策略（折叠）

选择自定义远端模式时显示现有字段：

- 显示名称
- 服务代码
- HTTPS 服务地址（本地调试地址沿用现有限制）
- Bearer 凭证
- 能力、超时、重试、降级策略
- 状态

所有下拉框统一使用 `SeedanceSelect`，所有展示文案使用中文映射。

AI MediaKit API Key 的交互规则：

- 新建火山直连 Provider 时必须填写。
- 编辑已有 Provider 时输入框保持空白；留空表示保留原 Key，不是清除。
- 更换 Key 时填写新值并覆盖旧密文，同时写管理员审计。
- 清除 Key 必须使用单独的显式操作；清除后 Provider 自动切为 `DISABLED`，不得继续接收新任务。
- Provider 查询接口只返回 `credential_configured=true/false`，绝不返回 API Key 明文、密文或可复原内容。
- API Key 不与 Ark API Key、火山账单 AK/SK 或 AIPDD API Key 共用字段或界面文案。

### 6.2 模型发布表单

- Provider 下拉项显示“名称 · 接入方式 · 状态”。
- 选择 MediaKit Provider 后，以结构化控件生成 enhancement specification：
  - 场景：首期固定 `aigc`；
  - 目标分辨率：按已验证能力选择；
  - 处理版本：`标准版` / `专业版`，极速版验证后再加入；
  - specification 版本：仍需冻结并显式保存。
- 自定义远端 Provider 保留原始 specification JSON 编辑能力。
- 已发布 offering 不因 Provider 后续编辑而改写历史快照。

### 6.3 列表与诊断

- Provider 类型显示为中文：`火山 AI MediaKit`、`自定义远端服务`。
- 显示凭证是否已配置，不显示 API Key 后缀也不回填密码框。
- 指标可继续按 `provider_type + service_code` 聚合；服务代码足以区分 MediaKit 与自定义供应商。
- 提交结果未知、取消不支持、鉴权失败使用明确中文原因。
- 不新增普通用户可访问的管理接口。

## 7. 计费和公共信息边界

### 7.1 AIPDD 财务事件

火山 AI MediaKit 仍按外部供应商结算：

```text
provider_type = DIRECT_EXTERNAL
service_code = volcengine_ai_mediakit_quality_enhance
provider_cost_rmb = NewAPI 已冻结的外部供应商成本
```

不要向 AIPDD 第一阶段接口发送 `provider_type=VOLCENGINE_MEDIAKIT`，否则会破坏现有枚举约束。适配器类型只保留在 NewAPI 的私有 Provider 配置/快照和非敏感证据中。

### 7.2 用户侧泄漏控制

以下公共输出不得增加 Provider、MediaKit、超分级别、内部服务代码或供应商错误原文：

- 公共模型列表和模型详情；
- Seedance 官方协议任务响应；
- OpenAI Video 兼容任务响应；
- 普通用户消费记录和导出；
- 下载文件名、代理 URL 和成品 metadata；
- 普通用户可见的错误消息。

管理员页面、管理员审计和内部指标可以展示真实超分信息。

## 8. 实施步骤

### 阶段 0：冻结外部契约

1. 对照最新火山官方文档和测试账号，冻结提交、查询、状态、错误、限流、URL 有效期、幂等和取消语义。
2. 保存脱敏的真实成功/失败/处理中响应样本作为测试 fixture。
3. 确认首期版本和对应成本矩阵。
4. 未完成幂等与取消确认前，只允许在测试环境保持 Provider 为 `DISABLED` 或受控灰度。

### 阶段 1：数据与管理 API

1. 增加 `adapter_type`、兼容默认值和历史快照读取规则。
2. 更新 Provider 保存请求，按合法组合做服务器端校验。
3. MediaKit 模式由服务器固定端点和服务代码，拒绝客户端覆盖；管理请求允许提交独立的 `mediakit_api_key` 并映射到加密凭证存储。
4. Provider 列表使用脱敏 DTO，返回 `credential_configured`。
5. 增加显式清除凭证语义并写管理员审计。

### 阶段 2：Provider 运行时

1. Provider 工厂按 `adapter_type` 分派。
2. 新增 MediaKit 请求/响应 DTO 和独立实现文件。
3. 实现提交、查询、状态映射、错误分类、证据脱敏。
4. 实现静态幂等/取消能力和提交未知处置。
5. 保持通用外部 Provider 现有协议兼容。

### 阶段 3：管理员前端

1. 增加接入方式下拉和条件表单。
2. 火山模式隐藏可编辑 URL、服务代码和原始协议 JSON。
3. offering 表单为 MediaKit 使用结构化规格控件。
4. Provider 表格、状态、错误原因全部中文化。
5. API Key 不回显，空输入不覆盖旧凭证。

### 阶段 4：计费与隐私回归

1. 验证发送 AIPDD 的 `provider_type` 仍为 `DIRECT_EXTERNAL`。
2. 验证 MediaKit 成本和用量证据进入冻结快照与账单事件。
3. 对公共模型、任务、日志、导出、代理下载和错误做泄漏测试。
4. 验证历史 Generic HTTP 订单仍使用旧适配器查询。

### 阶段 5：灰度验证

1. 使用专用测试 API Key 和短视频跑一条标准版任务。
2. 覆盖提交成功、处理中、完成、业务失败、401、429、5xx、查询超时。
3. 人工制造提交响应中断，确认不会重复创建任务。
4. 验证最终视频代理下载、AIPDD 财务投递和订单收益。
5. 测试通过后由管理员显式切换 Provider 为 `ACTIVE`。

## 9. 测试清单

### 9.1 Go

- `adapter_type` 合法组合、默认值和旧快照兼容。
- MediaKit 固定域名不能被请求体或数据库脏值覆盖。
- API Key 加密、更新保留、显式清除和 API 脱敏。
- Submit 请求路径、Bearer Header 和官方 JSON 字段。
- Query 路径转义、状态映射、`result.video_url` 提取。
- 业务失败和 HTTP 失败的 definitive/unknown 分类。
- 不安全提交重试被阻止。
- 不支持取消时不提前完成本地取消和退款。
- Generic HTTP Provider 全部既有测试继续通过。
- Provider 快照更新后历史订单仍使用原 adapter。
- AIPDD 财务事件仍输出 `DIRECT_EXTERNAL`。
- 公共 DTO 不泄漏超分字段。

建议命令：

```powershell
go test ./model ./controller ./service ./relay/channel/task/seedance
go test ./...
```

### 9.2 前端

- 两种接入方式切换时字段正确出现/隐藏。
- 页面没有新增原生 `<select>`。
- MediaKit 模式提交固定字段，自定义模式保持原协议。
- 凭证不回填，空值不覆盖。
- 中文状态和错误映射完整。

建议命令：

```powershell
cd web/default
bun run typecheck
bun run lint
bun run build
```

### 9.3 真实联调

- 公开可访问且无扩展名的签名视频 URL。
- 480p、720p、1080p 与已启用处理版本的组合。
- 输出 URL 到期时间与 NewAPI 媒体代理行为。
- 连续轮询、限流退避和任务终态稳定性。
- 同一 attempt 的异常重放不会重复产生可计费任务。

## 10. 验收标准

- 管理员在 Seedance 管理页能清楚选择“火山 AI MediaKit（本站直连）”或“自定义远端超分服务”。
- 火山模式不要求管理员填写 URL、服务代码或原始请求 JSON。
- 火山模式可以直接填写独立的 AI MediaKit API Key；新建时必填，编辑留空时保留原 Key，且前后端均不回显 Key。
- NewAPI 使用独立加密的 MediaKit API Key 调用正确的提交和查询端点。
- 一个 offering 可以绑定任一模式，订单创建时冻结完整 Provider 适配信息。
- 既有 Generic HTTP Provider 和历史订单无回归。
- 提交结果未知时不会因盲目重试产生重复任务。
- 不支持远端取消时不会伪造取消成功或错误退款。
- AIPDD 继续接受财务事件，外部供应商成本口径不变。
- 普通用户无法从任何公共接口看到超分实现细节。
- Go 测试、前端类型检查、Lint 和生产构建全部通过。

## 11. 可调整决策

实现前优先确认以下三项，其他机械工作可按本计划直接执行：

1. **版本范围**：建议首期仅标准版和专业版；是否同步开放极速版。
2. **取消体验**：建议远端不支持取消时拒绝本地取消；是否允许“仅停止本地等待但继续结算”的高级操作。
3. **未知提交处理**：建议无官方幂等保证时进入人工核对；是否已有可按 attempt 查询火山任务的内部手段。

## 12. 下一步实施指令

确认本计划后，可直接使用下面的指令启动实现：

> 按 `docs/seedance-super-resolution-dual-mode-plan.zh_CN.md` 实现 Seedance 超分双模式。首期启用火山 AI MediaKit 标准版和专业版；保留自定义远端服务兼容；AIPDD 自有超分不在本期范围。先完成契约 fixture、数据模型和后端适配器，再完成管理员 UI、计费/隐私回归和真实灰度验证。遇到官方幂等或取消语义无法确认时，不做危险默认，保留 Provider 为停用并报告阻塞。

## 13. 参考

- NewAPI 当前工作流：`service/seedance_workflow.go`
- NewAPI 管理 API：`controller/seedance_admin.go`
- NewAPI Provider/订单模型：`model/seedance.go`
- NewAPI 管理页：`web/default/src/features/seedance-admin/index.tsx`
- Java MediaKit 配置基线：`../aipdd-java/src/main/java/work/aipdd/core/openplatform/mediakit/service/OpenPlatformMediaKitConfigService.java`
- Java MediaKit 编排基线：`../aipdd-java/src/main/java/work/aipdd/core/modeloffering/service/ModelOfferingEnhanceOrchestrator.java`
- Java 契约测试：`../aipdd-java/src/test/java/work/aipdd/core/modeloffering/service/ModelOfferingEnhanceOrchestratorTest.java`
- 火山 AI MediaKit 画质增强（标准版和专业版）：<https://www.volcengine.com/docs/6448/2279230?lang=zh>
- 火山 AI MediaKit 查询任务信息 API：<https://www.volcengine.com/docs/6448/2278532?lang=zh>
- 火山 AI MediaKit 基础概念及幂等性控制：<https://www.volcengine.com/docs/6448/2300661?lang=zh>
- 火山 AI MediaKit 产品更新记录：<https://www.volcengine.com/docs/6448/2373721?lang=zh>
