# Agnes 图片与视频

本分类只覆盖 Agnes 图片、视频模型；文本模型不在这里维护。

| 能力 | 公开模型 | 创建接口 | 后续接口 |
| --- | --- | --- | --- |
| 图片生成 / 编辑 | `ap-agnes-image-2.1-flash` | `POST /v1/images/generations` / `POST /v1/images/edits` | 同步返回，无需轮询 |
| 视频普通版 | `ap-agnes-video-2.5` | `POST /v1/videos` | `GET /v1/videos/{task_id}` / `GET /v1/videos/{task_id}/content` |
| 视频 Flash | `ap-agnes-video-2.5-flash` | `POST /v1/videos` | `GET /v1/videos/{task_id}` / `GET /v1/videos/{task_id}/content` |

## 图片要点

- `size` 必填，可选 `1K`、`2K`、`3K`、`4K`；`n` 只能为 `1`。
- `ratio` 可选：`1:1`、`3:4`、`4:3`、`16:9`、`9:16`、`2:3`、`3:2`、`21:9`。
- JSON 生成接口可用 `image` / `images` 传 URL 或图片 Data URI；文件编辑使用 `multipart/form-data`，重复上传 1–8 个 `image` 文件。

## 视频要点

- `mode` 使用 `text_to_video`、`first_last_frame_to_video` 或 `reference_to_video`；兼容短值 `text`、`keyframe`、`reference`。
- 时长 `duration_seconds` 为 4–12 秒，默认 5 秒。
- 普通版支持 `720P`、`960P`、`2K`；Flash 仅支持 `720P`。
- `text_to_video` 不能带参考素材；首尾帧模式至少提供 `first_frame`；参考模式至少提供 `images`、`audios`、`videos` 之一。
- 普通版最多 8 张参考图、3 段参考音频、3 段参考视频；Flash 最多 5 张参考图、3 段参考音频，且不支持参考视频。
- 创建成功后保存返回的 `id`，只用该公开 ID 查询。完成状态为 `completed`，结果 URL 在 `metadata.url`。

价格和当前可用档位以 `GET /api/pricing` 为准。
