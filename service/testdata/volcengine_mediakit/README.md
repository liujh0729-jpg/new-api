# 火山 AI MediaKit 画质增强响应 fixture

## 来源与效力

这些 fixture **来自 2026-09-04 读取的火山官方公开请求/响应示例并已脱敏**，不是测试账号抓取的真实响应。它们锁定的是本仓库适配器
`service/seedance_mediakit_provider.go` 的解析契约：

- 提交端点 `POST /api/v1/tools/enhance-video`，请求字段 `video_url` / `scene` / `resolution` / `tool_version`；
- 查询端点 `GET /api/v1/tasks/{task_id}`；
- 官方响应使用裸信封；解析器仍兼容 Java 基线见过的 `data` 包裹信封；
- 结果地址位于 `result.video_url`；
- 查询结果的 `status=failed` 表示任务业务失败；`success=false` 表示查询操作失败；
- `expires_at` 是结果 URL 的到期时间，默认结果保留 24 小时，临近到期的查询会自动续期；
- 提交体的 `client_token` 是官方幂等键，重试同一任务会返回原 `task_id`。

## 未冻结的契约（阻塞 Provider 转 ACTIVE）

以下语义仍未完全冻结，因此首次创建的 Provider 保持 `DISABLED`，需要管理员显式启用：

| 项目 | 当前处置 | 解锁条件 |
| --- | --- | --- |
| 提交幂等保证 | **已由官方文档确认**；发送稳定的 `client_token`，`submit_retry_safe=true` | 真实账号灰度仍应验证返回同一 `task_id` |
| 任务取消 | `cancel_supported=false`，已受理任务拒绝本地取消 | 官方任务 API 出现取消接口 |
| 极速版 `fast` | 白名单只含 `standard` / `professional` | 单独验证开通状态与成本表 |
| 精确成本核算字段 | 只保存非敏感时长/分辨率/帧率证据，成本仍用冻结快照 | 真实响应样本确认字段 |

拿到测试账号的真实响应后，请用脱敏后的真实样本**替换**本目录文件（保持文件名不变），并同步修正
`mediaKitResponse` 与状态映射；测试会立即暴露差异。

## 脱敏规则

- 不保存任何 API Key、Authorization 头或可下载的签名 URL；
- `video_url` 一律替换为 `https://mediakit-output.example/redacted-signed-object`。
