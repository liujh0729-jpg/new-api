# AP Seedance 2.0 / 2.5 API 文档

Seedance 2.0 与 Seedance 2.5 共用任务创建和查询入口，但模型名、参数集合、默认值和素材约束不同。下文按版本分别说明，请勿跨版本套用默认值。

## 1. 两套调用格式（先选一套）

平台对外提供两套调用格式。二者背后是同一个 Seedance 任务能力：创建请求的请求体完全一致（都使用第 7 节或第 8 节的 Seedance 参数），区别只在路径和响应结构。

| 调用格式 | 创建任务 | 查询任务 | 成功状态 | 视频地址 | 时长字段 |
|---|---|---|---|---|---|
| **Seedance 官方兼容** | `POST /api/v3/contents/generations/tasks` | `GET /api/v3/contents/generations/tasks/{task_id}` | `succeeded` | `content.video_url` | 顶层 `duration` |
| **OpenAI Video** | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `completed` | `metadata.url` | `metadata.duration` |

**如何选择：**

- 已有 Seedance 官方 SDK 或调用代码 → 选 Seedance 官方兼容格式；
- 需要 OpenAI Video 响应结构 → 选 OpenAI Video 格式。

> 注意：不要根据请求体或 `task_*` ID 判断调用格式。两套入口接收相同的 Seedance 请求参数，返回的任务 ID 格式也相同（都是 `task_xxx`）。**URL 才是区分两套格式的唯一依据**：用哪个路径创建，就用对应的路径查询，创建和查询必须配套。

两套格式的查询响应差异对照：

| 查询响应字段 | Seedance 官方兼容 | OpenAI Video |
|---|---|---|
| 任务 ID | `id` | `id`、`task_id` |
| 成功状态 | `succeeded` | `completed` |
| 视频地址 | `content.video_url` | `metadata.url` |
| 时长 | 顶层 `duration` | `metadata.duration` |
| 其他字段 | `model`、`resolution`、`usage` | `object`、`model`、`progress`、`created_at`、`completed_at`、`metadata.output_format`、`metadata.resolution`、`usage` |

所有公网入口都返回平台生成的 `task_*` 任务 ID。该 ID 是不透明字符串，用于保证用户隔离、渠道切换和计费安全；不会返回上游内部的 `cgt-*` ID。

## 2. 通用行为

以下规则对两套格式同样适用。

- **认证**：所有接口使用 `Authorization: Bearer <NEWAPI_TOKEN>` 请求头，请求体为 `application/json`：

  ~~~http
  Authorization: Bearer <NEWAPI_TOKEN>
  Content-Type: application/json
  ~~~

- **异步执行**：创建成功后保存返回的任务 ID，轮询与创建路径对应的查询接口，直到进入成功或失败终态。
- **不提供的接口**：两套格式均不提供列表、删除或重试接口；Seedance 官方兼容格式也没有 `/result` 接口，视频地址直接位于查询响应的 `content.video_url`。OpenAI Video 格式的 `/v1/videos/{task_id}/content` 是视频内容代理地址，不是任务状态查询接口。

### 2.1 旧版兼容格式（仅存量客户端）

`POST /v1/video/generations` 与 `GET /v1/video/generations/{task_id}` 仅供已有客户端使用，新接入不建议使用。该格式响应为 `{"code":"success","data":{...}}` 结构，成功状态为 `succeeded`，当前不返回 `duration`。

## 3. Seedance 官方兼容格式

### 3.1 创建任务

~~~http
POST /api/v3/contents/generations/tasks
Authorization: Bearer <NEWAPI_TOKEN>
Content-Type: application/json
~~~

请求体使用第 7 节（Seedance 2.0）或第 8 节（Seedance 2.5）说明的参数。创建成功固定只返回公开任务 ID：

~~~json
{
  "id": "task_xxx"
}
~~~

### 3.2 查询任务

~~~http
GET /api/v3/contents/generations/tasks/{task_id}
Authorization: Bearer <NEWAPI_TOKEN>
~~~

将创建响应中的 `id` 替换到 `{task_id}`。任务成功时返回 Seedance 官方兼容的顶层任务对象：

~~~json
{
  "id": "task_xxx",
  "model": "AP Seedance-2.5 标准版",
  "status": "succeeded",
  "duration": 8,
  "resolution": "1080p",
  "content": {
    "video_url": "https://example.com/result.mp4"
  },
  "usage": {
    "completion_tokens": 400000,
    "total_tokens": 400000
  }
}
~~~

- 状态取值：`queued`、`running`、`succeeded`、`failed`、`cancelled`。
- `duration` 是顶层 JSON 数值，单位为秒。优先使用上游返回的有效实际时长；上游暂未返回时，使用创建请求中明确指定的固定正数；请求值为 `-1`、省略、零或无效时不使用该值；上游返回实际时长后以实际时长为准。
- `usage` 严格使用 Seedance 官方字段形状；`completion_tokens` 与 `total_tokens` 相等。若官方响应原有 `tool_usage`，该字段会一并保留。
- 这里的 Token 是按最终零售价值折算的输出 Token：`ceil(最终计费秒数 × 冻结的建议零售价（元/秒） × 1,000,000 ÷ 冻结的折算零售价（元/百万输出 Token）)`。BYOK 任务使用对应 PLATFORM 价格档位的建议零售价，不使用 BYOK 结算价。它不表示模型真实计算 Token，也不参与 New API 的二次计费。
- 仅成功终态且折算条件完整时返回 `usage`；排队、运行、失败、取消、缺少价格或有效秒数时省略该字段。Seedance 2.5 `duration=-1` 使用最终结算的实际时长。

## 4. OpenAI Video 格式

### 4.1 创建任务

~~~http
POST /v1/videos
Authorization: Bearer <NEWAPI_TOKEN>
Content-Type: application/json
~~~

请求体与 Seedance 官方兼容入口相同，使用第 7 节或第 8 节的参数。创建成功响应示例：

~~~json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "status": "queued",
  "progress": 0,
  "created_at": 1787184000
}
~~~

### 4.2 查询任务

~~~http
GET /v1/videos/{task_id}
Authorization: Bearer <NEWAPI_TOKEN>
~~~

将创建响应中的 `id` 替换到 `{task_id}`。任务成功时返回 OpenAI Video 格式：

~~~json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "status": "completed",
  "model": "AP Seedance-2.5 标准版",
  "progress": 100,
  "created_at": 1787184000,
  "completed_at": 1787184090,
  "metadata": {
    "url": "https://example.com/result.mp4",
    "duration": 8,
    "output_format": "mp4",
    "resolution": "1080p"
  },
  "usage": {
    "completion_tokens": 400000,
    "total_tokens": 400000
  }
}
~~~

- 成功状态为 `completed`；进行中可识别 `queued`、`in_progress`、`failed` 等状态。
- 任务未结束时不会返回 `completed_at`。
- `metadata` 保留官方返回的实际时长、输出格式和分辨率；`metadata.duration` 是可选字段，上游没有返回有效实际时长时会省略，字段缺失不代表视频时长为 0 秒。
- `usage` 与 Seedance 官方兼容查询返回同一组零售价等价 Token；缺少折算条件时省略，且不参与 New API 的按秒、分辨率任务计费。

## 5. 版本差异速查

| 项目 | Seedance 2.0 | Seedance 2.5 |
|---|---|---|
| 公开模型 | VIP、标准版、轻量版、高性价比版 | `AP Seedance-2.5 标准版` |
| 默认分辨率 | 请求指定或通过宽高推断 | `720p` |
| 比例 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4` | 2.0 的全部比例，外加 `21:9`、`adaptive`，默认 `adaptive` |
| 时长 | 正数，默认 `5` 秒 | `-1` 或 `4`–`30`，默认 `-1` |
| 模式表达方式 | 通过 `content` 中的素材角色（如 `first_frame`） | 通过 `omni_reference_task_type` 字段 |
| `seed` | 支持 | 不支持 |
| `service_tier` | 支持 | 不支持 |
| 时长结算方式 | 不适用 | 先按 30 秒预授权，按官方实际时长结算 |

## 6. 价格

价格和可用档位以动态目录为准：

~~~http
GET /api/pricing
~~~

同一模型的价格可能因分辨率、是否包含参考视频以及 PLATFORM（平台提供密钥）/ BYOK（自带密钥）模式而不同。客户端不要硬编码价格，也不要用无参考视频价格替代参考视频价格。

## 7. Seedance 2.0

### 7.1 模型

| 公开模型 | 可用分辨率 |
|---|---|
| `AP Seedance-2.0 VIP` | `480p`、`720p`、`1080p`、`4k` |
| `AP Seedance-2.0 标准版` | `480p`、`720p`、`1080p`、`4k` |
| `AP Seedance-2.0 轻量版` | `480p`、`720p`、`1080p` |
| `AP Seedance-2.0 高性价比版` | `1080p`、`4k` |

最终可用档位以 `GET /api/pricing` 返回的动态目录为准。其中 VIP、标准版和轻量版提供 `480p`，高性价比版不提供。

### 7.2 创建任务

创建请求可提交到第 3 节的 `POST /api/v3/contents/generations/tasks` 或第 4 节的 `POST /v1/videos`，不要在同一个调用流程中混用不同格式。旧版兼容入口为 `POST /v1/video/generations`。

#### 7.2.1 请求字段

| 字段 | 类型 | 必填 | Seedance 2.0 说明 |
|---|---|---|---|
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

`width` 与 `height` 同时提供时，按较短边推断分辨率：`480` → `480p`、`720` → `720p`、`1080` → `1080p`、`2160` → `4k`。新调用建议直接提交 `resolution` 和 `ratio`。

不要在 2.0 请求中使用 2.5 专属的 `adaptive`、`21:9`、`duration=-1` 或 `omni_reference_task_type`。

#### 7.2.2 `content` 结构

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

首尾帧模式不能与参考图片、参考视频或参考音频混用。两套兼容格式都支持使用公网 URL 或角色库 `asset://...` 素材。

#### 7.2.3 素材限制

- `content` 总素材数不超过 `12`。
- 参考图片不超过 `9` 张。
- 参考视频不超过 `3` 个。
- 参考音频不超过 `3` 个。
- 单个参考视频建议为 `2`–`15.2` 秒。
- 参考视频与参考音频总时长不超过 `15.2` 秒。
- 实际格式、大小和像素限制以当前模型能力契约和动态目录为准。

### 7.3 请求示例

#### 7.3.1 文生视频

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

#### 7.3.2 参考图生视频

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

#### 7.3.3 首尾帧

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

### 7.4 查询任务

查询路径与创建路径配套：`GET /api/v3/contents/generations/tasks/{task_id}`（Seedance 官方兼容）或 `GET /v1/videos/{task_id}`（OpenAI Video）；旧版为 `GET /v1/video/generations/{task_id}`。

响应结构分别见第 3.2 节和第 4.2 节。调用方应识别 `queued`、`in_progress`、`completed`、`failed` 等状态，并为网络错误采用有限次数重试。

### 7.5 计费与错误

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

## 8. Seedance 2.5

Seedance 2.5 的参数、默认值和条件约束与 2.0 不同。不要沿用 2.0 的 5 秒默认时长、旧比例、`seed` 或 `service_tier`。

### 8.1 模型

| 公开模型 | 上游模型 | 可用分辨率 |
|---|---|---|
| `AP Seedance-2.5 标准版` | `doubao-seedance-2-5-260628` | `480p`、`720p`、`1080p` |

客户端只提交公开模型名；上游模型名由平台绑定，不能由客户端覆盖。

### 8.2 创建任务

创建请求可提交到第 3 节的 `POST /api/v3/contents/generations/tasks` 或第 4 节的 `POST /v1/videos`，不要在同一个调用流程中混用不同格式。旧版兼容入口为 `POST /v1/video/generations`。

#### 8.2.1 请求字段

| 字段 | 类型 | 必填 | 默认值 | Seedance 2.5 说明 |
|---|---|---|---|---|
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

#### 8.2.2 明确不支持的字段

以下字段不会公开或转发给 Seedance 2.5：

- `frames` 或其他 FPS 控制字段
- `seed`
- `camera_fixed`
- `draft`
- `service_tier`

提交这些字段会被拒绝或清理，客户端不要依赖静默忽略行为。

#### 8.2.3 `content` 结构

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

是否进入"含参考视频"价格档位，以 `content` 中实际存在的 `reference_video` 为准，不使用兼容字段或通配价格推断。

### 8.3 模式约束

#### 8.3.1 纯文本或首尾帧

- 纯文本和首尾帧模式不提交 `omni_reference_task_type`。
- 首帧/尾帧不能与 `reference_image`、`reference_video`、`reference_audio` 混用。
- 首尾帧模式要求 `ratio=adaptive`。

#### 8.3.2 `auto` / `reference`

- 有参考图片、参考视频或参考音频时可以使用 `auto`。
- 需要明确按参考素材生成时使用 `reference`。
- 参考视频是否存在会影响价格变体。

#### 8.3.3 `edit`

`edit` 必须同时满足：

- 恰好一个参考视频。
- 参考视频时长为 `4`–`30` 秒。
- `ratio=adaptive`。
- `duration=-1`。
- 不与首帧或尾帧混用。

#### 8.3.4 `extend`

- 必须恰好包含一个参考视频。
- 必须使用 `ratio=adaptive`。
- 不与首帧或尾帧混用。
- `duration` 使用 `-1` 或合法的 `4`–`30` 秒整数。

### 8.4 素材限制

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

### 8.5 请求示例

#### 8.5.1 纯文本、自动时长

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

#### 8.5.2 参考视频与参考音频

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

#### 8.5.3 首尾帧

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

#### 8.5.4 编辑参考视频

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

#### 8.5.5 扩展视频并启用联网搜索

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

### 8.6 查询任务

查询路径与创建路径配套：`GET /api/v3/contents/generations/tasks/{task_id}`（Seedance 官方兼容）或 `GET /v1/videos/{task_id}`（OpenAI Video）；旧版为 `GET /v1/video/generations/{task_id}`。

响应结构分别见第 3.2 节和第 4.2 节。任务未结束时不会返回 `completed_at`。若官方终态暂时缺少有效 `duration`，任务保持待对账并继续重试，不会擅自按 5 秒或 30 秒结算。

### 8.7 预授权与最终结算

`duration` 为 `4`–`30` 时，按请求秒数和创建时冻结的单价快照预授权并结算。

`duration=-1` 或省略时：

1. 客户额度先按最长 `30` 秒预授权。
2. 任务成功后读取官方响应的实际 `duration`。
3. 使用创建时冻结的单价快照原子结算。
4. 预授权冻结的多余额度自动退回。
5. 缺少有效实际时长时保持待对账，不默认按 5 秒或 30 秒结算。

PLATFORM 与 BYOK 的客户单价、平台成本可能不同；是否含参考视频也会选择不同价格变体。所有价格以 `GET /api/pricing` 为准。

### 8.8 常见错误

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

## 9. 素材库与角色库

普通素材库和角色库是两种不同的功能：

- 普通素材库保存图片、视频和音频文件，生成时使用素材记录返回的公网 `url`。
- 角色库保存火山角色 Asset，生成时使用 NewAPI 的 `character_id`，或者使用已登记且已授权的 `asset://{asset_id}`。
- 真人角色必须先完成本人 H5 身份核验，再上传同一人的肖像图片。普通素材上传不能代替真人核验。

### 9.1 普通素材库（控制台 / Playground）

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

### 9.2 查询官方角色、虚拟角色和真人角色

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

### 9.3 创建个人虚拟角色（非真人）

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

### 9.4 创建真人角色

真人入库必须按以下顺序执行。

#### 9.4.1 创建本人核验会话

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

#### 9.4.2 查询核验结果

~~~http
GET /v1/virtual-characters/validation-sessions/{session_id}
Authorization: Bearer <NEWAPI_TOKEN>
~~~

会话状态为 `pending`、`succeeded`、`failed` 或 `expired`。只有 `succeeded` 才能继续上传肖像。仍在等待中的会话可以取消：

~~~http
DELETE /v1/virtual-characters/validation-sessions/{session_id}
~~~

#### 9.4.3 上传已核验人员的肖像

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

### 9.5 在 Seedance 请求中使用角色

Seedance 2.0 和 Seedance 2.5 都支持角色库，但所选模型名必须包含 `Seedance`。

#### 9.5.1 单角色：使用 `character_id`

这是推荐的单角色调用方式。使用兼容入口 `POST /v1/video/generations`；NewAPI 会鉴权并把角色转换成对应的 `asset://` 引用。由于该入口使用兼容请求结构，`resolution`、`ratio` 等 Seedance 参数放入 `metadata`：

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

#### 9.5.2 多角色：使用已登记的 `asset://`

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

每个 `asset://` 都必须已登记到本地角色库（一个 Asset 只能登记一次），且当前用户有权使用。未知 Asset、其他用户的私有真人、已撤权、过期、下线或火山失效的 Asset 都会被拒绝。直接 `asset://` 方式允许组合多个已授权角色，但仍受对应 Seedance 版本的素材数量和模式约束。

真人角色库的合规、状态和清理细节见 [真人形象素材库实施说明](./real-person-character-library.zh_CN.md)。

## 10. 附录：额度与逐笔账单查询

以下接口与 Seedance 创建任务使用同一个 `NEWAPI_TOKEN`。查询范围限定为调用所使用的 API Key，不会返回同一账号下其他 Key 的数据。

### 10.1 查询当前 API Key 余额

~~~http
GET /api/usage/token/
Authorization: Bearer <NEWAPI_TOKEN>
~~~

请求示例：

~~~bash
curl -sS "$BASE_URL/api/usage/token/" \
  -H 'Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
~~~

成功响应示例：

~~~json
{
  "code": true,
  "message": "ok",
  "data": {
    "object": "token_usage",
    "name": "default",
    "total_granted": 100000,
    "total_used": 20000,
    "total_available": 80000,
    "unlimited_quota": false,
    "model_limits": {},
    "model_limits_enabled": false,
    "expires_at": 0
  }
}
~~~

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.object` | string | 固定为 `token_usage` |
| `data.name` | string | 当前 API Key 名称 |
| `data.total_granted` | int | 当前 Key 总额度，等于 `total_used + total_available` |
| `data.total_used` | int | 当前 Key 累计已用额度 |
| `data.total_available` | int | 当前 Key 剩余额度；通常将该字段作为 Key 余额 |
| `data.unlimited_quota` | bool | 是否为无限额度 Key；为 `true` 时不要用 `total_available` 判断是否可用 |
| `data.model_limits` | object | 当前 Key 的模型调用上限映射；未配置时为空对象 |
| `data.model_limits_enabled` | bool | 是否启用模型调用上限 |
| `data.expires_at` | int64 | Key 过期时间，Unix 时间戳（秒）；`0` 表示永不过期 |

`total_granted`、`total_used`、`total_available` 都是站点内部原始额度单位（quota units），不是人民币或美元。该接口查询的是当前 API Key 的余额；如需账号下所有 Key 共享的账号余额，请登录控制台查询，不能将本接口结果当作账号总余额。

### 10.2 查询当前 API Key 的逐笔账单

~~~http
GET /api/log/token?p=1&page_size=20
Authorization: Bearer <NEWAPI_TOKEN>
~~~

| Query 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `p` | int | 否 | `1` | 页码，从 `1` 开始 |
| `page_size` | int | 否 | `10` | 每页条数，最大 `100`；兼容别名 `ps`、`size` |

请求示例：

~~~bash
curl -sS "$BASE_URL/api/log/token?p=1&page_size=20" \
  -H 'Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
~~~

成功响应示例：

~~~json
{
  "success": true,
  "message": "",
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 128,
    "items": [
      {
        "id": 1,
        "created_at": 1754496000,
        "type": 2,
        "token_name": "default",
        "model_name": "AP Seedance-2.5 标准版",
        "quota": 50000,
        "quota_cny": 0.73,
        "use_time": 120,
        "token_id": 34,
        "request_id": "req_xxx",
        "other": "{\"billing_source\":\"wallet\",\"billing_mode\":\"task_pricing\",\"quota_per_unit\":500000,\"usd_exchange_rate\":7.3}"
      }
    ]
  }
}
~~~

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.page` / `data.page_size` | int | 当前页码与每页条数 |
| `data.total` | int | 当前 Key 的日志总数，计数最多约 `10000` |
| `data.items[]` | array | 当前页逐笔日志，按最新记录在前排序 |
| `items[].created_at` | int64 | 日志时间，Unix 时间戳（秒） |
| `items[].type` | int | 日志类型：`1` 充值、`2` 消费、`3` 管理、`4` 系统、`5` 错误、`6` 退款 |
| `items[].model_name` | string | 本次调用的模型名 |
| `items[].quota` | int | 本次记录的原始额度；支出或退款方向需结合 `type` 判断 |
| `items[].quota_cny` | number | 本次额度对应的人民币等值，四舍五入到 6 位小数 |
| `items[].use_time` | int | 请求或任务耗时，单位为秒 |
| `items[].token_id` | int | 当前 API Key 的内部令牌 ID |
| `items[].request_id` | string | 请求 ID，可能为空；排查问题时建议保存 |
| `items[].other` | string | JSON 字符串，可能包含计费来源、计费模式、预扣/实扣额度和汇率快照，客户端需再次反序列化 |

该接口仅支持分页，不支持按时间或模型过滤。`quota_cny` 按计费时保存的额度与汇率快照换算；旧日志缺少快照时，按查询时的站点配置换算。更完整的字段说明见 [API Key 查询接口文档](api/token-query-apis.zh_CN.md)。

## 11. 官方参考

- [创建视频生成任务](https://docs.volcengine.com/docs/82379/1520757?lang=zh)
- [Seedance 2.5 兼容说明](https://docs.volcengine.com/docs/82379/2607688?lang=zh#2.5_compatibility)
- [查询视频生成任务](https://docs.volcengine.com/docs/82379/1521309?lang=zh)
