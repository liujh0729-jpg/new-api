# AIPDD 模型客户接口文档

本文面向使用平台 API 的客户，介绍如何通过 NewAPI 调用当前已开放的 AIPDD 异步图片、视频和音频模型。

客户只需要使用平台发放的 NewAPI Token，不需要也不应在客户端传递 AIPDD 上游 API Key。

> 模型目录和价格可能由服务方动态调整。本文覆盖 AIPDD 异步任务模型、Seedance 2.0 视频模型，以及 AIPDD 目录中的 Ollama 兼容文本模型；实际可用模型请以 `GET /v1/models` 返回结果为准。

## 1. 基本信息

将以下变量替换为平台提供的服务地址和 Token：

```bash
export BASE_URL="https://your-newapi.example.com"
export NEW_API_TOKEN="sk-xxxxxxxx"
```

所有请求使用以下请求头：

```http
Authorization: Bearer <NEW_API_TOKEN>
Content-Type: application/json
```

AIPDD 模型是异步任务模型，调用流程如下：

1. 调用创建接口提交任务。
2. 保存响应中的 `id` 或 `task_id`。
3. 每隔 5～15 秒调用对应查询接口。
4. 任务完成后读取结果 URL；任务失败时读取错误信息。

AIPDD 图片、视频和音频异步任务模型不能使用 `/v1/chat/completions` 或 `/v1/responses` 调用；AIPDD 目录中的 Ollama 兼容文本模型应使用 `/v1/chat/completions`。

## 2. 查询可用模型

```bash
curl "$BASE_URL/v1/models" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

本文覆盖的模型如下：

| 能力 | 模型 | 创建接口 | 查询接口 | 必填输入 |
| --- | --- | --- | --- | --- |
| 图生图 | `aipdd-flux-gguf` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `image`、`prompt` |
| 文生图 | `aipdd-flux-gguf-t2i` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `prompt` |
| 图生视频 | `aipdd-wan2.2-wanx` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `image`、`prompt` |
| 主体替换视频 | `aipdd-wan2.2-animater` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `video`、`prompt` |
| 动作迁移视频 | `aipdd-mimic-motion` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `motion_video`、`appearance_image` |
| 对口型视频 | `aipdd-latentsync-1.5` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `video`、`LoadAudio` |
| 声音复刻/语音合成 | `aipdd-indextts` | `POST /v1/audio/speech` | `GET /v1/audio/speech/{task_id}` | `input`、`audio` |
| Seedance 2.0 视频 | `AP Seedance-2.0 VIP`、`AP Seedance-2.0 标准版`、`AP Seedance-2.0 轻量版`、`AP Seedance-2.0 高性价比版` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `prompt` 或 `content` |

素材字段应填写图片、视频或音频 URL。URL 必须能被 NewAPI 服务端和上游任务服务访问，推荐使用公网 HTTPS URL。

业务参数可以直接放在 JSON 顶层；如果客户端 SDK 只支持扩展参数，也可以将同名参数放入 `metadata` 对象，顶层参数优先。当前任务接口不负责把本地 multipart 文件转换成 AIPDD 素材，使用本地文件时请先上传到对象存储或其他文件服务，再将得到的 URL 传入任务请求。

### 2.1 Ollama 兼容文本模型

AIPDD 目录中的 Ollama 模型是同步的 OpenAI 兼容文本模型，不需要使用异步任务接口。模型 ID 由 AIPDD 目录动态提供，下面的 `qwen3:8b` 仅为请求示例，实际名称请以 `/v1/models` 返回结果为准。

调用接口：`POST /v1/chat/completions`

```bash
curl "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3:8b",
    "messages": [
      {"role": "user", "content": "请用三句话介绍 AIPDD。"}
    ],
    "temperature": 0.7,
    "stream": false
  }'
```

文本结果从 `choices[0].message.content` 读取，响应遵循 OpenAI Chat Completions 格式。需要流式输出时可将 `stream` 设置为 `true`；不同模型的流式能力以服务方实际开放情况为准。Ollama 模型不要调用 `/v1/images/generations`、`/v1/videos` 或 `/v1/audio/speech`。

## 3. 创建任务

### 3.1 Flux 图生图

模型：`aipdd-flux-gguf`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `aipdd-flux-gguf` |
| `image` | string | 是 | 输入图片 URL |
| `prompt` | string | 是 | 图片修改或生成提示词 |

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf",
    "image": "https://example.com/input.png",
    "prompt": "将产品放在简洁的白色摄影棚中，柔和灯光，高级产品摄影"
  }'
```

### 3.2 Flux 文生图

模型：`aipdd-flux-gguf-t2i`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `aipdd-flux-gguf-t2i` |
| `prompt` | string | 是 | 图片生成提示词 |

`text`、`input` 也可作为提示词字段，但新项目建议统一使用 `prompt`。

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf-t2i",
    "prompt": "一只戴着黄色雨衣的猫，站在雨后的城市街道上，电影感"
  }'
```

### 3.3 Wan2.2 图生视频

模型：`aipdd-wan2.2-wanx`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `aipdd-wan2.2-wanx` |
| `image` | string | 是 | 输入图片 URL |
| `prompt` | string | 是 | 视频动作和镜头提示词 |
| `negative_prompt` | string | 否 | 反向提示词 |
| `fps` | string/number | 否 | 上游工作流支持的帧率 |

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-wanx",
    "image": "https://example.com/input.png",
    "prompt": "镜头缓慢向前推进，主体自然运动，画面稳定，电影感"
  }'
```

### 3.4 Wan2.2 主体替换视频

模型：`aipdd-wan2.2-animater`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `aipdd-wan2.2-animater` |
| `video` | string | 是 | 待处理的视频 URL |
| `prompt` | string | 是 | 主体替换或动作提示词 |
| `image` | string | 否 | 参考图片 URL |
| `filename_prefix` | string | 否 | 输出文件名前缀，一般不需要传 |

`load_video` 是 `video` 的兼容别名；新项目建议使用 `video`。

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-animater",
    "video": "https://example.com/source.mp4",
    "prompt": "保持动作连续，主体边缘自然，画面稳定"
  }'
```

### 3.5 MimicMotion 动作迁移

模型：`aipdd-mimic-motion`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `aipdd-mimic-motion` |
| `motion_video` | string | 是 | 提供动作的视频 URL |
| `appearance_image` | string | 是 | 提供人物或主体外观的图片 URL |

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

### 3.6 Latentsync 对口型

模型：`aipdd-latentsync-1.5`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `aipdd-latentsync-1.5` |
| `video` | string | 是 | 待处理的视频 URL |
| `LoadAudio` | string | 是 | 对口型使用的音频 URL，字段名大小写必须保持一致 |

`audio` 是 `LoadAudio` 的兼容别名。

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

### 3.7 IndexTTS 声音复刻

模型：`aipdd-indextts`

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 固定为 `aipdd-indextts` |
| `input` | string | 是 | 需要合成的文本 |
| `audio` | string | 是 | 参考音频 URL |
| `emotion_audio` | string | 否 | 情感参考音频 URL |

`text` 可作为 `input` 的兼容别名；`ref_audio`、`reference_audio` 可作为 `audio` 的兼容别名。

```bash
curl "$BASE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-indextts",
    "input": "这里填写需要合成的文本。",
    "audio": "https://example.com/reference.wav"
  }'
```

### 3.8 Seedance 2.0 视频生成

Seedance 2.0 模型通过 `/v1/videos` 创建异步视频任务。常见模型 ID 包括：

- `AP Seedance-2.0 VIP`
- `AP Seedance-2.0 标准版`
- `AP Seedance-2.0 轻量版`
- `AP Seedance-2.0 高性价比版`

实际可用的模型 ID 以 `GET /v1/models` 为准，模型名必须完全匹配。

请求参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | Seedance 2.0 模型 ID |
| `prompt` | string | 与 `content` 二选一 | 文本提示词；未传 `content` 时会转换为文本内容 |
| `content` | array | 与 `prompt` 二选一 | 多模态内容，可包含 `text`、`image_url`、`video_url`、`audio_url` |
| `resolution` | string | 与 `width`/`height` 二选一 | `720p`、`1080p` 或 `4k` |
| `ratio` | string | 否 | 例如 `16:9`、`9:16`、`1:1`、`4:3`、`3:4` |
| `width`、`height` | number | 否 | 未传 `resolution` 或 `ratio` 时用于自动推导 |
| `duration` | number | 否 | 视频时长，单位为秒；不传时使用模型目录默认值 |
| `generate_audio` | boolean | 否 | 是否生成音频 |
| `seed`、`priority`、`service_tier`、`callback_url` | — | 否 | 任务控制参数，按服务方开放能力使用 |

最简单的文生视频请求：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "prompt": "黄昏时分的未来城市，镜头缓慢向前推进，电影感",
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5,
    "generate_audio": false
  }'
```

带参考图片的多模态请求：

```json
{
  "model": "AP Seedance-2.0 标准版",
  "resolution": "1080p",
  "ratio": "16:9",
  "duration": 5,
  "content": [
    {"type": "text", "text": "主体自然行走，镜头平稳跟随"},
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://example.com/reference.png"}
    }
  ]
}
```

当 `content` 非空时，平台优先使用 `content`；未传 `content` 时，平台根据 `prompt` 创建文本内容。提交成功后保存任务 ID，并按照[查询任务](#5-查询任务)轮询；视频查询响应格式与其他 AIPDD 视频模型相同。

## 4. 创建响应

图片模型和音频模型的创建响应示例：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "image.generation.task",
  "created": 1770000000,
  "model": "aipdd-flux-gguf-t2i",
  "status": "queued",
  "metadata": {
    "endpoint_type": "image_generation"
  }
}
```

`aipdd-indextts` 的 `object` 为 `audio.speech.task`。

视频模型的创建响应示例：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "aipdd-wan2.2-wanx",
  "status": "queued",
  "progress": 0,
  "created_at": 1770000000
}
```

`id` 和 `task_id` 都是平台公开任务 ID，查询时使用任意一个即可。不要使用 AIPDD 上游返回的内部任务 ID。

## 5. 查询任务

### 图片任务

```bash
curl "$BASE_URL/v1/images/generations/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

### 视频任务

```bash
curl "$BASE_URL/v1/videos/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

也支持兼容路径：`GET /v1/video/generations/{task_id}`。

### 音频任务

```bash
curl "$BASE_URL/v1/audio/speech/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

## 6. 查询响应和结果读取

### 6.1 图片/音频任务

图片和音频查询接口返回任务包装对象：

```json
{
  "code": "success",
  "data": {
    "error": null,
    "status": "succeeded",
    "task_id": "task_xxxxxxxxxxxxxxxx",
    "url": "https://example.com/result.png",
    "output": [
      "https://example.com/result.png"
    ],
    "metadata": {
      "url": "https://example.com/result.png",
      "urls": [
        "https://example.com/result.png"
      ]
    }
  }
}
```

读取结果时建议按以下顺序兼容：

1. `data.output`。
2. `data.url`。
3. `data.metadata.urls`。
4. `data.metadata.url`。

### 6.2 视频任务

视频查询接口返回 OpenAI Video 风格对象：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "aipdd-wan2.2-wanx",
  "status": "completed",
  "progress": 100,
  "created_at": 1770000000,
  "completed_at": 1770000120,
  "metadata": {
    "url": "https://example.com/result.mp4"
  }
}
```

视频结果通常从 `metadata.url` 读取；如果生成多个结果，也可能返回 `metadata.urls`。

### 6.3 状态值

图片/音频查询接口使用：

| 状态 | 含义 |
| --- | --- |
| `queued` | 已提交，等待处理 |
| `processing` | 处理中 |
| `succeeded` | 已完成，可读取结果 |
| `failed` | 失败，请读取 `error` |

视频查询接口使用：

| 状态 | 含义 |
| --- | --- |
| `queued` | 已提交，等待处理 |
| `in_progress` | 处理中 |
| `completed` | 已完成，可读取结果 |
| `failed` | 失败，请读取 `error` |

## 7. 错误处理

错误通常返回以下结构：

```json
{
  "code": "unsupported_model",
  "message": "unsupported AIPDD model: xxx",
  "data": null
}
```

常见错误：

| HTTP 状态 | `code` 示例 | 说明 |
| --- | --- | --- |
| 400 | `missing_model` | 未传 `model` |
| 400 | `unsupported_model` | 模型不存在或未开放 |
| 400 | `invalid_endpoint` | 模型与接口类型不匹配 |
| 400 | `model_price_error` | 模型价格未配置或请求参数不符合价格目录 |
| 400 | `task_not_exist` | 任务不存在，或任务不属于当前 Token 用户 |
| 401/403 | — | Token 无效、过期或无权限 |
| 429 | — | 当前分组或上游负载过高，请稍后重试 |
| 500/502 | — | 网关或上游服务异常 |

任务创建成功后，如果上游任务最终失败，查询响应中的 `error` 会包含失败原因。客户端不要因为创建接口返回 200 就认定任务已经成功，必须轮询到终态。

## 8. 使用建议

- 不要把 AIPDD 上游 API Key 放入浏览器、移动端或客户业务日志。
- 输入素材优先使用稳定的公网 HTTPS URL；过期、不可访问或需要登录的 URL 可能导致任务失败。
- 轮询间隔建议为 5～15 秒，并设置整体超时时间。
- 任务 ID 请按字符串保存，不要转换成整数。
- 价格由平台模型目录和用户分组配置决定，客户端不要硬编码价格。
- 如果服务方未开放某个模型，即使请求格式正确，也会返回 `unsupported_model`。
