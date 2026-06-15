# AIPDD 七个模型用户使用文档

本文面向普通 API 用户。调用时只需要使用平台发放的 NewAPI Token，不需要 AIPDD 上游 Key。

## 调用方式

AIPDD 模型都是异步任务，不是聊天模型，不能用 `/v1/chat/completions` 调用。

基本流程：

1. 提交任务，返回 `task_id`
2. 每 5 到 15 秒用 `task_id` 查询一次结果
3. 状态为 `succeeded` 或 `completed` 后读取结果 URL

公共请求头：

```http
Authorization: Bearer <NEW_API_TOKEN>
Content-Type: application/json
```

示例环境变量：

```bash
export BASE_URL="https://newapi.jumcp.com/"
export NEW_API_TOKEN="sk-xxxx"
```

素材建议使用公网 HTTPS URL。也支持 `multipart/form-data` 上传本地文件。

## 模型列表

| 能力 | 模型名 | 创建接口 | 查询接口 | 主要输入 | 计费 |
| --- | --- | --- | --- | --- | --- |
| 图生图 | `aipdd-flux-gguf` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `image`，可选 `prompt` | 按次 |
| 文生图 | `aipdd-flux-gguf-t2i` | `POST /v1/images/generations` | `GET /v1/images/generations/{task_id}` | `prompt` 或 `text` | 按次 |
| 图生视频 | `aipdd-wan2.2-wanx` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `image`、`prompt`，可选 `negative_prompt`、`fps` | 按次 |
| 主体替换视频 | `aipdd-wan2.2-animater` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `video`、`prompt`，可选 `image` | 按次 |
| 动作迁移视频 | `aipdd-mimic-motion` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `motion_video`、`appearance_image` | 按次 |
| 对口型视频 | `aipdd-latentsync-1.5` | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `video`、`LoadAudio` | 按次 |
| 声音复刻 | `aipdd-indextts` | `POST /v1/audio/speech` | `GET /v1/audio/speech/{task_id}` | `input`、`audio` | 按次 |

## 提交任务示例

### 1. Flux 图生图

```bash
curl "${BASE_URL}v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf",
    "image": "https://example.com/input.png",
    "prompt": "一张高级感产品摄影图，柔和棚拍灯光"
  }'
```

### 2. Flux 文生图

```bash
curl "${BASE_URL}v1/images/generations" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-flux-gguf-t2i",
    "prompt": "一张高级感产品摄影图，柔和棚拍灯光"
  }'
```

### 3. Wan2.2 图生视频

```bash
curl "${BASE_URL}v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-wanx",
    "image": "https://example.com/input.png",
    "prompt": "镜头缓慢推进，画面稳定，电影感"
  }'
```

### 4. Wan2.2 主体替换视频

```bash
curl "${BASE_URL}v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-wan2.2-animater",
    "load_video": "https://example.com/source.mp4",
    "prompt": "主体动作自然，画面稳定",
    "negative_prompt": "低清晰度，畸变，闪烁"
  }'
```

### 5. MimicMotion 动作迁移视频

```bash
curl "${BASE_URL}v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-mimic-motion",
    "motion_video": "https://example.com/motion.mp4",
    "appearance_image": "https://example.com/person.png"
  }'
```

### 6. Latentsync 对口型视频

```bash
curl "${BASE_URL}v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-latentsync-1.5",
    "video": "https://example.com/source.mp4",
    "LoadAudio": "https://example.com/speech.wav"
  }'
```

### 7. IndexTTS 声音复刻

```bash
curl "${BASE_URL}v1/audio/speech" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "aipdd-indextts",
    "input": "这里填写需要合成的文本",
    "audio": "https://example.com/reference.wav"
  }'
```

可选传入 `emotion_audio` 作为情感参考音频。

## 查询结果

创建任务成功后会返回类似：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "status": "queued",
  "model": "aipdd-flux-gguf"
}
```

按模型类型查询：

```bash
# 图片
curl "${BASE_URL}v1/images/generations/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"

# 视频
curl "${BASE_URL}v1/videos/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"

# 音频
curl "${BASE_URL}v1/audio/speech/task_xxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

常见状态：

| 状态 | 含义 |
| --- | --- |
| `queued` | 已排队 |
| `processing` / `in_progress` | 生成中 |
| `succeeded` / `completed` | 已完成 |
| `failed` | 失败 |

完成后优先从以下字段读取结果 URL：

1. `metadata.url`
2. `data.url`
3. `output[0]`
4. `metadata.urls[0]`
5. `data.metadata.urls[0]`

## 本地文件上传

如果不方便提供公网 URL，可以用表单上传文件：

```bash
curl "${BASE_URL}v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -F "model=aipdd-wan2.2-wanx" \
  -F "prompt=镜头缓慢推进，电影感" \
  -F "image=@input.png"
```

常用文件字段：

| 模型名 | 文件字段 |
| --- | --- |
| `aipdd-flux-gguf` | `image` |
| `aipdd-wan2.2-wanx` | `image` |
| `aipdd-wan2.2-animater` | `video`，也可用 `load_video` 作为兼容别名；可选 `image` |
| `aipdd-mimic-motion` | `motion_video`、`appearance_image` |
| `aipdd-latentsync-1.5` | `video`、`LoadAudio`，也可用 `audio` 作为音频别名 |
| `aipdd-indextts` | `audio`，也可用 `ref_audio`；可选 `emotion_audio` |

## 常见问题

| 问题 | 处理方式 |
| --- | --- |
| `401` | 检查 `Authorization: Bearer <NEW_API_TOKEN>` 是否正确 |
| `model_not_found` | 联系管理员确认 AIPDD 渠道和模型已启用 |
| 一直 `queued` | 上游仍在排队，建议低频轮询或联系服务方检查队列 |
| 没有结果 URL | 任务未完成时不会返回结果 URL，请等到完成状态后再读取 |
