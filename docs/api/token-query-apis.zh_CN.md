# API Key 查询接口文档

| 接口 | 说明 |
| --- | --- |
| `GET /api/usage/token/` | 查询**当前 API Key** 的总额度、已用额度和剩余额度 |
| `GET /api/log/token` | 分页查询**当前 API Key** 的用量日志 |
| `GET /api/task/token` | 分页查询**当前 API Key 所属用户**的异步任务 |

Base URL：`https://susciyuan.com`

---

## 鉴权

| 项 | 说明 |
| --- | --- |
| Header | `Authorization: Bearer sk-<API_KEY>` |
| 兼容 | `/api/log/token`、`/api/task/token` 可省略 `Bearer`，直接传 `sk-<API_KEY>`；`/api/usage/token/` 必须使用 Bearer 格式 |
| 校验策略 | 只读鉴权：Key 存在即可；**不检查** Key 是否禁用、过期、额度耗尽 |
| 用户状态 | 所属用户被封禁时拒绝（HTTP `403`） |

```http
Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### 鉴权失败示例

未提供令牌（HTTP `401`）：

```json
{ "success": false, "message": "未提供令牌" }
```

无效令牌（HTTP `401`）：

```json
{ "success": false, "message": "无效的令牌" }
```

> `message` 文案可能随服务器语言设置变化。

---

## 通用分页

`GET /api/log/token` 与 `GET /api/task/token` 使用同一套分页参数与响应结构。余额接口不分页。

### Query 参数

| 参数 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `p` | int | 否 | `1` | 页码，从 `1` 开始 |
| `page_size` | int | 否 | `10` | 每页条数；兼容别名 `ps`、`size`；最大 `100` |

### 分页响应 `data`

```json
{
  "page": 1,
  "page_size": 20,
  "total": 128,
  "items": []
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `page` | int | 当前页码 |
| `page_size` | int | 每页条数 |
| `total` | int | 总条数（见各接口说明） |
| `items` | array | 当前页数据 |

### 分页接口通用成功/失败外壳

成功：

```json
{
  "success": true,
  "message": "",
  "data": { }
}
```

业务失败（多数仍为 HTTP `200`）：

```json
{
  "success": false,
  "message": "错误说明"
}
```

---

## 1. `GET /api/usage/token/`

查询当前 API Key 的额度和累计用量。该接口返回的是 **API Key 维度**的数据，不是账号下所有 Key 共享的账号总余额。

### 基本信息

| 项 | 值 |
| --- | --- |
| Method / Path | `GET /api/usage/token/`（规范路径包含末尾 `/`） |
| 数据范围 | **仅当前 Key** |
| Query 参数 | 无 |
| 额度单位 | 原始额度单位（quota units），不是人民币或美元 |

### 请求示例

```bash
curl -sS 'https://susciyuan.com/api/usage/token/' \
  -H 'Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
```

### 成功响应示例

```json
{
  "code": true,
  "message": "ok",
  "data": {
    "object": "token_usage",
    "name": "default",
    "total_granted": 100000,
    "total_used": 20000,
    "total_available": 80000,
    "unlimited_quota": false,
    "model_limits": {},
    "model_limits_enabled": false,
    "expires_at": 0
  }
}
```

### `data` 字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `object` | string | 固定为 `token_usage` |
| `name` | string | API Key 名称 |
| `total_granted` | int | 当前 Key 的总额度，等于 `total_used + total_available` |
| `total_used` | int | 当前 Key 的累计已用额度 |
| `total_available` | int | 当前 Key 的剩余额度；通常将该字段作为 Key 余额 |
| `unlimited_quota` | bool | 是否为无限额度 Key；为 `true` 时不要用 `total_available` 判断是否可用 |
| `model_limits` | object | 当前 Key 的模型调用上限映射；未配置时通常为空对象 |
| `model_limits_enabled` | bool | 是否启用模型调用上限 |
| `expires_at` | int64 | Key 过期时间，Unix 时间戳（秒）；`0` 表示永不过期 |

### 注意

1. `total_granted`、`total_used`、`total_available` 都是站点内部原始额度单位，接口不会自动换算成人民币或美元。
2. 该接口查询的是当前 Key，不是账号余额。需要账号级余额时，应使用控制台登录态接口 `GET /api/user/self`，读取响应中的 `quota`。
3. 成功响应使用 `code: true`，与另外两个分页接口的 `success: true` 外壳不同。

---

## 2. `GET /api/log/token`

分页查询当前 API Key 产生的用量日志。

### 基本信息

| 项 | 值 |
| --- | --- |
| Method / Path | `GET /api/log/token` |
| 数据范围 | **仅当前 Key**（按 `token_id` 过滤） |
| 排序 | `id` 降序（最新在前） |
| 过滤参数 | 无（仅分页） |
| `total` 上限 | 计数最多约 `10000`（与用户日志查询一致） |

### 请求示例

```bash
curl -sS 'https://susciyuan.com/api/log/token?p=1&page_size=20' \
  -H 'Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
```

### 成功响应示例

```json
{
  "success": true,
  "message": "",
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 128,
    "items": [
      {
        "id": 1,
        "user_id": 12,
        "created_at": 1754496000,
        "type": 2,
        "content": "",
        "username": "demo",
        "token_name": "default",
        "model_name": "gpt-4o",
        "quota": 1200,
        "quota_cny": 0.01752,
        "prompt_tokens": 100,
        "completion_tokens": 50,
        "use_time": 3,
        "is_stream": true,
        "channel": 0,
        "channel_name": "",
        "token_id": 34,
        "group": "default",
        "ip": "1.2.3.4",
        "request_id": "req_xxx",
        "other": "{\"model_ratio\":1,\"group_ratio\":1,\"pre_consumed_quota\":1500,\"quota_per_unit\":500000,\"usd_exchange_rate\":7.3}"
      }
    ]
  }
}
```

### `items[]` 字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | int | 展示用连续序号（按分页偏移重编号，**不是**数据库原始主键） |
| `user_id` | int | 用户 ID |
| `created_at` | int64 | Unix 时间戳（秒） |
| `type` | int | 日志类型，见下表 |
| `content` | string | 文本内容 |
| `username` | string | 用户名 |
| `token_name` | string | 令牌名称 |
| `model_name` | string | 模型名 |
| `quota` | int | 本次额度 |
| `quota_cny` | number | 本次额度对应的人民币等值，四舍五入到 6 位小数；消费或退款方向需结合 `type` 判断 |
| `prompt_tokens` | int | 输入 token |
| `completion_tokens` | int | 输出 token |
| `use_time` | int | 耗时（秒） |
| `is_stream` | bool | 是否流式 |
| `channel` | int | 渠道 ID（用户侧通常为 `0`） |
| `channel_name` | string | 渠道名（用户侧通常为空） |
| `token_id` | int | 令牌 ID |
| `group` | string | 分组 |
| `ip` | string | 请求 IP（视配置是否记录） |
| `request_id` | string | 请求 ID（可能为空） |
| `other` | string | JSON 字符串；返回前会去掉 `admin_info`、`stream_status` 等管理侧字段 |

### `type` 取值

| 值 | 含义 |
| --- | --- |
| `1` | 充值 |
| `2` | 消费 |
| `3` | 管理 |
| `4` | 系统 |
| `5` | 错误 |
| `6` | 退款 |

### `other` 常见字段（解析 JSON 后）

字段是否出现取决于请求类型与计费路径：

| 字段 | JSON 类型 | 说明 |
| --- | --- | --- |
| `model_ratio` | number | 模型倍率，可含小数 |
| `group_ratio` | number | 分组倍率，可含小数 |
| `completion_ratio` | number | 输出倍率，可含小数 |
| `model_price` | number | 模型单价，可含小数 |
| `cache_tokens` | number | 缓存 token 数，整数 |
| `cache_ratio` | number | 缓存倍率，可含小数 |
| `pre_consumed_quota` | number | 预扣额度，整数 |
| `actual_quota` | number | 实际结算额度，整数（任务等场景） |
| `request_path` | string | 请求路径 |
| `billing_source` | string | 计费来源，如 `wallet`、`subscription` |
| `billing_mode` | string | 计费模式，如 `tiered_expr`、`task_pricing` |
| `quota_per_unit` | number | 计费发生时的额度兑美元快照，即 1 美元对应的额度数 |
| `usd_exchange_rate` | number | 计费发生时的美元兑人民币汇率快照 |

> JSON 规范中的整数和小数均属于 `number` 类型；表格在说明中进一步标注了数值语义。

### 注意

1. 不会返回同账号下其他 API Key 的日志。
2. `other` 是字符串，客户端需自行反序列化。
3. `quota_cny = quota / quota_per_unit × usd_exchange_rate`。新日志使用计费时快照；旧日志没有快照时按查询时的站点配置换算。
4. 需要时间/模型等更细过滤时，请使用控制台登录态接口（如 `/api/log/self`）。

---

## 3. `GET /api/task/token`

分页查询当前 API Key 所属用户的异步任务列表。

### 基本信息

| 项 | 值 |
| --- | --- |
| Method / Path | `GET /api/task/token` |
| 数据范围 | **用户维度**（同账号下所有 Key 看到同一任务列表） |
| 排序 | `id` 降序 |
| 等价接口 | 与控制台 `GET /api/task/self` 数据结构一致，仅鉴权方式不同 |

### Query 参数

除通用分页外，支持：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `platform` | string | 否 | 平台过滤。常见：`suno`、`mj`；部分任务也可能是渠道类型数字字符串 |
| `task_id` | string | 否 | 精确匹配公开任务 ID |
| `status` | string | 否 | 任务状态，见下表 |
| `action` | string | 否 | 动作/能力，如 `generate`、`MUSIC`、`LYRICS` |
| `start_timestamp` | int64 | 否 | `submit_time >=`（Unix 秒） |
| `end_timestamp` | int64 | 否 | `submit_time <=`（Unix 秒） |

### `status` 取值

| 值 | 说明 |
| --- | --- |
| `NOT_START` | 未开始 |
| `SUBMITTED` | 已提交 |
| `QUEUED` | 排队中 |
| `IN_PROGRESS` | 进行中 |
| `SUCCESS` | 成功 |
| `FAILURE` | 失败 |
| `UNKNOWN` | 未知 |

### 请求示例

```bash
# 分页拉取成功任务
curl -sS 'https://susciyuan.com/api/task/token?p=1&page_size=20&status=SUCCESS' \
  -H 'Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'

# 按任务 ID 查询
curl -sS 'https://susciyuan.com/api/task/token?task_id=task_abc123' \
  -H 'Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
```

### 成功响应示例

```json
{
  "success": true,
  "message": "",
  "data": {
    "page": 1,
    "page_size": 20,
    "total": 2,
    "items": [
      {
        "id": 1001,
        "created_at": 1754496000,
        "updated_at": 1754496120,
        "task_id": "task_abc123",
        "platform": "suno",
        "user_id": 12,
        "group": "default",
        "channel_id": 0,
        "quota": 50000,
        "quota_cny": 0.73,
        "action": "MUSIC",
        "status": "SUCCESS",
        "fail_reason": "",
        "result_url": "https://example.com/result.mp4",
        "submit_time": 1754496000,
        "start_time": 1754496010,
        "finish_time": 1754496120,
        "progress": "100%",
        "properties": {
          "upstream_model_name": "demo-model",
          "origin_model_name": "demo-model"
        },
        "output": ["https://example.com/result.mp4"],
        "metadata": {
          "url": "https://example.com/result.mp4",
          "urls": ["https://example.com/result.mp4"]
        },
        "endpoint_type": "video",
        "media_type": "video",
        "task_kind": "generate",
        "output_modalities": ["video"],
        "data": {}
      }
    ]
  }
}
```

### `items[]` 字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | int64 | 内部任务主键 |
| `created_at` / `updated_at` | int64 | 创建/更新时间 |
| `task_id` | string | 对外公开任务 ID |
| `platform` | string | 平台标识 |
| `user_id` | int | 用户 ID |
| `group` | string | 分组 |
| `channel_id` | int | 渠道 ID；用户侧查询通常为 `0` |
| `quota` | int | 任务计费额度；任务失败时可能退还，不等同于最终净支出 |
| `quota_cny` | number | 任务额度对应的人民币等值，四舍五入到 6 位小数 |
| `action` | string | 动作类型 |
| `status` | string | 状态 |
| `fail_reason` | string | 失败原因 |
| `result_url` | string | 成功时的结果 URL（仅 `SUCCESS` 时通常有值） |
| `submit_time` / `start_time` / `finish_time` | int64 | 提交/开始/完成时间 |
| `progress` | string | 进度，如 `"0%"`、`"100%"` |
| `properties` | object | 任务属性（如上下游模型名） |
| `output` | string[] | 结果 URL 列表（若可解析） |
| `metadata` | object | 便捷元数据，常含 `url` / `urls` |
| `endpoint_type` / `media_type` / `task_kind` | string | 媒体/端点分类（若有） |
| `output_modalities` | string[] | 输出模态 |
| `data` | object | 上游原始/归一化任务数据 |
| `username` | string | 本接口一般不返回 |

### 注意

1. 返回的是用户全部任务，不是「仅当前 Key 创建的任务」。
2. 不要依赖 `channel_id` 做渠道识别（用户侧会被省略加载）。
3. `result_url` / `output` 是否有值取决于任务是否成功及结果是否已回写。
4. 新任务的 `quota_cny` 使用任务提交时的计费配置快照；旧任务没有快照时按查询时的站点配置换算。

---

## 对比

| 对比项 | `GET /api/usage/token/` | `GET /api/log/token` | `GET /api/task/token` |
| --- | --- | --- | --- |
| 鉴权 | API Key（只读，必须 Bearer 格式） | API Key（只读） | API Key（只读） |
| 数据范围 | 当前 Key 的额度与用量 | 当前 Key 的日志 | 当前用户的全部任务 |
| 分页 | 无 | `p` / `page_size` | `p` / `page_size` |
| 业务过滤 | 无 | 无 | `platform` / `task_id` / `status` / `action` / 时间窗 |
| 典型用途 | 查询 Key 余额 | 对账、用量排查 | 查询/轮询视频、音乐等异步任务 |

---

## HTTP 状态摘要

| 场景 | HTTP | 成功标记 |
| --- | --- | --- |
| 余额查询成功 | 200 | `code: true` |
| 日志/任务查询成功 | 200 | `success: true` |
| 业务错误（如无效令牌上下文） | 200 | 通常为 `success: false` |
| 未带 / 无效 Authorization | 401 | `success: false` |
| 用户被封禁 | 403 | `success: false` |
| 服务端异常 | 500 | `success: false` |
