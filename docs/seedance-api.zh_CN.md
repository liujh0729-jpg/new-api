# Seedance 2.0 API 调用文档

> 文档版本：2026-07-18  
> 适用范围：当前 New API 部署中的 AP Seedance 2.0 模型  
> 验证状态：16 个付费正向用例和 9 个零计费异常用例均已真实验证

## 1. 接口概览

| 功能 | 方法 | 路径 | 说明 |
| --- | --- | --- | --- |
| 获取模型列表 | `GET` | `/v1/models` | 获取当前令牌可调用的模型 |
| 创建视频任务 | `POST` | `/v1/videos` | 推荐使用的 Seedance 创建接口 |
| 查询视频任务 | `GET` | `/v1/videos/{task_id}` | 查询状态、进度、结果或失败原因 |
| 获取视频内容 | `GET` | `/v1/videos/{task_id}/content` | 通过平台代理读取已完成的视频 |
| 兼容创建接口 | `POST` | `/v1/video/generations` | New API 通用视频接口 |
| 兼容查询接口 | `GET` | `/v1/video/generations/{task_id}` | 返回通用任务结构 |
| 基于任务继续生成 | `POST` | `/v1/videos/{video_id}/remix` | 使用已有任务作为来源继续生成 |

所有接口均使用 New API 用户令牌鉴权：

```http
Authorization: Bearer <NEW_API_TOKEN>
Content-Type: application/json
```

示例中的地址和令牌均为占位符：

```bash
BASE_URL="https://你的平台域名"
NEW_API_TOKEN="你的用户令牌"
```

## 2. 当前模型与分辨率

模型名必须与 `GET /v1/models` 返回值完全一致。当前真实验证的能力如下：

| 模型 | 480p | 720p | 1080p | 4K |
| --- | :---: | :---: | :---: | :---: |
| `AP Seedance-2.0 VIP` | 支持 | 支持 | 支持 | 支持 |
| `AP Seedance-2.0 标准版` | 支持 | 支持 | 支持 | 支持 |
| `AP Seedance-2.0 轻量版` | 支持 | 支持 | 支持 | 不支持 |
| `AP Seedance-2.0 高性价比版` | 不提供 | 不提供 | 支持 | 支持 |

模型目录和价格可以动态更新，生产调用前应以实时模型列表和价格目录为准。

获取模型列表：

```bash
curl "$BASE_URL/v1/models" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

## 3. 创建视频任务

### 3.1 最小请求

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "prompt": "黄昏时分的未来城市，镜头连续缓慢向前推进，保持建筑结构和光影稳定",
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5,
    "generate_audio": false
  }'
```

创建成功返回 HTTP 200：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "AP Seedance-2.0 标准版",
  "status": "queued",
  "progress": 0,
  "created_at": 1784340000
}
```

`id` 与 `task_id` 当前值相同。客户端应保存该公开任务 ID，不要依赖服务方内部任务编号。

### 3.2 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | :---: | --- |
| `model` | string | 是 | 模型名，必须完全匹配模型列表 |
| `prompt` | string | 二选一 | 文本提示词；`content` 为空时自动转换为文本内容项 |
| `content` | array | 二选一 | 官方多模态内容数组；非空时优先于 `prompt` |
| `resolution` | string | 条件必填 | `480p`、`720p`、`1080p`、`4k`；也可由宽高推导 |
| `ratio` | string | 否 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4` |
| `width` | integer | 否 | 与 `height` 同时提供，用于推导分辨率和比例 |
| `height` | integer | 否 | 与 `width` 同时提供，用于推导分辨率和比例 |
| `duration` | number | 否 | 视频时长，单位为秒；未传时通常使用目录默认值 5 秒 |
| `generate_audio` | boolean | 否 | 是否生成同步音频；显式 `false` 会原样保留 |
| `seed` | integer | 否 | 随机种子；显式 `0` 会原样保留 |
| `priority` | integer | 否 | 任务优先级；当前已验证 `0` 与 `1` 的字段接收 |
| `return_last_frame` | boolean | 否 | 是否要求返回末帧；是否产生末帧以模型结果为准 |
| `service_tier` | string | 否 | 服务档位能力参数；`default` 可省略 |
| `callback_url` | string | 否 | 公网回调地址；必须能被服务方直接访问 |
| `metadata` | object | 否 | 参数回退和扩展字段容器，例如 `watermark` |

以下规则会保留显式零值或 `false`：

```json
{
  "seed": 0,
  "priority": 0,
  "generate_audio": false,
  "return_last_frame": false
}
```

### 3.3 `content` 多模态结构

文本：

```json
{
  "type": "text",
  "text": "保持人物身份、服装、动作方向和镜头运动连续"
}
```

参考图片：

```json
{
  "type": "image_url",
  "role": "reference_image",
  "image_url": {
    "url": "https://example.com/reference.jpg"
  }
}
```

参考视频：

```json
{
  "type": "video_url",
  "role": "reference_video",
  "video_url": {
    "url": "https://example.com/reference.mp4"
  }
}
```

参考音频：

```json
{
  "type": "audio_url",
  "role": "reference_audio",
  "audio_url": {
    "url": "https://example.com/reference.wav"
  }
}
```

图片、视频和音频 URL 必须能从公网直接读取，不应依赖浏览器 Cookie、内网地址或短时间内过期的签名。

### 3.4 参数优先级

当同一参数同时出现在根级和 `metadata` 时，根级参数优先：

```text
content:    根级非空 content > metadata.content > prompt 自动转换
resolution: 根级 resolution > metadata.resolution > width/height 推导
ratio:      根级 ratio > metadata.ratio > width/height 推导
其他字段:   根级显式值 > metadata 同名字段
```

支持从 `metadata` 回退读取的字段包括：

```text
content、resolution、ratio、duration、seed、callback_url、return_last_frame、
service_tier、generate_audio、priority
```

扩展字段可以保留在 `metadata` 中，例如：

```json
{
  "metadata": {
    "watermark": true
  }
}
```

### 3.5 宽高推导规则

`width` 与 `height` 必须同时提供且为正整数。系统按短边推导分辨率：

| 短边 | 推导结果 |
| ---: | --- |
| 480 | `480p` |
| 720 | `720p` |
| 1080 | `1080p` |
| 2160 | `4k` |

宽高约分后必须得到受支持的比例之一。例如：

```json
{
  "model": "AP Seedance-2.0 VIP",
  "prompt": "镜面电梯内的一镜到底运动，保持反射与主体动作严格同步",
  "width": 2160,
  "height": 2880,
  "duration": 5
}
```

该请求会推导为 `resolution: "4k"`、`ratio: "3:4"`。

## 4. 多模态调用示例

### 4.1 图片与音频生成视频

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 VIP",
    "resolution": "720p",
    "ratio": "1:1",
    "duration": 5,
    "content": [
      {
        "type": "text",
        "text": "夜雨中的连续追踪镜头，脚步、雨滴和呼吸与参考音频同步，保持人物身份稳定"
      },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {"url": "https://example.com/character.jpg"}
      },
      {
        "type": "audio_url",
        "role": "reference_audio",
        "audio_url": {"url": "https://example.com/reference.wav"}
      }
    ],
    "generate_audio": true,
    "seed": 314159
  }'
```

### 4.2 参考视频延续

```bash
curl "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $NEW_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "AP Seedance-2.0 标准版",
    "resolution": "1080p",
    "ratio": "16:9",
    "duration": 5,
    "content": [
      {
        "type": "text",
        "text": "从参考视频的运动状态无缝延续，保持镜头速度、光线方向和环境结构一致"
      },
      {
        "type": "video_url",
        "role": "reference_video",
        "video_url": {"url": "https://example.com/reference.mp4"}
      }
    ]
  }'
```

### 4.3 文本、图片、视频和音频组合

```json
{
  "model": "AP Seedance-2.0 标准版",
  "resolution": "1080p",
  "ratio": "1:1",
  "duration": 5,
  "content": [
    {
      "type": "text",
      "text": "保持主体身份、物体数量、遮挡恢复、反射和声音时序连续"
    },
    {
      "type": "image_url",
      "role": "reference_image",
      "image_url": {"url": "https://example.com/character.jpg"}
    },
    {
      "type": "video_url",
      "role": "reference_video",
      "video_url": {"url": "https://example.com/reference.mp4"}
    },
    {
      "type": "audio_url",
      "role": "reference_audio",
      "audio_url": {"url": "https://example.com/reference.wav"}
    }
  ],
  "generate_audio": true,
  "priority": 0
}
```

## 5. 查询任务

```bash
curl "$BASE_URL/v1/videos/task_xxxxxxxxxxxxxxxx" \
  -H "Authorization: Bearer $NEW_API_TOKEN"
```

可能的状态：

| 状态 | 含义 |
| --- | --- |
| `queued` | 已进入队列 |
| `in_progress` | 正在生成 |
| `completed` | 已完成 |
| `failed` | 生成失败 |

成功响应示例：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "AP Seedance-2.0 标准版",
  "status": "completed",
  "progress": 100,
  "created_at": 1784340000,
  "completed_at": 1784340200,
  "metadata": {
    "url": "https://example.com/generated-video.mp4"
  }
}
```

失败响应会在任务对象中提供错误信息：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "AP Seedance-2.0 标准版",
  "status": "failed",
  "progress": 100,
  "error": {
    "code": "content_policy_violation",
    "message": "服务方返回的具体失败原因"
  }
}
```

平台会保留服务方返回的错误码和详细原因，同时移除认证信息和内部敏感字段。

## 6. Python 完整轮询示例

```python
import time
import requests

BASE_URL = "https://你的平台域名"
TOKEN = "你的用户令牌"

headers = {
    "Authorization": f"Bearer {TOKEN}",
    "Content-Type": "application/json",
}

payload = {
    "model": "AP Seedance-2.0 标准版",
    "prompt": "黄昏城市中的连续稳定运镜，保持建筑结构与反射一致",
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5,
    "generate_audio": False,
}

create_response = requests.post(
    f"{BASE_URL}/v1/videos",
    headers=headers,
    json=payload,
    timeout=60,
)
create_response.raise_for_status()
created = create_response.json()
task_id = created.get("task_id") or created["id"]

while True:
    query_response = requests.get(
        f"{BASE_URL}/v1/videos/{task_id}",
        headers=headers,
        timeout=30,
    )
    query_response.raise_for_status()
    task = query_response.json()
    status = task.get("status")

    if status == "completed":
        print(task.get("metadata", {}).get("url"))
        break
    if status == "failed":
        raise RuntimeError(task.get("error") or task)

    time.sleep(10)
```

## 7. Node.js 完整轮询示例

```javascript
const baseUrl = 'https://你的平台域名';
const token = '你的用户令牌';

const headers = {
  Authorization: `Bearer ${token}`,
  'Content-Type': 'application/json',
};

const createResponse = await fetch(`${baseUrl}/v1/videos`, {
  method: 'POST',
  headers,
  body: JSON.stringify({
    model: 'AP Seedance-2.0 标准版',
    prompt: '黄昏城市中的连续稳定运镜，保持建筑结构与反射一致',
    resolution: '720p',
    ratio: '16:9',
    duration: 5,
    generate_audio: false,
  }),
});

if (!createResponse.ok) {
  throw new Error(await createResponse.text());
}

const created = await createResponse.json();
const taskId = created.task_id || created.id;

for (;;) {
  const queryResponse = await fetch(`${baseUrl}/v1/videos/${taskId}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!queryResponse.ok) {
    throw new Error(await queryResponse.text());
  }

  const task = await queryResponse.json();
  if (task.status === 'completed') {
    console.log(task.metadata?.url);
    break;
  }
  if (task.status === 'failed') {
    throw new Error(JSON.stringify(task.error || task));
  }

  await new Promise((resolve) => setTimeout(resolve, 10_000));
}
```

## 8. 错误格式与常见错误

创建阶段的错误响应格式：

```json
{
  "code": "unsupported_ratio",
  "message": "Seedance ratio \"2:1\" is not supported",
  "data": null
}
```

当服务方提供额外信息时，`data` 可能包含：

```json
{
  "type": "invalid_request_error",
  "param": "content[1].video_url",
  "request_id": "服务方请求编号"
}
```

| HTTP 状态 | 错误码 | 原因与处理方式 |
| ---: | --- | --- |
| 400 | `missing_model` | 缺少 `model` |
| 400 | `missing_content` | `prompt` 为空且未提供非空 `content` |
| 400 | `invalid_content` | `content` 不是非空数组，或元素缺少 `type` |
| 400 | `missing_resolution` | 未提供 `resolution`，也没有可推导的宽高 |
| 400 | `invalid_dimensions` | 宽高缺失一项、不是正整数或无法推导 |
| 400 | `unsupported_ratio` | 比例不在支持列表中 |
| 400 | `unsupported_resolution` | 所选模型不支持目标分辨率 |
| 400 | `unsupported_frames` | 当前接口不接受帧数，请改用 `duration` |
| 400 | `unsupported_frame_rate` | 当前接口不接受帧率字段，请改用 `duration` |
| 400/422 | `seedance_task_create_failed` | 服务方拒绝创建；根据 `message` 修正素材或参数 |
| 401 | — | 令牌缺失、无效或已删除 |
| 403 | — | 用户、令牌、模型权限或分组不允许调用 |
| 429 | — | 当前分组负载已饱和，请稍后重试查询或重新发起任务 |
| 500/502 | — | 平台或服务方暂时异常；保留请求编号用于排查 |

客户端应同时记录：

- HTTP 状态码；
- 响应中的 `code`、`message` 和 `data`；
- 响应头 `X-Oneapi-Request-Id`；
- 创建成功后的 `task_id`。

## 9. 当前能力限制

1. 当前 AP Seedance 2.0 接口使用 `duration` 表示秒数，不接受 `frames`、`fps`、`framespersecond` 或 `frames_per_second`。
2. `AP Seedance-2.0 轻量版` 当前不支持 4K。
3. 参考视频必须能被公网直接读取；每帧像素总数应至少为 409600，建议使用不低于 720p 的视频。
4. 仅提供音频而没有文本、图片或视频主体内容的请求不属于当前支持组合。
5. 服务方可能依据素材内容规则拒绝包含真人的参考视频。遇到此类错误时，应改用纯文本生成，或更换为符合要求的参考素材。
6. `service_tier` 属于能力参数；如果服务方明确拒绝该字段，应移除后重新构造请求，不能对同一个创建请求盲目重试。
7. 回调地址必须为公网可访问地址。没有公网回调时，应通过任务查询接口轮询结果。

## 10. 计费规则

Seedance 按异步任务计费，当前主要计费维度为：

- 模型；
- 分辨率；
- 时长秒数；
- 是否包含参考视频；
- 用户分组倍率。

平台人民币显示使用固定结算汇率：

```text
1 美元 = 7.3 元人民币
```

概念公式：

```text
人民币费用 = 美元秒单价 × 计费秒数 × 分组倍率 × 7.3
```

含参考视频和不含参考视频可能使用不同单价。当前验证中，VIP1 的常规倍率为 0.78；480p 使用原价策略，不应用该折扣。实际价格可能动态同步，请以平台实时价格目录和最终消费日志为准。

参数在创建任务前被平台拒绝时，不会产生成功任务，净费用应为零。任务创建成功后，最终费用以任务结算、消费日志和账户余额变化为准。

## 11. 重试与生产建议

1. 创建任务的 `POST /v1/videos` 不应盲目重试。
2. 如果创建请求已经返回 `task_id`，只轮询该任务，不要重复提交。
3. 如果创建请求因网络中断导致结果不明确，先通过请求编号、平台任务记录或管理员日志确认是否已创建任务。
4. `GET /v1/videos/{task_id}` 可以在网络失败时安全重试，建议间隔 5–10 秒。
5. 对 429 和临时 5xx 使用带上限的指数退避；不要无限重试。
6. 下载结果后建议保存媒体类型、文件大小、时长、分辨率和校验值。
7. 不要在日志中记录完整令牌、管理员密码或带签名参数的素材 URL。

## 12. 上线前检查清单

- [ ] 令牌属于正确的普通用户和分组。
- [ ] `GET /v1/models` 能看到目标模型。
- [ ] 目标分辨率在该模型的能力表中。
- [ ] 图片、视频和音频 URL 可从公网读取且不会短时间过期。
- [ ] 使用 `duration`，未发送帧数或帧率字段。
- [ ] 保存响应头请求编号和创建成功后的任务 ID。
- [ ] 创建接口不做盲目重试。
- [ ] 轮询逻辑能处理 `queued`、`in_progress`、`completed` 和 `failed`。
- [ ] 失败时展示平台返回的详细 `code` 和 `message`。
- [ ] 费用按系统固定汇率 7.3 与消费日志进行对账。
