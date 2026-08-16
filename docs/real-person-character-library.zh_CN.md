# 真人形象素材库实施说明

## 1. 结论与边界

本实现把真人形象作为角色库中的独立私有来源 `volc_real_person`，而不是普通素材标签。数据关系保持为“一位真人 = 一个火山 `LivenessFace` Asset Group = 一个可用 Image Asset ID = 一条 NewAPI 角色记录”。生成请求最终仍引用 `asset://{asset_id}`，但真人 Asset ID 不能绕过本地授权闸门直接透传。

公开素材池继续使用火山官方虚拟人像；自然人无论是否公众人物，都必须由肖像权人本人确认授权范围并完成火山身份核验。本系统不支持把街拍、图库真人或任意公网人脸 URL 注册成共享“路人真人”。

正式启用前必须确认火山账号已开通真人 Assets API 集成权益。若只有控制台权益，不能把当前链路当作端到端 API 入库方案，也不建议控制台与 API 混用。

角色库导航模块仍由系统侧边栏配置控制；管理员需要同时启用该模块、角色库主开关和真人开关。真人开关不会绕过侧边栏与 provider 配置的双重可见性控制。

参考的火山文档：

- [录入真人形象素材](https://docs.volcengine.com/docs/82379/2315856?lang=zh)
- [查询真人核验结果](https://docs.volcengine.com/docs/82379/2333588?lang=zh)
- [版权和人像素材使用规则](https://docs.volcengine.com/docs/82379/2525200?lang=zh)
- [Seedance 高级创作权益包](https://docs.volcengine.com/docs/82379/2377608?lang=zh)

## 2. 火山契约

真人链路使用以下契约：

1. `CreateVisualValidateSession` 创建 H5 核验会话，保存加密后的 `BytedToken` 和 H5 地址。
2. 肖像权人先在 NewAPI 落地页查看并确认用途、地区、平台、行业、商业使用开关和有效期，再跳转火山 H5。
3. 火山回调与本地 `state`、`BytedToken` 做绑定校验；`GetVisualValidateResult` 返回真人 `GroupId`。
4. 使用 `GetAssetGroup` 验证组归属、Project、`GroupType=LivenessFace` 和 Active 状态。
5. 使用 `ListAssets` 且明确传入 `GroupType=LivenessFace`、`GroupIds`、`ProjectName`。只有“恰好一个 Active Image”时才激活角色；没有则继续同步，多个则进入 `ambiguous` 阻断状态。
6. 后续每次生成都使用唯一 `ProviderAssetID`，而不是 Group ID。Group ID 只用于归属、一致性和生命周期反查。

角色库中的火山 AK/SK、区域和 Project 用于 Assets 控制面。管理员无需再单独创建或绑定 `DoubaoVideo` 通道；角色视频会自动使用现有 AIPDD Seedance 渠道，保留 AIPDD 的公开档位、计费和增强链路，再由 AIPDD 映射到官方 Ark Seedance 模型。AIPDD 内部 Ark 凭证必须与角色资产属于同一火山账号和 Project，确保 `asset://` 可见。

## 3. 本地模型与状态

新增两类审计记录：

- `virtual_character_authorizations`：所有者、有效期、商业使用、用途/地区/平台/行业、协议版本、肖像权人确认时间、同意凭证哈希、火山组/Asset 状态、上次反查时间、撤回和过期时间。
- `virtual_character_task_references`：一个任务使用的全部角色和 Asset ID，以及生成当时不可变的授权快照。它支持“一个真人 + 多个官方虚拟人像”的组合审计。

授权主要状态为：

```text
pending -> synchronizing -> active
                       \-> ambiguous / provider_unavailable
active -> expired / revoked
任何未完成状态 -> failed
```

`creating`、`blocked`、`deleting` 是角色可用性状态。只有角色为 `active`、授权为 `active`、授权尚未到期、授权证据完整、火山组与 Asset 状态有效时，才允许创建新视频任务。

迁移对历史 `volc_real_person` 行采用拒绝默认：没有结构化授权记录的旧行会被标记为 blocked，并要求重新授权；已有新授权记录的行不会在后续启动中被重复封禁。

## 4. 安全主链路

支持两种调用方式：

- `character_id`：先反查当前用户可访问的角色，再注入该角色唯一的 `asset://`。
- 直接 `asset://`：每一个 ID 都必须先在本地角色库唯一注册；未知、重复注册、非本人私有真人、过期或火山失效的 ID 均拒绝。

校验中间件同时覆盖 `/v1/video/generations`、`/pg/video/generations`、`/v1/videos`、remix、Kling 和 Jimeng 的视频创建入口；入口或模型不支持 Seedance 角色素材时采用拒绝默认。使用 `character_id` 时不能再附带其他参考素材；直接 `asset://` 则允许组合多个已注册引用，并逐一鉴权和记录快照。

每次请求执行两次授权检查：第一次在创建本地任务绑定前；第二次在绑定后、进入转发处理前，用于封住撤权/过期/下线并发窗口。第二次失败会回滚仍处于 submitting 的绑定和多引用占位。删除或撤权会立即拒绝新请求，但会等待已进入处理中的任务终态后再清理火山 Asset Group。

火山状态默认缓存两分钟；缓存过期的真人请求会同步调用火山反查。维护任务还会周期检查授权到期、火山失效、待激活 Asset、过期 H5 会话、删除重试和 90 天任务审计保留。

## 5. API 与界面

主要接口：

- `POST /v1/virtual-characters/validation-sessions`：创建真人授权/核验会话。
- `GET /v1/virtual-characters/validation-sessions/{id}`：查询会话。
- `POST /v1/virtual-characters/{id}/sync`：所有者主动同步火山状态。
- `DELETE /v1/virtual-characters/{id}`：撤回真人授权并进入清理队列。
- `/api/virtual-characters` 下提供相同的控制台接口。

创建会话的授权体示例：

```json
{
  "name": "签约演员 A",
  "language": "zh",
  "authorization": {
    "valid_until": 1818201600,
    "commercial_use_allowed": true,
    "purposes": ["产品演示"],
    "regions": ["中国大陆"],
    "platforms": ["品牌官网", "社交媒体"],
    "industries": ["零售"],
    "agreement_accepted": true
  }
}
```

控制台角色库拆分为“公共角色 / 我的虚拟形象 / 我的真人 / 任务历史”。“我的真人”展示授权期限、商业属性和火山状态，支持身份核验、手动同步、创建视频和撤回授权。管理员设置包含独立真人额度、Premium 提示以及火山 Assets AK/SK、区域和 Project；不再显示视频通道绑定项。生成弹窗会从当前用户可用模型中筛选名称包含 `seedance`（不区分大小写）的模型。

## 6. 保守决策与验收

- 火山 Group 中存在多个 Active Image 时不自动挑选，避免把同组不同妆造误认为同一个固定角色。
- 直接 `asset://` 未在本地注册时拒绝，而不是假设它是安全的官方素材；官方目录应先同步进公共角色库。
- 授权范围目前作为创建时的可审计合同与生成快照保存。系统尚未从自然语言提示自动推断行业/用途，因此无法自动证明提示词符合每个业务范围；调用方和内容审核仍须履行用途合规责任。
- 本地只保存元数据、状态和证据哈希，不复制或对外分发火山平台素材。

上线验收至少应覆盖：角色库 Assets 与视频上游同账号同项目联调、名称包含 `seedance` 的可用模型筛选、真人 H5 成功/拒绝/过期、零/一/多 Active Image、跨用户直接 Asset ID、任务提交/重试/轮询、撤权并发、授权到期、火山下线、组合真人与官方虚拟人像、清理重试，以及 SQLite/MySQL/PostgreSQL 三种迁移。
