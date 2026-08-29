# Seedance 2.0 / 2.5

两套创建/查询入口接收同一套请求体，**用哪个路径创建就用对应路径查询**。

| 格式 | 创建 | 查询 | 成功状态 | 视频地址 |
|---|---|---|---|---|
| 官方兼容 | `POST /api/v3/contents/generations/tasks` | `GET /api/v3/contents/generations/tasks/{task_id}` | `succeeded` | `content.video_url` |
| OpenAI Video | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `completed` | `metadata.url` |

新接入用官方兼容或 OpenAI Video。单角色 `character_id` 两条入口都支持，`resolution` / `ratio` 放在请求体顶层。

## 版本差异

| 项目 | 2.0 | 2.5 |
|---|---|---|
| 模型 | `AP Seedance-2.0 VIP` / `标准版` / `轻量版` / `高性价比版` | `AP Seedance-2.5 标准版` |
| 分辨率 | VIP：480p–4k；标准/轻量：480p/720p；高性价比：1080p/4k | 480p / 720p / 1080p，默认 720p |
| 比例 | 16:9 / 9:16 / 1:1 / 4:3 / 3:4 | 另加 21:9、`adaptive`，默认 `adaptive` |
| 时长 | 正数秒，默认 5 | `-1` 或 4–30，默认 `-1` |
| `seed` / `service_tier` | 支持 | 不支持 |

价格以 `GET /api/pricing` 为准。不提供列表、删除、重试；官方兼容也没有 `/result`。

## 素材库

`GET/POST /v1/virtual-characters`。官方角色 `scope=public&source_type=volc_preset`；私有素材 `volc_aigc`；真人先核验再上传肖像。多角色在 `content` 里写已登记的 `asset://{asset_id}`。
