# AIPDD 能力用户调用指南

本文面向使用 NewAPI/AIPDD 网关 Token 的普通 API 用户。用户需要拿到你平台发放的 API Token 来调用当前 NewAPI 服务；不需要直接登录上游 AIPDD，也不需要拿到管理员配置在渠道里的上游 AIPDD API Key。

AIPDD 在本系统中是一个上游异步任务渠道。用户请求进入 NewAPI 后，NewAPI 会在内部调用 AIPDD：

- 创建上游任务：`POST https://api.aipdd.work/comfyui/task/create`
- 查询上游任务：`GET https://api.aipdd.work/comfyui/task/{task_id}`

用户侧只会看到 NewAPI 生成的公开 `task_id`，真实 AIPDD 任务 id 和上游 AIPDD API Key 都由系统保存和管理。

## 基本概念

调用 AIPDD 能力分两步：

1. 提交任务，拿到 `task_id`
2. 用 `task_id` 轮询结果

这些能力不是聊天模型，不能通过 `/v1/chat/completions` 调用，也不会出现在聊天操练场里。

所有请求都使用 NewAPI 用户令牌：

```http
Authorization: Bearer <NEW_API_TOKEN>
Content-Type: application/json
```

示例中的 `BASE_URL` 是你的 NewAPI 服务地址，例如：

```bash
export BASE_URL="https://your-new-api.example.com"
export NEW_API_TOKEN="sk-xxxx"
```

## 可用模型

| 能力 | 模型名 | 创建接口 | 查询接口 | 计费 |
| --- | --- | --- | --- | --- |
| Flux 图生图 | `aipdd-flux-gguf` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | 按次 |
| Flux 文生图 | `aipdd-flux-gguf-t2i` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | 按次 |
| Wan2.2 图生视频 | `aipdd-wan2.2-wanx` | `POST /v1/videos` 或 `POST /v1/video/generations` | `GET /v1/videos/{task_id}` 或 `GET /v1/video/generations/{task_id}` | 按秒 |
| Wan2.2 主体替换 | `aipdd-wan2.2-animater` | `POST /v1/videos` 或 `POST /v1/video/generations` | 同视频查询接口 | 按次 |
| mimicmotion 动作替换 | `aipdd-mimic-motion` | `POST /v1/videos` 或 `POST /v1/video/generations` | 同视频查询接口 | 按次 |
| Latentsync 对口型视频 | `aipdd-latentsync-1.5` | `POST /v1/videos` 或 `POST /v1/video/generations` | 同视频查询接口 | 按次 |
| IndexTTS 声音复刻 | `aipdd-indextts` | `POST /v1/audio/speech` | `GET /v1/audio/speech/{task_id}` | 按次 |

素材 URL 必须能被 NewAPI 服务端和 AIPDD 上游访问。建议使用公网 HTTPS URL。

也可以用 `multipart/form-data` 上传本地素材文件。NewAPI 会在服务端使用管理员配置在 AIPDD 渠道里的上游 AIPDD API Key 调用 aipdd-api 的 `/oss/upload`，拿到 OSS URL 后自动写入任务参数；普通用户不需要知道上游 AIPDD API Key。

常用文件字段：

| 模型名 | 文件字段 |
| --- | --- |
| `aipdd-flux-gguf` | `image` |
| `aipdd-wan2.2-wanx` | `image` |
| `aipdd-wan2.2-animater` | `load_video` |
| `aipdd-mimic-motion` | `motion_video`、`appearance_image` |
| `aipdd-latentsync-1.5` | `video`、`LoadAudio` |
| `aipdd-indextts` | `audio`、可选 `emotion_audio` |

## 创建任务

### Flux 图生图

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf",
    "image": "https://example.com/input.png",
    "prompt": "a cinematic product photo, soft studio lighting"
  }'
```

必填参数：

- `image`：输入图片 URL

可选参数：

- `prompt` 或 `positive_prompt`：图片提示词
- `negative_prompt`

本地图片上传写法：

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -F "model=aipdd-flux-gguf" \
  -F "prompt=a cinematic product photo, soft studio lighting" \
  -F "image=@input.png"
```

### Flux 文生图

```bash
curl "$BASE_URL/v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf-t2i",
    "prompt": "a cinematic product photo, soft studio lighting"
  }'
```

必填参数：

- `prompt` 或 `text`：图片提示词

### Wan2.2 图生视频

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-wanx",
    "prompt": "slow camera push in, cinematic movement",
    "image": "https://example.com/input.png",
    "duration": 5
  }'
```

必填参数：

- `image`：输入图片 URL
- `prompt`：视频提示词

`duration` 只允许 `5` 或 `10`。不传时默认按 `5` 秒处理。

本地图片上传写法：

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -F "model=aipdd-wan2.2-wanx" \
  -F "prompt=slow camera push in, cinematic movement" \
  -F "duration=5" \
  -F "image=@input.png"
```

### Wan2.2 主体替换

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-animater",
    "load_video": "https://example.com/source.mp4",
    "WanVideoTextEncodeCached_positive_prompt": "natural motion, stable subject",
    "WanVideoTextEncodeCached_negative_prompt": "low quality, distorted, flicker"
  }'
```

必填参数：

- `load_video`：输入视频 URL
- `WanVideoTextEncodeCached_positive_prompt`
- `WanVideoTextEncodeCached_negative_prompt`

`filename` 可以不传，系统会从视频 URL 自动提取。

### mimicmotion 动作替换

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

必填参数：

- `motion_video`：动作参考视频 URL
- `appearance_image`：主体外观图片 URL

### Latentsync 对口型视频

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

必填参数：

- `video`：输入视频 URL
- `LoadAudio`：驱动口型的音频 URL

`filename` 可以不传，系统会从视频 URL 自动提取。

### IndexTTS 声音复刻

```bash
curl "$BASE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-indextts",
    "input": "要合成的文本内容",
    "metadata": {
      "audio": "https://example.com/reference.wav"
    }
  }'
```

必填参数：

- `input`：要合成的文本
- `metadata.audio`：参考音频 URL

可选参数：

- `metadata.emotion_audio`：情感参考音频 URL

本地音频上传写法：

```bash
curl "$BASE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -F "model=aipdd-indextts" \
  -F "input=要合成的文本内容" \
  -F "audio=@reference.wav"
```

## 创建响应

图片和音频任务会返回通用异步任务对象：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "image.generation.task",
  "created": 1778932446,
  "model": "aipdd-flux-gguf-t2i",
  "status": "queued",
  "metadata": {
    "endpoint_type": "image-generation"
  }
}
```

视频任务会返回 OpenAI Video 风格对象：

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

后续轮询只需要保存 `task_id`。

## 查询任务

图片任务：

```bash
curl "$BASE_URL/v1/images/generations/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

视频任务：

```bash
curl "$BASE_URL/v1/videos/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

兼容视频查询路径：

```bash
curl "$BASE_URL/v1/video/generations/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

音频任务：

```bash
curl "$BASE_URL/v1/audio/speech/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

建议每 5 到 15 秒轮询一次。任务通常需要排队和执行时间，不要高频请求。

## 查询响应

图片和音频任务完成后，结果 URL 会出现在 `data.url`、`data.output` 或 `data.metadata.urls`：

```json
{
  "code": "success",
  "message": "",
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

视频任务完成后，结果 URL 会出现在 `metadata.url`，多结果时也可能有 `metadata.urls`：

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

建议客户端统一按下面顺序取结果：

1. `metadata.url`
2. `data.url`
3. `result_url`
4. `output[0]`
5. `metadata.urls[0]` 或 `data.metadata.urls[0]`

## 状态说明

AIPDD 上游使用数字状态，NewAPI 会映射成用户更容易理解的状态。

| AIPDD `task_status` | 上游含义 | 图片/音频查询状态 | 视频查询状态 |
| --- | --- | --- | --- |
| `0` | 待领取/发布成功 | `queued` | `queued` |
| `1` | 已被领取 | `queued` | `queued` |
| `2` | 执行中 | `processing` | `in_progress` |
| `3` | 执行完毕 | `succeeded` | `completed` |
| `4` | 执行失败 | `failed` | `failed` |

失败时，错误原因会尽量映射到 `fail_reason`、`data.error` 或视频对象的 `error.message`。

## 计费说明

| 模型 | 计费方式 |
| --- | --- |
| `aipdd-flux-gguf` | 按次 |
| `aipdd-flux-gguf-t2i` | 按次 |
| `aipdd-wan2.2-wanx` | 按秒 |
| `aipdd-wan2.2-animater` | 按次 |
| `aipdd-mimic-motion` | 按次 |
| `aipdd-latentsync-1.5` | 按次 |
| `aipdd-indextts` | 按次 |

Wan2.2 图生视频只支持两档：

- `duration: 5`：0.1 元
- `duration: 10`：0.2 元

等价单价为 `0.02 元/秒`。创建任务时会按提交时长预扣费。

## 参数放置规则

工作流参数既可以放在顶层 JSON，也可以放在 `metadata` 里。

顶层写法：

```json
{
  "model": "aipdd-mimic-motion",
  "motion_video": "https://example.com/motion.mp4",
  "appearance_image": "https://example.com/person.png"
}
```

`metadata` 写法：

```json
{
  "model": "aipdd-mimic-motion",
  "metadata": {
    "motion_video": "https://example.com/motion.mp4",
    "appearance_image": "https://example.com/person.png"
  }
}
```

为了可读性，业务参数较少时推荐放在顶层；参数较多时推荐放进 `metadata`。

## 常见错误

### 401 Invalid token

NewAPI 用户令牌错误、过期、被禁用，或请求头没有带：

```http
Authorization: Bearer <NEW_API_TOKEN>
```

### 503 model_not_found

通常表示管理员没有配置可用的 AIPDD 渠道，或渠道没有启用对应模型。

### 400 duration must be 5 or 10 seconds

`aipdd-wan2.2-wanx` 的 `duration` 只能传 `5` 或 `10`。

### 一直 queued

这通常表示上游任务仍在排队或未被执行节点领取。可以继续低频轮询，或联系服务方检查 AIPDD 上游任务队列。

### 没有结果 URL

任务未完成时不会有结果 URL。只有状态为 `succeeded` 或 `completed` 后，才应读取 `url`、`output` 或 `metadata.url`。

## Node.js 调用示例

```js
const BASE_URL = process.env.BASE_URL;
const TOKEN = process.env.NEW_API_TOKEN;

async function createFluxTask() {
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
  });

  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

async function fetchImageTask(taskId) {
  const res = await fetch(`${BASE_URL}/v1/images/generations/${taskId}`, {
    headers: {
      Authorization: `Bearer ${TOKEN}`,
    },
  });

  if (!res.ok) throw new Error(await res.text());
  return res.json();
}
```

## 本地烟测脚本

仓库提供了一个用户视角测试脚本：

```bash
node bin/aipdd-user-smoke-test.mjs \
  --base-url "$BASE_URL" \
  --token "$NEW_API_TOKEN" \
  --image-url "https://example.com/input.png" \
  --video-url "https://example.com/input.mp4" \
  --audio-url "https://example.com/reference.wav"
```

只创建任务、不轮询：

```bash
node bin/aipdd-user-smoke-test.mjs --no-poll
```

只测部分模型：

```bash
node bin/aipdd-user-smoke-test.mjs --only flux-i2i,flux-t2i,wanx
```

## 与 AIPDD 上游的关系

用户不直接调用 AIPDD 的 `/comfyui/task/create`。NewAPI 会把用户提交的模型和参数转换成 AIPDD 需要的 payload：

```json
{
  "task_name": "aipdd-wan2.2-wanx",
  "task_type": "0",
  "task_cost": 5000,
  "task_service": "comfyui",
  "script_id": "3eae5a25-98cf-4658-aa9f-c48bb41043a6",
  "script_code": "aipdd_wan2.2_wanx",
  "task_content": "{\"image\":\"https://example.com/input.png\",\"prompt\":\"...\",\"duration\":5}"
}
```

AIPDD 返回的真实任务 id 会保存在 NewAPI 本地任务记录里。用户始终使用 NewAPI 返回的 `task_xxx` 查询结果。
