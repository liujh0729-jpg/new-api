# Seedance 2.0 / 2.5

两套创建/查询入口接收同一套请求体，**用哪个路径创建就用对应路径查询**。

| 格式 | 创建 | 查询 | 成功状态 | 视频地址 |
|---|---|---|---|---|
| 官方兼容 | `POST /api/v3/contents/generations/tasks` | `GET /api/v3/contents/generations/tasks/{task_id}` | `succeeded` | `content.video_url` |
| OpenAI Video | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `completed` | `metadata.url` |

Seedance 的 OpenAI Video 创建成功码为 `202`；官方兼容创建成功码为 `200`。两条协议都在服务器接受任务后返回公开任务 ID。

新接入用官方兼容或 OpenAI Video。单角色 `character_id` 两条入口都支持，`resolution` / `ratio` 放在请求体顶层。

Seedance 售卖模型会固定输出 FPS，默认 24。调用方可省略 `framespersecond`；若显式传入，必须与售卖模型配置一致，否则返回 400。这里的 FPS 指最终输出视频 FPS，不是输入素材 FPS。普通 2.5 模型仍不接受 FPS 变体。

## 版本差异

| 项目 | 2.0 | 2.5 |
|---|---|---|
| 模型 | `AP Seedance-2.0 VIP` / `标准版` / `轻量版` / `高性价比版` | `AP Seedance-2.5 标准版` |
| 分辨率 | VIP：480p–4k；标准/轻量：480p/720p；高性价比：1080p/4k | 480p / 720p / 1080p，默认 720p |
| 比例 | 16:9 / 9:16 / 1:1 / 4:3 / 3:4 | 另加 21:9、`adaptive`，默认 `adaptive` |
| 时长 | 正数秒，默认 5 | `-1` 或 4–30，默认 `-1` |
| `seed` / `service_tier` | 支持 | 不支持 |

价格以 `GET /api/pricing` 为准。官方兼容支持 `GET /api/v3/contents/generations/tasks` 列表与任务删除/取消；OpenAI Video 支持 `DELETE /v1/videos/{task_id}`。两条协议都没有 `/result`，成品统一通过 `GET /v1/videos/{task_id}/content` 获取并支持 Range。

## 素材库

`GET/POST /v1/virtual-characters`。官方角色 `scope=public&source_type=volc_preset`；私有素材 `volc_aigc`；真人先核验再上传肖像。多角色在 `content` 里写已登记的 `asset://{asset_id}`。
