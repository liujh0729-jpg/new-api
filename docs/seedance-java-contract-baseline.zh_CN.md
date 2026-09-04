# Seedance Java 契约基线与允许差异

## 可复现基线

- 仓库：`E:/AIPDD/aipdd-java`
- `java_reference_commit`：`83137cd59afa23dd42b7050d47c2d9de64374385`
- 官方协议实现：`OpenPlatformSeedanceController.java`、`OpenPlatformSeedanceProxyService.java`
- OpenAI 视频协议实现：`TokenMarketMediaController.java`、`TokenMarketMediaService.java`
- 主要可执行证据：`OpenPlatformSeedanceProxyServiceTest.java`、`OpenPlatformSeedanceErrorAdviceTest.java`、`TokenMarketMediaService*Test.java`、`TokenMarketMediaUpstreamClientTest.java`
- 固化 fixture：`relay/channel/task/seedance/testdata/java-golden.json`

该 commit 仅作为协议行为证据。NewAPI 不运行或调用 Java，也不向 Java 双写任务。

## 保持一致的行为

1. 官方入口为 `POST/GET(list)/GET(id)/DELETE /api/v3/contents/generations/tasks`，创建返回 HTTP 200 和仅含公开 `id` 的对象。
2. OpenAI 入口为 `POST/GET/DELETE /v1/videos`，创建返回 HTTP 202，删除返回 HTTP 204。
3. 官方列表只读本实例、本用户的任务快照，响应根为 `items` 与 `total`，不代理上游列表。
4. 官方处理中状态不含成品 `content`；OpenAI 处理中映射为 `queued` 或 `in_progress`。
5. 客户回调只在完整工作流进入公开终态后发送；处理中不得提前发送。
6. 视频内容接口支持 200/206、`Range`、`Content-Range` 和 `Accept-Ranges`。
7. 显式 `false`、`0` 和协议规定的 `-1` 不得因 Go 零值处理被丢失。
8. 公共模型名、公共任务 ID 和中性下载地址替换所有上游身份；公共响应不出现私有阶段、执行任务号或服务费用项。
9. 任务只能通过创建它的公共协议执行列表、查询和删除；Official 与 OpenAI 入口不互相读取同一任务，仅共享中性内容下载路径。

## 本设计批准的边界差异

| 行为 | Java 基线 | NewAPI | 原因与回归证据 |
|---|---|---|---|
| 公开任务 ID | 官方旧链路使用 `cgt-*`，OpenAI 媒体使用 UUID | 使用 NewAPI 既有 `task_*` 公共任务命名空间 | 独立渠道必须与 NewAPI 的鉴权、查询和媒体代理共用任务身份；adapter/create 测试固定响应形状 |
| 结果地址 | Java 旧链路可返回持久化对象地址 | 统一返回 `/v1/videos/{task_id}/content` 中性代理地址 | 防止对象键、域名和响应 Header 暴露内部处理节点；视频代理 Header 允许列表测试覆盖 |
| 供应商响应 Header | Java 媒体代理可透传上游 `ETag` 和完整 `Content-Type` 参数 | Seedance 代理不转发供应商 `ETag`，并剥离 `Content-Type` 私有参数，只保留 Range 与中性缓存所需 Header | 本设计的防泄漏要求优先于缓存标识透传；Header 脱敏回归测试覆盖，200/206 与 Range 语义不变 |
| Ark 回调接收 | Java 用公开 callback token 路由接收 Ark 回调 | NewAPI 不向 Ark 转发客户 callback，使用已有轮询器收敛任务 | 避免新增无需公开暴露的入站路由；最终客户回调仍保持终态语义，并有租约、重试和死信 |
| 凭证与扣费 | Java 旧链路持有旧任务、旧价格与旧钱包 | NewAPI 持有 Ark 凭证、本地订单和下游预扣；AIPDD 仅异步登记服务应收 | 本设计明确改变的系统边界；outbox 与 AIPDD 幂等修订测试覆盖 |
| 模型发布 | Java 读取旧展示模型目录 | NewAPI 读取已验证渠道下的版本化 offering | 新旧任务不迁移、不双写；价格、Provider 和凭证版本在订单创建时冻结 |

未列入上表的协议差异视为缺陷。真实火山账单字段、出账延迟和首期外部处理供应商语义不在此 fixture 中伪造，必须在灰度前用真实账户样本补充。
