# AIPDD 八类端点默认接入 — 实施清单（定稿）

实测站点 `https://api.aipdd.work`，catalog schemaVersion=2，revision `7494d4ff…`，awcoin 汇率 10000 awcoin = 1 元。
原始响应留存：`.tmp-aipdd/catalog.json`、`.tmp-aipdd/models.json`。

## 已确认决策
1. ASR / 图生图 走**同步直连**（LTX 仍走异步任务）
2. 决策模型 `jev` 由**上游补进 catalog**
3. 老 9 条 ComfyUI 兜底硬编码：**删代码 + 清库**
4. `imagePricing` 少收费漏洞：**本次一并修**

## 实测契约摘要

| 类目 | 端点 | 形态 | 模型 | 计费 |
|---|---|---|---|---|
| 文字 | `/v1/chat/completions`(+`/v1/responses`,`/v1/messages`) | 同步流式/非流式 | 7 个 LLM | per_token |
| 图片生成 | `/v1/images/generations` | **同步 200 直返 data[].url** | ap-image-2.0/fast-2.0/flare-2.5/lite-2.0/sunburst-2.5/jimeng-image-4.0/nano-2.1 | per_call + **imagePricing 分档** |
| 图生图 | `/v1/images/edits` | **同步；JSON(image/image[]) 与 multipart 均实测 200** | 上述 `supportsEdits:true` 的 + ap-qwen-image-edit | 同上 |
| 视频 | `/v1/videos` | 异步 | ap-minimax-h3(per_second), ap-ltx-2.5(available:false, shared_task) | per_second / per_unit |
| TTS | `/v1/audio/speech` | 异步任务 | ap-indextts-2 | per_call 1000 |
| ASR | `/v1/audio/transcriptions` | **同步 multipart，OpenAI 标准** | ap-funasr | per_unit 100 awcoin/秒 |
| 决策 | `/v1/systemone` | **同步；非 OpenAI 格式** | jev → 归一 jev-latest | per_token 2800/百万 input，output 单价 0 |
| 向量 | `/v1/embeddings` | 同步 | ap-qwen3-embedding:0.6b（1024 维） | per_token 300/百万 |

### jev 请求/响应（实测）
```jsonc
// → POST /v1/systemone
{"model":"jev","state":{...},                      // state 必填
 "questions":{"q1":{"type":"choice",               // 判别式，目前只验证到 choice
   "instructions":"…","criteria":{"yes":"…","no":"…"}}}}  // criteria 必填 dict，key 即候选
// ← 200
{"model":"jev","answers":{"q1":{"type":"choice","choice":"yes","confidence":0.92,
 "probabilities":{"yes":0.96,"no":0.04}}},
 "usage":{"input_tokens":325,"output_tokens":32}}   // ← 非 prompt_tokens/completion_tokens
```

### ASR
- `{"text":"…"}`（默认 json）**不含 duration**
- `response_format=verbose_json` → 含 `duration`、`segments`、`language`
- 响应头带 `x-aipdd-task-id`；网关侧必须自行解析音频时长兜底
- 10 秒 wav 实测 200

---

## P0 上游 catalog 契约（你侧发版）
在 `/v1/new-api/catalog` 的 `models[]` 增加 jev，字段需满足网关 schemaVersion≥2 校验（cache 两价必填、protocols 白名单）：
```jsonc
{"id":"jev","name":"jev","description":"AIPDD 决策模型…",
 "inputModalities":["text"],"outputModalities":["decision"],"available":true,
 "execution":{"protocol":"systemone","path":"/v1/systemone"},
 "protocols":["systemone"],
 "pricing":{"pricingModel":"per_token","currency":"awcoin","enabled":true,
   "promptPerMillion":2800,"completionPerMillion":0,
   "cacheReadPerMillion":0,"cacheWritePerMillion":0},
 "features":{"byProtocol":{"systemone":{"input_modalities":["text"],
   "usage":{"nonStreaming":true,"streaming":false}}}}}
```
其余保持不变（`token_market_shared`、`audio-transcriptions`、`imagePricing` 由网关侧适配吸收）。
`ap-qwen-image-edit` 当前 `/v1/images/edits` 返 503「暂无可用媒体服务」→ 请确认该模型是否应发布为可编辑。

---

## P1 常量与契约层

**`constant/endpoint_type.go:16`** — 新增两个类型（字符串必须与上游字面量一致）：
- `EndpointTypeAudioTranscription EndpointType = "audio-transcriptions"`
- `EndpointTypeSystemOne EndpointType = "systemone"`
- 图生图不新增类型，复用 `image-generation` + capability 上的 `SupportsEdits` 标记

**`common/endpoint_defaults.go:29`** — 补两条：`audio-transcriptions → /v1/audio/transcriptions`、`systemone → /v1/systemone`（`embeddings` 已存在）

**`constant/aipdd.go`**
- `:9-20` 删除 `AIPDDModelFluxGGUF`/`…T2I`/`Wan22Wanx`/`Wan22Animater`/`MimicMotion`/`Latentsync15`/`IndexTTS`/`LTX23`/`LTX23StartEnd` 常量
- `:114` `AIPDDWan22WanxRMBPerSecond` 删除
- `:140-346` `AIPDDCapabilities` 整张硬编码表删除 → 改为空表（catalog 是唯一事实源）；**先 grep 全部引用**（relay 里 LTX duration 校验、前端 playground 常量、测试）再删
- `:600-606` `IsAIPDDExcludedModel` 去掉 `funasr`（`lightx2v`/`seedvr2` 暂留）
- `:352` `AIPDDOpenAIModelList []string` → `[]AIPDDModelMeta{ID, Protocols, EndpointTypes, EmbeddingDimensions, Available}`；`SetAIPDDOpenAIModels`/`IsAIPDDOpenAIModel`/`GetAIPDDOpenAIModelList` 同步改造
- `:561-573` `GetAIPDDEndpointTypes` 重写：capability → 其 EndpointType（并附加 edits 能力）；meta → protocols 映射（embeddings/systemone/chat→openai/responses→openai-response）
- `AIPDDCapability` 增加 `SupportsEdits bool`、`MaxInputImages int`、`ImagePricingTiers`、`SyncDirect bool`

**`pkg/aipddcatalog/atomic.go`**
- `:235` adapterCode 白名单 + `token_market_shared`
- `:284-298` protocols 白名单 + `embeddings`、`systemone`（含 byProtocol 键）
- `AtomicPricing` + `ImagePricing *AtomicImagePricing`（`defaultUnitRmb`/`tiers[].match.unitRmb`/`policy`/`supportedSizes`/`supportedQualities`/`maxInputImages`/`maxOutputImages`/`supportsEdits`/`resolutions`）
- `AtomicModel.Features` + `EmbeddingDimensions`
- `RuntimeCapabilities()`：对 `shared_task` 且 endpointType ∈ {audio-transcriptions, image-generation} 的条目打 `SyncDirect=true`，其 ExecutionPath 不参与任务提交
- `TaskAWCoinPrice`：保留 floor 价，档位交给 P4 计费层解析

**`pkg/aipddcatalog/catalog.go:395-423`** `inferEndpointType`/`parseEndpointType` 白名单扩充；推导顺序加入 embedding→embeddings、asr/transcription→audio-transcriptions、decision→systemone（当前会误判成 image-generation）

**`model/aipdd_catalog_sync.go:482-490`** `aipddCatalogEndpointType` 白名单同步扩充

---

## P2 路由与中继

**`router/relay-router.go`** — 新增 `POST /v1/systemone` → `controller.Relay(c, types.RelayFormatOpenAI)`（走非流式 JSON helper）。
`/v1/embeddings:173`、`/v1/audio/transcriptions:178`、`/v1/images/edits:168` 现状已是同步 `controller.Relay`，**不改路由**，只保证 P4 元数据把这些模型写对 endpoints。

**`relay/constant/relay_mode.go`** — 新增 `RelayModeSystemOne`；`Path2RelayMode:68-124` 加 `/v1/systemone` 前缀匹配

**`controller/relay.go:36-57`** — systemone 分发到非流式 JSON helper（复用 EmbeddingHelper 形态）

**`relay/channel/openai/adaptor.go`**
- `GetRequestURL:166-172` 默认分支原样透传，systemone/embeddings/edits/transcriptions 天然命中，**无需新增分支**
- `ConvertImageRequest:448-577` multipart 路径已实测兼容上游，不动
- `ConvertAudioRequest:382-446` 需为 AIPDD+transcription **注入 `response_format=verbose_json`** 以取 duration；响应回裁剪见 P4

**`relay/channel/task/aipdd/adaptor.go:2218-2231`** `endpointTypeFromPath` 补 `/v1/audio/transcriptions`、`/v1/images/edits`、`/v1/embeddings`、`/v1/systemone`（防误判 invalid_endpoint）；同步条目命中后走直连而非 RelayTask

**`relay/relay_task.go:1217-1258`** `endpointTypeFromTaskPath`/`mediaTypeFromEndpoint`/`endpointTypeFromMediaType` 同步补新类型

---

## P3 渠道与库初始化

- **删代码兜底后清库**：新增一次性迁移（SQLite/MySQL/PG 三库兼容），删除 `vendor=AIPDD` 且模型名命中老 9 条的 `models` 行、其 `abilities`、以及 `aipdd_model_price`/`aipdd_task_pricing` option 中的残留条目
- `removeStaleAIPDDModelsTx`（`:450-470`）在新 catalog 生效后自动清理不在册的旧模型；两者叠加即可
- `upsertManagedAIPDDChannelTx`/`replaceManagedAIPDDAbilitiesTx` 无需改结构，新模型名自动进渠道与 abilities
- `BackfillAIPDDDefaultModels`（`model/channel.go:285`）在空表时回填，确认新模型能写入

---

## P4 计费（含漏洞修复）

- **imagePricing 选档**：新增匹配器，按请求 `quality` / `size` / 像素面积匹配 `tiers[].match`，policy 三种（`gpt-image-pixels-v1` 按像素、`quality-tiers-v1`、`size-tiers-v1`）；未命中回落 `chargeConfig.amountAwcoin`；输出张数按 `maxOutputImages`/`n` 乘算；应用点 `relay/helper/price.go`
- **ASR 按秒**：优先用 `verbose_json.duration`；缺失时网关侧解析音频头（wav fmt chunk / mp3 xing）兜底，仍取不到按 1 秒保底并记 warning
- **jev**：`usage.input_tokens→prompt_tokens`、`output_tokens→completion_tokens` 归一后进 per_token 计费；input 侧需含 `state`+`questions`+隐藏开销（实测单问 301~338 token），**不能按字符估**
- **embeddings**：`usage.prompt_tokens` 走现有 per_token，completion 单价 0

### P4 落地情况

**imagePricing 分档（已修）**
- `constant.AIPDDImagePricing` 只保留 `DefaultUnitAWCoin` + `Tiers[{Match,UnitAWCoin}]`；上游的 `policy`/`resolutions`/`supportedSizes`/`supportedQualities`/`maxInputImages`/`maxOutputImages` 无消费方，已从 `AtomicImagePricing` 删除（`SupportsEdits` 保留，图生图靠它）
- 匹配规则两趟：**先严格按字面/像素面积匹配**（`"2K"` == `2048x2048`），全部落空才把像素尺寸向上取到分辨率档。反过来会把已标价的 `1024x1536`（2K 档 0.3 元）按 0.4 元收，实测目录里 ap-image-2.0 就是这个形状
- 多个档位同时命中取**最贵**；`match` 里出现未知键 → 该档不参与匹配
- 应用点不是 `price.go`，而是 `dto/openai_image.go GetTokenCountMeta()`：把档位价折进既有 `ImagePriceRatio`，预扣与结算共用同一份 `PriceData`，无需新增计费分支；张数仍由 `relay/image_handler.go` 的 `OtherRatios["n"]` 承担
- 实测档位倍率（`.tmp-aipdd` 对真目录校验）：ap-image-2.0/fast-2.0 1024=1.5 2048=2 4096 与 3840x2160=3；flare-2.5 同形状但 1024x1536=2（quality 档）；lite-2.0 1/1.25/1.5；sunburst-2.5 1.33/1.67/2.33；nano-2.1 1K=0.8 2048=1 4096=1.2

**ASR 按秒（已修）**
- 目录里 ap-funasr 是 `per_unit` + `chargeConfig.unit=second`，同步直连；按次价会「10 分钟录音收 1 秒钱」。`model/aipdd_catalog_task_pricing.go:syncAIPDDSynchronousPerSecondPricingTx` 把它导成**无分辨率档**的 `unit=second` task_pricing（`pkg/aipddcatalog.SynchronousPerSecondPrices()` 只收同步端点，视频按秒模型仍归任务管线），接在 `applyAIPDDCatalog` 三个导入器之后
- `relay/helper/price.go:SynchronousDurationUnitPriceUSD()` 是唯一的判定口径：同步 + 按秒 + `task_pricing` + 平铺档 → 预扣**只按 1 秒**，实测时长在响应阶段以 `OtherRatios["seconds"]` 乘算（与图片 `n` 同形）
- 请求侧 `ConvertAudioRequest`：只有客户端格式可重建（空/`json`/`text`/`verbose_json`）才改写上游 `response_format=verbose_json`；`srt`/`vtt`/`audio` 原样透传，绝不破坏响应
- 响应侧 `OpenaiSTTHandler`：时长优先级 = 上游 `duration` → `common.GetAudioDuration` 解上传文件 → 1 秒保底 + warning；随后把 verbose_json 裁回客户端要的格式（`json` 只留 `{"text"}`，`text` 出 `text/plain`，`verbose_json` 原样）
- `relay/audio_handler.go`：按时长计费时补 `TotalTokens/PromptTokens >= 1`（`service/text_quota.go:303` 会把 0 token 的额度清零），并在日志里记「转写时长 X 秒」

**jev / embeddings（确认后无需改动）**
- 两者都走 `OpenaiHandlerWithUsage` / `OpenaiHandler`，`relay-openai.go:582-587` 已把 `input_tokens/output_tokens` 归一到 `prompt/completion_tokens`，jev 的非标准 usage 天然被吃下；per_token 价由 `controller/ratio_sync.go:aipddCatalogRatioData` 生成 `tiered_expr`，预扣只是估算、结算按上游 usage
- embeddings 走 `default` 分支 → `PostTextConsumeQuota`，`total_tokens>0` 不会被清零；completion 单价 0 由表达式 `c * 0` 承担

---

## P5 元数据与前端

- 后端：`upsertAIPDDModelsTx:301-330` tag/描述按 8 类补齐（决策/向量/ASR/编辑当前无对应文案）；`model/pricing_default.go`、`common/endpoint_defaults.go` 输出新端点
- 前端：`features/pricing/constants.ts:67-98` ENDPOINT_TYPES + labels；`pricing/lib/model-metadata.ts:36-48` 能力集合；`features/models/constants.ts:169-176` 端点→路径表（后端孪生）；`channels/components/dialogs/channel-test-dialog.tsx:89-115` 端点选项 + `controller/channel-test.go:98-196,688-727` 响应解析（注意 AIPDD 在 `:70` 被跳过测试）；`usage-logs/.../task-logs-columns.tsx:75-77` 媒体类型映射；playground 需支持 embeddings / edits 文件上传 / ASR 上传 / systemone 结构化编辑
- i18n：6 个 locale 补齐新端点文案

### P5 落地情况

**后端元数据：8 类一处定义**

- 新增 `model/aipdd_model_metadata.go`：端点类型 → 中文类目/描述一张表（文字模型/向量模型/决策模型/图片生成/图生图/视频生成/语音合成/语音识别）。`aipddCapabilityTags()` 在其上再叠「同步返回 / 异步任务」和「按次 / 按时长」，`aipddCapabilityDescription()` 取首个命中类目的描述。中文 tag + 中文描述的形态与 `model/cn_providers_defaults.go` 既有约定一致
- `upsertAIPDDModelsTx` 先取 `catalog.RuntimeCapabilities()` 建 `capabilityByName`，再据此写 `Tags`/`Description`。这是刻意的：**存的 tag 不可能与实际调用方式不一致**（同步/异步、按次/按秒都来自同一个决策函数）。没有 capability 的协议派生模型（如 `ap-qwen3-embedding:0.6b`）走 `aipddLLMModelTags` / `aipddEndpointDescription`
- `model/pricing_default.go` 的 AIPDD 默认目录行改为同一张 capability 表驱动（`Tags`/`Description`/`EndpointTypes` 与入库同一口径），ComfyUI 静态兜底已删。`common/endpoint_defaults.go` 13 条端点路径核对完整（含 `image-edits`/`audio-transcriptions`/`systemone`），无需再改
- 测试面：`model/channel_env_test.go:444` 的 `require.Contains(item.Tags, "ComfyUI")` 改 `"视频生成"`

**前端**

- 定价筛选：`features/pricing/constants.ts` 的 `ENDPOINT_TYPES` 补 `image-edits`/`audio-transcriptions`/`systemone`/`openai-response-compact`，`getEndpointTypeLabels()` 为新增类目提供本地化 label（品牌类 Chat/Response/Anthropic/Gemini/Rerank 维持原样）；`pricing-columns.tsx`、`model-card.tsx` 的端点徽标统一读这份 label，且**没有引入 `useMemo`**，`memo` 包装仍然有效
- 能力→模态：`pricing/lib/model-metadata.ts` 端点集合补 `systemone`，`IMAGE_OUTPUT_ENDPOINTS = image-generation|image-edits`、`AUDIO_INPUT_ENDPOINTS = audio-transcriptions`，图生图与 ASR 因此在模型详情页正确归入图片/音频
- 端点→路径：`features/models/constants.ts` `ENDPOINT_TEMPLATES` 补三条新端点（后端 `endpoint_defaults.go` 的孪生表，两处必须同步）
- 模型详情页示例：`model-details-api.tsx` 新增 `buildImageEditSample`（multipart `-F` / `client.images.edit`）、`buildTranscriptionSample`、`buildSystemOneSample` 三个 builder，tab 文案本地化
- 任务日志：`task-logs-columns.tsx` 的 `TASK_KIND_META` 改读 catalog 的 taskKind（`image_generation`/`image_to_image`/`video_generation`/`voice_clone`…），旧的 kind 保留用于历史行。`ENDPOINT_MEDIA_TYPES` 有意不含 `audio-transcriptions`——ASR 是同步转写，产出是文本，没有可预览的音频结果
- 模型测试平台：`model-test-platform-data.ts` 按真实 catalog 换成 7 条（`ap-chat-3.8-flash`、`ap-qwen3-embedding:0.6b`、`ap-image-2.0`、`ap-qwen-image-edit`、`ap-ltx-2.5`、`ap-indextts-2`、`ap-funasr`），上传类字段带 `file: true` 走 multipart；5 条老 ComfyUI 条目删除。P6 又补了第 8 条 `ap-minimax-h3`（视频目录里真正可用的那个），并把 `ap-ltx-2.5` 的时长约束成 5/10
- playground：`constants.ts` 新增 `isImageEditModel()`，`index.tsx:365` 对编辑模型做「必须有参考图」的前置校验；LTX 前缀判定补 `ap-ltx`，否则 `ap-ltx-2.5` 的尺寸选项会静默变空数组
- 渠道：`channel-type-config.ts` type 58 只保留被消费的 `defaultBaseUrl`（drawer 用它自动填 `base_url`），删掉 drawer 里 `currentType === 58 && typeConfig.supportedModels` 的自动填充块——AIPDD 模型由 catalog 同步，不该再写死。`hints` 沿用文件既有形态（`getChannelTypeHints` 目前零调用方，故未给它补 i18n）

**i18n**

- 24 个新 key × 6 locale（en/zh/fr/ja/ru/vi）全部入表：4 个端点 label、7 个测试平台模型名、5 条模型说明、6 个字段 label（输入文本/编辑指令/图片数量/语言/响应格式/音频文件 URL）、2 个任务日志 label（视频生成/语音克隆）。P6 为 `ap-minimax-h3` 卡再加 4 个（模型名、时长（秒）、视频分辨率、按秒与必填参数说明），合计 28 个
- `bun run i18n:sync` 后 6 个 locale 的 missingCount/extrasCount 均为 0，新增 key 无一条被判为 untranslated（untranslated 计数与改动前完全一致）
- 踩坑记录：`scripts/sync-i18n.mjs` **只在 locale 文件之间对齐，不会从源码 harvest 新 key**。改完代码后它报告 missingCount 0，但 diff 显示 0 个 key 被新增——新 `t()` key 必须手工补进 `en.json` 及其 5 个兄弟文件
- 未纳入本次范围：另有 18 个 key 在 `en.json` 缺失，全部来自 2026-09-06 就已存在的文件（playground LTX 首尾帧面板、MiniMax H3 首尾帧面板、wallet `use-payment`、`model-pricing-sheet`），与 AIPDD 改造无关

**偏差与遗留（明确记录）**

1. **`channel-test-dialog.tsx` 没有加 5 个媒体端点选项**。已加过又回退：`controller/channel-test.go:683 buildTestRequest` 只认识 embeddings / image-generation / rerank / responses / chat，未知 endpointType 会把 chat 请求体 POST 到媒体路径；且 AIPDD 渠道在 `:70` 就被 `unsupportedTestChannelTypes` 跳过。给出选项却测不出真实结果只会误导，故维持不做，渠道文档改为引导用模型测试平台验证
2. **playground 尚不支持 embeddings / 图生图上传 / ASR 上传 / systemone 结构化编辑**——模型测试平台能测，playground 不能，属未完成的配套
3. `isLTX23*` 一组判定已成死代码（catalog 只剩 `ap-ltx-2.5`），但仍织在 payload builder 与 `index.tsx` 控制流里，删除需要单独回归，本次未动
4. 决策模型 jev 这次只落了 `systemone` 的端点 label、路径与请求示例，**实际模型仍等 P0 上游 catalog 发版**
5. 文档同步：`docs/model-config-guide.zh_CN.html`（8 类能力表 + 调用示例改为同步/异步正确形态）、`docs/model-test-platform.zh_CN.html`（内嵌模型目录换成 7 条 catalog 条目，脚本 `node --check` 通过）、`docs/aipdd_vs_newapi_rmb_price_compare.csv` 9 行 comfyui 快照加下线注记（测量值原样保留），`.meta.json` 补 `legacy_comfyui_rows`。`docs/dockerhub-aipdd-deploy.zh_CN.md:60` 已写明「不再内置默认 AIPDD 模型、不回退 comfyui_workflow」，无需改

**验证到什么程度**：后端 `gofmt` + `go build`（按包清单，绕开 `main.go` 的 web embed）+ `go test ./model/... ./pkg/aipddcatalog/...` 全绿；前端 `bun run typecheck`（`tsc -b`）通过、编辑过的文件跑过 `bunx prettier --write`；两份 HTML 文档做过标签配对与 JS 语法检查。**页面未在浏览器里实测过**，UI 呈现留到 P6 冒烟时目视确认。

---

## P6 验证

- 单测：`constant/aipdd_test.go`、`pkg/aipddcatalog/*_test.go`、`model/aipdd_catalog_sync_test.go`、`model/channel_env_test.go`、`router/relay_router_aipdd_test.go`、`relay/relay_task_*_test.go`
- httptest 假 catalog：用真实 `catalog.json` 作 fixture，覆盖 `token_market_shared`、`audio-transcriptions`、`systemone`、`embeddings`、`imagePricing`
- 真机冒烟：扩 `bin/aipdd-user-smoke-test.mjs` 跑 8 类端到端（含计费对账）
- 回归：老 ComfyUI 模型确认从 `/v1/models`、定价页、渠道 abilities 全部消失

### P6 落地情况

**真实 catalog 端到端单测**（新增 `model/aipdd_production_catalog_sync_test.go`，4 个用例，全部走 httptest 信封 → `SyncAIPDDCatalog` → 库 / 运行时 / 定价页）

- fixture 是 `pkg/aipddcatalog/testdata/aipdd_catalog.json`：生产 `/v1/new-api/catalog` 的**内层 `data`**（revision `7494d4ff009fab…`，12 条 capability + 8 条 model = 20 个名字，`rmbPerAwcoin=0.0001`）。汇率在测试里钉成 7.3，所以换算口径固定为 `awcoin / 73000 = USD`，所有期望值都是按目录原值算出来的，不是估的
- 用例 1「八类初始化」：类目成员**从落库的 `Model.Endpoints` 读回**再断言，并叠中文 tag（文字模型 7、向量模型 1、图片生成 7、图生图 6、视频生成 2、语音合成 1、语音识别 1、决策模型 nil 且就地注明 P0 未发版）；受管渠道断言 type 58 / 名称 / 启用 / `group=default` / BaseURL 即 fixture 地址 / models 与 catalog 集合相等；运行时 `constant.GetAIPDDCapability` 断言 `ap-funasr`、`ap-qwen-image-edit`、`ap-image-2.0` 为同步，`ap-indextts-2`、`ap-minimax-h3` 为异步任务；abilities 逐条校验 group 与 enabled；9 个老 ComfyUI 名在 models/abilities 里 0 行
- 用例 2「价格全导入」：9 个按次模型逐个断言目录 AW 币价（2000/4000/2000/2400/3000/1500/5000/1000/10000）换算后的 USD；`ap-qwen-image-edit` 单独钉成回归值 `0.136986301369863`；`ap-funasr` 平铺 `unit=second` 单价 = 100 awcoin；`ap-minimax-h3` 三档 900/1200/1500；确认**没有**生成 `ModelRatio` option（per_token 价由 `controller/ratio_sync.go` 的倍率同步路径负责，不属 catalog apply）；`ap-ltx-2.5` 隐藏、其余不隐藏；`InvalidatePricingCache()` 后 `GetPricing()` 恰为 `20-1` 条，按次 = `QuotaType 1`、按秒 = `BillingModeTaskPricing`、LLM = `QuotaType 0`
- 用例 3「幂等」：第二次同步 Added/Removed/UpdatedPrices 全 0，快照表只有 1 行且 revision、`SourceBaseURL`、payload 都对得上
- 用例 4：`isTokenMarketPerCallCapability` 同时覆盖 `token_market_media` 与 `token_market_shared`，per_second 与 comfyui 仍为 false

**修掉的少收费 bug（决策 4）**

- `isTokenMarketPerCallCapability` 原先只认 `token_market_media`，而 `ap-qwen-image-edit` 是 `token_market_shared` + `per_call` → 固定价**从来没被导入过**，于是落到 `setting/ratio_setting/model_ratio.go:469,809` 的未知模型默认倍率 37.5，按 token 收——一次 10000 awcoin 的编辑实际只收到零头。改成两种 adapter 都认，同步 `UpdatedPrices` 由 10 变 11，编辑价入库 `0.136986301369863` USD
- 时长档导入器 `isTokenMarketDurationCapability` **有意不放宽**：`tokenMarketCapabilityAvailable` 要求 `pricingBasis=display`，funasr 的 basis 为空，放宽会把 funasr 刚建好的 task_pricing 删掉
- 记录一个潜在缺口（本次不改）：若上游把 `ap-ltx-2.5`（`token_market_shared` + `per_unit`/second + 异步）开放，它既不在同步按秒导入器（只收同步端点）也不在 media 时长导入器（只收 `token_market_media`）→ 没有时长价可导

**真机八类实测**（直连 `https://api.aipdd.work`；key 只经 `AIPDD_PROBE_KEY` 环境变量传入，未写入任何文件）

| 类目 | 模型 | 实测结果 |
|---|---|---|
| 文字模型 | `ap-chat-3.8-flash` | 503 `token_market_pool_exhausted`；同端点换 `ap-deepseek-v4-pro` 200 → 端点与鉴权通，失败是该模型池耗尽 |
| 向量模型 | `ap-qwen3-embedding:0.6b` | 200，21.8 s |
| 图片生成 | `ap-image-2.0` | 200，41.4 s，`data[].url` |
| 图生图 | `ap-image-2.0` | 200（generations 系模型接受参考图）；`ap-qwen-image-edit` 503「当前模型暂无可用的媒体服务」 |
| 视频生成 | `ap-minimax-h3` | 202 `QUEUED` → 轮询 `SUCCEEDED`，`actual_quantity=1.0`、`settled_awcoin=900`，与目录 480p×1s 完全一致 |
| 音频模型 | `ap-indextts-2` | 400「缺少必填参数：audio」（探针没有公网可访问的参考音频） |
| 语音识别 | `ap-funasr` | 200，10.3 s，默认 `json` 形态 `{"text":"我。"}`——**不含 duration**，正是 P4 要注入 `verbose_json` 的依据 |
| 决策模型 | — | 400「该接口仅接受平台维护模型」，等 P0 上游发版 |

**冒烟脚本改造 + 计费对账**（`bin/aipdd-user-smoke-test.mjs`）

- 9 个老 ComfyUI 用例换成 8 类，每例带 `category`、`kind`（`sync` / `sync-multipart` / `async`）与 `billing.kind`（`tokens` / `per_call` / `per_second`），`--only` 可按用例名或模型名挑选
- 对账口径：调用前后各读一次 `/v1/dashboard/billing/usage`，delta ≤ 0 直接判 FAIL（等于「没计费」）；给了 `AIPDD_SMOKE_EXPECTED_USD_<NAME>` 时按 `AIPDD_USAGE_UNIT`（`usd-cents` 默认 / `cny-fen` / `raw-quota`）换算并与目录价比对，超出 `AIPDD_BILLING_TOLERANCE_PERCENT`（默认 2%）判 FAIL；按秒用例优先读响应 `duration`，读不到才回落请求时长
- `reportLegacyModels()`：9 个下线名只要还有一个出现在 `/v1/models` 就整体失败——这就是 P6 要求的那条回归
- 视频参数按协议分流：默认 `token-market` 发 `duration_seconds` + `video_resolution`（`--video-resolution` 默认 480p），`--video-protocol shared-task` 改发 `duration`；`--duration` 上限随之取 1-15 / 1-20
- 决策模型用例改成 `model` + `state` + `questions`（`dto.SystemOneRequest` 与 `relay/helper/valid_request.go:117-132` 三者必填），没配 `--systemone-model` 时 SKIP 而不是发一个必失败的请求
- 验证方式：本机起桩网关（按 `relay/channel/task/aipdd/adaptor.go:226-248` 与 `valid_request.go` 同样的字段校验规则）跑通全部 8 类，并跑反向用例证明桩确实会拒错参数（给 `ap-minimax-h3` 发 `duration` → 400）。**经本网关的真机冒烟还没做**，需要你的 NewAPI 用户令牌：`node bin/aipdd-user-smoke-test.mjs --base-url … --token sk-…`

**网关比上游严的一处口径（要你定）**：上游允许 `ap-minimax-h3` 只传 `duration`（实测 202，默认按 480p×1s 预估 900 awcoin）；本网关的 `token_market_video` 分支强制 `duration_seconds` + `video_resolution`（选档和预扣需要）。照上游文档写的客户端经网关会 400。两份文档已按网关口径写明，是否放宽成「缺省走默认档」留给 P0 一起决定。

**测试平台与文档同步**

- `model-test-platform-data.ts` 新增 `ap-minimax-h3` 卡（`duration_seconds` 1-15 滑杆、`video_resolution` 480p/768p/1080p、`ratio`）；`ap-ltx-2.5` 的时长从 1-15 滑杆改成 **5/10 下拉**——`normalizeDurationSeconds` 对非 `aipdd-ltx-2-3` 的 shared_task 模型只接受 5 或 10，原来的滑杆会构造出必失败的取值
- 4 个新 i18n key × 6 locale；数据表 58 个 `labelKey`/`noteKey`/`billingKey`/`fileLabelKey` 在 6 个语言文件里逐条核对无缺失；`bun run i18n:sync` 通过，`bun run typecheck`（`tsc -b`）通过，编辑过的文件 `bunx prettier --check` 通过
- `docs/model-config-guide.zh_CN.html`：视频生成行改写为按协议写参数，新增 `ap-minimax-h3` 的 curl 例（含轮询），并在目录行标注 `ap-ltx-2.5` 上游不可用
- `docs/model-test-platform.zh_CN.html`：同步新增该卡、时长改 5/10 下拉、`duration_seconds` 等数值字段统一序列化成 JSON number（网关对 `duration_seconds` 用的是 `positiveFloat`，字符串会当缺失）；两份 HTML 做了标签配对与内联 JS 语法检查

**验证到什么程度**：`gofmt -l` 干净、按包清单 `go test` 全绿（含新增 4 个用例）、`go vet` 对改动包干净、`node --check` + 桩网关 8 类全通过。**仍未做**：P5 的 UI 没在浏览器里目视确认；经本网关的真机 8 类冒烟还没跑（缺 NewAPI 用户令牌）；P0 未发版所以决策模型只有端点没有可跑模型。

---

## 风险与遗留

1. **P0 未发版则 jev 无法自动接入**（决策模型整条搁浅）
2. `ap-qwen-image-edit` 编辑接口 503「当前模型暂无可用的媒体服务」，需上游确认（同图 `ap-image-2.0` 在 `/v1/images/edits` 实测 200，可先用它验证链路）
3. ~~`V1ModelsListHidden` 需确认~~ 已由 `model/aipdd_production_catalog_sync_test.go` 钉死：`available=false` 的 `ap-ltx-2.5` 进隐藏集、其余 19 个不进。`ap-chat-3.8-flash` 是 available=true 且**不**隐藏，实测它的失败是上游池耗尽（503 `token_market_pool_exhausted`），不是目录状态问题——这类「目录说可用、上游暂时没路由」的模型仍会暴露入口，需上游侧解决
4. 删除 LTX/首尾帧常量牵连后端 relay 专用逻辑与前端 playground 常量，**删前必须 grep 全引用并逐项判定是真死代码还是别处在用**
5. ASR 注入 verbose_json 会改变返回给客户端的响应体形态，属破坏性变更，需开关或按客户端显式请求透传
6. 网关对 `token_market_video` 强制 `duration_seconds` + `video_resolution`，比上游严（上游实测允许省略，默认按 480p×1s 预估）。照上游文档写的客户端经本网关会 400，是否放宽成「缺省走默认档」待确认
7. ~~`token_market_shared` + `per_unit`/second 的**异步**模型没有时长价导入器~~ 已解决：异步按秒导入器已补，且 `available` 不再参与定价判定（2026-10-11 口径），`ap-ltx-2.5` 虽被上游标为 `available=false` 也照样有时长价（批发 ¥0.4/秒 → 零售 ¥0.44/秒），不会再出现「能调用但没价」

---

## 实施进度

### 已完成：契约层 + 拆兜底 + 清库（原 P1，以及原 P3 的清库项）

新增端点类型 `image-edits`、`audio-transcriptions`、`systemone`（`embeddings` 复用已有），并在 `common/endpoint_defaults.go` 补齐路径映射。

真实生产 catalog（schemaVersion 2，11 条 capability + 8 条 model）现在可以通过 `Validate()`；改造前它会被整份拒绝（adapter 白名单不含 `token_market_shared`、protocol 白名单不含 `embeddings`/`systemone`），只能退回旧快照或首次部署直接失败。

与原方案的偏差（均为实测后的收敛，需在 P6 前确认无异议）：

1. **图生图改为独立端点类型**。原方案写「不新增类型，复用 `image-generation` + `SupportsEdits` 标记」，实际实现同时给出：主 `EndpointType` 保持 `image-generation`，`SupportsEdits=true` 时把 `image-edits` **追加**进 `AIPDDCapability.EndpointTypes`，因此一个模型可同时出现在两个端点的筛选里。理由：`Model.Endpoints` 是列表，定价页/渠道测试/playground 都按列表匹配，单类型无法表达「同一模型两个入口」。
2. `AIPDDModelMeta` 只保留 `ID` + `Protocols`。方案里列的 `EndpointTypes`/`EmbeddingDimensions`/`Available` 三项当前没有消费方（EndpointTypes 由 protocols 映射得出，Available 走 `V1ModelsListHidden`），先不加，避免存了没人读的字段。
3. `AtomicImagePricing` 的 `MaxInputImages`/`MaxOutputImages`/`SupportsEdits` 中，`SupportsEdits` 已用于第 1 条；两个 `Max*Images` 与 `resolutions[].Available` 暂无消费方，字段已解析进 `AIPDDImagePricing` 但 P4 若不用则删。
4. 同步直连的判定键从 endpointType 改成 **taskKind**：`image_to_image → /v1/images/edits`、`speech_to_text → /v1/audio/transcriptions`，落在 `RuntimeCapabilities()` 里置 `Synchronous=true`。endpointType 不可靠（`ap-qwen-image-edit` 上游就把它标成了 `image-generation`）。
5. 端点映射的唯一实现在 catalog 侧（`catalog.ModelEndpointTypes()`），因为 `activateAIPDDCatalog` 在 DB 事务之后才跑，`upsertAIPDDModelsTx` 不能依赖 `constant.GetAIPDDEndpointTypes`。协议→端点映射本身仍只有一份（`constant.AIPDDEndpointTypesForProtocols`）。
6. `funasr` 从 `IsAIPDDExcludedModel` 摘除后，屏蔽只剩 `lightx2v`/`seedvr2`。`IsAIPDDFunASRModel` 随之删除。

删除的兜底：`constant.AIPDDCapabilities` 硬编码表、9 个模型名常量、`AIPDDWan22WanxRMBPerSecond`、`model/pricing_default.go` 里 7 条 AIPDD 静态目录行、`setting/ratio_setting/model_ratio.go` 里 7 条 AIPDD 静态价格。`ResetAIPDDCapabilities()` 现在等价于清空——AIPDD 没有内置默认模型。

清库迁移：`model/aipdd_legacy_cleanup.go` 的 `RemoveLegacyAIPDDModels()`，接在 `migrateDB`/`migrateDBFast` 的 `EnsureAIPDDDefaults()` 之前，幂等，只删老 9 条名字命中的 `models`（硬删）、AIPDD 渠道的 `abilities` 与 `Channel.Models` 字段。三库兼容：全部走 GORM，无方言 SQL、无保留列名。

测试面调整：老表删除后依赖它的测试改成 catalog 形态夹具——
`relay/channel/task/aipdd/legacy_workflow_test.go`（新增，ComfyUI 工作流参数表搬到测试侧）、
`controller/aipdd_fixture_test.go`（新增）、
`model/channel_env_test.go`、`model/channel_fun_asr_test.go`、
`relay/relay_task_media_test.go`、`pkg/aipddcatalog/{catalog,atomic}_test.go`、`constant/aipdd_test.go`。
一处覆盖按设计取消：`TestListModelsIncludesDefaultPricedAIPDDModelWhenModelPriceOptionIsStale` 断言的正是「编译期内置价格」这一被删除的兜底，改写成 `TestListModelsRequiresExplicitPriceForAIPDDModels`，明确 AIPDD 模型必须有 catalog 同步价或管理员价才会出现在 `/v1/models`（对应 `relay/helper/price.go:263-265` 的既有约束）。

`go build` + `go test ./constant/... ./common/... ./pkg/... ./model/... ./relay/... ./router/... ./controller/... ./service/... ./setting/...` 全绿。

### 已完成：路由与中继（原 P2）

`/v1/systemone` 接入：`RelayModeSystemOne` + `Path2RelayMode` 前缀匹配 + `types.RelayFormatSystemOne` + `dto.SystemOneRequest`（`state`/`questions` 以 `json.RawMessage` 原样透传，`GeneralOpenAIRequest` 会丢掉这两个字段）+ `relay/systemone_handler.go` + `controller/relay.go` 分发 + `router` 注册（`httpRouter`，即 `/v1` + Distribute，与 embeddings 同级）。响应侧在 openai adaptor 的 `DoResponse` 加 `RelayModeSystemOne` 分支复用 `OpenaiHandlerWithUsage`：它既原样回写上游 body，又已把 `usage.input_tokens`/`output_tokens` 归一到 prompt/completion，正好是 jev 的计费口径。`middleware/distributor.go:323` 的通用 JSON 分支已能取出 `model`，无需改动。

同步判定切换：`isAIPDDSynchronousMedia` 改为读 `capability.Synchronous`。该标志在 `RuntimeCapabilities()` 里由两条来源置位——catalog taskKind（`image_to_image`→`/v1/images/edits`、`speech_to_text`→`/v1/audio/transcriptions`），以及协议自述（`execution.protocol == token_market_image`，即 7 个图片生成模型本来就在 `/v1/images/generations` 同步直返）。**只按 taskKind 判定会让图片生成回退成异步任务**，这是实测真实 catalog 后发现的回归点。`token_market_audio`（ap-indextts-2）与 `token_market_video`、`shared_task` 仍走任务路径，与方案的 TTS 行一致。

守卫：任务提交侧 `ValidateRequestAndSetAction` 对 `cfg.Synchronous` 直接返回 `invalid_endpoint`，同步模型不会被投进 RelayTask。

偏差：方案里「`endpointTypeFromPath` 与 `relay_task.go` 的 endpoint/media 识别补新类型」没做——同步端点根本不进任务路径，`resolveTaskMediaInfo` 另有 `OutputModalities` 兜底，加了没有消费方。ASR 注入 `verbose_json` 挪到 P4 与裁剪、按秒计费一起做（单注入会改变客户端可见响应形态）。

`go build` + 全量 `go test` 再次全绿；新增 `dto`/`relay/helper`/`relay/constant`/`router`/`task/aipdd` 侧的 systemone 与同步判定测试。

### 已完成：计费（原 P4）

见「P4 计费（含漏洞修复）」小节的 P4 落地情况：imagePricing 分档匹配器、ASR 同步按秒导入与实长结算、jev/embeddings 确认无需改动。

### 已完成：元数据与前端（原 P5）

见「P5 元数据与前端」小节的 P5 落地情况：8 类元数据收口到 `model/aipdd_model_metadata.go` 并由 `RuntimeCapabilities()` 驱动，前端定价/详情页/任务日志/测试平台/playground/渠道 6 处对齐，24 个新 i18n key × 6 locale 补齐，用户文档与价格快照同步。

### 已完成：验证（原 P6）

见「P6 验证」小节的 P6 落地情况：真实生产 catalog 的 4 个端到端用例（八类初始化 / 每个已发布价格 / 幂等 + 快照 / per_call 适配器），`isTokenMarketPerCallCapability` 少收费修复（`ap-qwen-image-edit` 编辑价 0.136986301369863 USD 从此有回归断言），直连上游的八类实测记录，`bin/aipdd-user-smoke-test.mjs` 扩到 8 类并接上 `/v1/dashboard/billing/usage` 对账与老 9 名回归，测试平台补齐 `ap-minimax-h3` 卡并把 `ap-ltx-2.5` 时长约束成 5/10，两份用户文档同步。

### 待做

- **P0（上游侧）**：把 `jev` 发布进 `/v1/new-api/catalog`，否则决策模型整条只有端点与展示，没有可同步的模型
- **经网关的真机八类冒烟**：需要 NewAPI 用户令牌，`node bin/aipdd-user-smoke-test.mjs --base-url … --token sk-… --image-file … --audio-file … --audio-url …`；按类给 `AIPDD_SMOKE_EXPECTED_USD_<NAME>` 才会做价格比对
- **P5 UI 的浏览器目视确认**：定价筛选、模型详情页示例、任务日志列、测试平台新卡都只做了类型/构建/键位层面的验证
- 遗留功能缺口：playground 的 embeddings / 图生图上传 / ASR 上传 / systemone 编辑；渠道列表自动测试不支持 5 个媒体端点
- 待你定的两条口径：网关对 `token_market_video` 的强制参数是否放宽成默认档；`token_market_shared` 异步按秒模型的时长价导入器是否要补（见风险 6、7）
