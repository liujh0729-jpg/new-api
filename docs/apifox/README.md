# Apifox 公开文档

对外只发 `project.json` 里的 `publicDocsUrl`。仓库契约是 `docs/openapi/public.json`，按 7 类展示：

1. 基础接口
2. 通用文本生成
3. 通用图片生成
4. 通用视频生成
5. Seedance
6. MiniMax H3
7. Agnes

不要在 Apifox 云端手改字段后再当真相源。完整中继目录仍在 `docs/openapi/relay.json`，不再导入公开文档站。Seedance、MiniMax H3、Agnes 的专用 OpenAI 图片/视频入口会与通用路径重复，因此分别由 `docs/apifox/seedance/`、`docs/apifox/minimax/`、`docs/apifox/agnes/` 在同步后补进对应目录。

## 一次性准备

1. 安装 CLI：`npm i -g apifox-cli@latest`
2. 登录：`apifox login --with-token <访问令牌>`，或设置环境变量 `APIFOX_ACCESS_TOKEN`
3. 项目已指定为 NewAPI 团队：https://app.apifox.com/project/8772616 （ID `8772616`）
4. 在 Apifox 客户端打开该项目 → **项目设置 → 功能设置 → 外部 AI 编辑权限**，允许 Agent 写入
5. 运行 `.\bin\sync-apifox.ps1`。公开文档：https://s.apifox.cn/fea0b520-e6d9-489c-ae5e-109391c771dd

官方 MCP（`apifox-mcp-server`）只能读，不能写。写入用下面的同步脚本。MCP 配置示例见 `mcp.json.example`。

## 接口变更后

```powershell
.\bin\sync-apifox.ps1
```

Agent 必须先改 `docs/openapi/public.json`，再跑同步。不再平行维护独立的 Markdown 接口字段表。

同步脚本会把 `public.json` 中的每个 operation 全量写回对应接口，统一鉴权设置，并将首个
`servers` 地址设为文档站默认正式环境；随后再补充各模型分类下与通用路径重复的专用接口。
脚本结束前会回读全部公开 operation，校验路径、方法、描述、鉴权及单元素
`required` / `enum` 数组未被 PowerShell 展开。不要绕过这些断言单独执行云端导入。
