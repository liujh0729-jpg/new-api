# AIPDD Seedance 定价同步与 480p 支持改造说明

> **合同更正（2026-07-17）**：本文关于“目录价格随 API Key 的 PLATFORM/BYOK 身份切换”的设计已被取代，不再作为实现依据。`/v1/new-api/catalog` 现在以 `pricingBasis: "display"` 固定发布展示售价，权威字段为 `displayAmountAwcoinPerSecond` 和 `displayVideoInputAwcoinPerSecond`；BYOK 价格通过独立 `byok...` 字段返回。NewAPI 只接受该严格合同：缺少 `pricingBasis: "display"` 或任一展示价字段时立即拒绝同步，不读取旧目录价格字段，也不得将 `byok...` 用作展示售价。本文其余关于旧计费身份合同的内容仅保留为历史记录；480p 能力与执行链说明仍可参考。

## 1. 背景

AIPDD 的 Seedance 展示模型近期调整了计费和执行规则：

- Seedance 价格按目标分辨率和是否包含参考视频精确匹配。
- AIPDD API Key 可能对应 `PLATFORM` 或 `BYOK` 两种计费模式。
- V171 将 BYOK 售价调整为“标准产品售价减 provider 成本”。
- 480p→480p 不再视为无处理的原生直出，而是同分辨率质量增强链路。

NewAPI 当前已经可以从 AIPDD 的认证原子目录读取 Seedance 价格矩阵，并在任务创建时按分辨率、参考视频和时长精确预扣。此次改造不应在 NewAPI 内复制 AIPDD 的定价公式，而应继续将携带渠道 API Key 获取的 `/v1/new-api/catalog` 作为唯一价格事实源。

本文集中说明以下三个问题：

1. AIPDD 目录失败回退快照没有按 API Key 身份隔离。
2. V171 生效后，BYOK 目录仍使用错误的 `enhancementOnly` 语义。
3. 480p→480p 展示模型可能无法进入目录或未被 NewAPI 完整验证。

---

## 2. 必须保持的计费原则

### 2.1 价格事实源

NewAPI 必须使用 AIPDD 认证原子目录返回的价格：

```http
GET /v1/new-api/catalog
Authorization: Bearer {AIPDD_API_KEY}
X-API-Key: {AIPDD_API_KEY}
```

同一个 AIPDD Base URL 使用不同 API Key 时，可能返回不同计费模式和价格矩阵，因此目录不是公共静态数据。

### 2.2 NewAPI 不复制上游定价公式

NewAPI 不应自行计算：

```text
BYOK 售价 = PLATFORM 标准产品售价 - provider 成本
```

该公式由 AIPDD 负责执行并固化到目录的 `amountAwcoinPerSecond`。NewAPI 只消费最终价格。

### 2.3 Seedance 实际扣费公式保持不变

```text
AWCoin = ceil(max(minimumAWCoin, amountAWCoinPerSecond × seconds))

quota = AWCoin
        × usdPerAWCoin
        × QuotaPerUnit
        × groupRatio
```

NewAPI 的实际扣费继续走 Seedance `EstimateExactQuota`，不得退回只使用固定 `ModelPrice` 的按次扣费。

`ModelPrice` 只适合作为起步价或目录展示价，不能代表所有分辨率和参考视频场景的实际价格。

---

## 3. 问题一：目录失败回退快照没有按 API Key 身份隔离

### 3.1 当前行为

NewAPI 请求 AIPDD 原子目录时会携带渠道 API Key，但本地快照目前只保存和校验：

```text
source_base_url
```

相关代码：

- `pkg/aipddcatalog/atomic.go`
- `model/aipdd_catalog_snapshot.go`
- `model/aipdd_catalog_sync.go`

当前失败回退流程只检查快照的 Base URL 是否与本次同步地址一致，没有验证快照是否属于当前 API Key 身份。

### 3.2 风险场景

1. AIPDD 渠道使用 PLATFORM Key 成功同步。
2. NewAPI 保存 PLATFORM 价格快照。
3. 管理员将渠道 Key 更换为 BYOK Key。
4. 首次 BYOK 目录同步因网络或上游故障失败。
5. NewAPI 发现 Base URL 相同，恢复旧 PLATFORM 快照。
6. 后续任务使用 BYOK Key 请求 AIPDD，但 NewAPI 按 PLATFORM 价格向用户预扣。

反方向同样存在风险：BYOK 快照也可能被错误用于 PLATFORM Key。

### 3.3 修改要求

为目录快照增加不可逆的 API Key 指纹：

```go
SourceKeyFingerprint string `json:"source_key_fingerprint" gorm:"type:varchar(64);not null"`
```

建议使用：

```text
SHA-256(strings.TrimSpace(apiKey))
```

要求：

- 数据库、日志和接口响应中不得保存或输出原始 API Key。
- `SyncAIPDDCatalog` 必须根据本次实际使用的渠道 Key 计算指纹。
- `restoreAIPDDCatalogSnapshot` 必须同时校验 Base URL 和 Key 指纹。
- `previousAIPDDCatalogModels` 也必须按 Base URL 和 Key 指纹读取上一次目录。
- 只有 `source_base_url + source_key_fingerprint` 同时匹配时，才允许使用失败回退快照。
- 对于带认证 Key 的同步，旧版无指纹快照必须拒绝恢复，不能静默兼容。
- 错误信息只能说明“目录快照身份不匹配”，不得包含 Key 或完整指纹。

### 3.4 快照存储建议

推荐将快照从固定单例改为按以下组合保存：

```text
(source_base_url, source_key_fingerprint)
```

这样可以保留同一 AIPDD 地址下不同认证身份的最后可用目录。

如果暂时继续使用单例快照，至少必须做到：

- 同 Key、同 Base URL 可以失败回退。
- Key 发生变化时，即使 Base URL 相同也必须失败关闭。

### 3.5 多 Key 渠道要求

API Key 指纹只能解决“快照恢复错误”，不能自动解决多 Key 轮询时的价格身份不一致。

如果 AIPDD 渠道开启多 Key，应在启用或同步时：

1. 使用所有启用 Key 分别获取认证目录。
2. 校验 Seedance 的 `billingMode`、价格矩阵和关键执行元数据一致。
3. 如果同一渠道内混用 PLATFORM 与 BYOK Key，拒绝启用或拒绝目录同步。
4. 如果不同 Key 返回不同价格 revision，应给出明确的配置错误。

NewAPI 不能用某一个 Key 的目录价格为另一个计费身份的 Key 预扣。

### 3.6 验收测试

- 同 Base URL、同 Key，同步失败时能够恢复快照。
- PLATFORM Key 成功同步后切换为 BYOK Key，同步失败时不得恢复 PLATFORM 快照。
- BYOK Key 成功同步后能够保存独立的 BYOK 快照。
- 不同 Base URL 不得互相恢复。
- 带认证 Key 时不得恢复旧版无指纹快照。
- 数据库、日志、错误响应中均不出现原始 Key。
- 多 Key 渠道混用 PLATFORM/BYOK 时同步失败并给出清晰错误。

---

## 4. 问题二：BYOK 的 `enhancementOnly` 语义已经不准确

### 4.1 V171 后的真实规则

V171 将 BYOK 的对外售价调整为：

```text
BYOK amount = PLATFORM 标准产品售价 - provider 成本
```

这里的 provider 成本是平台代付上游 Seedance 本体模型时产生的成本。BYOK 用户自行承担 provider 成本，因此 AIPDD 只收取扣除该成本后的平台服务价格。

这不等同于“只收增强成本”。

### 4.2 当前错误目录字段

AIPDD 当前目录在 BYOK 模式下仍返回：

```json
{
  "billingMode": "BYOK",
  "billingDimension": "enhancementOnly"
}
```

该描述会产生以下误导：

- 让下游以为 BYOK 售价只等于增强服务价格。
- 无法解释为什么没有增强步骤的 SKU 仍可能存在 BYOK 收费。
- 无法准确审计“标准售价减 provider 成本”的价格来源。

### 4.3 建议的目录契约

应将“价格变体选择维度”和“价格生成公式”拆成两个字段。

PLATFORM：

```json
{
  "billingMode": "PLATFORM",
  "billingDimension": "hasReferenceVideo",
  "billingFormula": "direct"
}
```

BYOK：

```json
{
  "billingMode": "BYOK",
  "billingDimension": "hasReferenceVideo",
  "billingFormula": "standardMinusProviderCost"
}
```

字段含义：

- `billingMode`：当前认证身份使用 PLATFORM 还是 BYOK 价格。
- `billingDimension`：价格变体仍按 `hasReferenceVideo` 匹配。
- `billingFormula`：当前 `amountAwcoinPerSecond` 的生成依据。

不应继续输出 `enhancementOnly`。

### 4.4 AIPDD 上游修改要求

修改 `NewApiCapabilityCatalogService` 的 Seedance 目录输出：

- PLATFORM 输出 `billingFormula=direct`。
- BYOK 输出 `billingFormula=standardMinusProviderCost`。
- 两种模式的 `billingDimension` 都输出 `hasReferenceVideo`。
- `priceVariants` 继续输出最终的 `amountAwcoinPerSecond` 和 `minimumAwcoin`。
- 目录 revision 必须因为这些字段或价格变化而更新。

该问题不能只在 NewAPI 内修复。AIPDD 必须先输出正确契约，NewAPI 再负责解析和保存。

### 4.5 NewAPI 修改要求

扩展 Seedance 分辨率价格结构：

```go
type AIPDDSeedanceResolutionPricing struct {
    BillingMode            string                       `json:"billingMode,omitempty"`
    BillingDimension       string                       `json:"billingDimension,omitempty"`
    BillingFormula         string                       `json:"billingFormula,omitempty"`
    DefaultDurationSeconds float64                      `json:"defaultDurationSeconds"`
    DefaultFramesPerSecond float64                      `json:"defaultFramesPerSecond"`
    PriceVariants          []AIPDDSeedancePriceVariant `json:"priceVariants"`
}
```

同时要求：

- 原子目录反序列化后不得丢失上述字段。
- 目录保存为快照并再次恢复后，字段必须保持一致。
- 同一展示模型所有分辨率的 `billingMode` 和 `billingFormula` 必须一致。
- 将 `billingMode`、`billingFormula` 写入 `AIPDDTaskExecutionSnapshot`。
- 消费日志记录目录 revision、billing mode、billing formula、目标分辨率和参考视频变体。
- NewAPI 不得根据 `billingFormula` 重新计算价格，只使用上游最终返回的 `amountAwcoinPerSecond`。

### 4.6 兼容处理

在 AIPDD 和 NewAPI 滚动发布期间，可以暂时接受旧目录没有 `billingFormula`，但应：

- 将其记录为 `legacy` 或空值。
- 不得继续把 `enhancementOnly` 当作 V171 的正确 BYOK 公式。
- 上游和下游都更新完成后，增加一致性校验或告警。

### 4.7 验收测试

- PLATFORM 目录完整保留 `billingMode=PLATFORM` 和 `billingFormula=direct`。
- BYOK 目录完整保留 `billingMode=BYOK` 和 `billingFormula=standardMinusProviderCost`。
- BYOK 目录不再出现 `enhancementOnly`。
- 无参考视频和有参考视频仍能选择各自价格变体。
- 目录经过拉取、落库、恢复、激活后计费语义字段不丢失。
- 任务快照和消费日志可以确认本次任务使用的计费模式和公式。
- 新增字段不改变现有 Seedance 精确扣费结果。

---

## 5. 问题三：Seedance 480p→480p 支持不完整

### 5.1 业务语义

AIPDD 当前支持 480p→480p 展示模型，但它不是普通原生直出：

```text
Seedance 本体输出 480p
→ 质量增强
→ 最终仍输出 480p
```

因此：

```text
sourceResolution = 480p
targetResolution = 480p
superResolutionLevel > 0
存在 enhance step
```

NewAPI 不负责执行增强链路，只需要识别目录中存在 480p SKU、按 480p 价格计费并将请求转发给 AIPDD。

### 5.2 当前 NewAPI 后端能力

NewAPI 后端没有写死 720p 最低分辨率。它会检查同步目录是否存在对应价格：

```go
if _, ok := seedanceResolutionPricing(cfg, resolution); !ok {
    return nil, "unsupported_resolution", ...
}
```

所以只要认证原子目录包含：

```json
{
  "pricing": {
    "byResolution": {
      "480p": {
        "defaultDurationSeconds": 5,
        "defaultFramesPerSecond": 24,
        "priceVariants": [
          {
            "hasReferenceVideo": false,
            "amountAwcoinPerSecond": 0,
            "minimumAwcoin": 0
          },
          {
            "hasReferenceVideo": true,
            "amountAwcoinPerSecond": 0,
            "minimumAwcoin": 0
          }
        ]
      }
    }
  }
}
```

NewAPI 后端即可支持 480p。示例中的价格仅表示结构，真实数值必须使用上游目录返回值。

### 5.3 可能的实际阻塞点

V171 将 480p→480p 改造成“同分辨率但包含增强步骤”的 offering。如果只执行数据库迁移，却没有同步部署 AIPDD Mapper 的特殊查询条件，480p offering 会被目录过滤掉。

旧查询通常只接受：

```text
sourceResolution == targetResolution 且没有 enhance step
```

或者：

```text
superResolutionLevel > 0
且 sourceResolution != targetResolution
且存在 enhance step
```

480p→480p 两个条件都不满足，因此必须显式允许：

```text
adapterCode = seedance
且 sourceResolution = 480p
且 targetResolution = 480p
且 superResolutionLevel > 0
且存在已启用 enhance step
```

V171 数据迁移和 `ModelOfferingMapper` 的 480p 特殊条件必须一起发布。

### 5.4 NewAPI 修改要求

- 原子目录解析必须保留 `pricing.byResolution["480p"]`。
- 不得因为 480p 的源分辨率和目标分辨率相同而在 NewAPI 侧过滤。
- `resolution: "480p"` 必须原样转发给 AIPDD。
- 实际价格必须选择 480p 下匹配的 `hasReferenceVideo` 变体。
- NewAPI 不得自行跳过、调用或模拟 AIPDD 的增强步骤。
- 目录缺少 480p 时才返回 `unsupported_resolution`。
- 上游新增 480p 后，NewAPI 必须通过目录同步立即获得该能力。

### 5.5 Playground 分辨率展示

当前 Playground 已在本地常量中为以下模型提供 480p：

- AP Seedance-2.0 VIP
- AP Seedance-2.0 标准版
- AP Seedance-2.0 轻量版

高性价比版当前只提供 1080p 和 4K，不应强行显示 480p。

长期建议将 Playground 改为：

1. 优先使用认证原子目录的 `pricing.byResolution` 键生成分辨率选项。
2. 目录元数据不可用时，才回退本地硬编码常量。
3. 只展示当前模型目录中实际存在的目标分辨率。

这样以后新增或删除 480p、2K、4K 时，不必修改 NewAPI 前端代码。

### 5.6 同步要求

AIPDD 发布 V171 和 480p offering 查询修正后，必须触发：

```http
POST /api/channel/{AIPDD_CHANNEL_ID}/aipdd/sync
```

同步后检查快照：

```text
capabilities[].pricing.byResolution.480p
```

如果最新认证目录或本地快照没有 480p，NewAPI 继续返回 `unsupported_resolution` 属于预期行为，不能在下游伪造价格。

### 5.7 验收测试

- 同步含 480p 的目录后，运行时能力保留 `ByResolution["480p"]`。
- 纯文本 480p 请求选择 `hasReferenceVideo=false` 价格。
- 带参考视频的 480p 请求选择 `hasReferenceVideo=true` 价格。
- 480p 小数时长和 `frames / fps` 的向上取整与 AIPDD 一致。
- 上游请求体保持 `resolution: "480p"`。
- VIP、标准版、轻量版在目录包含 480p 时显示该选项。
- 高性价比版目录没有 480p 时不得错误显示。
- 旧快照没有 480p、新目录有 480p 时，同步后立即生效。
- PLATFORM 和 BYOK 分别使用各自认证目录中的 480p 价格。

---

## 6. 推荐发布顺序

### 阶段一：完成 AIPDD 上游

1. 发布 V171 数据迁移。
2. 同步发布 `ModelOfferingMapper` 对 Seedance 480p→480p 增强 offering 的特殊查询支持。
3. 将 BYOK 目录语义改为：

```text
billingMode = BYOK
billingDimension = hasReferenceVideo
billingFormula = standardMinusProviderCost
```

4. 验证认证目录分别使用 PLATFORM Key 和 BYOK Key 返回正确价格。

### 阶段二：完成 NewAPI

1. 为 AIPDD 目录快照增加 API Key 指纹隔离。
2. 处理旧快照兼容和失败关闭策略。
3. 解析、保存并审计 `billingMode`、`billingDimension`、`billingFormula`。
4. 增加 480p 精确计费和请求转发测试。
5. Playground 优先使用目录中的分辨率列表。

### 阶段三：同步和验证

1. 部署 AIPDD。
2. 部署 NewAPI。
3. 使用当前 AIPDD 渠道 Key 执行目录同步。
4. 检查同步结果的 revision、billing mode 和 480p 价格矩阵。
5. 分别验证 PLATFORM、BYOK、无参考视频、有参考视频和 480p 请求。

---

## 7. 不在本次改造范围内的行为

- 不在 NewAPI 中复制 AIPDD 的 RMB、AWCoin 或 provider 成本计算公式。
- 不将 Seedance 视频计费迁移到面向 token 的 `tiered_expr`。
- 不允许 NewAPI 根据本地默认价格伪造缺失的 480p SKU。
- 不由 NewAPI 执行 AIPDD 的质量增强步骤。
- 不因目录价格变化修改已经创建任务的计费快照。
- 不在任何数据库、日志或错误响应中暴露 AIPDD API Key。

---

## 8. 完成检查清单

- [ ] AIPDD PLATFORM/BYOK 认证目录返回不同但正确的价格。
- [ ] BYOK 目录不再使用 `enhancementOnly`。
- [ ] NewAPI 快照按 Base URL 和 API Key 指纹隔离。
- [ ] 不匹配身份的快照不会被失败回退使用。
- [ ] 多 Key 渠道不能混用不同计费身份。
- [ ] NewAPI 任务快照记录目录 revision 和计费模式。
- [ ] Seedance 480p→480p 出现在适用模型的认证目录中。
- [ ] NewAPI 可以提交、计费和查询 480p 任务。
- [ ] Playground 分辨率选项与目录一致。
- [ ] 所有新增测试通过。
