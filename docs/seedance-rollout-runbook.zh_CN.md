# Seedance 独立渠道实施与灰度门禁

本文记录已经固化到代码和测试的业务规则，以及必须由真实外部账户完成的上线门禁。详细架构见 `seedance-channel-aipdd-async-postpaid-design.zh_CN.md`，协议基线见 `seedance-java-contract-baseline.zh_CN.md`。

## 已冻结规则

1. 完整结果未交付时，下游模型售价和 AIPDD 服务售价都归零并退款。已发生或待确认的火山成本、已进入执行的外部处理成本继续保留，订单允许出现负收益；NewAPI 收益仅扣火山成本，外部处理成本只进入 AIPDD 服务利润，禁止跨利润口径重复扣除。
2. 第一阶段服务售价按一次调用计价，使用 offering 中的整数微元人民币和版本号。订单创建时冻结价格、节点、规格、凭证版本及证据哈希，后续配置不重算历史订单；任何会改变订单快照的 offering 修改都必须使用新的 `pricing_version`，仅启停不要求换版。
3. `DIRECT_EXTERNAL` 提交结果未知时只用相同 attempt 和幂等键重试；已返回明确失败时终止任务处理。第一阶段只接受 `retry_policy.mode=SAME_ATTEMPT_UNKNOWN_ONLY` 和 `fallback_policy.mode=NONE`，未支持的策略键会被后端拒绝，不会静默忽略。第一阶段不自动跨节点降级，避免重复结果和重复成本。
4. 直连节点成本由 NewAPI 的版本化价格和用量证据管理。未来 `AIPDD_INTERNAL` 的内部成本只存在于 AIPDD 执行账本，不跨边界复制。
5. AIPDD 授信额度只决定 `NORMAL/ARREARS`，不拒绝已交付账单。第一阶段不因欠费自动停用 NewAPI 渠道，停用由授权财务或管理员执行。
6. 无逐单稳定关联时采用 `largest_remainder_order_id_v1`：只对人工或已验证规则选出的候选订单，按正整数权重分摊；余数按小数余量降序、平台订单号升序分配。微元分配总和必须严格等于账单行金额。
7. 每个订单冻结 AIPDD 财务地址、加密 API Key 和配置 revision；轮换后旧积压事件继续使用其订单快照，新事件使用新配置，因此允许新旧 Key 属于不同 API Key scope，但旧 Key 必须保留到对应 outbox 排空，或在重放前恢复。投递收到 401/403 时只暂停同一渠道、同一配置 revision 的事件，新 revision 继续认领；管理员重放暂停事件时也只恢复该 scope，所有暂停 scope 清空后才清除渠道级汇总告警。
8. 公共模型名不得含内部处理、节点或自带密钥含义；成品统一用公开任务 ID 命名并通过 `/v1/videos/{task_id}/content` 交付。响应 Header 使用允许列表。
9. Java 行为基线固定为 commit `83137cd59afa23dd42b7050d47c2d9de64374385`。允许差异只以契约基线文档中的清单为准。
10. Ark `CreateContentsGenerationsTasks` 没有客户端幂等键。NewAPI 必须在发送首个字节前原子提交公共任务、统一订单和 `GENERATION` attempt；明确的 4xx（408 除外）才判定为拒绝。连接错误、空响应、408、3xx、5xx 或成功响应无法解析均进入 `SUBMISSION_OUTCOME_UNKNOWN`，不得由通用渠道重试或轮询器再次提交；任务只由统一超时事务关闭并产生幂等退款。后续若经人工或上游证据取得真实 Ark 任务号，可原子补挂到原 attempt。官方请求字段以 [CreateContentsGenerationsTasks API Explorer](https://api.volcengine.com/api-explorer/debug?action=CreateContentsGenerationsTasks&groupName=%E8%A7%86%E9%A2%91%E7%94%9F%E6%88%90API&serviceCode=ark&version=2024-01-01) 为准。

## 已落地的本地交付面

- NewAPI 已提供独立 Seedance 渠道、版本化 offering、加密 Ark/费用中心凭证、冻结的处理节点快照、统一订单、attempt、服务用量、计费 outbox、客户终态回调、成本账单/修订/分摊和审计表。敏感配置新写入使用 `enc:v2` 每记录随机数据密钥的信封加密，环境 `CRYPTO_SECRET` 仅作为主密钥来源；读取仍兼容既有 `enc:v1` 密文。
- 官方协议与 OpenAI 视频协议共享同一处理链路；创建响应只在本地任务、订单和首个 attempt 提交成功后返回。客户 callback 不转发给 Ark，只由终态回调 Worker 投递。
- 每个订单冻结其创建协议；Official 与 OpenAI 的列表、查询和删除入口互相隔离。用户消费日志只记录公开模型名和聚合后的按次售价，不写入 Ark 模型映射。
- 生成任务轮询、计费 outbox、终态回调和火山账单分别由独立维护循环驱动；外部维护循环使用租约、退避、幂等和死信/认证暂停状态，实例重启后可继续收敛。
- 计费 400/422 只保存对方返回的安全 `code/message` 并进入死信，不落库调试字段或响应原文。死信不能原样重放；管理员修复权威订单/费用项后使用“生成修订”，系统保留旧事件并以相同费用项创建幂等的更高 revision。
- 财务投递管理接口支持按渠道、实例 UUID、平台订单号和状态过滤；管理表格直接显示统一订单号与服务费用项号，便于跨 NewAPI/AIPDD 对账。
- 管理页面路径为 `/seedance-management`。运维指标通过仅管理员可访问的 `GET /api/seedance-admin/metrics?channel_id=<id>` 提供，页面每 15 秒刷新；增强失败按 `provider_type + service_code + reason` 聚合，提交结果未知按 `provider_type` 累计，正常待提交任务不计入；计费失败按每次失败尝试的 HTTP 状态累计，网络失败使用状态 `0`。普通用户访问该管理面返回 HTTP 403；凭证写入、offering 发布、outbox 重放和账单导入要求 Root 权限。
- 价格、渠道配置和凭证轮换审计均记录操作者、时间、前后版本及不含密钥的摘要。
- 火山凭证轮换后，旧版本先进入 `RETIRING` 并保留一小时交接窗口；维护任务只有在该版本绑定的全部订单进入成功、失败或取消终态后，才清除旧 Ark/AK/SK 密文并写入 `AUTO_RETIRE` 审计。指纹、掩码和版本号继续保留用于追溯；已销毁版本不能再创建订单。
- AIPDD 已提供 `POST /api/finance/v1/service-usage-events` 和后付费账户管理接口；成功结果使用 AIPDD 标准 `ApiResponse.data` 信封。授权管理员可通过隐藏接口 `GET /admin/finance/service-postpaid-accounts/ledger` 分别查看来源订单与服务费用项，预计收益不会显示为确认收益。NewAPI 同时兼容无信封响应，便于灰度期间回滚。
- 火山账单 Worker 使用官方 Go SDK 的 `ListBillDetail`，每小时分页重扫本月和上月；先按已验证产品代码查询，再以 `configuration_code` 精确白名单做本地 SKU 过滤，避免同一产品下的其他 Seedance 型号混入。游标带租约并在失败后退避。账单金额只接受 CNY 定点值并精确换算为微元，原始行使用去敏允许列表落库，内容变化自动形成更高 revision。
- 火山账单、AIPDD 财务投递和客户回调分别运行在独立维护循环中，不与生成任务轮询串行；任一外部财务或回调端点超时都不得延迟生成状态推进。
- 所有微元利润、失败成本和账单修订差额都使用受检 `int64` 运算；超出可表示范围时整笔配置或事务失败，不允许回绕后入账。金额字符串转换覆盖 `MinInt64` 边界且不使用浮点数。
- 自动拉取的账单行默认不猜测订单关系，先进入人工对账队列。管理员可在成本页提交已验证的终态订单候选与正整数权重，系统按 `largest_remainder_order_id_v1` 守恒分摊、关闭对账项并投递成本修订事件；非终态订单会在分摊前及行锁内再次拒绝，避免与终态事务并发改写财务版本。
- 成本分摊落库后由独立补偿扫描确认财务修订事件已进入 outbox；即使进程恰好在两步之间退出，下一轮轮询也会按账单 revision 幂等补齐。
- Ark 生成提交采用 durable-before-send：任务、订单和生成 attempt 先落库，再发送创建请求。提交结果未知时保留同一公开任务且绝不自动重投；轮询器不会把缺少上游任务号的独立 Seedance 任务提前批量置失败，统一超时事务负责失败终态、零售价和一次性退款。删除未知提交只做本地取消，不会把公开任务号误发给 Ark。
- 增强节点已把成本归属 `provider_type` 与线协议 `adapter_type` 分离，并实现 `GENERIC_HTTP` 与 `VOLCENGINE_MEDIAKIT` 双模式。MediaKit 使用服务器固定官方地址/服务代码、独立加密 API Key、结构化规格、历史快照适配器、受检 ClientToken、查询结果去敏和“不支持远端取消”的保守语义；管理页可新增、编辑、轮换或显式清除凭证。火山官方文档确认新域名为 `mediakit.cn-beijing.volces.com`，并将请求体 `client_token` 定义为幂等控制凭证、限制为最多 64 个可打印 ASCII 字符；实现以稳定 attempt ID（必要时稳定哈希）生成该字段，并只在官方 24 小时幂等窗口内自动重试。窗口外保留未知 attempt 供人工核对，绝不重新 POST。该代码能力不等于生产能力已验收，真实账号仍需验证同一 token 的返回任务与计费表现。

## 新实例初始化

1. 在渠道页创建类型 `Seedance` 的独立渠道。通用渠道 Key 只填占位值 `managed`，运行时不会使用它。
2. 打开“Seedance 管理”。页面会自动读取全局 `AIPDD_INSTANCE_ID` 作为站点 ID，无需在每个渠道中重复填写；仅在系统未配置该变量时才显示人工填写兜底。然后保存 AIPDD 财务地址和后付费 API Key。服务地址必须为 HTTPS，开发机只允许 loopback HTTP。
3. 新增 Ark API Key；如准备拉取费用中心账单，同时录入独立的只读 AK/SK。点击“验证并激活”会分别探测 Ark 和费用中心 `ListBillDetail` 权限，任一已配置能力验证失败都不会激活该版本。
4. 新增并启用 `DIRECT_EXTERNAL` 处理节点，冻结其幂等、查询、取消、超时和失败语义。
5. 在 AIPDD 管理接口为同一 `api_key_id + instance_id` 开启后付费账户和授信额度。
6. 发布 offering。保存动作会原子更新渠道的公开模型列表和模型映射。
7. 真实账单样本确认产品代码和目标型号的 `configuration_code` 后，在基础配置中同时填写两组精确白名单并启用火山账单自动拉取。只配置 `ark_bd` 等产品代码会混入其他型号；任一白名单为空或没有已验证 AK/SK 时，后端拒绝启用。

## 真实外部能力门禁

以下项目没有凭证或真实样本时不能用模拟数据宣称完成：

- 火山费用中心 AK/SK 只读权限、账单产品/实例/计费项字段及出账延迟。
- Seedance 任务与账单明细是否存在稳定的一对一关联维度。
- 首期外部处理供应商在相同幂等键下的响应丢失、重复提交、查询、取消、实际成本和回调行为。
- Ark 创建接口没有客户端幂等键或按客户端令牌反查任务的契约，因此“Ark 已受理但响应永久丢失”无法自动发现真实上游任务号；当前策略只保证不重复提交、客户最终退款并保留火山成本对账入口，无法保证回收这类孤儿任务的结果。该残余风险必须纳入灰度告警和人工成本对账。
- 火山 AI MediaKit 必须使用独立于 Ark 的专用 API Key 完成真实短视频灰度，并确认账号开通范围、可用分辨率/版本、ClientToken 幂等行为、查询/错误样本、结果 URL 有效期和实际成本。官方当前结果链接默认保留 24 小时，剩余不足 2 小时时查询可自动续期，任务查询范围为创建后 30 天；首期必须验证 24 小时内下载，长期留存需要独立对象存储方案。缺少这些证据时 Provider 与 offering 均保持 `DISABLED`。

## 2026-09-04 本机验收记录

- FlyEnv PostgreSQL/Redis、AIPDD Java 和 NewAPI 均已启动并通过健康检查；AIPDD 的活动连接已确认指向 FlyEnv PostgreSQL `aipdd_dev` 和本机 Redis。NewAPI 按两套系统数据库隔离规则继续使用自身 SQLite 数据库。NewAPI 全量 `go test ./...` 通过；AIPDD 的服务计费、后付费账户、旧账回填、服务路由安全和公开文档可见性定向测试均通过。
- 已用真实 Ark 模型完成一笔生成任务，NewAPI 统一订单、生成 attempt、结果媒体代理、AIPDD 费用项和后付费流水均收敛。该次增强阶段使用的是仅供本机冒烟验证的直通节点，不代表真实外部处理供应商已验收。
- 已注入 AIPDD 计费不可用、服务端受理后响应丢失、401/403、Worker 租约过期和恢复重放；响应丢失重试复用完全相同的 `Idempotency-Key`，视频终态未被财务故障回滚，恢复后本地事件、outbox、AIPDD 来源订单与服务费用项仍各只有一条。
- 已用真实费用中心样本确认目标账单产品代码 `ark_bd` 和型号 `configuration_code=Doubao_Seedance_2.5`。样本仍是聚合账单，当前实现将无法证明逐单关系的账单送入人工对账队列，不自动猜测分摊。
- 公共 OpenAPI 与 Seedance Apifox 示例已通过禁用词扫描并同步到 Apifox；`docs/openapi/public_contract_test.go` 已将同一禁用词规则固化为自动化门禁，覆盖公开 OpenAPI 和全部 Seedance Apifox 补充文件。文档地址为 <https://s.apifox.cn/fea0b520-e6d9-489c-ae5e-109391c771dd>。
- AIPDD 目录边界已在拉取、快照激活和启动清理三处执行；运行态公共价格接口中旧 AIPDD Seedance 条目数为 0。独立 offering 保持停用，待真实供应商合同和生产售价冻结后再发布。
- NewAPI 原先配置的 AIPDD 目录 Key 不存在于当前本地 AIPDD 数据库，启动只能使用同源快照；现已复用数据库中既有且目录接口验证为 200 的管理员 Key。重启后原子目录同步 revision 为 `c64baf08d17f1490a0c4fc7a4705f2888f6d799d266395d27fd7287807d2ba98`、`snapshot=false`、新增 6/下线 12，启动阶段未产生 stderr；AIPDD 原子目录和 NewAPI `/v1/models` 的 Seedance 条目均为 0。后续 stderr 仅记录了本次黑盒故意触发的无效旧 Token 和未发布模型拒绝事件。
- 已创建一次性普通 NewAPI 用户执行运行态黑盒：Seedance 管理面 403，跨用户官方任务与成品 404，跨协议查询返回不泄漏内部信息的 400，未发布 offering 的两套创建入口 503，任务与消费列表 200；公共响应扫描未出现第 4.1 节内部增强概念。测试 Token 和用户随后均已删除。AIPDD Java 当前没有旧 Seedance 任务可供查询；同一真实 NewAPI 灰度订单在 Java 表中的 `platform_order_id` 匹配数为 0，因此只完成了“不双写”证明，旧任务持续可查仍需真实历史样本。
- 下游回调队列使用独立 30 秒截止时间，避免无响应端点长期占住串行队列；队列会持久化最近 HTTP 状态，网络/本地错误保持为 0，2xx 和非 2xx 均记录真实状态。回归测试覆盖截止时间取消、非 2xx 退避、崩溃后过期租约接管、第 8 次失败进入死信且不再发送，以及 OpenAI 回调只序列化公开允许字段。管理员订单表展示回调状态、尝试次数、最近 HTTP 状态和受限错误摘要，普通用户 DTO 不包含这些字段。
- 火山生成成功但增强失败时，模型售价和 AIPDD 服务费均归零并幂等退还下游预扣；已发生的火山预计成本和已受理增强任务的供应商成本证据继续保留，因此订单允许显示负的预计收益。远端任务只有在适配器证明支持且实际取消成功后才执行本地取消和退款；不支持取消时返回中性冲突错误并保持订单运行。
- 独立 offering 发布时会接管中性的字节跳动（火山引擎）模型元数据，并从 offering 的整数微元总价生成公共价格；VIP 比例在价格展示、预扣和统一订单售价中保持一致。同名可执行 offering 总价冲突时不向公共价格页报出不确定价格。
- `/api/finance/v1/**` 已在 Spring Security 路由层要求 API Key 身份；无凭证运行态请求返回 401，普通 JWT 在控制器解析前返回 403，具体实例和账户归属仍由业务层继续校验。运行态 `/v3/api-docs` 中该路径数量为 0。
- Seedance 失败/取消终态事务现在同时创建唯一的本地下游退款义务；维护任务可在终态提交后的进程崩溃窗口恢复。自动化故障注入覆盖钱包与订阅、未刷新的批量预扣、重复维护执行和唯一退款日志，确认余额、Token、用户/渠道已用额度只回退一次。运行态 SQLite 已创建 `seedance_customer_refunds` 与退款日志唯一索引。
- 通用“我的任务”投影已对独立 Seedance 任务重建严格的用户 DTO：成品地址只返回本站 `/v1/videos/{task_id}/content` 代理，忽略私有供应商 URL、上游模型名和任意持久化 Provider 字段；失败原因统一为通用视频处理失败。回归测试同时注入供应商 URL、执行任务号、Provider 类型和私有模型名，确认响应均不包含这些值。
- 独立 Seedance 创建阶段的凭证、渠道配置、内部处理节点、定价快照和本地事务异常只保留在内部日志；公共错误边界将这些失败映射为中性 `model_not_found`、`video_service_unavailable` 或 `server_error`，不会返回数据库地址、凭证、Provider 类型或内部服务名称。
- AIPDD 计费地址、加密 API Key 和配置 revision 现在随订单冻结。Key 轮换后，旧积压事件继续使用原加密快照并归属原账户；401/403 只暂停同一配置 revision 的 outbox，不再阻塞新 Key scope。升级前创建且快照为空的旧订单会在首次外发前原子冻结当时完整 scope；回归测试覆盖首次失败后轮换 Key 仍使用旧 Key 重试、旧 Key 被拒绝时新 Key 事件仍能同步，以及旧 Key 恢复后的独立重放。
- Ark 创建的崩溃窗口已收窄到外部接口本身不可消除的“受理成功但任务号永久丢失”：自动化测试确认 Ark 收到请求前本地任务、统一订单和生成 attempt 已存在；传输结果未知后再次进入同一调用不会产生第二次 Ark 请求；408/3xx/5xx 按未知处理，明确 4xx 按失败处理；Ark 4xx 的正文、私有模型和账户信息不会进入公共错误；缺失上游任务号不会被普通轮询提前失败，超时会生成唯一退款义务；未知提交的取消也不会使用公开任务号调用 Ark。
- NewAPI 已用最新代码重启；运行态 SQLite 的 `seedance_orders` 已确认包含计费地址、配置 revision、加密凭据快照和独立 `finance_revision`，旧终态订单无缺失版本；`media_enhancement_providers.adapter_type` 迁移也已生效，当前 4 个已填充敏感字段均为 `enc:v2`，源码与文档的高风险凭据形态扫描未发现明文。NewAPI 与 AIPDD 健康检查均返回 200，启动 stderr 为 0。NewAPI 全量 Go 测试通过，`go vet ./model ./service` 通过；前端 `build:check` 与完整本地测试集 61 项通过。
- 凭证回收维护任务接入后再次重启并执行只读运行态审计：有密文的 ACTIVE 火山凭证为 1，无密文的 ACTIVE 凭证为 0，仍含密文的 RETIRED 凭证为 0，RETIRING 凭证为 0，引用 RETIRED 凭证的非终态订单为 0；增强 Provider 和 offering 的启用数仍均为 0。审计程序只输出计数且执行后已删除。
- AIPDD 来源订单利润已改为按同一订单全部服务费用项的当前售价合计计算；新增费用项、高 revision 退款、乱序旧 revision 和火山实际成本确认不重复扣费均有服务测试，真实 PostgreSQL 映射测试验证两项服务合计与利润更新。费用项 `revision` 与订单 `source_revision` 已分离，跨费用项乱序事件不能用较旧快照覆盖已确认火山成本；NewAPI 对旧终态订单按已有费用项最大 revision 回填订单版本，后续每次终态或成本变更独立递增。AIPDD 仅在 `volcengine_cost_status=CONFIRMED` 且实际成本非空时使用实际成本，其余状态只计算预计收益；不一致的状态/金额组合会在 inbox 落库前拒绝。Flyway `V289/V290/V292` 已在本机执行并回填既有摘要，当前无无效订单版本、状态不一致或利润不一致记录。
- AIPDD 计费入口在 inbox 落库前校验结算终态、服务起止时间、数据库字段宽度和规范的 SHA-256 证据格式；协议错误返回校验失败，不会以数据库截断/约束异常或不可审计哈希进入账本。
- 本轮最终回归中，NewAPI 全量 Go 测试通过；AIPDD 全量测试 1229 项通过、0 失败、0 错误、3 跳过，其中订单/费用项版本、管理员投影和真实 PostgreSQL 映射定向测试 21 项全部通过。

验证前可保持自动拉取关闭，并将去敏账单行通过受限接口 `POST /api/seedance-admin/volcengine-bills/import` 导入。请求包含 `channel_id`、账单行 ID、revision、账期、产品、整数微元金额、去敏 source 及已验证的候选订单/权重。没有候选时只入原始账单并创建人工对账项，不自动猜测关系。自动拉取启用后，同样先把未匹配行送入队列；通过管理页或 `POST /api/seedance-admin/volcengine-bills/{bill_item_id}/reconcile` 完成人工确认分摊。

## 灰度核对

- 仅向内部管理员和测试站点开放渠道与模型。
- 注入财务 5xx、429、401、响应丢失和 Worker 租约过期，确认视频任务不受影响，恢复后只扣一次。
- 对账 NewAPI 订单、AIPDD 费用项/应收流水和火山账单总额；实际成本修订会通过同一 outbox 发送更高 revision。
- 用普通 API Key 扫描模型列表、任务创建/查询/列表/删除、错误、任务日志、下载文件名、URL、Header 和 metadata；确认 Seedance 结果代理不转发供应商 `ETag`、自定义 Header 或 `Content-Type` 私有参数。
- Java 旧任务继续由旧系统查询；新渠道不迁移、不双写。稳定观察期结束前不得停用旧查询链路。

## 外部协议依据

- 火山 AI MediaKit 画质增强（标准版和专业版）：<https://www.volcengine.com/docs/6448/2279230?lang=zh>
- 火山 AI MediaKit 查询任务信息 API：<https://www.volcengine.com/docs/6448/2278532?lang=zh>
- 火山 AI MediaKit 基础概念及幂等性控制：<https://www.volcengine.com/docs/6448/2300661?lang=zh>
- 火山 AI MediaKit 产品更新记录（含服务域名迁移）：<https://www.volcengine.com/docs/6448/2373721?lang=zh>
