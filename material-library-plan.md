# 素材库管理 + Playground @ 引用素材 — 实现计划

## Context（背景）

当前 playground（操练场）的 image/video 模式支持上传参考媒体（reference media），但每次都要现场上传、用完即弃，无法沉淀复用。用户希望新增一个素材库：把常用的图片/视频/音频素材集中管理，并能在 playground 输入框通过 @ 快速选中已有素材，作为本次生成的参考附件，免去重复上传。

### 产品决策

1. 素材类型：媒体文件（图片 / 视频 / 音频）
2. 引用方式：@ 选中后作为「生成参考附件」参与生成（复用现有 SeedanceReference / reference 机制）
3. 归属权限：用户私有（按 user_id 隔离，登录用户即可访问，参考 Token 模式）
 4. 存储方式：**素材库只存链接，不存文件**。上传时根据模型渠道选择存储后端，最终将可访问链接存入 DB 的 `url` 字段。两种存储后端：
   - **AIPDD 模型**：文件传到 AIPDD OSS（复用 `UploadFileToOSS`），得到公网 url 入库。
   - **其他模型（预留）**：文件存本地磁盘（`data/materials/{user_id}/{uuid}.ext`），通过 `/pg/material/file/:id` 鉴权路由返回静态文件链接入库。
   素材库对外统一只暴露 `url` 字段，使用方不感知存储后端差异。

### 预期结果

用户在「素材库」页面上传/管理媒体素材；在 playground image/video 模式点击 @ 弹出素材选择器，选中后该素材以其 `url` 作为远程附件加入，提交生成时上游直接拉取该 url。

---

## 架构方案概述

- **存储**：素材库只存链接。DB 表 `materials` 存元数据 + `url`（可访问链接）。上传时根据渠道选择后端：
  - AIPDD：文件上传到 OSS（复用 `relay/channel/task/aipdd.UploadFileToOSS`），得公网 url 入库。
  - 其他模型（预留）：文件存本地 `data/materials/{user_id}/{uuid}.ext`，通过 `/pg/material/file/:id` 鉴权路由访问，链接入库。
  - DB 结构不变，`url` 字段统一存链接，使用方不感知后端差异。
- **后端**：新增 Material 模型 + CRUD + Controller + 路由 + 静态文件路由，全部按 `user_id` 隔离，路由挂在 `/pg`（UserAuth）下。
- **前端管理页**：镜像 redemption-codes 整套，新增 `features/materials/` + 路由 + 侧边栏菜单（用户可见分组）。
- **playground @ 引用**：给通用附件系统加 `addRemote`（接纳「已有 url、无 sourceFile」的远程附件），把现有仅插入 @ 文本的按钮升级为 cmdk 素材选择器弹窗；选中即 `addRemote`。提交链路天然保留远程 url（`prompt-input.tsx:756-772` 已确认非 blob url 原样保留），video 模式 `resolveSeedanceReferenceURLs` 仅对 `sourceFile` 上传，远程素材天然跳过上传。

---

## 后端改动

### 1. 新建 `model/material.go`

GORM 模型（遵循 Rule 2 三库兼容：时间用 `int64`+`bigint`；Rule 6：可选标量用指针 + `omitempty`）。结构镜像 `model/redemption.go`。

```go
type Material struct {
    Id          int            `json:"id"`
    UserId      int            `json:"user_id" gorm:"index"`
    Name        string         `json:"name" gorm:"type:varchar(191);index"`  // 展示名，可改
    Type        string         `json:"type" gorm:"type:varchar(16);index"`   // image|video|audio
    MimeType    string         `json:"mime_type" gorm:"type:varchar(128)"`
    FileName    string         `json:"file_name" gorm:"type:varchar(255)"`   // 原始上传文件名
    Url         string         `json:"url" gorm:"type:varchar(1024)"`        // 可访问链接（OSS 公网 url 或本地静态文件 url）
    StorageType string         `json:"storage_type" gorm:"type:varchar(16);default:oss"` // oss|local，预留扩展
    FileSize    int64          `json:"file_size" gorm:"bigint"`
    Width       *int           `json:"width,omitempty"`     // 可选元数据，MVP 可留空
    Height      *int           `json:"height,omitempty"`
    Duration    *float64       `json:"duration,omitempty"`  // 秒，可选
    Status      int            `json:"status" gorm:"default:1"`
    CreatedTime int64          `json:"created_time" gorm:"bigint"`
    UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
    DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

const (
    MaterialTypeImage = "image"
    MaterialTypeVideo = "video"
    MaterialTypeAudio = "audio"

    StorageTypeOSS   = "oss"
    StorageTypeLocal = "local"
)
```

**CRUD 方法**：
- `GetMaterialsByUser(userId, startIdx, num) ([]*Material, int64, error)`
- `SearchMaterialsByUser(userId int, keyword, typeFilter string, startIdx, num int) (...)` — name LIKE + 可选 type =
- `GetMaterialByIdAndUser(id, userId int) (*Material, error)` — 改名/删除前校验归属
- `(m *Material) Insert() error`
- `(m *Material) UpdateName() error` — `Select("name","updated_time")`
- `DeleteMaterialByIdAndUser(id, userId int) error` — 软删

### 2. 新建 `controller/material.go`

统一响应 `common.ApiSuccess/ApiError`；分页 `common.GetPageQuery(c)`；用户 id `c.GetInt("id")`。镜像 `controller/redemption.go`。

**上传抽象层**：抽一个 controller 包内辅助函数 `uploadMaterialFile(file multipart.File, header *multipart.FileHeader)`，内部按渠道决定存储方式：
- 当前唯一实现：取 AIPDD channel + key → `taskaipdd.UploadFileToOSS` → 返回 OSS 公网 url + `StorageTypeOSS`。
- 后续扩展：对于其他模型渠道，写本地文件 → 返回 `/pg/material/file/:id` 路径 + `StorageTypeLocal`。

```go
func uploadMaterialFile(file multipart.File, header *multipart.FileHeader) (url string, storageType string, err error)
```

- `UploadMaterial(c)` — `c.FormFile("file")` → 30MB 校验（沿用 `playgroundReferenceUploadMaxBytes`）→ 调用 `uploadMaterialFile` 得 url + storageType → 按 Content-Type 前缀判定 Type（`image/`/`video/`/`audio/`，非这三类拒绝）→ Insert → 返回 material（含 url）。同时 `getPlaygroundAIPDDUploadChannel()` 从私有函数改为 controller 包内可复用（去掉私有前缀或导出）。
- `GetMaterials(c)` / `SearchMaterials(c)` — 分页列表 / 关键词+类型搜索
- `UpdateMaterial(c)` — ShouldBindJSON 改 name，`GetMaterialByIdAndUser` 校验归属
- `DeleteMaterial(c)` — `DeleteMaterialByIdAndUser`（软删 DB；存储文件不自动清理，见风险点）

### 3. 修改 `router/relay-router.go`

在现有 `playgroundUtilityRouter`（前缀 `/pg`、已挂 `UserAuth()`，约 64-70 行）内追加：

```go
// CRUD
playgroundUtilityRouter.POST("/material/upload", controller.UploadMaterial)
playgroundUtilityRouter.GET("/material", controller.GetMaterials)
playgroundUtilityRouter.GET("/material/search", controller.SearchMaterials)
playgroundUtilityRouter.PUT("/material", controller.UpdateMaterial)
playgroundUtilityRouter.DELETE("/material/:id", controller.DeleteMaterial)

// 本地存储文件的静态访问路由（传入 material id，按 user_id 鉴权后返回文件）
playgroundUtilityRouter.GET("/material/file/:id", controller.ServeMaterialFile)
```

（OSS 文件直接通过其公网 url 访问，不需要 `/material/file` 路由。）

### 4. 修改 `model/main.go`

`migrateDB()` 的 `AutoMigrate(...)` 列表和 `migrateDBFast()` 的 `migrations` 切片两处都加 `&Material{}`（项目有两套迁移函数，漏一处 fast 模式不建表）。三库由 GORM `AutoMigrate` 处理。

### 5. （可选）i18n `i18n/keys.go` + `en/zh` JSON

加 `MsgMaterialNameLength`、`MsgMaterialUploadFailed`、`MsgMaterialTypeUnsupported` 等；MVP 也可先复用通用 key。

---

## 前端改动

### 6. 新建 `web/default/src/features/materials/`（镜像 redemption-codes/）

```
index.tsx                         // MaterialsProvider + SectionPageLayout，标题 t('Material Library')
api.ts                            // 端点用 /pg 前缀（非 /api）
types.ts / constants.ts / lib.ts  // zod schema、类型枚举、accept、30MB、formatSize
components/
  materials-provider.tsx          // open/currentRow/refreshTrigger
  materials-table.tsx             // DataTablePage；image 列显示 <img src=url> 缩略图，video/audio 显示图标
  materials-columns.tsx
  materials-primary-buttons.ts
  materials-mutate-drawer.tsx     // 改名
  materials-delete-dialog.tsx
  materials-dialogs.tsx
```

**api.ts 端点**（注意 `/pg` 前缀）：

```ts
api.post('/pg/material/upload', formData)               // multipart
api.get(`/pg/material?p=${p}&page_size=${ps}`)
api.get(`/pg/material/search?keyword=&type=&p=&page_size=`)
api.put('/pg/material', { id, name })
api.delete(`/pg/material/${id}`)
```

### 7. 新建路由 `web/default/src/routes/_authenticated/materials/index.tsx`

镜像 redemption 路由，但权限放宽到登录用户（不做 `role < ROLE.ADMIN` 校验，`_authenticated` 布局已保证登录态）。

### 8. 修改 `web/default/src/hooks/use-sidebar-data.ts`

把「素材库」菜单项加到用户可见分组（general/personal，紧邻 Playground，不是 admin 分组）：

```ts
{ title: t('Material Library'), url: '/materials', icon: ImageIcon }
```

### 9. 修改 `web/default/src/components/ai-elements/prompt-input.tsx`（新增 `addRemote`）

复用现有附件系统接纳「已有 url、无 sourceFile」的远程附件。三处改动：

1. `AttachmentsContext` 类型（约 99 行）加 `addRemote: (item: { url: string; mediaType?: string; filename?: string }) => void`。
2. `PromptInputProvider`（199-246）实现 `addRemote`（`setAttachements` 追加一条 `{ id: nanoid(), type:'file', url, mediaType, filename }`，不设 `sourceFile`）。
3. Playground 用的是本地 `PromptInput`（无 Provider 包裹），第 3 处是必经路径；第 2 处保持两套实现一致。`PromptInputAttachment`（302 行）已支持用 `data.url` 渲染 image 缩略图，video/audio 显示占位图标。

### 10. 修改 `web/default/src/features/playground/components/playground-input.tsx`（@ 升级为素材选择器）

- 现有 `handleInsertReferenceMarker`（208-213）仅插入 @ 文本 → 改为打开素材选择器弹窗。
- @ 按钮当前仅 `isVideoMode`（512-523）→ 改为 `isImageMode || isVideoMode` 都显示。
- 弹窗复用项目内置 `components/ui/command.tsx`（cmdk）做 `CommandDialog`：`useQuery` 拉当前用户素材（按 mode 过滤 type），搜索走 `searchMaterials`；选中 → `usePromptInputAttachments().addRemote({ url: m.url, mediaType: m.mime_type, filename: m.file_name })` → 关闭。
- 作用域注意：素材选择器须渲染在 `<PromptInput>...</PromptInput>` 子树内（`usePromptInputAttachments` 依赖 context）。
- 交互：按钮触发为主（MVP）；textarea 输入末尾 @ 自动弹出为可选增强。

### 11. `web/default/src/features/playground/index.tsx`（确认链路）

- **video 模式**：理论零改动。`resolveSeedanceReferenceURLs`（约 369 行）仅当 `sourceFile` 存在才上传，远程素材（无 `sourceFile`）直接用其 url 转 `SeedanceReference`；`validateVideoInput` 按 `references.length` 计数，远程素材有合法 `url`+`mediaType` 可被 `getSeedanceReferenceKind` 识别，不会误报。
- **image 模式**：现有 `handleSendMessage`（约 216-221）image 分支只取 `sourceFile` 转 base64，会忽略远程附件。如需 image 模式也能 @ 素材，需扩展：远程 url 附件 fetch → base64（或直传 url，取决于 image 生成上游是否接受 url）。作为本阶段一个明确子任务；若上游只接受 base64，则提交时对素材 url 做 `fetch→base64`。

### 12. 前端 i18n

`web/default/src/i18n/locales/{en,zh,...}.json` 补 flat key：`"Material Library"`、`"Upload material"`、`"Select material"`、`"No materials yet"` 等（至少 en + zh）。

---

## 执行步骤

---

### 步骤 1 — 后端：Model + Controller + 路由 + 迁移

**目标**：用 curl / Postman 可完成素材上传→列表→改名→删除全流程，DB 正常落表。

#### 1.1 新建 `model/material.go`

- [ ] 定义 `Material` 结构体（GORM 模型，字段见上文）
- [ ] 定义 `MaterialTypeImage/Video/Audio` 常量
- [ ] 实现 `GetMaterialsByUser(userId, startIdx, num)`
- [ ] 实现 `SearchMaterialsByUser(userId, keyword, typeFilter, startIdx, num)`
- [ ] 实现 `GetMaterialByIdAndUser(id, userId)`
- [ ] 实现 `(m *Material) Insert()`
- [ ] 实现 `(m *Material) UpdateName()`
- [ ] 实现 `DeleteMaterialByIdAndUser(id, userId)`（软删）

#### 1.2 新建 `controller/material.go`

- [ ] 实现上传抽象层 `uploadMaterialFile(file, header)` → 当前走 AIPDD OSS，返回 url + `StorageTypeOSS`
- [ ] `UploadMaterial` — FormFile → 30MB 校验 → 调用 `uploadMaterialFile` 得 url + storageType → 按 MIME 前缀判定 type → Insert → 返回
- [ ] `GetMaterials` — 分页列表
- [ ] `SearchMaterials` — keyword + type 过滤
- [ ] `UpdateMaterial` — ShouldBindJSON → GetMaterialByIdAndUser 校验归属 → UpdateName
- [ ] `DeleteMaterial` — DeleteMaterialByIdAndUser
- [ ] `ServeMaterialFile` — 按 id 查 material → user_id 鉴权 → 按 storage_type 返回文件或 404（本地存 储返回文件内容；OSS 返回重定向或不支持）
- [ ] 可选：对 MIME type 非 image/video/audio 返回错误

#### 1.3 修改 `model/main.go`

- [ ] `migrateDB()` 的 `AutoMigrate(...)` 列表加 `&Material{}`
- [ ] `migrateDBFast()` 的 `migrations` 切片加 `&Material{}`

#### 1.4 修改 `router/relay-router.go`

- [ ] 在 `playgroundUtilityRouter` 组内注册 6 条路由（POST /upload, GET /, GET /search, PUT /, DELETE /:id, GET /file/:id）

#### 1.5 （可选）后端 i18n

- [ ] `i18n/keys.go` 加 key，`i18n/zh.json` / `i18n/en.json` 补文案

**验证**：`go build ./... && go vet ./...` → 启动服务 → curl 上传文件、列表、改名、删除、越权校验。

---

### 步骤 2 — 前端管理页：素材库界面

**目标**：浏览器侧边栏出现「素材库」菜单，进入后可上传/浏览/改名/删除素材。

#### 2.1 新建 `web/default/src/features/materials/`

- [ ] `api.ts` — 5 个端点函数，前缀 `/pg`
- [ ] `types.ts` — Material 类型、上传参数 zod schema
- [ ] `constants.ts` — accept 类型、大小限制（30MB）
- [ ] `lib.ts` — `formatSize` 等工具函数
- [ ] `components/materials-provider.tsx` — open / currentRow / refreshTrigger 状态
- [ ] `components/materials-columns.tsx` — 列定义（image 显示 `<img>` 缩略图，video/audio 显示图标）
- [ ] `components/materials-table.tsx` — DataTablePage
- [ ] `components/materials-primary-buttons.ts` — 上传按钮
- [ ] `components/materials-mutate-drawer.tsx` — 改名抽屉
- [ ] `components/materials-delete-dialog.tsx` — 删除确认
- [ ] `components/materials-dialogs.tsx` — 弹窗组合
- [ ] `index.tsx` — 组装 Provider + SectionPageLayout

#### 2.2 新建路由 `web/default/src/routes/_authenticated/materials/index.tsx`

- [ ] 镜像 `redemption/index.tsx`，去掉 admin 校验

#### 2.3 修改 `web/default/src/hooks/use-sidebar-data.ts`

- [ ] 加 `{ title: t('Material Library'), url: '/materials', icon: ImageIcon }` 到用户可见分组

#### 2.4 前端 i18n

- [ ] `web/default/src/i18n/locales/zh.json` / `en.json` 补 Material Library、Upload material、Select material、No materials yet 等 key

**验证**：`bun run dev` → 浏览器打开 → 侧边栏有菜单 → 进入页面 → 上传 → 列表刷新 → 改名 → 删除。

---

### 步骤 3 — Playground @ 引用：素材选择器 + addRemote

**目标**：playground 输入框 @ 按钮弹出素材选择器，选中后素材以远程附件加入，提交时跳过重复上传。

#### 3.1 修改 `web/default/src/components/ai-elements/prompt-input.tsx`

- [ ] `AttachmentsContext` 类型加 `addRemote: (item: { url: string; mediaType?: string; filename?: string }) => void`
- [ ] `PromptInputProvider` 实现 `addRemote`（追加 `{ id: nanoid(), type:'file', url, mediaType, filename }`）
- [ ] 本地 `usePromptInputAttachments`（playground 用）同样支持 `addRemote`

#### 3.2 修改 `web/default/src/features/playground/components/playground-input.tsx`

- [ ] `handleInsertReferenceMarker` 改为打开素材选择器弹窗
- [ ] @ 按钮从仅 video 模式扩展为 image + video 模式
- [ ] 实现 `MaterialSelectorDialog`（基于 `components/ui/command.tsx` 的 CommandDialog）：
  - [ ] `useQuery` 拉取当前用户素材（按 mode 过滤 type：image 模式只列 image，video 模式列 image+video）
  - [ ] 搜索框实时调用 `searchMaterials`
  - [ ] 选中 → `addRemote({ url: m.url, mediaType: m.mime_type, filename: m.file_name })` → 关闭弹窗
- [ ] 确保弹窗渲染在 `<PromptInput>...</PromptInput>` 子树内

#### 3.3 确认/扩展 `web/default/src/features/playground/index.tsx`

- [ ] **video 模式**：确认 `resolveSeedanceReferenceURLs` 对远程附件（无 sourceFile）跳过上传，直接使用 url
- [ ] **image 模式**（可选）：如上游不接受 url，对远程素材 url 做 fetch → base64 补充到请求体

#### 3.4 端到端验证

> 起后端 `go run .` + 前端 `bun run dev`

- [ ] step1: 素材库上传图片 → DB 落表 url 为 OSS 地址
- [ ] step2: 浏览器直接访问 OSS url 可打开
- [ ] step3: playground video 模式 → @ → 选择器列出素材 → 选中 → 输入框上方出现附件 chip
- [ ] step4: 输入提示词 → 发送 → 网络面板无 `/pg/reference-media/upload` 调用 → 请求体 `SeedanceReference.url` = OSS url
- [ ] step5: 素材库改名/删除正常生效
- [ ] step6: 换账号登录看不到他人素材，删他人素材失败

---

## 验证方案

### 起服务

```bash
# 后端（项目根）：默认 SQLite，启动即 AutoMigrate 建 materials 表
go run .

# 前端（web/default）
bun install   # 首次
bun run dev
```

前置：后台需有一个 enabled 的 AIPDD channel（上传依赖它换 OSS url；与现有 reference-media 上传同一依赖）。

### 端到端链路

1. **上传**：侧边栏→素材库→上传图片（<30MB）→ 列表出现缩略图；DB `materials` 新增一行，`type=image`、`url` 为可访问链接（OSS 公网地址）、`storage_type=oss`。
2. **链接可达**：浏览器直接打开该 url 能看到图片。
3. **@ 选中**：playground 切 video 模式→点 @→选择器列出素材→选中→输入框上方出现附件 chip。
4. **作为参考提交**：填提示词→发送。网络面板确认没有对 `/pg/reference-media/upload` 的调用（远程素材跳过上传）；`/pg/video/generations` 请求体里 `SeedanceReference.url` = 素材 OSS url；上游生成任务正常推进。
5. **改名/删除**：改名后刷新生效；删除后列表消失、DB 软删（`deleted_at` 非空）。
6. **越权检查**：换账号登录，`GET /pg/material` 看不到他人素材；删除他人 id 失败（`GetMaterialByIdAndUser` 查不到）。

### 静态检查

```bash
go vet ./...
go build ./...
```

---

## 风险点与注意事项

1. **AIPDD channel 依赖**：当前唯一上传后端是 AIPDD OSS（同现有 reference-media 上传）。无可用 channel 时上传应返回清晰错误（复用 `playgroundReferenceUploadError` 风格）。后续接入本地存储后可降低此依赖。
2. **存储文件不随软删清理**：删除素材仅软删 DB 记录。OSS 文件保留（未确认 AIPDD 有删除 OSS 接口）；本地文件暂不清理（后续可通过定时任务清理 orphan 文件）。MVP 可接受少量残留文件。
3. **migrateDBFast 易漏**：两套迁移函数都要加 `&Material{}`。
4. **附件 context 作用域**：素材选择器组件必须在 `<PromptInput>` 子树内调用 `addRemote`/`usePromptInputAttachments`。
5. **image 模式引用为增量项**：video 模式天然可用；image 模式需处理远程 url→base64（视上游接口而定），不阻塞 video 主链路。
6. **每用户配额/容量**：MVP 不限素材数量与总容量；如需，后续在 `UploadMaterial` 加 `count(*) where user_id` 与总字节上限校验。
7. **本地存储目录**：`data/materials/` 需确保 Go 进程有读写权限；部署时挂载持久化卷。
8. **保护信息**：复用/新增文件中涉及 QuantumNous / new-api 的版权头、模块路径等保持不变（Rule 5）。

---

## 关键文件清单

### 新建

| 文件 | 说明 |
|------|------|
| `model/material.go` | GORM 模型 + 按 user_id 隔离 CRUD |
| `controller/material.go` | 上传(抽象层→OSS)/列表/搜索/改名/删除 + 本地文件静态访问 `ServeMaterialFile` |
| `web/default/src/features/materials/**` | 管理页整套（镜像 redemption-codes） |
| `web/default/src/routes/_authenticated/materials/index.tsx` | 登录态路由 |

### 修改

| 文件 | 说明 |
|------|------|
| `router/relay-router.go` | 注册 `/pg/material/*`（UserAuth） |
| `model/main.go` | 两处迁移注册 `&Material{}` |
| `web/default/src/components/ai-elements/prompt-input.tsx` | `AttachmentsContext.addRemote`（Provider + 本地） |
| `web/default/src/features/playground/components/playground-input.tsx` | @ 升级为素材选择器 |
| `web/default/src/features/playground/index.tsx` | 确认 video 零改动 / image 模式可选扩展 |
| `web/default/src/hooks/use-sidebar-data.ts` | 侧边栏菜单项 |
| `i18n/`、`web/default/src/i18n/locales/*.json` | 文案 |
