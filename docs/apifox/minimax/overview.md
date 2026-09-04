# MiniMax H3

MiniMax H3 只公开一个模型 ID：**`ap-minimax-h3`**。创建任务走 **`POST /v1/videos`**，查询走 **`GET /v1/videos/{task_id}`**。旧的五个场景模型 ID 已直接下线，不提供别名兼容。

价格和当前可用档位以 `GET /api/pricing` 为准。

## 公共请求字段

- `model`、`duration_seconds` 和 `video_resolution` 必填。
- `duration_seconds` 必须是正整数秒；不要使用 `duration`。
- `video_resolution` 取值可为 `480p`、`768p`、`1080p`，但具体取值受模型限制；不要使用 `resolution`。
- `ratio` 默认 `16:9`，具体取值受模型限制。
- 参考图使用 `image_urls` 数组，参考音频使用 `audio_urls` 数组。不要使用 `images` 或单值 `audio`。
- 可传 `request_kind` 明确选择能力；不传时由请求参数自动判断。
- 素材 URL 必须非空且能被上游访问。

## 能力模式与限制

| `request_kind` | `prompt` | 分辨率与时长 | `ratio` | 图片 | 音频 | `seed` |
|---|---|---|---|---|---|---|
| `text_to_video` | 必填 | 480p / 768p：1–15 秒 | 16:9 / 9:16 / 1:1 | 不支持 | 不支持 | 不支持 |
| `reference_to_video` | 必填 | 480p / 768p：1–15 秒；1080p：1–10 秒 | 16:9 / 9:16 / 1:1 | `image_urls` 1–9 个 | 不支持 | 支持整数 |
| `multimodal_to_video` | 必填 | 480p / 768p：1–15 秒；1080p：1–10 秒 | 16:9 / 9:16 | `image_urls` 1–9 个 | `audio_urls` 1–3 个 | 支持整数 |
| `image_audio_lipsync` | 不使用 | 480p / 768p / 1080p：1–15 秒 | 16:9 / 9:16 | `image_urls` 恰好 1 个 | `audio_urls` 恰好 1 个 | 不支持 |
| `first_last_frame_to_video` | 必填 | 480p / 768p：1–15 秒 | 16:9 / 9:16 | `first_frame` 和 `last_frame` 均必填；不传 `image_urls` | 不支持 | 不支持 |

自动判断顺序为：首尾帧字段、无素材文生视频、仅图片参考、单图单音频且无提示词的口型同步，其他图音组合为多模态。建议需要稳定行为的调用方显式传 `request_kind`。`seed` 只在多图参考和多图多音频能力上生效。分辨率、比例或时长组合不在表中时，请求会被拒绝。

## 请求示例

文生视频：

```json
{
  "model": "ap-minimax-h3",
  "request_kind": "text_to_video",
  "prompt": "一只橘猫在木桌旁喝水",
  "duration_seconds": 5,
  "video_resolution": "768p",
  "ratio": "16:9"
}
```

多图参考：

```json
{
  "model": "ap-minimax-h3",
  "request_kind": "reference_to_video",
  "prompt": "保持参考图中角色一致，缓慢走向镜头",
  "duration_seconds": 10,
  "video_resolution": "1080p",
  "ratio": "1:1",
  "image_urls": ["https://example.com/reference-1.png"],
  "seed": 42
}
```

多图多音频：

```json
{
  "model": "ap-minimax-h3",
  "request_kind": "multimodal_to_video",
  "prompt": "保持参考人物一致，按音频节奏说话",
  "duration_seconds": 8,
  "video_resolution": "768p",
  "ratio": "9:16",
  "image_urls": ["https://example.com/reference-1.png"],
  "audio_urls": ["https://example.com/voice.wav"]
}
```

图片音频同步视频：

```json
{
  "model": "ap-minimax-h3",
  "request_kind": "image_audio_lipsync",
  "duration_seconds": 7,
  "video_resolution": "1080p",
  "ratio": "16:9",
  "image_urls": ["https://example.com/portrait.png"],
  "audio_urls": ["https://example.com/voice.wav"]
}
```

首尾帧生视频：

```json
{
  "model": "ap-minimax-h3",
  "request_kind": "first_last_frame_to_video",
  "prompt": "从首帧自然过渡到尾帧",
  "duration_seconds": 5,
  "video_resolution": "768p",
  "ratio": "16:9",
  "first_frame": "https://example.com/first.png",
  "last_frame": "https://example.com/last.png"
}
```

## 任务结果

创建接口返回 OpenAI Video 对象，初始状态为 `queued`。使用返回的 `id` 查询；状态可能为 `queued`、`in_progress`、`completed` 或 `failed`。成功时视频地址在 `metadata.url`，失败时查看 `error.message` 和 `error.code`。
