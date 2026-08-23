# AP Seedance 2.0 / 2.5 API 文档

Seedance 2.0 与 Seedance 2.5 共用任务创建和查询入口，但使用不同的模型名、参数集合、默认值和素材约束。本文在同一份文档内分版本说明；请勿跨版本复用默认值。

## 公共调用方式

~~~http
Authorization: Bearer <NEWAPI_TOKEN>
Content-Type: application/json
~~~

创建任务：

~~~http
POST /v1/videos
POST /v1/video/generations
~~~

查询任务：

~~~http
GET /v1/videos/{task_id}
GET /v1/video/generations/{task_id}
GET /v1/video/generations/{task_id}/result
~~~

任务为异步执行。创建成功后保存返回的 `id`，再轮询查询接口，直到进入成功或失败终态。

## 版本差异速查

| 项目 | Seedance 2.0 | Seedance 2.5 |
|---|---|---|
| 公开模型 | VIP、标准版、轻量版、高性价比版 | `AP Seedance-2.5 标准版` |
| 默认分辨率 | 请求指定或通过宽高推断 | `720p` |
| 比例 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4` | 2.0 比例 + `21:9`、`adaptive`，默认 `adaptive` |
| 时长 | 正数，默认 `5` 秒 | `-1` 或 `4`–`30`，默认 `-1` |
| 模式字段 | 通过素材角色表达 | `omni_reference_task_type` |
| `seed` | 支持 | 不支持 |
| `service_tier` | 支持 | 不支持 |
| 自动时长结算 | 不适用 | 先按 30 秒预授权，按官方实际时长结算 |

## 价格

价格和可用档位以动态目录为准：

~~~http
GET /api/pricing
~~~

同一模型的价格可能因分辨率、是否包含参考视频以及 PLATFORM/BYOK 模式而不同。客户端不要硬编码价格，也不要用无参考视频价格替代参考视频价格。

## 素材库与真人角色库调用

普通素材库和角色库是两套不同的能力：

- 普通素材库保存图片、视频和音频文件，生成时使用素材记录返回的公网 `url`。
- 角色库保存火山角色 Asset，生成时使用 NewAPI 的 `character_id`，或者使用已登记且已授权的 `asset://{asset_id}`。
- 真人角色必须先完成本人 H5 身份核验，再上传同一人的肖像图片。普通素材上传不能代替真人核验。

### 1. 普通素材库（控制台 / Playground）

普通素材库接口位于 `/pg`，使用控制台用户会话鉴权，不接受 NewAPI Bearer Token：

~~~http
POST   /pg/material/upload
GET    /pg/material
GET    /pg/material/search
PUT    /pg/material
DELETE /pg/material/{id}
~~~

上传使用 `multipart/form-data`：

| 字段 | 必填 | 说明 |
|---|---:|---|
| `file` | 是 | 图片、视频或音频文件 |
| `source_type` | 否 | 普通上传使用 `material` |

上传成功后使用响应 `data.url` 作为 Seedance 参考素材：

~~~json
{
  "success": true,
  "data": {
    "id": 123,
    "name": "reference.png",
    "type": "image",
    "source_type": "material",
    "url": "https://example.com/static/materials/reference.png"
  }
}
~~~

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "content": [
    { "type": "text", "text": "保持参考图中的主体外观" },
    {
      "type": "image_url",
      "image_url": { "url": "https://example.com/static/materials/reference.png" },
      "role": "reference_image"
    }
  ],
  "resolution": "720p",
  "ratio": "adaptive",
  "duration": 4,
  "omni_reference_task_type": "reference"
}
~~~

纯 API Key 调用方不能使用 `/pg/material`。这类调用方应把文件放在 Ark 可访问的公网 URL，再把 URL 写入 `content`。无论素材来自平台素材库还是外部 URL，仍需满足所选 Seedance 版本的格式、大小、数量和模式限制。

### 2. 查询官方角色、虚拟角色和真人角色

角色库 `/v1` 接口使用 NewAPI Token：

~~~http
Authorization: Bearer <NEWAPI_TOKEN>
~~~

角色来源：

| `source_type` | 含义 | 范围 |
|---|---|---|
| `volc_preset` | 平台同步的火山官方角色 | `scope=public` |
| `volc_aigc` | 当前用户上传创建的虚拟角色 | `scope=private` |
| `volc_real_person` | 当前用户完成核验的真人角色 | `scope=private` |

查询可用官方角色：

~~~http
GET /v1/virtual-characters?scope=public&source_type=volc_preset&status=active
~~~

查询当前用户可用的真人角色：

~~~http
GET /v1/virtual-characters?scope=private&source_type=volc_real_person&status=active
~~~

还可以使用 `keyword`、`gender`、`nationality`、`age_band` 和分页参数过滤。角色记录中的关键字段为：

~~~json
{
  "id": 321,
  "scope": "private",
  "source_type": "volc_real_person",
  "name": "签约演员 A",
  "status": "active",
  "validation_status": "accepted",
  "provider_asset_id": "provider-asset-id"
}
~~~

只有 `status=active` 的角色可以创建新任务。真人角色还必须具有有效授权且火山 Asset 状态正常。

### 3. 创建个人虚拟角色（非真人）

虚拟角色使用一张非真人角色图片创建，不走真人身份核验：

~~~bash
curl -X POST "$BASE_URL/v1/virtual-characters" \
  -H "Authorization: Bearer $NEWAPI_TOKEN" \
  -F "file=@character.png" \
  -F "name=虚拟角色 A" \
  -F "description=原创非真人角色" \
  -F "tags=[\"虚拟角色\"]"
~~~

创建响应的 `data.id` 即 `character_id`。处理期间角色通常为 `creating`；轮询 `GET /v1/virtual-characters/{id}`，直到 `status=active` 后再用于视频生成。

### 4. 创建真人角色

真人入库必须按以下顺序执行。

#### 4.1 创建本人核验会话

~~~http
POST /v1/virtual-characters/validation-sessions
Authorization: Bearer <NEWAPI_TOKEN>
Content-Type: application/json
~~~

~~~json
{
  "name": "签约演员 A",
  "description": "品牌宣传片演员",
  "tags": ["真人", "演员"],
  "language": "zh"
}
~~~

`language` 支持 `zh`、`en`，默认 `zh`。响应示例：

~~~json
{
  "success": true,
  "data": {
    "id": "validation-session-id",
    "status": "pending",
    "launch_url": "https://example.com/api/virtual-characters/validation/launch/...",
    "expires_at": 1787185800,
    "character_id": 321
  }
}
~~~

必须由肖像权人本人打开 `launch_url` 并完成火山 H5 核验。不得把 NewAPI Token 交给被核验人。

#### 4.2 查询核验结果

~~~http
GET /v1/virtual-characters/validation-sessions/{session_id}
Authorization: Bearer <NEWAPI_TOKEN>
~~~

会话状态为 `pending`、`succeeded`、`failed` 或 `expired`。只有 `succeeded` 才能继续上传肖像。仍在等待中的会话可以取消：

~~~http
DELETE /v1/virtual-characters/validation-sessions/{session_id}
~~~

#### 4.3 上传已核验人员的肖像

H5 成功后先查询角色：

~~~http
GET /v1/virtual-characters/{character_id}
~~~

当响应为 `asset_upload_required=true` 时，上传与已核验人员一致的图片：

~~~bash
curl -X POST "$BASE_URL/v1/virtual-characters/321/asset" \
  -H "Authorization: Bearer $NEWAPI_TOKEN" \
  -F "file=@portrait.jpg"
~~~

上传后轮询 `GET /v1/virtual-characters/321`。也可以由所有者主动同步一次火山状态：

~~~http
POST /v1/virtual-characters/321/sync
~~~

只有角色达到以下条件后才能生成视频：

- `status=active`
- `validation_status=accepted`
- `asset_upload_required=false` 或未返回
- `provider_asset_id` 非空
- `authorization.status=active`

撤回真人授权并进入清理流程：

~~~http
DELETE /v1/virtual-characters/321
~~~

### 5. 在 Seedance 请求中使用角色

Seedance 2.0 和 Seedance 2.5 都支持角色库，但所选模型名必须包含 `Seedance`。

#### 5.1 单角色：使用 `character_id`

这是推荐的单角色调用方式。使用兼容入口 `POST /v1/video/generations`；NewAPI 会鉴权并把角色转换成对应的 `asset://` 引用。由于角色中间件使用兼容请求结构，`resolution`、`ratio` 等 Seedance 参数放入 `metadata`：

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "prompt": "角色在城市街道上自然行走",
  "character_id": 321,
  "duration": 4,
  "metadata": {
    "resolution": "720p",
    "ratio": "adaptive"
  }
}
~~~

使用 `character_id` 时不能再提供 `image`、`images`、首尾帧或 `content` 中的其他参考图片、视频和音频，否则返回 `character_reference_conflict`。

Seedance 2.0 的写法相同，只需换成 2.0 模型并使用 2.0 支持的比例和时长。例如：

~~~json
{
  "model": "AP Seedance-2.0 轻量版",
  "prompt": "角色看向镜头并挥手",
  "character_id": 321,
  "duration": 5,
  "metadata": {
    "resolution": "480p",
    "ratio": "16:9"
  }
}
~~~

旧字段 `character_asset_id` 会被忽略，不要继续使用。

#### 5.2 多角色：使用已登记的 `asset://`

需要组合多个角色时，可使用 `POST /v1/videos`，在官方 `content` 中直接引用各角色记录的 `provider_asset_id`：

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "content": [
    { "type": "text", "text": "两位角色在咖啡馆交谈" },
    {
      "type": "image_url",
      "image_url": { "url": "asset://provider-asset-a" },
      "role": "reference_image"
    },
    {
      "type": "image_url",
      "image_url": { "url": "asset://provider-asset-b" },
      "role": "reference_image"
    }
  ],
  "resolution": "720p",
  "ratio": "adaptive",
  "duration": 4,
  "omni_reference_task_type": "reference"
}
~~~

每个 `asset://` 都必须已在本地角色库唯一登记，且当前用户有权使用。未知 Asset、其他用户的私有真人、已撤权、过期、下线或火山失效的 Asset 都会被拒绝。直接 `asset://` 方式允许组合多个已授权角色，但仍受对应 Seedance 版本的素材数量和模式约束。

真人角色库的合规、状态和清理细节见 [真人形象素材库实施说明](./real-person-character-library.zh_CN.md)。

## Seedance 2.0

### 1. 模型

| 公开模型 | 可用分辨率 |
|---|---|
| `AP Seedance-2.0 VIP` | `480p`、`720p`、`1080p`、`4k` |
| `AP Seedance-2.0 标准版` | `480p`、`720p`、`1080p`、`4k` |
| `AP Seedance-2.0 轻量版` | `480p`、`720p`、`1080p` |
| `AP Seedance-2.0 高性价比版` | `1080p`、`4k` |

最终可用档位以 `GET /api/pricing` 返回的动态目录为准。Seedance 2.0 的 VIP、标准版和轻量版均提供 `480p`；高性价比版不提供 `480p`。

### 2. 创建任务

~~~http
POST /v1/videos
Authorization: Bearer <NEWAPI_TOKEN>
Content-Type: application/json
~~~

兼容入口：`POST /v1/video/generations`。

#### 2.1 请求字段

| 字段 | 类型 | 必填 | Seedance 2.0 说明 |
|---|---|---:|---|
| `model` | string | 是 | 上表中的公开模型名 |
| `prompt` | string | 条件必填 | 与 `content` 至少提供一个 |
| `content` | array | 条件必填 | 文本和参考素材列表 |
| `resolution` | string | 条件必填 | `480p`、`720p`、`1080p`、`4k`；受模型能力限制，未给 `width/height` 时必填 |
| `ratio` | string | 否 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4` |
| `duration` | number | 否 | 正数秒，默认 `5`，可使用小数 |
| `seconds` | number | 否 | `duration` 的兼容别名 |
| `width` / `height` | integer | 否 | 一起提供时可推断分辨率和比例 |
| `image` / `images` | string/object/array | 否 | 单图、多图兼容字段 |
| `generate_audio` | boolean | 否 | 是否生成音频；显式 `false` 会被保留 |
| `watermark` | boolean | 否 | 是否添加水印 |
| `output_format` | string | 否 | `mp4` 或 `mov` |
| `seed` | integer | 否 | Seedance 2.0 可用 |
| `service_tier` | string | 否 | Seedance 2.0 可用 |
| `priority` | integer | 否 | 调度优先级 |
| `callback_url` | string | 否 | 状态回调地址 |
| `return_last_frame` | boolean | 否 | 是否返回末帧信息 |
| `character_id` | integer | 否 | NewAPI 角色库兼容字段；使用 `/v1/video/generations` |

`width` 与 `height` 同时提供时，短边映射为：`480` → `480p`、`720` → `720p`、`1080` → `1080p`、`2160` → `4k`。新调用建议直接提交 `resolution` 和 `ratio`。

不要在 2.0 请求中使用 2.5 专属的 `adaptive`、`21:9`、`duration=-1` 或 `omni_reference_task_type`。

#### 2.2 `content` 结构

~~~json
[
  { "type": "text", "text": "电影感的海边日落" },
  {
    "type": "image_url",
    "image_url": { "url": "https://example.com/reference.png" },
    "role": "reference_image"
  }
]
~~~

| `type` | 内容字段 | 常用 `role` |
|---|---|---|
| `text` | `text` | 不需要 |
| `image_url` | `image_url.url` | `reference_image`、`first_frame`、`last_frame` |
| `video_url` | `video_url.url` | `reference_video` |
| `audio_url` | `audio_url.url` | `reference_audio` |

首尾帧模式不能与参考图片、参考视频或参考音频混用。兼容调用可以使用平台支持的公网 URL 或角色库 `asset://...` 素材。

#### 2.3 素材限制

- `content` 总素材数不超过 `12`。
- 参考图片不超过 `9` 张。
- 参考视频不超过 `3` 个。
- 参考音频不超过 `3` 个。
- 单个参考视频建议为 `2`–`15.2` 秒。
- 参考视频与参考音频总时长不超过 `15.2` 秒。
- 实际格式、大小和像素限制以当前模型能力契约和动态目录为准。

### 3. 请求示例

#### 3.1 文生视频

~~~json
{
  "model": "AP Seedance-2.0 标准版",
  "prompt": "雨后的霓虹街道，镜头缓慢向前移动",
  "resolution": "1080p",
  "ratio": "16:9",
  "duration": 5,
  "generate_audio": false,
  "output_format": "mp4"
}
~~~

#### 3.2 参考图生视频

~~~json
{
  "model": "AP Seedance-2.0 标准版",
  "content": [
    { "type": "text", "text": "保持角色外观，人物回头看向镜头" },
    {
      "type": "image_url",
      "image_url": { "url": "https://example.com/character.png" },
      "role": "reference_image"
    }
  ],
  "resolution": "1080p",
  "ratio": "9:16",
  "duration": 5
}
~~~

#### 3.3 首尾帧

~~~json
{
  "model": "AP Seedance-2.0 VIP",
  "content": [
    { "type": "text", "text": "从清晨自然过渡到黄昏" },
    { "type": "image_url", "image_url": { "url": "https://example.com/first.png" }, "role": "first_frame" },
    { "type": "image_url", "image_url": { "url": "https://example.com/last.png" }, "role": "last_frame" }
  ],
  "resolution": "1080p",
  "ratio": "16:9",
  "duration": 5
}
~~~

### 4. 查询任务

~~~http
GET /v1/videos/{task_id}
Authorization: Bearer <NEWAPI_TOKEN>
~~~

兼容入口：

- `GET /v1/video/generations/{task_id}`
- `GET /v1/video/generations/{task_id}/result`

创建响应示例：

~~~json
{
  "id": "video_task_123",
  "task_id": "video_task_123",
  "object": "video",
  "model": "AP Seedance-2.0 标准版",
  "status": "queued",
  "progress": 0,
  "created_at": 1787184000
}
~~~

成功响应示例：

~~~json
{
  "id": "video_task_123",
  "task_id": "video_task_123",
  "object": "video",
  "status": "completed",
  "model": "AP Seedance-2.0 标准版",
  "progress": 100,
  "created_at": 1787184000,
  "completed_at": 1787184060,
  "metadata": {
    "url": "https://example.com/result.mp4",
    "duration": 5,
    "output_format": "mp4",
    "resolution": "1080p"
  }
}
~~~

任务未结束时不会伪造 `completed_at`。调用方应识别 `queued`、`in_progress`、`completed`、`failed` 等状态，并为网络错误采用有限次数重试。

### 5. 计费与错误

- 创建任务时按模型、分辨率和时长预授权。
- PLATFORM 与 BYOK 单价可能不同；以 `GET /api/pricing` 为准。
- 在途任务使用创建时冻结的价格快照；重复查询或结算重放不会重复扣费。

| 错误 | 原因 |
|---|---|
| `invalid_resolution` | 所选模型不支持该分辨率 |
| `invalid_ratio` | 比例不属于 Seedance 2.0 支持集合 |
| `invalid_duration` | 时长不是有效正数 |
| `invalid_content` | 文本、素材数量或素材结构不合法 |
| `mixed_input_modes` | 首尾帧与参考素材混用 |
| `insufficient_quota` | 可用额度不足 |

## Seedance 2.5

Seedance 2.5 的参数、默认值和条件约束与 2.0 不同。不要沿用 2.0 的 5 秒默认时长、旧比例、`seed` 或 `service_tier`。

### 1. 模型

| 公开模型 | 上游模型 | 可用分辨率 |
|---|---|---|
| `AP Seedance-2.5 标准版` | `doubao-seedance-2-5-260628` | `480p`、`720p`、`1080p` |

客户端只提交公开模型名；上游模型名由平台绑定，不能由客户端覆盖。

### 2. 创建任务

~~~http
POST /v1/videos
Authorization: Bearer <NEWAPI_TOKEN>
Content-Type: application/json
~~~

兼容入口：`POST /v1/video/generations`。

#### 2.1 请求字段

| 字段 | 类型 | 必填 | 默认值 | Seedance 2.5 说明 |
|---|---|---:|---|---|
| `model` | string | 是 | — | 固定为 `AP Seedance-2.5 标准版` |
| `content` | array | 条件必填 | — | 与 `prompt` 至少提供一个 |
| `prompt` | string | 条件必填 | — | 本地转换为文本内容，不原样转发 Ark |
| `resolution` | string | 否 | `720p` | `480p`、`720p`、`1080p` |
| `ratio` | string | 否 | `adaptive` | `16:9`、`4:3`、`1:1`、`3:4`、`9:16`、`21:9`、`adaptive` |
| `duration` | integer | 否 | `-1` | `-1` 或 `4`–`30`；`-1` 表示由模型决定 |
| `omni_reference_task_type` | string | 否 | `auto` | `auto`、`reference`、`edit`、`extend`；无参考素材时不转发 |
| `generate_audio` | boolean | 否 | `true` | 是否生成音频；显式 `false` 会被保留 |
| `watermark` | boolean | 否 | `false` | 是否添加水印 |
| `output_format` | string | 否 | `mp4` | `mp4` 或 `mov` |
| `return_last_frame` | boolean | 否 | `false` | 是否返回末帧信息 |
| `callback_url` | string | 否 | — | 任务状态回调 URL |
| `execution_expires_after` | integer | 否 | `172800` | `3600`–`259200` 秒 |
| `priority` | integer | 否 | `0` | `0`–`9` |
| `safety_identifier` | string | 否 | — | 最长 64 个 ASCII 字符 |
| `tools` | array | 否 | — | 当前支持 `[{"type":"web_search"}]` |

`width`、`height`、`media_mode` 仅供 NewAPI 本地兼容转换，不会原样进入 Ark 请求。

#### 2.2 明确不支持的字段

以下字段不会公开或转发给 Seedance 2.5：

- `frames` 或其他 FPS 控制字段
- `seed`
- `camera_fixed`
- `draft`
- `service_tier`

提交这些字段会被拒绝或清理。客户端不要依赖静默忽略行为。

#### 2.3 `content` 结构

~~~json
[
  { "type": "text", "text": "镜头穿过雨后的未来城市" },
  {
    "type": "image_url",
    "image_url": { "url": "https://example.com/reference.png" },
    "role": "reference_image"
  },
  {
    "type": "audio_url",
    "audio_url": { "url": "https://example.com/reference.mp3" },
    "role": "reference_audio"
  }
]
~~~

| `type` | 内容字段 | 常用 `role` |
|---|---|---|
| `text` | `text` | 不需要 |
| `image_url` | `image_url.url` | `reference_image`、`first_frame`、`last_frame` |
| `video_url` | `video_url.url` | `reference_video` |
| `audio_url` | `audio_url.url` | `reference_audio` |

是否进入“含参考视频”价格档位，以 `content` 中实际存在的 `reference_video` 为准，不使用兼容字段或通配价格推断。

### 3. 模式约束

#### 3.1 纯文本或首尾帧

- 纯文本和首尾帧模式不提交 `omni_reference_task_type`。
- 首帧/尾帧不能与 `reference_image`、`reference_video`、`reference_audio` 混用。
- 首尾帧模式要求 `ratio=adaptive`。

#### 3.2 `auto` / `reference`

- 有参考图片、参考视频或参考音频时可以使用 `auto`。
- 需要明确按参考素材生成时使用 `reference`。
- 参考视频是否存在会影响价格变体。

#### 3.3 `edit`

`edit` 必须同时满足：

- 恰好一个参考视频。
- 参考视频时长为 `4`–`30` 秒。
- `ratio=adaptive`。
- `duration=-1`。
- 不与首帧或尾帧混用。

#### 3.4 `extend`

- 必须恰好包含一个参考视频。
- 必须使用 `ratio=adaptive`。
- 不与首帧或尾帧混用。
- `duration` 使用 `-1` 或合法的 `4`–`30` 秒整数。

### 4. 素材限制

| 项目 | 限制 |
|---|---|
| 请求体 | 最大 `64 MB` |
| `content` 总数 | 最大 `50` |
| 图片数 | 最大 `30` |
| 视频数 | 最大 `10` |
| 音频数 | 最大 `10` |
| 单张图片 | 最大 `30 MB`；宽高 `300`–`6000` px；宽高比 `0.4`–`2.5` |
| 单个视频 | 最大 `200 MB`；MP4/MOV；`24`–`60` FPS |
| 视频总时长 | 最大 `30` 秒 |
| 单个音频 | 最大 `15 MB`；WAV/MP3 |
| 音频总时长 | 最大 `30` 秒 |

素材 URL 必须能被服务端公网访问，并返回与内容一致的媒体类型。

### 5. 请求示例

#### 5.1 纯文本、自动时长

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "content": [
    { "type": "text", "text": "一艘帆船穿过清晨薄雾，电影感运镜" }
  ],
  "resolution": "720p",
  "ratio": "adaptive",
  "duration": -1,
  "generate_audio": true,
  "watermark": false,
  "output_format": "mp4"
}
~~~

#### 5.2 参考视频与参考音频

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "content": [
    { "type": "text", "text": "保留主体动作节奏，改成水墨风格" },
    { "type": "video_url", "video_url": { "url": "https://example.com/reference.mp4" }, "role": "reference_video" },
    { "type": "audio_url", "audio_url": { "url": "https://example.com/reference.mp3" }, "role": "reference_audio" }
  ],
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 4,
  "omni_reference_task_type": "reference",
  "execution_expires_after": 172800
}
~~~

#### 5.3 首尾帧

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "content": [
    { "type": "text", "text": "镜头从室内平滑移动到窗外雪景" },
    { "type": "image_url", "image_url": { "url": "https://example.com/first.png" }, "role": "first_frame" },
    { "type": "image_url", "image_url": { "url": "https://example.com/last.png" }, "role": "last_frame" }
  ],
  "resolution": "1080p",
  "ratio": "adaptive",
  "duration": 4,
  "return_last_frame": true
}
~~~

#### 5.4 编辑参考视频

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "content": [
    { "type": "text", "text": "将天空替换为极光，保持原镜头运动" },
    { "type": "video_url", "video_url": { "url": "https://example.com/source.mov" }, "role": "reference_video" }
  ],
  "resolution": "1080p",
  "ratio": "adaptive",
  "duration": -1,
  "omni_reference_task_type": "edit",
  "output_format": "mov"
}
~~~

#### 5.5 扩展视频并启用联网搜索

~~~json
{
  "model": "AP Seedance-2.5 标准版",
  "content": [
    { "type": "text", "text": "自然延续镜头，保持光线和运动方向" },
    { "type": "video_url", "video_url": { "url": "https://example.com/source.mp4" }, "role": "reference_video" }
  ],
  "resolution": "480p",
  "ratio": "adaptive",
  "duration": 4,
  "omni_reference_task_type": "extend",
  "priority": 5,
  "safety_identifier": "tenant-user-001",
  "tools": [{ "type": "web_search" }]
}
~~~

### 6. 查询任务

~~~http
GET /v1/videos/{task_id}
Authorization: Bearer <NEWAPI_TOKEN>
~~~

兼容入口：

- `GET /v1/video/generations/{task_id}`
- `GET /v1/video/generations/{task_id}/result`

创建响应示例：

~~~json
{
  "id": "video_task_25_123",
  "task_id": "video_task_25_123",
  "object": "video",
  "status": "queued",
  "progress": 0,
  "model": "AP Seedance-2.5 标准版",
  "created_at": 1787184000
}
~~~

成功响应会在 `metadata` 中保留官方返回的实际时长、输出格式和分辨率：

~~~json
{
  "id": "video_task_25_123",
  "task_id": "video_task_25_123",
  "object": "video",
  "status": "completed",
  "model": "AP Seedance-2.5 标准版",
  "progress": 100,
  "created_at": 1787184000,
  "completed_at": 1787184090,
  "metadata": {
    "url": "https://example.com/result.mov",
    "duration": 8,
    "output_format": "mov",
    "resolution": "1080p"
  }
}
~~~

任务未结束时不会伪造 `completed_at`。若官方终态暂时缺少有效 `duration`，任务保持待对账并继续重试，不会擅自按 5 秒或 30 秒结算。

### 7. 预授权与最终结算

`duration` 为 `4`–`30` 时，按请求秒数和创建时冻结的单价快照预授权并结算。

`duration=-1` 或省略时：

1. Java 钱包和 NewAPI 客户额度先按最长 `30` 秒预授权。
2. 任务成功后读取官方响应的实际 `duration`。
3. 使用创建时冻结的单价快照原子结算。
4. 多冻结的额度自动退回。
5. 缺少有效实际时长时保持待对账，不默认扣 5 秒或 30 秒。

PLATFORM 与 BYOK 的客户单价、平台成本可能不同；是否含参考视频也会选择不同价格变体。所有价格以 `GET /api/pricing` 为准。

### 8. 常见错误

| 错误 | 原因 |
|---|---|
| `invalid_resolution` | 不是 `480p`、`720p`、`1080p` |
| `invalid_ratio` | 比例不受支持，或当前模式要求 `adaptive` |
| `invalid_duration` | 不是 `-1` 或 `4`–`30` 的整数 |
| `invalid_omni_reference_task_type` | 模式不是 `auto/reference/edit/extend` |
| `invalid_edit_input` | edit 的参考视频数量、时长、比例或 duration 不合规 |
| `mixed_input_modes` | 首尾帧与参考素材混用 |
| `unsupported_parameter` | 使用了 `seed`、`service_tier`、`frames` 等禁用字段 |
| `material_limit_exceeded` | 素材数量、大小、尺寸、格式、FPS 或总时长超限 |
| `insufficient_quota` | 30 秒预授权或固定时长额度不足 |

### 9. 官方参考

- [创建视频生成任务](https://docs.volcengine.com/docs/82379/1520757?lang=zh)
- [Seedance 2.5 兼容说明](https://docs.volcengine.com/docs/82379/2607688?lang=zh#2.5_compatibility)
- [查询视频生成任务](https://docs.volcengine.com/docs/82379/1521309?lang=zh)
