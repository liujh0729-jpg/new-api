# User4 Seedance 调用失败分析报告

> 环境：`http://14.103.100.4:6070`  
> 对象：`User4`（用户 ID = 7）  
> 时间范围：以 2026-07-22 近期任务/日志为主  
> 结论日期：2026-07-23

## 1. 结论摘要

**不是平台故障，也不是额度问题。**

`User4` 近期 Seedance 失败，核心是 **请求参数/素材不符合上游 Seedance 约束**。

最典型错误：

```text
status_code=400
The parameter `content` specified in the request is not valid:
first/last frame content cannot be mixed with reference media content.
```

含义：

> **首尾帧模式** 与 **参考素材模式** 不能在同一次 `content` 请求中混用。

---

## 2. 用户概况

| 项目 | 值 |
| --- | --- |
| 用户名 | `User4` |
| 用户 ID | `7` |
| 分组 | `default` |
| 调用令牌 | `视频生成测试` |
| 通道 | `AIPDD`（channel_id=2） |
| 主要失败模型 | `AP Seedance-2.0 轻量版` |

说明：账号余额充足，失败请求中创建阶段 400 基本不扣费；已创建后失败的任务已退款。

---

## 3. 近期情况

### 3.1 任务结果

| 模型 | 数量 | 结果 |
| --- | ---: | --- |
| `aipdd_ltx_2.3 (首尾帧)` | 11 | 全部成功 |
| `aipdd_ltx_2.3` | 1 | 成功 |
| `AP Seedance-2.0 轻量版` | 4 | 全部失败 |

### 3.2 失败类型

#### A. 已创建任务后失败（任务列表可见）

| 时间 (UTC) | task_id | 原因 |
| --- | --- | --- |
| 2026-07-22 07:57 | `task_ARvwu2V8i34Y9wljxzmdgg9TPySg8yfX` | `Invalid video_url`（参考视频 URL 无效） |
| 2026-07-22 07:22 | `task_RvZCtiVU8cYSKWZphnyG8NwqRvogThOf` | `任务处理失败，请稍后重试` |
| 2026-07-22 06:48 | `task_uXEJyh8B7tMhhxq65QwR7WW1gJw3YDGX` | `任务处理失败，请稍后重试` |
| 2026-07-22 06:41 | `task_oE1f1xm25BJNDFPE941EMUWzoZA8wKwv` | `任务处理失败，请稍后重试` |

#### B. 创建阶段直接 400（日志可见）

| 时间 (UTC) | 错误 |
| --- | --- |
| 09:02 | 输入图片疑似真人 |
| **09:00** | **首尾帧与参考素材混用** |
| 08:46 | 输入视频疑似真人 |
| 08:06 | 参考视频时长 > 15.2 秒 |
| 08:05 | `duration` 参数非法 |

---

## 4. 问题根因

### 4.1 核心问题：模式混用

Seedance `content` 只允许二选一：

| 模式 | 允许的 role | 禁止同时出现 |
| --- | --- | --- |
| 首尾帧模式 | `first_frame` / `last_frame` | `reference_image` / `reference_video` / `reference_audio` |
| 参考素材模式 | `reference_image` / `reference_video` / `reference_audio` | `first_frame` / `last_frame` |

`User4` 的典型报错，就是把两套模式写进了同一个请求。

### 4.2 其他问题

1. **参考视频 URL 无效**：不可公网访问、过期签名、格式错误  
2. **参考视频过长**：r2v 场景要求 `<= 15.2` 秒  
3. **真人审核拦截**：参考图/视频被判定含真人  
4. **duration 非法**：时长不在模型允许范围

### 4.3 容易混淆点

`aipdd_ltx_2.3 (首尾帧)` 成功，不代表 Seedance 也能按同样方式混用参数。  
LTX 首尾帧 与 Seedance 参考素材，是两套不同调用语义。

---

## 5. 正确调用方法

接口：`POST /v1/videos` 或 `POST /v1/video/generations`  
鉴权：`Authorization: Bearer <用户令牌>`

### 5.1 方案 A：参考生视频（推荐）

只使用 `reference_*`，不要带 `first_frame` / `last_frame`。

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 轻量版",
    "resolution": "1080p",
    "ratio": "9:16",
    "duration": 5,
    "generate_audio": false,
    "content": [
      {
        "type": "text",
        "text": "镜头平稳推进，保持主体身份和光影连续"
      },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {"url": "https://example.com/reference.png"}
      }
    ]
  }'
```

参考视频示例：

```json
{
  "type": "video_url",
  "role": "reference_video",
  "video_url": {"url": "https://example.com/reference.mp4"}
}
```

### 5.2 方案 B：首尾帧生视频

只使用 `first_frame` / `last_frame`，不要再带 `reference_*`。

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
      {
        "type": "text",
        "text": "从首帧平滑过渡到尾帧，动作自然"
      },
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

### 5.3 错误示例（会 400）

```json
{
  "content": [
    {"type": "text", "text": "..."},
    {"type": "image_url", "role": "first_frame", "image_url": {"url": "..."}},
    {"type": "image_url", "role": "reference_image", "image_url": {"url": "..."}},
    {"type": "video_url", "role": "reference_video", "video_url": {"url": "..."}}
  ]
}
```

原因：首尾帧 + 参考素材混用。

---

## 6. 调用约束清单

| 项目 | 要求 |
| --- | --- |
| 模式 | 首尾帧 **或** 参考素材，二选一 |
| 图片 role | `reference_image` / `first_frame` / `last_frame` |
| 视频 role | `reference_video` |
| 音频 role | `reference_audio`（通常需同时有图或视频参考） |
| 素材 URL | 公网 HTTPS，可直接访问 |
| 参考视频时长 | 建议 `2 ~ 15.2` 秒 |
| 输出时长 | 使用合法 `duration`（常见如 5 秒） |
| 审核 | 避免真人图/视频 |
| 数量建议 | 总计 ≤12；图 ≤9；视频 ≤3；音频 ≤3 |

---

## 7. 处理建议

给用户侧：

1. 先明确模式：首尾帧，或参考素材，不要混用  
2. 检查参考视频 URL 是否公网可访问  
3. 参考视频压到 `15.2` 秒以内  
4. 换非真人素材重试  
5. `duration` 使用模型支持的合法值

给运营/支持侧：

1. 可直接回复：这是上游参数校验，不是系统故障  
2. 重点排查 `content.role` 是否混用  
3. 已失败任务已退款，无需额外补额度

---

## 8. 一句话结论

**User4 的 Seedance 报错，是因为请求把首尾帧和参考素材写在了同一个 `content` 里，并叠加了无效参考视频、超长参考视频、真人审核等问题。按“二选一模式 + 合法素材 URL/时长”重新调用即可。**
