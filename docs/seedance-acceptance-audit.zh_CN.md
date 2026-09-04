# Seedance 第 19 节验收追踪

> 审计日期：2026-09-04  
> 设计基线：`seedance-channel-aipdd-async-postpaid-design.zh_CN.md`  
> Java 基线：`83137cd59afa23dd42b7050d47c2d9de64374385`

本文把“代码已经实现”和“生产外部能力已经验收”分开记录。`已自动化` 表示行为已进入持续测试；`已本机验证` 表示已使用当前 FlyEnv 运行态或真实火山样本验证；`待外部门禁` 表示没有真实账户、价格或观察期就不能勾选。当前结论是：代码闭环和本机回归已通过，生产 offering 仍应保持停用。

## 验收矩阵

| 设计验收项 | 状态 | 证据与剩余工作 |
| --- | --- | --- |
| 公共 API 不暴露 BYOK、增强拓扑、Provider、服务代码或内部任务字段 | 已自动化/已本机验证 | `docs/openapi/public_contract_test.go`、`relay/channel/task/seedance/contract_test.go`、`relay/relay_task_media_test.go` 和公共错误测试共同覆盖文档、任务 DTO 与错误边界；另以一次性普通用户实际请求模型、跨用户任务、创建、列表、消费及管理接口，扫描未发现第 4.1 节禁用概念，账号和 Token 已在测试后删除。 |
| 两套协议状态与 Java 基线一致，内部阶段不可区分 | 已自动化 | Java golden fixture 差分覆盖 Official/OpenAI 查询状态；两套适配器只投影统一工作流的普通非终态。 |
| 模型页、消费记录和用户任务只展示一次模型调用与聚合售价 | 已自动化 | offering 公共价格、VIP 比例、消费日志和严格用户任务 DTO 均有回归测试；内部费用项只在管理员面可见。 |
| 文件名、下载地址、Header 与 metadata 不泄漏内部实现 | 已自动化 | `controller/video_proxy_test.go` 覆盖公开任务 ID 文件名、Range、Header 允许列表、Content-Type 参数清理和外部 URL 拒绝。 |
| AIPDD 执行/财务能力只存在于服务间路由和内部文档 | 已本机验证 | Spring Security 对 `/api/finance/v1/**` 强制 API Key，普通 JWT 在控制器前被拒绝；运行态 `/v3/api-docs` 不发布该路由。当前首期没有启用 AIPDD 自有增强执行。 |
| Java commit、golden fixtures 与允许差异可复现 | 已自动化 | commit 已冻结，fixtures 与允许差异记录在 `seedance-java-contract-baseline.zh_CN.md`，Go 差分测试随全量测试执行。 |
| Official 与 OpenAI Adapter 共享统一订单、工作流和 attempt | 已自动化 | 创建、查询、删除的协议隔离和同一工作流持久化均有测试；创建响应晚于本地事务提交。 |
| method/status/字段/零值/错误/回调/下载/Range 符合基线或批准差异 | 已自动化 | golden snapshot、callback 边界、公开错误、媒体代理和协议隔离测试覆盖；官方列表另以真实控制器调用覆盖用户隔离、协议隔离、软删除、过滤与分页边界；允许差异以基线文档为唯一清单。 |
| 不为“通用化”增加 Java 不存在的公共兼容分支 | 已自动化 | 公开 snapshot 精确比较，OpenAPI/Apifox 禁用词门禁阻止内部字段进入公共契约。 |
| 公共契约写入 OpenAPI 并同步 Apifox | 已本机验证 | `docs/openapi/public.json` 已纳入自动化扫描；当前发布地址记录在运行手册。后续每次公共契约修改仍需重新同步 Apifox。 |
| 只有管理员可访问 Seedance 管理和凭证操作 | 已自动化 | 后端 `StrictAdminAuth`、Root 变更权限和前端 `/403` 路由保护均已实现；普通用户 403 有测试。 |
| Ark、AK/SK、AIPDD 与增强密钥仅由使用方加密保存 | 已自动化/已本机验证 | NewAPI 敏感字段采用 `enc:v2`，模型 JSON 不序列化密文；运行态已检查已填充敏感字段。AIPDD 不持有 Ark/费用中心/MediaKit 密钥。凭证轮换把旧版本置为 `RETIRING`，维护任务在一小时交接窗口结束且该版本所有订单终结后才清除 Ark/AK/SK 密文并写审计；测试同时证明历史订单可继续绑定旧版本、已销毁版本不能创建新订单。 |
| Direct Provider 在 AIPDD 计费不可用时仍能完成任务 | 已自动化 | 工作流故障注入确认业务终态与财务 outbox 解耦，恢复后幂等补扣。 |
| 编排只依赖 Provider 接口，Provider 与版本进入快照 | 已自动化 | `EnhancementProvider`、adapter capability、offering 快照与 pricing-version 变更规则均有测试；Provider 切换不改变订单 ID。 |
| AIPDD 恢复后补扣且重复投递不重复扣费 | 已自动化/已本机验证 | 覆盖受理后响应丢失、同一 Idempotency-Key 重试、scope 隔离、死信修订和恢复重放；AIPDD 本机账本仅生成一条费用项。 |
| 每单有稳定统一订单号，重试使用独立 attempt | 已自动化 | durable-before-send、未知生成结果不重投、增强同 attempt 重试和后挂上游任务号均有测试。 |
| NewAPI 与 AIPDD 不跨库读写 | 已本机验证 | 两套服务分别使用 NewAPI SQLite 与 FlyEnv PostgreSQL，通过认证财务 API 交换事件；没有跨库连接配置。 |
| AIPDD 以认证 scope 与订单/费用项复合身份幂等记账 | 已自动化/已本机验证 | AIPDD 定向测试和真实 Ark 冒烟流水已验证 `api_key_id + instance_id + platform_order_id + service_line_item_id`；结算状态、起止时间、定长字段和规范 SHA-256 证据在 inbox 前校验，错误事件不会污染幂等记录。 |
| 售价、服务售价、供应商成本、火山成本和两侧收益口径不混淆 | 已自动化/已本机验证 | 整数微元、冻结快照、外部成本归属和收益公式均有回归测试；AIPDD 以订单下所有服务费用项的当前售价合计计算 NewAPI 收益，多费用项新增、高 revision 退款及火山实际成本确认不重复扣服务费均有定向测试。费用项 revision 与订单 `source_revision` 独立，跨费用项乱序事件不会回退来源订单快照。Flyway `V289/V292` 已在本机执行，现有来源订单版本均有效且聚合利润不一致数为 0。生产金额仍待产品/财务提供并生成新 `pricing_version`。 |
| 火山成本未确认时只显示预计收益 | 已自动化/已本机验证 | 订单成本状态与管理员投影区分预计/确认；AIPDD 仅接受 `CONFIRMED + actual cost` 或“非确认状态 + 无 actual cost”的一致组合，账单 revision 确认后才使用实际成本。Flyway `V290` 已执行，本机状态不一致与利润不一致记录均为 0。 |
| 火山账单分摊严格守恒且可追溯 | 已自动化/已本机验证 | `largest_remainder_order_id_v1` 覆盖正负修订、余数和歧义拒绝；真实账单样本确认了产品与配置代码，但逐单关系仍进入人工对账。 |
| 余额不足形成欠费，不回滚已交付任务 | 已自动化 | AIPDD 后付费故障/欠费状态与 NewAPI 业务终态解耦，欠费计数只进入管理员指标。 |
| Seedance 不进入 Token 市场或 AIPDD 渠道目录 | 已自动化/已本机验证 | AIPDD 目录拉取、快照激活和启动清理均移除旧 Seedance 条目；独立 offering 才能发布公共模型。 |
| Java 旧任务继续可查，新旧任务不双写 | 部分本机验证/待外部门禁 | 当前 AIPDD Java 表中旧 Seedance 任务数为 0，无法凭空验证旧任务查询；真实 NewAPI 灰度订单的 `platform_order_id` 在 Java 表中匹配数为 0，已证明该样本未双写。仍需带一条真实旧任务样本完成观察期查询验证，再决定停止旧入口获客。 |
| 计费/执行故障、响应丢失、重复/乱序事件、崩溃和 Key scope 失效有自动化测试 | 已自动化 | Go 与 Java 定向测试覆盖 outbox、refund、revision、lease、unknown outcome、401/403 scope 和补偿扫描；MediaKit 提交结果未知只允许在官方 24 小时幂等窗口内复用同一 attempt 重试，窗口外转人工核对。火山成功而增强失败时，测试证明模型售价与服务费归零、已发生的火山预计成本和供应商成本保留、预计收益为负且客户只退款一次；远端支持/不支持取消的两个相反分支也均有回归。下游回调另覆盖独立 30 秒截止时间、失败退避、过期租约接管、第 8 次死信、最后 HTTP 状态记录和 OpenAI 公开字段投影，回调失败不修改任务终态。 |

## 尚不能宣称完成的生产门禁

1. 提供独立的 AI MediaKit API Key，并确认该账户已开通画质增强能力；Ark API Key 不能复用。
2. 冻结首发 `resolution` 与 `tool_version` 组合，以及公开模型名、Ark 私有模型映射、模型售价、AIPDD 服务售价、MediaKit 供应商成本、Ark 预计成本和新的 `pricing_version`。
3. 获得一次短视频付费灰度授权，验证同一 `client_token` 的真实幂等结果、查询/失败样本、24 小时结果 URL 时效和账单金额；若要求长期留存，另行提供 VOD/TOS 目标并实现持久化交付。
4. 提供至少一条真实 Java 旧 Seedance 任务样本，在灰度观察期确认旧查询链路持续可用；普通用户黑盒泄漏审计和 NewAPI 新任务不双写已完成本机验证。
5. 上述证据全部通过后，才可启用 Provider、发布 offering 和进入阶段 4 灰度；任一项失败都保持 `DISABLED`。

## 本次自动化基线

- NewAPI：`go test ./...` 通过，其中包含公开 OpenAPI/Apifox 防泄漏门禁。
- NewAPI 前端：完整测试集 61 项通过，`build:check` 通过。
- AIPDD：服务计费、管理员投影与真实 PostgreSQL 映射本轮定向测试 21 项通过；全量测试 1229 项通过、0 失败、0 错误、3 跳过，覆盖后付费、旧账回填、服务路由安全和公开文档可见性。
- 运行态：AIPDD 目录凭据已切换到数据库中既有且验证通过的管理员 Key；NewAPI 重启后原子目录同步 `snapshot=false`，订单 `finance_revision` 列已生效且旧终态订单缺失版本数为 0；AIPDD Flyway 为 `V292`，来源订单无无效版本、成本状态不一致或利润不一致。两服务健康检查均为 200、最新启动 stderr 均为 0，增强 Provider 与 offering 启用数均为 0，AIPDD 与 NewAPI 公共模型接口的 Seedance 条目均为 0。
- 普通用户黑盒：一次性普通账号访问 Seedance 管理面为 403，跨用户任务不可读，未发布 offering 的两套创建入口均为 503，任务/消费列表为 200，禁用内部概念扫描为 0；测试 Token 和账号已删除。
