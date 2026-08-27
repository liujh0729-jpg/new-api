# 视频生成 API 对接确认清单答复

按第 1–26 条顺序答复。请使用公开模型名（如 `AP Seedance-2.5 标准版`）和任务 ID（`task_*`）。价格以 `GET /api/pricing` 为准。

---

## 一、协议与模型标识

### 1. submit / query 是否有完整定义，有没有 OpenAPI / Postman

有两套接口，创建和查询必须配套：

| 格式 | 创建 | 查询 | 成功状态 | 视频地址 |
|---|---|---|---|---|
| Seedance 兼容 | `POST /api/v3/contents/generations/tasks` | `GET /api/v3/contents/generations/tasks/{task_id}` | `succeeded` | `content.video_url` |
| OpenAI Video | `POST /v1/videos` | `GET /v1/videos/{task_id}` | `completed` | `metadata.url` |

认证：`Authorization: Bearer <API_TOKEN>`。创建成功只返回 `{"id":"task_xxx"}`。

有 OpenAPI 和中文接口文档。暂无 Postman。

### 2. 模型 ID 是稳定别名还是带版本后缀？升级后旧 ID 还能用吗？

是稳定产品名，不带日期后缀：

- 2.0：`AP Seedance-2.0 VIP` / `标准版` / `轻量版` / `高性价比版`
- 2.5：`AP Seedance-2.5 标准版`

未宣布下线前，旧名称继续可用。请不要传 `doubao-seedance-*`。

### 3. 能否锁到具体官方版本？会不会路由漂移？

不能锁官方版本号，只认公开模型名。同一模型名按分辨率、是否含参考视频、PLATFORM / BYOK 走固定配置，不会随机换模型。

---

## 二、异步任务状态机与回调

### 4. 状态枚举

| 格式 | 非终态 | 终态 |
|---|---|---|
| Seedance 兼容 | `queued`、`running` | `succeeded`、`failed`、`cancelled` |
| OpenAI Video | `queued`、`in_progress` | `completed`、`failed` |

没有公开的 `expired`。超时记为 `failed`，并退回预授权。

### 5. 有没有 TTL？超时后是 failed 还是 expired？谁判定？

有。2.5 可传 `execution_expires_after`，默认 48 小时（3600–259200 秒）。平台超时默认 1440 分钟。超时由平台判定，转为 `failed` 并退款，不是 `expired`。

### 6. 是否支持回调？签名、重试、会不会漏推？

支持 `callback_url`（HTTPS）。终态推一次，body 与查询响应相同。

不验签，无约定重试，可能漏推。请以第 7 条查询为准。建议每 5–15 秒轮询，不要打太勤。

### 7. 回调丢了能不能自己查回来？

能。用创建返回的 `task_*` 查：

- `GET /api/v3/contents/generations/tasks/{task_id}`
- 或 `GET /v1/videos/{task_id}`

只能查本 Key 下的任务。

---

## 三、结果交付与 URL 生命周期

### 8. URL 有效期多久？服务端能不能下载转存？转存另收费吗？

返回 HTTPS 地址，不承诺固定有效期，请按短时链接处理。

**可以服务端下载转存到自有 OSS/S3，不另收费。** 成功后请尽快转存。也可走 `GET /v1/videos/{task_id}/content`。

### 9. 结果带不带封面、尾帧、大小、时长、格式、分辨率？

返回时长、分辨率、格式、视频 URL。

不返回文件大小，不保证封面。`return_last_frame=true` 时可能带尾帧，不保证必有。

### 10. 有没有水印？能不能关？

由 `watermark` 控制。2.5 默认 `false`，可关闭。打开则为模型水印。

### 11. 大文件下载有没有并发 / 带宽限制？

结果 URL 无单独承诺的并发或带宽上限。内容代理接口超时约 60 秒。

---

## 四、计费与用量回传

### 12. 按什么单位收钱？

按 **秒 × 分辨率档**。含参考视频、PLATFORM / BYOK 单价可能不同。

2.5 `duration=-1` 时先按 30 秒预授权，成功后按实际秒数结算。单价看 `GET /api/pricing`。

### 13. 成功响应里的 usage 能不能精确对账？

`usage.completion_tokens` 和 `usage.total_tokens` 是零售价等价输出 Token，不是模型真实计算 Token，也不是实际扣费金额。两者按以下公式计算且数值相同：

`ceil(最终计费秒数 × 冻结的建议零售价（元/秒） × 1,000,000 ÷ 冻结的折算零售价（元/百万输出 Token）)`

它不参与 New API 的二次计费；成功任务缺少折算价格或有效秒数时会省略 `usage`。精确对账仍使用：

对账用：

```http
GET /api/log/token?p=1&page_size=20
```

看 `model_name`、`quota`、`quota_cny`（人民币，6 位小数）、`type`（`2` 消费 / `6` 退款）。请自行保存 `task_id` 对照，账单接口不按任务 ID 过滤。

### 14. 失败 / 取消 / 审核拦截收费吗？时长变短怎么收？

不收成功价，预授权退回。

没有缓存去重。成功但时长短了，按实际秒数结算，多扣的退回。2.5 若还没有有效时长，先不结算。

### 15. 有没有余额和逐笔账单？

有，都是当前 API Key：

- `GET /api/usage/token/`：余额
- `GET /api/log/token`：逐笔明细，可用于月底对账

账号总余额看控制台，不要把单把 Key 当成全账户。

---

## 五、幂等与重试

### 16. 支不支持幂等键？

不支持。没有 `Idempotency-Key` / `client_request_id`。重复提交就是新任务。

### 17. 重试会不会重复计费、重复生成？

会。同一任务只认创建返回的那个 `task_*`。

### 18. 超时但任务可能已经创建了，怎么对齐？

拿到 `id` 就只查、不再提交。没拿到响应就没有找回键，再提交会再开一单。请用业务侧请求号自己做映射。

---

## 六、限流与并发

### 19. 并发任务数、RPM / QPS 各是多少？

不按并发任务数硬拒。按请求频率（RPM / QPS）和 Key 的 `model_limits` 限流。具体数字按账号配置，需要时对接确认。

### 20. 超限返回什么？拒绝还是排队？

拒绝，不排队。HTTP `429`，错误码 `aipdd_rate_limited`。可稍后重试。

---

## 七、错误码与合规

### 21. 审核拦截是独立错误码，还是混在 failed 里？

独立错误码。任务失败时 `status=failed`，同时带 `error.code` / `error.message`。创建阶段直接 4xx。

常见码：

- 审核：`aipdd_content_policy_violation`，以及 `*SensitiveContentDetected*`
- 参数：`invalid_resolution`、`invalid_ratio`、`invalid_duration`、`invalid_content`、`mixed_input_modes`、`insufficient_quota`
- 其他：`aipdd_rate_limited`、`aipdd_task_failed`、`task_not_exist`

### 22. 哪些能重试，哪些不能？

能重试：网络错误、5xx、超时、`429`。

不能重试：审核拦截、参数非法、余额不足、鉴权失败。

有 `retryable` 字段时以该字段为准。

### 23. 审核拦截收费吗？留不留记录？

不收费，预授权退回。留任务和错误记录。粒度到 `error.code`、`error.message`。

---

## 八、稳定性、安全与对接工程

### 24. 怎么鉴权？能多 Key 吗？有 IP 白名单吗？

Bearer Token。支持多 Key 隔离。控制台可为 Key 配 IP 白名单（`allow_ips`）。

### 25. 支持 BYOK 吗？

支持，可绑定自有官方 Key。PLATFORM 与 BYOK 价格可能不同，以 `GET /api/pricing` 为准。

### 26. 接口变更怎么通知？提前多久？

没有固定提前天数。文档和 `GET /api/pricing` 为准。通知渠道可另行约定。
