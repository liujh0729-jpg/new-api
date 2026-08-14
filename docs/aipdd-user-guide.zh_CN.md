# AIPDD 渠道模型用户调用文档

本文面向 API 用户，说明如何通过 NewAPI 调用 **AIPDD 渠道**已开放的模型。调用方只需要使用平台发放的 NewAPI Token，不需要也不应在客户端传递 AIPDD 上游 API Key。

更新时间：2026-08-11。

> 模型目录和价格可能由服务方动态调整。实际可用模型、参数约束和价格请以 `GET /v1/models` 与 `/api/pricing` 返回结果为准。

## 1. 能力类型

| 类型 | 示例模型 | 响应方式 |
| --- | --- | --- |
| 文本（同步/流式） | AIPDD 目录中的 Ollama 模型（如 `qwen3:8b`） | `POST /v1/chat/completions` 直接返回 |
| 图片（同步） | 千问 `qwen-image` / `qwen-image-edit` 系列 | `POST /v1/images/generations` 或 `POST /v1/images/edits` 直接返回 |
| 图片（异步） | Flux 图生图 / 文生图、AIPDD 千问单/双/三参考图编辑 | 创建后轮询，`succeeded` 后取结果 |
| 视频（异步） | Wan2.2、LTX、MimicMotion、Latentsync、AP Seedance | 创建后轮询；`/v1/videos` 使用 `completed`，兼容路径实时查询使用 `succeeded`，非实时分支也可能返回内部大写状态 |
| 音频（异步） | IndexTTS | 创建后轮询，`succeeded` 后取结果 |

AIPDD 异步图片、视频、音频模型不能使用 `/v1/chat/completions` 或 `/v1/responses`；Ollama 文本模型应使用 `/v1/chat/completions`。

## 2. 基本流程

文本模型：

1. 调用 `POST /v1/chat/completions`。
2. 从 `choices[0].message.content` 读取结果（流式读 `choices[0].delta.content`）。

千问同步图片模型（如 `qwen-image-2.0-pro`、`qwen-image-edit-plus`）：

1. 调用 `POST /v1/images/generations` 或 `POST /v1/images/edits`。
2. 从响应的 `data[0].url` 读取图片 URL，或在请求 `response_format` 为 `b64_json` 时读取 `data[0].b64_json`。
3. 不需要调用查询接口轮询任务状态。

异步任务模型：

1. 调用创建接口提交任务。
2. 保存响应中的 `id` 或 `task_id`（公开任务 ID，按字符串保存）。
3. 每隔 5～15 秒调用对应查询接口。
4. 状态为 `succeeded`、`completed` 或内部成功状态 `SUCCESS` 后读取结果 URL；失败时读取 `error`。

> 模型 ID 以 `aipdd_qwen_image_edit_` 开头的三种千问参考图编辑能力属于异步任务，使用 `/v1/images/generations` 创建和查询，不使用同步的 `/v1/images/edits` 调用方式。

公共请求头：

```http
Authorization: Bearer <NEW_API_TOKEN>
Content-Type: application/json
```

示例环境变量：

```bash
export BASE_URL="https://your-newapi.example.com"
export NEW_API_TOKEN="sk-xxxxxxxx"
```

查询可用模型：

```bash
curl "$BASE_URL/v1/models" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

## 3. 模型总览

| 能力 | 模型名 | 创建接口 | 查询接口 | 必填输入 |
| --- | --- | --- | --- | --- |
| Ollama 文本 | 以 `/v1/models` 为准 | `POST /v1/chat/completions` | 无 | `messages` |
| 千问文生图 | 以 `/v1/models` 为准（如 `qwen-image-2.0-pro`） | `POST /v1/images/generations` | 无 | `input.messages` 或 `prompt` |
| 千问图像编辑 | 以 `/v1/models` 为准（如 `qwen-image-edit-plus`） | `POST /v1/images/edits` | 无 | `input.messages` 中的图片和文本 |
| 千问单参考图编辑（异步） | `aipdd_qwen_image_edit_single_reference` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `image_1`、`prompt` |
| 千问双参考图编辑（异步） | `aipdd_qwen_image_edit_dual_reference` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `image_1`、`image_2`、`prompt` |
| 千问三参考图编辑（异步） | `aipdd_qwen_image_edit_triple_reference` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `image_1`、`image_2`、`image_3`、`prompt` |
| Flux 图生图 | `aipdd-flux-gguf` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `image`、`prompt` |
| Flux 文生图 | `aipdd-flux-gguf-t2i` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `prompt` |
| Wan2.2 图生视频 | `aipdd-wan2.2-wanx` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `image`、`prompt` |
| Wan2.2 主体替换 | `aipdd-wan2.2-animater` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `video`、`prompt` |
| LTX 2.3 图生视频 | `aipdd_ltx_2.3` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `image`、`prompt` |
| LTX 2.3 首尾帧 | `aipdd_ltx_2.3 (首尾帧)` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | 首帧、尾帧、`prompt`、`timeline_data`、时长/帧数 |
| MimicMotion 动作迁移 | `aipdd-mimic-motion` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `motion_video`、`appearance_image` |
| Latentsync 对口型 | `aipdd-latentsync-1.5` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `video`、`LoadAudio` |
| IndexTTS 声音复刻 | `aipdd-indextts` | `POST /v1/audio/speech` | `GET /v1/audio/speech/{task_id}` | `input`、`audio` |
| AP Seedance 2.0 | `AP Seedance-2.0 VIP` / `标准版` / `轻量版` / `高性价比版` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `prompt` 或 `content`；并需可解析的 `resolution`；角色图片可用 `asset://` 或 `character_id`（见 [4.13.5](#4135-角色图片-asset--与角色库引用)） |

说明：

- 素材字段填写图片、视频或音频的 **公网 HTTPS URL**。URL 必须能被 NewAPI 服务端和 AIPDD 上游访问。
- **任务接口不会**把本地 multipart 文件自动上传到对象存储。本地文件请先上传到对象存储或其他文件服务，再把 URL 传入请求。
- 业务参数可放在 JSON 顶层，也可放入 `metadata`（**顶层优先**）。
- 视频模型可使用 `/v1/videos` 或兼容路径 `/v1/video/generations`。两条路径创建链路相同；查询响应格式不同，见第 7 节。
- 目录同步后可能出现额外模型；参数与必填项以当前开放能力为准。
- FunASR 等部分上游能力会被 NewAPI 刻意过滤，即使上游有目录项也不会对用户开放。

### 3.1 异步任务通用请求格式

下面两种写法等价：

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

### 3.2 图片张数（`n`）

图片类异步任务可传生成数量，下列字段等价（取正整数）：

- `n`
- `image_count`
- `count`

当目录/工作流声明了批量参数（如 `batch_size`）时，会映射到上游；`n > 1` 时计费可按张数倍率结算。是否支持多图取决于模型目录。

## 4. 调用示例

### 4.1 Ollama 兼容文本模型

AIPDD 目录中的 Ollama 模型为同步 OpenAI 兼容文本模型。模型 ID 动态提供，下列 `qwen3:8b` 仅为示例。

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

结果从 `choices[0].message.content` 读取。需要流式时将 `stream` 设为 `true`。不要对 Ollama 模型调用图片/视频/音频异步接口。

### 4.2 Flux 图生图

模型：`aipdd-flux-gguf`  
接口：`POST /v1/images/generations`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `image` | 是 | 输入图片 URL；也可用 `images` 数组（取第一张） |
| `prompt` | 是 | 正向提示词，映射为工作流 `positive_prompt` |
| `positive_prompt` | 否 | 优先级高于 `prompt` |
| `negative_prompt` | 否 | 反向提示词 |
| `n` / `image_count` / `count` | 否 | 生成张数，见 [3.2](#32-图片张数n) |

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf",
    "image": "https://example.com/input.png",
    "prompt": "将产品放在简洁的白色摄影棚中，柔和灯光，高级产品摄影",
    "n": 1
  }'
```

### 4.3 Flux 文生图

模型：`aipdd-flux-gguf-t2i`  
接口：`POST /v1/images/generations`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `prompt` / `text` / `input` | 是 | 提示词；新项目建议统一用 `prompt` |
| `n` / `image_count` / `count` | 否 | 生成张数 |

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf-t2i",
    "prompt": "一只戴着黄色雨衣的猫，站在雨后的城市街道上，电影感"
  }'
```

### 4.4 千问文生图

模型：千问 `qwen-image` 系列；具体可用模型以 `GET /v1/models` 返回结果为准。下面以 `qwen-image-2.0-pro` 为例。

接口：`POST /v1/images/generations`

千问图片模型使用同步返回，不需要保存任务 ID 或轮询。推荐使用百炼兼容的 `input.messages` 格式：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 是 | 当前开放的 `qwen-image` 系列模型 |
| `input.messages` | 是 | 用户消息；文生图时 `content` 中放一个 `text` |
| `parameters.n` | 否 | 生成数量 |
| `parameters.size` | 否 | 图片尺寸，例如 `1328*1328` |
| `parameters.negative_prompt` | 否 | 反向提示词 |
| `parameters.prompt_extend` | 否 | 是否启用提示词扩展 |
| `parameters.watermark` | 否 | 是否添加水印 |

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-2.0-pro",
    "input": {
      "messages": [
        {
          "role": "user",
          "content": [
            {"text": "一只戴着黄色雨衣的猫，站在雨后的城市街道上，电影感"}
          ]
        }
      ]
    },
    "parameters": {
      "n": 1,
      "size": "1328*1328",
      "prompt_extend": true,
      "watermark": false
    }
  }'
```

也可以使用简化的 OpenAI 风格请求：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-2.0-pro",
    "prompt": "一只戴着黄色雨衣的猫，站在雨后的城市街道上，电影感"
  }'
```

### 4.5 千问图像编辑

模型：千问 `qwen-image-edit` 系列；具体可用模型以 `GET /v1/models` 返回结果为准。下面以 `qwen-image-edit-plus` 为例。

接口：`POST /v1/images/edits`

图片使用公网 HTTPS URL，放在 `input.messages[].content[].image` 中；编辑指令放在同一条消息的 `text` 中。接口同步返回，不需要轮询。

```bash
curl "$BASE_URL/v1/images/edits" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "input": {
      "messages": [
        {
          "role": "user",
          "content": [
            {"image": "https://example.com/input.png"},
            {"text": "把图片背景改成安静的森林，并保持主体不变"}
          ]
        }
      ]
    },
    "parameters": {
      "n": 1,
      "prompt_extend": true,
      "watermark": false
    }
  }'
```

成功响应统一为 OpenAI 图片格式：

```json
{
  "created": 1760000000,
  "data": [
    {
      "url": "https://example.com/generated.png",
      "b64_json": "",
      "revised_prompt": ""
    }
  ]
}
```

#### 4.5.1 AIPDD 千问单/双/三参考图编辑（异步）

下列三个以 `aipdd_` 开头的模型由 AIPDD ComfyUI 工作流执行，与上面的同步 `qwen-image-edit` 系列不是同一种调用方式：

| 模型 | 参考图字段 | 必填字段 |
| --- | --- | --- |
| `aipdd_qwen_image_edit_single_reference` | `image_1` | `image_1`、`prompt` |
| `aipdd_qwen_image_edit_dual_reference` | `image_1`、`image_2` | `image_1`、`image_2`、`prompt` |
| `aipdd_qwen_image_edit_triple_reference` | `image_1`、`image_2`、`image_3` | `image_1`、`image_2`、`image_3`、`prompt` |

- 创建接口：`POST /v1/images/generations`
- 查询接口：`GET /v1/images/generations/{task_id}`

请求约束：

- 每个 `image_N` 都传一条可由 NewAPI 和 AIPDD 节点访问的公网 HTTPS 图片 URL。
- 推荐把 `image_1`、`image_2`、`image_3` 和 `prompt` 直接放在 JSON 顶层。
- 多参考图不要改传通用 `images` 数组；请使用模型声明的精确编号字段，避免多张图片被错误映射。
- 创建接口返回异步任务，必须保存 `id` 或 `task_id` 并轮询，不能直接读取 `data[0].url`。

单参考图：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd_qwen_image_edit_single_reference",
    "image_1": "https://example.com/reference-a.png",
    "prompt": "保持主体不变，把背景改成夜晚的霓虹城市"
  }'
```

双参考图：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd_qwen_image_edit_dual_reference",
    "image_1": "https://example.com/reference-a.png",
    "image_2": "https://example.com/reference-b.png",
    "prompt": "融合两张参考图的主体和配色，生成一张完整海报"
  }'
```

三参考图：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd_qwen_image_edit_triple_reference",
    "image_1": "https://example.com/reference-a.png",
    "image_2": "https://example.com/reference-b.png",
    "image_3": "https://example.com/reference-c.png",
    "prompt": "融合三张参考图的主体、构图和色彩，生成一张统一风格的海报"
  }'
```

创建成功后会返回 `queued` 状态的任务对象。轮询示例：

```bash
curl "$BASE_URL/v1/images/generations/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

状态变为 `succeeded` 后，优先从 `data.output[0]` 读取结果 URL；兼容读取位置还有 `data.url` 和 `data.metadata.urls[0]`。上游可用节点较少或 ComfyUI 正忙时，任务可能暂时保持 `queued`，建议每 5～15 秒轮询。

### 4.6 Wan2.2 图生视频

模型：`aipdd-wan2.2-wanx`  
接口：`POST /v1/videos`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `image` | 是 | 输入图片 URL |
| `prompt` | 是 | 视频动作与镜头提示词 |
| `negative_prompt` | 否 | 反向提示词 |
| `fps` | 否 | 帧率，按上游工作流透传 |

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

说明：当前默认按 **按次** 计费。请求里即使带了 `duration`/`seconds`，也不一定会作为上游工作流参数转发；是否按时长计费以同步后的模型目录 `billingType` 为准。

### 4.7 Wan2.2 主体替换

模型：`aipdd-wan2.2-animater`  
接口：`POST /v1/videos`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `video` | 是 | 源视频 URL；兼容 `load_video` |
| `prompt` / `positive_prompt` | 是 | 正向提示词 |
| `image` | 否 | 参考图；兼容 `fullpath`、`reference_image` |
| `filename_prefix` | 否 | 输出文件名前缀，一般不需要 |

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

### 4.8 LTX 2.3 图生视频

模型：`aipdd_ltx_2.3`  
接口：`POST /v1/videos`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `prompt` | 是 | 视频提示词 |
| `image` | 是 | 输入图片 URL |
| `negative_prompt` / `negativePrompt` | 否 | 反向提示词 |
| `width`、`height` | 否 | 分辨率；省略时常用默认 `640x640` |
| `duration` / `seconds` | 否 | 时长 **1～20** 秒整数；会换算 `numFrames = duration * 24 + 1` |
| `numFrames` / `num_frames` | 否 | 帧数；与 `duration` 同时传时必须匹配 |
| `frameRate` / `fps` | 否 | 固定为 **24**；传其他值会报错 |
| `seed` | 否 | 随机种子 |

允许分辨率（`width`×`height`）：

`1280x704`、`704x1280`、`704x704`、`640x640`、`640x480`、`480x640`、`480x480`

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd_ltx_2.3",
    "prompt": "镜头缓慢推进，主体自然运动，画面稳定",
    "image": "https://example.com/input.png",
    "width": 1280,
    "height": 704,
    "duration": 5
  }'
```

### 4.9 LTX 2.3 首尾帧视频

模型：`aipdd_ltx_2.3 (首尾帧)`  
接口：`POST /v1/videos`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `first_frame` / `first_frame_image` / `image` | 是 | 首帧图片 URL；`images[0]` 也可 |
| `last_frame` / `last_frame_image` / `image_tail` | 是 | 尾帧图片 URL；`images[1]` 也可 |
| `prompt` | 是 | 映射为 `local_prompts` 与 `global_prompt` |
| `timeline_data` | 是 | 时间线数据，必须是 **合法 JSON**（对象或数组；字符串会先解析） |
| `length` / `numFrames` / `duration` | 是（其一） | 帧长；`duration` 时 `length = duration * 24 + 1` |
| `audio` / `audio_url` | 否 | 参考音频 URL |
| `width`、`height` | 视目录 | 按目录要求传 |

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd_ltx_2.3 (首尾帧)",
    "first_frame": "https://example.com/first.png",
    "last_frame": "https://example.com/last.png",
    "prompt": "从首帧平滑过渡到尾帧，动作自然",
    "timeline_data": {"segments": []},
    "duration": 5
  }'
```

### 4.10 MimicMotion 动作迁移

模型：`aipdd-mimic-motion`  
接口：`POST /v1/videos`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `motion_video` | 是 | 动作参考视频；兼容 `video`、`load_video` |
| `appearance_image` | 是 | 外观参考图；兼容 `image` |

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

### 4.11 Latentsync 对口型

模型：`aipdd-latentsync-1.5`  
接口：`POST /v1/videos`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `video` | 是 | 输入视频 URL |
| `LoadAudio` | 是 | 驱动口型的音频 URL；**大小写必须一致**；兼容 `audio` |

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

### 4.12 IndexTTS 声音复刻

模型：`aipdd-indextts`  
接口：`POST /v1/audio/speech`

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `input` / `text` | 是 | 要合成的文本 |
| `audio` | 是 | 参考音频 URL；兼容 `ref_audio`、`reference_audio`、`voice`；也可放在 `metadata.audio` |
| `emotion_audio` | 否 | 情感参考音频 URL |

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

### 4.13 AP Seedance 2.0 视频

接口：`POST /v1/videos` 或 `POST /v1/video/generations`  
查询：`GET /v1/videos/{task_id}`（OpenAI 风格，推荐）或 `GET /v1/video/generations/{task_id}`（兼容路径，返回通用 `code/data` 封装）

执行协议为 `seedance_official` 时，NewAPI 会归一化请求并转发到 Seedance 官方任务接口。

**OpenAI 风格（通用字段）与 Seedance 官方写法均支持。**  
四个档位的请求字段集合相同；**唯一按档位变化的能力是 `resolution`**。

#### 4.13.1 档位与可用分辨率

模型名须与 `/v1/models` 完全匹配：

| 档位 | 模型名 | 可用 `resolution` | 说明 |
| --- | --- | --- | --- |
| VIP | `AP Seedance-2.0 VIP` | `720p`、`1080p`、`4k` | 全分辨率高优先级档 |
| 标准版 | `AP Seedance-2.0 标准版` | `720p`、`1080p`、`4k` | 与 VIP 同分辨率能力 |
| 轻量版 | `AP Seedance-2.0 轻量版` | `720p`、`1080p` | 不支持 `4k` |
| 高性价比版 | `AP Seedance-2.0 高性价比版` | `1080p`、`4k` | 不支持 `720p` |

> 实测同步价格目录中：`480p` 在 VIP / 标准版 / 轻量版均返回 `model_price_error`（HTTP 400），**当前不可用**。Playground 若仍展示 `480p`，以价格目录校验结果为准，不要按 UI 展示直接提交。  
> 同步价格目录是分辨率能力的最终依据；**不会**因 `width`/`height` 能推导出 `4k` 就自动放行未定价档位，也**不会**自动降级。

#### 4.13.2 公共可用参数

| 参数 | 必填 | 可用取值 / 约束 | 说明 |
| --- | --- | --- | --- |
| `model` | 是 | 见上表四个档位全名 | 必须完全匹配 |
| `prompt` 或 `content` | 二选一 | 至少其一非空 | 未传非空 `content` 时，系统将 `prompt` 转为 `{"type":"text","text":...}` |
| `resolution` | 与尺寸二选一 | **仅限该档位可用值** | 优先根级，其次 `metadata.resolution`；也可由 `width`/`height` 推导 |
| `ratio` | 否 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4` | 未传时可由 `width`/`height` 推导 |
| `duration` / `seconds` | 否 | 正数秒；不传默认按 **5 秒**计费与提交 | 优先直接传秒数 |
| `width`、`height` | 否 | 须同时为正整数 | 仅在未传 `resolution`/`ratio` 时用于推导；短边 `720`→`720p`，`1080`→`1080p`，`2160`→`4k` |
| `content` | 否 | 非空数组；每项必须有 `type` | 官方多模态内容，见 [4.13.3](#4133-content-项与参考素材) |
| `image` / `images` | 否 | 公网 HTTPS URL，或 `asset://{asset_id}` | 首帧 / 参考图简写；`images` 会转为 `reference_image`；也可写入 `content` |
| `character_id` | 否 | 正整数 | NewAPI 角色库角色 ID；一个角色固定对应一张图片；仅 `POST /v1/video/generations`（及控制台 `/pg/video/generations`）支持，见 [4.13.5](#4135-角色图片-asset--与角色库引用) |
| `character_asset_id` | 否 | 已废弃 | 仅为旧客户端兼容而静默忽略，不会选择图片、写入 metadata 或转发上游 |
| `generate_audio` | 否 | `true` / `false` | 是否生成同步音频；显式 `false` 会保留 |
| `seed` | 否 | 整数 | 随机种子 |
| `service_tier` | 否 | 字符串 | 上游服务档位控制 |
| `priority` | 否 | 整数；显式 `0` 会保留 | 任务优先级 |
| `callback_url` | 否 | HTTPS URL | 任务完成回调 |
| `return_last_frame` | 否 | 布尔 | 是否返回末帧 |
| `metadata` | 否 | 对象 | 上述业务字段也可放这里；**根级优先于 `metadata`** |

参数优先级：

```text
content:    根级非空 content > metadata.content > prompt 转换
resolution: 根级 resolution > metadata.resolution > width/height 推导
ratio:      根级 ratio > metadata.ratio > width/height 推导
```

注意：部分 Seedance 档位不接受 `frames` / `frames_per_second` 推导时长，请直接传 `duration`。

#### 4.13.3 `content` 项与参考素材

| `type` | 相关字段 | `role` 示例 | 约束 |
| --- | --- | --- | --- |
| `text` | `text` | — | 文生视频主提示 |
| `image_url` | `image_url.url` | `reference_image`、`first_frame`、`last_frame` | 公网 HTTPS 图片 URL，或火山官方素材引用 `asset://{asset_id}`（见 [4.13.5](#4135-角色图片-asset--与角色库引用)） |
| `video_url` | `video_url.url` | `reference_video` | 公网 HTTPS 视频 URL；含参考视频可能走不同价格变体 |
| `audio_url` | `audio_url.url` | `reference_audio` | 公网 HTTPS 音频 URL；通常需同时有图或视频参考 |

**重要：同一请求的 `content` 只能二选一，不可混用。**

| 模式 | 允许的 `role` | 禁止同时出现 |
| --- | --- | --- |
| 参考素材模式 | `reference_image`、`reference_video`、`reference_audio` | `first_frame`、`last_frame` |
| 首尾帧模式 | `first_frame`、`last_frame` | `reference_image`、`reference_video`、`reference_audio` |

混用会返回上游 `400`，典型错误：

```text
The parameter `content` specified in the request is not valid:
first/last frame content cannot be mixed with reference media content.
```

补充约束：

1. 普通素材须为 **可被 NewAPI 与上游访问的公网 HTTPS URL**；任务接口不会自动上传本地文件，也不接受私有链接、内网地址、过期签名 URL。角色托管图片另见 `asset://`（[4.13.5](#4135-角色图片-asset--与角色库引用)）。
2. 参考视频单条建议时长约 **2～15.2 秒**；参考视频/音频总时长建议不超过 **15.2 秒**。超时会返回类似 `video duration ... must be less than or equal to 15.2` 的错误。
3. 输出时长请直接传合法 `duration` / `seconds`；非法值会返回 `duration` 参数无效。
4. 参考图/视频若被判定含真人，上游可能直接拒绝（如 `input image/video may contain real person`）。写实人像应先入库拿到 `asset://`，再作为 `reference_image` 引用。
5. `Invalid video_url` 通常表示参考视频 URL 无效或不可访问，不是模型名错误。

参考素材数量建议上限（与 Playground 校验一致）：

| 类型 | 上限 |
| --- | --- |
| 合计 | 12 |
| 图片 | 9 |
| 视频 | 3 |
| 音频 | 3 |

#### 4.13.4 调用示例

OpenAI 风格文生视频（标准版；`width`/`height` 会推导为 `resolution=720p`、`ratio=16:9`）：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "prompt": "黄昏时分的未来城市，镜头缓慢向前推进，电影感",
    "width": 1280,
    "height": 720,
    "duration": 5,
    "generate_audio": false
  }'
```

参考素材模式（VIP + 参考图，推荐日常使用）：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 VIP",
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
    ],
    "generate_audio": false
  }'
```

首尾帧模式（只使用 `first_frame` / `last_frame`，不要再带 `reference_*`）：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 轻量版",
    "resolution": "1080p",
    "ratio": "9:16",
    "duration": 5,
    "content": [
      {"type": "text", "text": "从首帧平滑过渡到尾帧，动作自然"},
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {"url": "https://example.com/first.png"}
      },
      {
        "type": "image_url",
        "role": "last_frame",
        "image_url": {"url": "https://example.com/last.png"}
      }
    ]
  }'
```

错误示例（会 400：首尾帧与参考素材混用）：

```json
{
  "model": "AP Seedance-2.0 轻量版",
  "resolution": "1080p",
  "duration": 5,
  "content": [
    {"type": "text", "text": "..."},
    {
      "type": "image_url",
      "role": "first_frame",
      "image_url": {"url": "https://example.com/first.png"}
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://example.com/ref.png"}
    },
    {
      "type": "video_url",
      "role": "reference_video",
      "video_url": {"url": "https://example.com/ref.mp4"}
    }
  ]
}
```

轻量版（仅 `720p` / `1080p`）：

```json
{
  "model": "AP Seedance-2.0 轻量版",
  "prompt": "海边日落，缓慢推进",
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 5
}
```

高性价比版（仅 `1080p` / `4k`）：

```json
{
  "model": "AP Seedance-2.0 高性价比版",
  "prompt": "城市夜景航拍，霓虹倒映在湿润路面",
  "resolution": "4k",
  "ratio": "16:9",
  "duration": 5,
  "generate_audio": true
}
```

#### 4.13.5 角色图片 `asset://` 与角色库引用

Seedance 2.0 支持引用火山托管角色图片。**上游官方写法**是把素材 ID 写成 `asset://{asset_id}`，放在 `content` 的 `image_url.url`（`role` 为 `reference_image`）中。NewAPI 会先把每个 `asset://` 反查到本地角色库并鉴权，再转发上游；未知或重复注册的 Asset ID 会被拒绝。NewAPI 角色库采用“一角色一图”，上传新图片必须创建新角色，不支持在已有角色下追加或替换图片。

真人形象使用独立的私有授权链路：肖像权人须查看并确认用途、地区、平台、行业、商业使用和有效期，再完成火山 H5 身份核验；系统随后从 `LivenessFace` Group 中确认唯一 Active Image Asset。完整设计及管理员前置条件见 [真人形象素材库实施说明](real-person-character-library.zh_CN.md)。

使用 API Key 上传角色：

- 创建：`POST /v1/virtual-characters`，请求格式为 `multipart/form-data`。
- 列表：`GET /v1/virtual-characters?scope=public` 查询公开角色；`scope=private` 查询当前 API Key 用户的私有角色。可用 `source_type=volc_aigc` 或 `source_type=volc_real_person` 区分“我的虚拟形象”和“我的真人”。不传 `scope` 时默认为 `public`。
- 查询：`GET /v1/virtual-characters/{id}`，轮询响应中的 `data.status`；正常状态流转为 `creating → active`，终态失败为 `failed`，并在 `data.last_error` 中返回原因。
- 鉴权：以上接口均使用 `Authorization: Bearer $NEW_API_TOKEN`。角色归属于 API Key 对应的用户，并沿用该用户的角色配额与上传限流。
- 字段：`name` 和 `file` 必填；`description`、`tags` 可选。`tags` 可传 JSON 字符串数组或逗号分隔文本。
- 图片：支持 JPG/JPEG、PNG、WebP、GIF、HEIC，最大 30 MB。

公开角色支持分页及条件筛选：

```bash
curl "$BASE_URL/v1/virtual-characters?scope=public&p=1&page_size=20&keyword=摄影师&gender=男&age_band=20-40" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

查询当前用户的私有角色：

```bash
curl "$BASE_URL/v1/virtual-characters?scope=private&p=1&page_size=20&status=active" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

可用筛选参数包括 `keyword`、`nationality`、`gender`、`age_band` 和 `status`。`age_band` 可选 `0-20`、`20-40`、`40-60`、`60-80`、`80-100`；`status` 可选 `creating`、`active`、`blocked`、`offline`、`deleting`、`failed`。响应中的 `data.page.items` 为角色数组，`data.used` / `data.limit` 表示当前用户私有角色配额。

```bash
curl "$BASE_URL/v1/virtual-characters" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -F 'name=街头摄影师' \
  -F 'description=短发，深色夹克，写实风格' \
  -F 'tags=["写实","现代"]' \
  -F 'file=@./character.png;type=image/png'
```

创建成功返回 HTTP `201`。响应中的角色初始为 `creating`，例如：

```json
{
  "success": true,
  "data": {
    "id": 12,
    "name": "街头摄影师",
    "status": "creating",
    "provider_asset_id": "asset-2026xxxx-xxxxx",
    "cover_url": "/api/virtual-characters/12/preview",
    "mime_type": "image/png"
  }
}
```

随后查询处理状态：

```bash
curl "$BASE_URL/v1/virtual-characters/12" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

| 引用方式 | 字段 | 适用创建接口 | 说明 |
| --- | --- | --- | --- |
| 已注册 `asset://` | `content[].image_url.url`，或简写 `images` / `image` | 支持 Seedance 的视频创建入口 | 传角色库中已唯一注册的火山素材 ID，**请求里请写完整** `asset://{asset_id}`；每个 ID 都会反查所有权、授权期限和火山状态 |
| NewAPI 角色库 | `character_id` | 支持 Seedance 的视频创建入口；控制台为 `POST /pg/video/generations` | 平台内部角色 ID；中间件读取角色的唯一 `provider_asset_id`，注入 `images: ["asset://..."]` 后再转发上游 |

两种 ID 不要混用：

| 名称 | 来源 | 示例 |
| --- | --- | --- |
| 火山 `asset_id` / `provider_asset_id` | 角色库详情里的唯一图片引用，或上游素材接口返回值 | `asset-2026xxxx-xxxxx` → 请求里写 `asset://asset-2026xxxx-xxxxx` |
| `character_id` | NewAPI 角色库角色主键 | `12` |

约束：

1. 提示词用「图片1」「图片2」按顺序指代表材，**不要**在文案里写 `asset://` 原文。
2. 多个已注册 `asset://` 可在同一 `content` 中组合，并会逐一鉴权和写入任务审计；它也可与公网 HTTPS 参考图混用。仍须遵守 [4.13.3](#4133-content-项与参考素材) 的「参考素材模式 / 首尾帧模式」二选一，不可与 `first_frame`/`last_frame` 混用。
3. 使用 `character_id` 时：**不要**再同时传 `image` / `images` / `first_frame` / `last_frame` / `input_reference`，也不要再传非空的 `metadata.content`；否则返回 `character_reference_conflict`。
4. 真人角色无论通过 `character_id` 还是直接 `asset://` 使用，都必须属于当前用户、授权未过期、授权证据完整，并与管理员绑定的豆包视频通道及火山账号/项目一致。
5. 旧请求中的 `character_asset_id` 会被静默忽略；JSON 与 multipart 请求都不会将其转发到 metadata 或上游。
6. 模型必须是当前账号可用的 Seedance 档位；角色及其唯一图片须为可访问且状态为 Active。直接 `asset://` 必须先同步或创建到本地角色库，不能用它绕过真人授权链路。

官方 `asset://` 示例（可用 `/v1/videos`）：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5,
    "content": [
      {"type": "text", "text": "图片1中的角色缓慢转身，保持外观一致，电影感"},
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {"url": "asset://asset-2026xxxx-xxxxx"}
      }
    ],
    "generate_audio": false
  }'
```

等价简写（`images` 会转为 `reference_image`）：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "prompt": "图片1中的角色缓慢转身，保持外观一致",
    "images": ["asset://asset-2026xxxx-xxxxx"],
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5
  }'
```

角色库封装示例（须走 `/v1/video/generations`）：

```bash
curl "$BASE_URL/v1/video/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "character_id": 12,
    "prompt": "以图片1中的角色为主体，缓慢走向镜头，自然光",
    "duration": 5,
    "metadata": {
      "ratio": "16:9",
      "resolution": "720p"
    }
  }'
```

## 5. 素材字段别名（URL 字符串）

任务请求中的素材字段应传 URL 字符串。常见别名如下（新项目优先用推荐名）：

| 模型 | 推荐字段 | 兼容别名 |
| --- | --- | --- |
| `aipdd_qwen_image_edit_single_reference` | `image_1`、`prompt` | 无；请使用精确字段 |
| `aipdd_qwen_image_edit_dual_reference` | `image_1`、`image_2`、`prompt` | 无；请使用精确字段 |
| `aipdd_qwen_image_edit_triple_reference` | `image_1`、`image_2`、`image_3`、`prompt` | 无；请使用精确字段 |
| `aipdd-flux-gguf` | `image` | `images`（取第一张） |
| `aipdd-flux-gguf-t2i` | `prompt` | `text`、`input` |
| `aipdd-wan2.2-wanx` | `image`、`prompt` | `images` |
| `aipdd-wan2.2-animater` | `video`、`prompt` | `load_video`；参考图 `fullpath`、`reference_image` |
| `aipdd_ltx_2.3` | `image`、`prompt` | `images`；`negative_prompt` ↔ `negativePrompt` |
| `aipdd_ltx_2.3 (首尾帧)` | `first_frame`、`last_frame` | `first_frame_image`、`last_frame_image`、`image_tail`、`images[0/1]` |
| `aipdd-mimic-motion` | `motion_video`、`appearance_image` | `video`/`load_video`；`image` |
| `aipdd-latentsync-1.5` | `video`、`LoadAudio` | `load_video`；`audio` |
| `aipdd-indextts` | `input`、`audio` | `text`；`ref_audio`、`reference_audio`、`voice`、`metadata.audio` |
| AP Seedance 2.0 | `content` / `prompt`、`resolution` | 参考图：`image`/`images`（HTTPS 或 `asset://`）；角色库：`character_id`（仅 `/v1/video/generations`；旧 `character_asset_id` 会被忽略） |

## 6. 创建响应

图片/音频异步任务：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "image.generation.task",
  "created": 1770000000,
  "model": "aipdd-flux-gguf-t2i",
  "status": "queued",
  "metadata": {
    "endpoint_type": "image-generation"
  }
}
```

`aipdd-indextts` 的 `object` 为 `audio.speech.task`，`metadata.endpoint_type` 为 `audio-speech`。

视频任务（含 AP Seedance）：

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

`id` 与 `task_id` 均为平台公开任务 ID，查询时任选其一。不要使用上游内部任务 ID。

## 7. 查询任务

```bash
# 图片
curl "$BASE_URL/v1/images/generations/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"

# 视频 OpenAI 风格（推荐）
curl "$BASE_URL/v1/videos/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"

# 视频兼容路径
curl "$BASE_URL/v1/video/generations/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"

# 音频
curl "$BASE_URL/v1/audio/speech/$TASK_ID" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

### 7.1 图片 / 音频 / 兼容视频路径查询响应

`GET /v1/images/generations/{task_id}`、`GET /v1/audio/speech/{task_id}`，以及 `GET /v1/video/generations/{task_id}`，在实时查询成功时通常返回：

```json
{
  "code": "success",
  "data": {
    "error": null,
    "status": "succeeded",
    "task_id": "task_xxxxxxxxxxxxxxxx",
    "url": "https://example.com/result.png",
    "output": ["https://example.com/result.png"],
    "metadata": {
      "urls": ["https://example.com/result.png"]
    }
  }
}
```

如果渠道不支持实时查询，兼容路径会返回任务记录的通用 DTO，例如 `data.status` 可能是 `IN_PROGRESS` 或 `SUCCESS`，而不是上例中的小写状态。客户端处理兼容路径时应同时接受这两组值；如果需要固定的 OpenAI 状态枚举，请使用 `GET /v1/videos/{task_id}`。

结果读取顺序建议：`data.output` → `data.url` → `data.metadata.urls` → `data.metadata.url`。

### 7.2 `/v1/videos/{task_id}` 查询响应

OpenAI Video 风格：

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

视频结果通常从 `metadata.url` 读取；多结果时也可能有 `metadata.urls`。兼容读取顺序：

1. `metadata.url`
2. `data.url`
3. `result_url`
4. `output[0]`
5. `metadata.urls[0]`
6. `data.metadata.urls[0]`

失败时可能带 `error.message` / `error.code`（Seedance 官方失败会尽量透传上游原因）。

### 7.3 状态值

| 状态 | 场景 | 含义 |
| --- | --- | --- |
| `queued` | 创建成功 / 查询 | 已提交或排队中 |
| `processing` | 图片/音频/兼容视频路径查询 | 处理中 |
| `in_progress` | `GET /v1/videos/{task_id}` | 处理中 |
| `succeeded` | 图片/音频/兼容视频路径查询 | 完成，可取结果 |
| `completed` | `GET /v1/videos/{task_id}` | 完成，可取结果 |
| `failed` | 全部 | 失败，读取 `error` |
| `NOT_START` | 兼容路径非实时查询 | 尚未开始 |
| `SUBMITTED` | 兼容路径非实时查询 | 已提交 |
| `QUEUED` | 兼容路径非实时查询 | 排队中 |
| `IN_PROGRESS` | 兼容路径非实时查询 | 处理中 |
| `SUCCESS` | 兼容路径非实时查询 | 已完成，可取结果 |
| `FAILURE` | 兼容路径非实时查询 | 失败，读取错误信息 |
| `UNKNOWN` | 兼容路径非实时查询 | 未知状态 |

创建接口返回 200 只表示任务已提交，不代表生成成功，必须轮询到终态。

## 8. 计费说明

| 模型类型 | 计费方式 |
| --- | --- |
| Ollama 文本 | 按 token |
| Flux / Wan / Mimic / Latentsync / IndexTTS / LTX | 默认多为按次；Flux 在 `n>1` 且目录支持批量时按张数倍率 |
| AP Seedance | 按分辨率档位、时长；参考视频可能触发不同价格变体 |
| 目录声明 `duration_seconds` 的模型 | 按时长秒数（以同步目录为准，不要按模型名硬编码） |

具体金额以 `/api/pricing` 与用户分组倍率为准，客户端不要硬编码价格。

## 9. 常见错误

| 错误 | 说明 | 处理 |
| --- | --- | --- |
| `401` / `403` | Token 无效、过期或无权限 | 检查 `Authorization: Bearer <TOKEN>` |
| `missing_model` | 未传 `model` | 补全模型名 |
| `model_not_found` / `unsupported_model` | 无可用渠道或模型未开放 | 查 `/v1/models`，联系管理员 |
| `invalid_endpoint` | 模型与接口不匹配 | 图片 `/v1/images/generations`，视频 `/v1/videos`，音频 `/v1/audio/speech` |
| `invalid_duration` | 时长不在允许范围 | LTX 用 1～20 秒；其他按时长计费模型看目录约束 |
| `model_price_error` | 价格未配置或参数不符合价格目录 | 检查分辨率/时长/模型定价 |
| `image_1` / `image_2` / `image_3` 或 `prompt` 缺失 | 千问参考图工作流缺少必填字段 | 按模型要求补齐精确编号的图片字段与 `prompt` |
| `comfyui_instance_busy` / `Task queue timed out because no device was available` | 上游 ComfyUI 实例繁忙或没有可用节点接单 | 稍后重新创建任务；避免高频重复提交 |
| `missing_content` / `missing_resolution` / `unsupported_resolution` / `unsupported_ratio` | Seedance 参数不完整或不受支持 | 按 [4.13](#413-ap-seedance-20-视频) 补全 `content`/`resolution`/`ratio` |
| `first/last frame content cannot be mixed with reference media content` | Seedance 在同一 `content` 中混用了首尾帧与参考素材 | 二选一：只用 `first_frame`/`last_frame`，或只用 `reference_*` |
| `Invalid video_url` | 参考视频 URL 无效或不可访问 | 换公网 HTTPS 可直链视频；勿用本地路径/内网/过期签名 |
| 参考视频时长 `> 15.2` 秒 | r2v 参考视频超限 | 将参考视频压到约 2～15.2 秒 |
| `duration` 参数无效 | 输出时长不合法 | 直接传合法 `duration`/`seconds`（常见如 5） |
| 参考视频/图片含真人等敏感内容 | 上游隐私/审核错误 | 去掉参考素材、换非真人素材，或改用已入库的 `asset://` 引用；也可改文生视频 |
| `invalid_character_id` | 角色库字段不合法 | `character_id` 必须为正整数 |
| `character_model_not_allowed` | 非 Seedance 模型却传了 `character_id` | 仅 Seedance 2.0 档位支持角色库引用 |
| `character_reference_conflict` | `character_id` 与其他参考素材同传 | 去掉 `image`/`images`/`content` 等外部参考，只保留角色库字段 |
| `character_not_found` / `character_unavailable` | 角色或图片不可用 | 确认账号可访问、角色为 Active 且具有有效 `provider_asset_id`，或改传已知可用的 `asset://` |
| `asset` 不存在 / 不属于当前账户 | 火山侧无法解析 `asset://` | 确认 `asset_id` 正确，且与出片所用上游账号/项目一致 |
| `timeline_data must be valid JSON` | 首尾帧 LTX 时间线非法 | 传合法 JSON 对象/数组 |
| LTX 分辨率/帧率/时长错误 | 不在允许范围 | 见 [4.8](#48-ltx-23-图生视频) |
| `task_not_exist` | 任务不存在或不属于当前用户 | 使用创建响应中的公开 `task_id` |
| `429` | 限流或上游负载高 | 稍后重试 |
| 一直 `queued` | 上游排队 | 继续低频轮询 |
| 无结果 URL | 未完成或上游未返回 URL | 等终态后再读；仍空则联系服务方 |

错误体示例：

```json
{
  "code": "unsupported_model",
  "message": "unsupported AIPDD model: xxx",
  "data": null
}
```

## 10. 异步图片 Node.js 示例

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
    headers: { Authorization: `Bearer ${TOKEN}` },
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

## 11. 上线前验证

```bash
# Flux 文生图创建
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf-t2i",
    "prompt": "a small red cube on a white background"
  }'

# 千问三参考图编辑创建（异步）
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd_qwen_image_edit_triple_reference",
    "image_1": "https://example.com/reference-a.png",
    "image_2": "https://example.com/reference-b.png",
    "image_3": "https://example.com/reference-c.png",
    "prompt": "combine all references into one cohesive poster"
  }'

# AP Seedance 创建（分辨率须匹配该档位价格目录）
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "prompt": "a small red cube rotating on a white background",
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5
  }'
```

仓库提供用户视角 smoke test（默认覆盖 Flux / Wan / Animater / Mimic / Latentsync / IndexTTS；**不含** LTX、Seedance 与上述三个千问参考图模型）：

```bash
node bin/aipdd-user-smoke-test.mjs \
  --base-url "$BASE_URL" \
  --token "$NEW_API_TOKEN" \
  --image-url "https://example.com/input.png" \
  --video-url "https://example.com/input.mp4" \
  --audio-url "https://example.com/reference.wav"

# 只创建不等待
node bin/aipdd-user-smoke-test.mjs --no-poll \
  --base-url "$BASE_URL" \
  --token "$NEW_API_TOKEN" \
  --image-url "https://example.com/input.png" \
  --video-url "https://example.com/input.mp4" \
  --audio-url "https://example.com/reference.wav"

# 只测部分模型
node bin/aipdd-user-smoke-test.mjs --only flux-i2i,flux-t2i,wanx \
  --base-url "$BASE_URL" \
  --token "$NEW_API_TOKEN" \
  --image-url "https://example.com/input.png"
```

## 12. 使用建议与上游关系

- 不要把 AIPDD 上游 API Key 放入浏览器、移动端或客户业务日志。
- 输入素材优先使用稳定的公网 HTTPS URL；过期、需登录或内网 URL 易导致任务失败。
- 轮询间隔建议 5～15 秒，并设置整体超时。
- 任务 ID 按字符串保存，不要转成整数。
- 用户不直接调用 AIPDD 上游接口；NewAPI 负责路由、协议转换，并保存上游真实任务 ID。用户侧始终只用公开 `task_xxx` 查询结果。
- 若服务方未开放某模型，即使请求格式正确也会返回 `unsupported_model`。
