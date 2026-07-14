# NewAPI 线上模型用户调用文档

本文面向普通 API 用户，说明如何通过当前 NewAPI 服务调用已公开的 DeepSeek、Doubao 和 AIPDD 模型。调用方只需要使用平台发放的 NewAPI Token，不需要知道上游供应商 API Key。

更新时间：2026-05-30。

本文的模型列表按当前线上配置整理。不同类型模型的响应方式不同：

- DeepSeek 文本模型：同步或流式返回 OpenAI/Anthropic 兼容响应，不支持 `/v1/responses`。
- Doubao Seedream 图片模型：同步返回图片结果。
- Doubao Seedance 视频模型：异步任务，创建后返回 `task_id`，再轮询查询结果。
- AIPDD 图片、视频、音频模型：异步任务，创建后返回 `task_id`，再轮询查询结果。

## 基本流程

文本和同步图片模型：

1. 调用对应创建接口。
2. 从响应中直接读取 `choices[0].message.content` 或 `data[0].url`。

异步任务模型：

1. 调用创建接口提交任务。
2. 创建成功后保存响应里的 `task_id`。
3. 每 5 到 15 秒调用查询接口。
4. 状态为 `succeeded` 或 `completed` 后读取结果 URL。

公共请求头：

```http
Authorization: Bearer <NEW_API_TOKEN>
Content-Type: application/json
```

示例环境变量：

```bash
export BASE_URL="https://your-newapi.example.com"
export NEW_API_TOKEN="sk-xxxx"
```

## 模型总览

| 能力 | 模型名 | 创建接口 | 查询接口 | 响应/计费 |
| --- | --- | --- | --- | --- |
| DeepSeek 文本 | `deepseek-v4-pro` | `POST /v1/chat/completions` 或 `POST /v1/messages` | 无 | 同步或流式 / 按 token |
| Doubao 图片生成 | `doubao-seedream-4-5` | `POST /v1/images/generations` | 无 | 同步图片 / 按次 |
| Doubao 视频生成 | `doubao-seedance-2.0` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | 异步任务 / 按次 |
| Doubao 视频生成 | `doubao-seedance-1.5` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | 异步任务 / 按次 |
| Flux 图生图 | `aipdd-flux-gguf` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | 异步任务 / 按次 |
| Flux 文生图 | `aipdd-flux-gguf-t2i` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | 异步任务 / 按次 |
| Wan2.2 图生视频 | `aipdd-wan2.2-wanx` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | 异步任务 / 按次 |
| Wan2.2 主体替换 | `aipdd-wan2.2-animater` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | 异步任务 / 按次 |
| MimicMotion 动作迁移 | `aipdd-mimic-motion` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | 异步任务 / 按次 |
| Latentsync 对口型 | `aipdd-latentsync-1.5` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | 异步任务 / 按次 |
| IndexTTS 声音复刻 | `aipdd-indextts` | `POST /v1/audio/speech` | `GET /v1/audio/speech/{task_id}` | 异步任务 / 按次 |

素材 URL 必须能被 NewAPI 服务端和上游供应商访问，推荐使用公网 HTTPS URL。AIPDD 任务接口不负责把 multipart 本地文件转换成上游素材；请先上传文件，再把得到的 HTTP(S) URL 写入任务参数。带二进制文件项的 multipart 请求会返回 `aipdd_file_upload_not_supported`。

视频模型可使用 OpenAI 风格路径 `/v1/videos`，也可使用 NewAPI 通用路径 `/v1/video/generations`。两条路径共用任务创建与查询链路；Seedance 的通用参数转换和官方字段透传在两条路径上保持一致。

## 异步任务通用请求格式

AIPDD 和 Doubao Seedance 这类异步任务的业务参数可以直接放在 JSON 顶层，也可以放在 `metadata` 对象中。下面两种写法等价：

```json
{
  "model": "aipdd-mimic-motion",
  "motion_video": "https://example.com/motion.mp4",
  "appearance_image": "https://example.com/person.png"
}
```

```json
{
  "model": "aipdd-mimic-motion",
  "metadata": {
    "motion_video": "https://example.com/motion.mp4",
    "appearance_image": "https://example.com/person.png"
  }
}
```

为减少歧义，建议用户侧优先使用本文示例里的字段名。

## 调用示例

### DeepSeek 文本

模型：`deepseek-v4-pro`

接口：

- OpenAI 兼容：`POST /v1/chat/completions`
- Anthropic 兼容：`POST /v1/messages`

不要使用 `POST /v1/responses` 调用 `deepseek-v4-pro`。如果 SDK 默认走 Responses API，请切换到 Chat Completions 模式。

OpenAI 兼容请求：

```bash
curl "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "用三句话解释 NewAPI 网关的作用"}
    ],
    "temperature": 0.6
  }'
```

流式请求：

```bash
curl "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "user", "content": "写一个 TypeScript 防抖函数"}
    ],
    "stream": true
  }'
```

Anthropic 兼容请求：

```bash
curl "$BASE_URL/v1/messages" \
  -H "x-api-key: $NEW_API_TOKEN" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "用三句话解释 NewAPI 网关的作用"}
    ]
  }'
```

OpenAI 兼容响应中，文本结果在 `choices[0].message.content`；流式响应按 SSE 读取 `choices[0].delta.content`。

### Doubao Seedream 图片生成

模型：`doubao-seedream-4-5`

接口：`POST /v1/images/generations`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `prompt` | 图片提示词。 |

常用可选参数：

| 参数 | 说明 |
| --- | --- |
| `size` | 图片尺寸。Seedream 4.5 要求总像素数至少 `3,686,400`，推荐 `1920x1920`、`2560x1440`、`1440x2560` 或 `2048x2048`。 |
| `n` | 生成数量。 |
| `response_format` | 建议省略。不要在用户请求或通道参数覆盖里传对象格式。 |
| `watermark` | 是否加水印，布尔值。 |
| `image` 或 `images` | 参考图 URL，按上游 OpenAI 兼容图片参数透传。 |

文生图示例：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedream-4-5",
    "prompt": "a premium product photo of a ceramic coffee cup, soft studio lighting",
    "size": "1920x1920",
    "watermark": false
  }'
```

同步图片响应示例：

```json
{
  "created": 1778932446,
  "data": [
    {
      "url": "https://example.com/result.png",
      "revised_prompt": ""
    }
  ]
}
```

### Seedance 视频生成（Doubao 与 AP）

模型：

- `doubao-seedance-2.0`
- `doubao-seedance-1.5`
- `AP Seedance-2.0 VIP`
- `AP Seedance-2.0 标准版`
- `AP Seedance-2.0 轻量版`
- `AP Seedance-2.0 高性价比版`

接口：`POST /v1/videos` 或 `POST /v1/video/generations`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `prompt` 或 `content` | 视频内容。未传非空 `content` 时，系统会把 `prompt` 转换为一个 `text` 内容项。 |

常用可选参数：

| 参数 | 说明 |
| --- | --- |
| `image` | 首帧或参考图 URL。传入后按图生视频处理。 |
| `duration` 或 `seconds` | 视频时长。不传时默认按 5 秒计费和提交。 |
| `resolution` | Seedance 分辨率档位，例如 `720p`、`1080p`、`4k`。根级值优先于 `metadata.resolution`。 |
| `ratio` | 画幅比例，例如 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`。根级值优先于 `metadata.ratio`。 |
| `width`、`height` | 通用视频尺寸。未传 `resolution` 时按短边 720、1080、2160 推导对应档位；未传 `ratio` 时同时推导画幅。 |
| `content` | Seedance 官方多模态数组，支持 `text`、`image_url`、`video_url`、`audio_url` 及上游扩展字段。非空时原样优先。 |
| `generate_audio` | 是否生成同步音频。显式 `false` 会保留。 |
| `seed`、`service_tier`、`priority`、`callback_url` | Seedance 官方参数，支持根级直传；metadata 仅在根级缺失时补充。 |
| `metadata.watermark` | 是否加水印，布尔值。 |

Seedance 参数归一化优先级如下：

```text
content:    根级非空 content > metadata.content > prompt 转换
resolution: 根级 resolution > metadata.resolution > width/height 推导
ratio:      根级 ratio > metadata.ratio > width/height 推导
```

同步价格目录是分辨率能力的最终依据。尺寸能够推导为 `4k` 不代表每个模型都支持 `4k`；目录缺少对应档位时接口返回 HTTP 400，不会自动降级。

文生视频示例：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "prompt": "a cinematic slow push-in shot of a futuristic city at sunset",
    "duration": 5,
    "width": 1280,
    "height": 720,
    "fps": 24,
    "generate_audio": false
  }'
```

以上请求会为 AP Seedance 自动生成根级 `content`、`resolution: "720p"` 和 `ratio: "16:9"`。调用方也可以直接使用官方结构：

```json
{
  "model": "AP Seedance-2.0 标准版",
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 5,
  "content": [
    {
      "type": "text",
      "text": "保持主体动作连续"
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {
        "url": "https://example.com/reference.png"
      }
    }
  ],
  "generate_audio": false
}
```

图生视频示例：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-1.5",
    "prompt": "the camera slowly moves forward, natural motion, cinematic lighting",
    "image": "https://example.com/start-frame.png",
    "duration": 5
  }'
```

创建成功后会返回 OpenAI Video 风格任务对象：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "doubao-seedance-2.0",
  "status": "queued",
  "progress": 0,
  "created_at": 1778932446
}
```

查询方式：

```bash
curl "$BASE_URL/v1/videos/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

### Flux 图生图

模型：`aipdd-flux-gguf`

接口：`POST /v1/images/generations`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `image` | 输入图片 URL。也可以使用 `images` 数组，系统取第一张。 |

可选参数：

| 参数 | 说明 |
| --- | --- |
| `prompt` | 正向提示词，会自动映射到工作流 `positive_prompt`。 |
| `positive_prompt` | 正向提示词。优先级高于 `prompt`。 |
| `negative_prompt` | 反向提示词。 |

JSON 示例：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf",
    "image": "https://example.com/input.png",
    "prompt": "a cinematic product photo, soft studio lighting",
    "negative_prompt": "low quality, blurry"
  }'
```

### Flux 文生图

模型：`aipdd-flux-gguf-t2i`

接口：`POST /v1/images/generations`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `prompt` 或 `text` 或 `input` | 文生图提示词。 |

JSON 示例：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf-t2i",
    "prompt": "a cinematic product photo, soft studio lighting"
  }'
```

### Wan2.2 图生视频

模型：`aipdd-wan2.2-wanx`

接口：`POST /v1/videos`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `image` | 输入图片 URL。也可以使用 `images` 数组，系统取第一张。 |
| `prompt` | 视频提示词。 |

可选参数：

| 参数 | 说明 |
| --- | --- |
| `negative_prompt` | 反向提示词。 |
| `fps` | 帧率参数，按上游工作流支持值透传。 |

JSON 示例：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-wanx",
    "image": "https://example.com/input.png",
    "prompt": "slow camera push in, stable motion, cinematic"
  }'
```

### Wan2.2 主体替换

模型：`aipdd-wan2.2-animater`

接口：`POST /v1/videos`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `video` | 源视频 URL。兼容旧字段 `load_video`。 |
| `prompt` 或 `positive_prompt` | 正向提示词，会映射到工作流 `positive_prompt`。 |

可选参数：

| 参数 | 说明 |
| --- | --- |
| `image` | 可选参考图片 URL。兼容旧字段 `fullpath`。 |
| `filename_prefix` | 工作流输出文件名前缀。普通调用一般不需要传。 |

JSON 示例：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-animater",
    "video": "https://example.com/source.mp4",
    "prompt": "natural motion, stable subject"
  }'
```

### MimicMotion 动作迁移

模型：`aipdd-mimic-motion`

接口：`POST /v1/videos`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `motion_video` | 动作参考视频 URL。 |
| `appearance_image` | 目标主体外观图片 URL。 |

JSON 示例：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-mimic-motion",
    "motion_video": "https://example.com/motion.mp4",
    "appearance_image": "https://example.com/person.png"
  }'
```

### Latentsync 对口型

模型：`aipdd-latentsync-1.5`

接口：`POST /v1/videos`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `video` | 输入视频 URL。 |
| `LoadAudio` | 驱动口型的音频 URL。字段大小写需要保持为 `LoadAudio`。 |

JSON 示例：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-latentsync-1.5",
    "video": "https://example.com/source.mp4",
    "LoadAudio": "https://example.com/speech.wav"
  }'
```

### IndexTTS 声音复刻

模型：`aipdd-indextts`

接口：`POST /v1/audio/speech`

必填参数：

| 参数 | 说明 |
| --- | --- |
| `input` 或 `text` | 要合成的文本。 |
| `audio` | 参考音频 URL。也可用 `ref_audio`、`reference_audio` 或 `voice`。 |

可选参数：

| 参数 | 说明 |
| --- | --- |
| `emotion_audio` | 情感参考音频 URL。 |

JSON 示例：

```bash
curl "$BASE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-indextts",
    "input": "这里填写需要合成的文本",
    "audio": "https://example.com/reference.wav"
  }'
```

## AIPDD 素材 URL

图片、视频和音频素材字段只接受 NewAPI 与 AIPDD 上游均可访问的 HTTP(S) URL。可以使用只含文本 URL 字段的 `multipart/form-data`，但不能包含二进制文件项；本地文件必须先上传到对象存储或其他文件服务。

## 异步任务创建响应

AIPDD 图片和音频任务创建成功后返回通用异步任务对象：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "image.generation.task",
  "created": 1778932446,
  "model": "aipdd-flux-gguf",
  "status": "queued",
  "metadata": {
    "endpoint_type": "image-generation"
  }
}
```

音频任务的 `object` 为 `audio.speech.task`。

AIPDD 视频任务和 Doubao Seedance 视频任务创建成功后返回 OpenAI Video 风格对象：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "aipdd-wan2.2-wanx",
  "status": "queued",
  "progress": 0,
  "created_at": 1778932446
}
```

`task_id` 是 NewAPI 生成的公开任务 ID。上游供应商真实任务 ID 会保存在服务端，不会直接暴露给用户。

## 查询任务

AIPDD 图片任务：

```bash
curl "$BASE_URL/v1/images/generations/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

AIPDD 和 Doubao Seedance 视频任务：

```bash
curl "$BASE_URL/v1/videos/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

AIPDD 音频任务：

```bash
curl "$BASE_URL/v1/audio/speech/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

## 查询响应

AIPDD 图片和音频查询接口返回通用任务响应。未完成时可能没有 `url` 或 `output`：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_xxx",
    "status": "processing",
    "url": "",
    "output": null,
    "metadata": {
      "urls": []
    },
    "error": null
  }
}
```

完成后会返回结果 URL：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_xxx",
    "status": "succeeded",
    "url": "https://example.com/result.png",
    "output": ["https://example.com/result.png"],
    "metadata": {
      "urls": ["https://example.com/result.png"]
    },
    "error": null
  }
}
```

视频查询接口 `GET /v1/videos/{task_id}` 返回 OpenAI Video 风格对象，适用于 AIPDD 视频模型和 Doubao Seedance：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "aipdd-wan2.2-wanx",
  "status": "completed",
  "progress": 100,
  "created_at": 1778932446,
  "completed_at": 1778932520,
  "metadata": {
    "url": "https://example.com/result.mp4"
  }
}
```

客户端建议按下面顺序取结果 URL：

1. `metadata.url`
2. `data.url`
3. `result_url`
4. `output[0]`
5. `metadata.urls[0]`
6. `data.metadata.urls[0]`

## 状态说明

异步任务创建接口成功时统一返回 `queued`，表示任务已提交。DeepSeek 文本和 Doubao Seedream 图片是同步响应，不进入任务状态流转。

查询接口常见状态：

| 状态 | 出现场景 | 含义 |
| --- | --- | --- |
| `queued` | AIPDD 图片、AIPDD 音频、视频 | 已提交或排队中。 |
| `processing` | AIPDD 图片、AIPDD 音频通用查询响应 | 生成中。 |
| `in_progress` | 视频 OpenAI Video 响应 | 生成中。 |
| `succeeded` | AIPDD 图片、AIPDD 音频通用查询响应 | 已完成，可以读取结果 URL。 |
| `completed` | 视频 OpenAI Video 响应 | 已完成，可以读取 `metadata.url`。 |
| `failed` | AIPDD 图片、AIPDD 音频、视频 | 任务失败。 |

AIPDD 上游返回的是数字 `task_status` 和 `task_result`；Doubao Seedance 上游返回 `pending`、`processing`、`succeeded`、`failed` 等字符串状态。NewAPI 会转换成上面的用户状态。对用户侧来说，是否完成应以查询响应里的 `status` 和结果 URL 为准。

## 计费说明

| 模型 | 计费方式 |
| --- | --- |
| `deepseek-v4-pro` | 按 token |
| `doubao-seedream-4-5` | 按次 |
| `doubao-seedance-2.0` | 按次 |
| `doubao-seedance-1.5` | 按次 |
| `aipdd-flux-gguf` | 按次 |
| `aipdd-flux-gguf-t2i` | 按次 |
| `aipdd-wan2.2-wanx` | 按次 |
| `aipdd-wan2.2-animater` | 按次 |
| `aipdd-mimic-motion` | 按次 |
| `aipdd-latentsync-1.5` | 按次 |
| `aipdd-indextts` | 按次 |

具体扣费金额以线上 `/api/pricing` 返回的模型价格和用户分组倍率为准。

## 常见错误

| 错误 | 说明 | 处理方式 |
| --- | --- | --- |
| `401` | Token 无效、过期、被禁用，或未带认证头。 | 检查 `Authorization: Bearer <NEW_API_TOKEN>`。 |
| `model_not_found` | 没有可用渠道或渠道未启用对应模型。 | 联系管理员检查渠道和模型配置。 |
| `unsupported_model` | 模型名不在当前模型列表中，或该供应商适配器不支持。 | 检查 `model` 是否拼写正确。 |
| `invalid_endpoint` | 模型和接口不匹配。 | 图片模型走 `/v1/images/generations`，视频模型走 `/v1/videos`，音频模型走 `/v1/audio/speech`。 |
| `not implemented` | 调用了当前渠道不支持的接口。 | DeepSeek 不要走 `/v1/responses`，改用 `/v1/chat/completions`。 |
| `task_not_exist` | 查询的 `task_id` 不属于当前用户或不存在。 | 确认保存的是创建响应里的公开 `task_id`。 |
| 一直 `queued` | 上游仍在排队或未开始执行。 | 继续低频轮询，必要时联系服务方检查上游队列。 |
| 没有结果 URL | 任务尚未完成，或上游完成但没有返回有效 URL。 | 等待完成状态后再读取；若已完成仍为空，联系服务方排查。 |

## AIPDD 异步图片 Node.js 示例

```js
const BASE_URL = process.env.BASE_URL
const TOKEN = process.env.NEW_API_TOKEN

async function createImageTask() {
  const res = await fetch(`${BASE_URL}/v1/images/generations`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${TOKEN}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      model: 'aipdd-flux-gguf',
      image: 'https://example.com/input.png',
      prompt: 'a cinematic product photo',
    }),
  })

  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

async function fetchImageTask(taskId) {
  const res = await fetch(`${BASE_URL}/v1/images/generations/${taskId}`, {
    headers: {
      Authorization: `Bearer ${TOKEN}`,
    },
  })

  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

async function waitForImage(taskId) {
  for (;;) {
    const task = await fetchImageTask(taskId)
    const data = task.data || task
    if (data.status === 'failed') {
      throw new Error(data.error || data.fail_reason || 'task failed')
    }
    if (data.status === 'succeeded') {
      return data.url || data.output?.[0] || data.metadata?.urls?.[0]
    }
    await new Promise((resolve) => setTimeout(resolve, 5000))
  }
}
```

## 上线前验证

可以先用真实 NewAPI Token 和公网素材 URL 做最小链路验证。DeepSeek 文本、Doubao Seedream 图片和 Doubao Seedance 视频可以直接用本文 curl 示例验证。

DeepSeek 文本最小验证：

```bash
curl "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [{"role": "user", "content": "ping"}],
    "max_tokens": 32
  }'
```

Doubao Seedream 最小验证：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedream-4-5",
    "prompt": "a small red cube on a white background",
    "size": "1920x1920"
  }'
```

Doubao Seedance 最小验证：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2.0",
    "prompt": "a small red cube rotating on a white background",
    "duration": 5
  }'
```

AIPDD 模型仓库提供了用户视角 smoke test，可以验证创建、轮询、结果解析链路。

```bash
node bin/aipdd-user-smoke-test.mjs \
  --base-url "$BASE_URL" \
  --token "$NEW_API_TOKEN" \
  --image-url "https://example.com/input.png" \
  --video-url "https://example.com/input.mp4" \
  --audio-url "https://example.com/reference.wav"
```

只验证创建任务、不等待结果：

```bash
node bin/aipdd-user-smoke-test.mjs --no-poll \
  --base-url "$BASE_URL" \
  --token "$NEW_API_TOKEN" \
  --image-url "https://example.com/input.png" \
  --video-url "https://example.com/input.mp4" \
  --audio-url "https://example.com/reference.wav"
```

只验证部分模型：

```bash
node bin/aipdd-user-smoke-test.mjs --only flux-i2i,flux-t2i,wanx \
  --base-url "$BASE_URL" \
  --token "$NEW_API_TOKEN" \
  --image-url "https://example.com/input.png"
```

## 与上游供应商的关系

用户不直接调用 DeepSeek、Doubao 或 AIPDD 上游接口。NewAPI 会根据模型选择对应渠道，把用户请求转换成上游需要的格式，并保存异步任务的上游真实任务 ID。

异步任务场景下，用户侧始终只使用 NewAPI 返回的公开 `task_xxx` 查询结果。
