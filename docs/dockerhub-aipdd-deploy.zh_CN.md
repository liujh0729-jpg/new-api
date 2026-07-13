# 阿里云 ACR 镜像部署方案

本文用于从阿里云 ACR 拉取 New API、PostgreSQL 和 Redis 镜像并部署服务，同时说明后台如何添加渠道和模型。生产部署和版本升级统一使用 ACR，不再使用 DockerHub 作为运行时镜像源。

## 镜像信息

| 项目 | 内容 |
| --- | --- |
| New API 公网镜像地址 | `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest` |
| PostgreSQL 公网镜像地址 | `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15` |
| Redis 公网镜像地址 | `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest` |
| 当前 New API 镜像 digest | `sha256:d1c68946c722eefe52866bdb53a69116ee36b0adffa88e73b8d9527aee4b6f2f` |
| 目标架构 | `linux/amd64` |
| 服务端口 | `3000` |

拉取命令：

```bash
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker pull crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
```

如需按 digest 固定版本：

```bash
docker pull crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd@sha256:d1c68946c722eefe52866bdb53a69116ee36b0adffa88e73b8d9527aee4b6f2f
```

## 版本发布与更新规则

以后更新项目 Docker 版本时，先使用当前项目代码构建并推送到 ACR，再到部署服务器执行更新。不要再向旧外部镜像仓库发布，也不要让生产服务器从外部镜像仓库拉取应用、PostgreSQL 或 Redis 镜像。

在开发机或 CI 中执行：

```bash
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker build --platform linux/amd64 \
  -t crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest .
docker push crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
```

如果 PostgreSQL 或 Redis 版本变化，也必须把对应版本同步到上面记录的 ACR 公网地址，并同步修改 Compose 文件。部署服务器只执行 `docker compose pull` 和 `docker compose up -d`，不要使用 `--build`。

## 部署前准备

需要准备：

- 一台已安装 Docker 和 Docker Compose 的服务器。
- 一个 `AIPDD_API_KEY`。先到 [app.aipdd.work](https://app.aipdd.work) 注册并获取。
- 一个固定的 `SESSION_SECRET`，生产环境不要每次重启变化。
- 数据库。推荐 PostgreSQL；单机轻量部署也可以用 SQLite。
- Redis。单机可选，生产环境推荐启用。

生成随机密钥：

```bash
openssl rand -hex 32
```

注意：该镜像默认启用 AIPDD 引导检查和上游模型 catalog 同步，使用 AIPDD 内置模型时必须设置 `AIPDD_API_KEY`。设置后系统会自动请求 `https://api.aipdd.work/v1/capabilities`、`/v1/models` 和 `/system/awcoin-rate`，并创建或同步名为 `AIPDD` 的渠道；旧版 `/scripts/admin/comfyui_workflow` 和 `/fee-rules` 仅作为兼容回退。

## 方案一：Docker Compose 部署

创建部署目录：

```bash
mkdir -p /opt/new-api-aipdd
cd /opt/new-api-aipdd
mkdir -p data logs
```

创建 `.env`：

```env
POSTGRES_PASSWORD=change-this-postgres-password
REDIS_PASSWORD=change-this-redis-password
AIPDD_API_KEY=change-this-aipdd-api-key
SESSION_SECRET=change-this-random-session-secret
```

创建 `docker-compose.yml`：

```yaml
version: "3.4"

services:
  new-api:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
    container_name: new-api
    restart: always
    command: --log-dir /app/logs
    ports:
      - "3000:3000"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    environment:
      - SQL_DSN=postgresql://root:${POSTGRES_PASSWORD}@postgres:5432/new-api
      - REDIS_CONN_STRING=redis://:${REDIS_PASSWORD}@redis:6379
      - AIPDD_API_KEY=${AIPDD_API_KEY}
      - AIPDD_CATALOG_SYNC_ON_BOOT=true
      - SESSION_SECRET=${SESSION_SECRET}
      - TZ=Asia/Shanghai
      - ERROR_LOG_ENABLED=true
      - BATCH_UPDATE_ENABLED=true
      - NODE_NAME=new-api-node-1
    depends_on:
      - postgres
      - redis
    networks:
      - new-api-network
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O - http://localhost:3000/api/status | grep -o '\"success\":\\s*true' || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3

  postgres:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15
    container_name: new-api-postgres
    restart: always
    environment:
      POSTGRES_USER: root
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: new-api
    volumes:
      - pg_data:/var/lib/postgresql/data
    networks:
      - new-api-network

  redis:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest
    container_name: new-api-redis
    restart: always
    command: ["redis-server", "--requirepass", "${REDIS_PASSWORD}"]
    networks:
      - new-api-network

volumes:
  pg_data:

networks:
  new-api-network:
    driver: bridge
```

启动：

```bash
docker compose pull
docker compose up -d
```

查看状态：

```bash
docker compose ps
docker logs -f new-api
```

访问：

```text
http://服务器IP:3000
```

首次访问会进入初始化向导，按页面提示创建管理员账号。

## 方案二：单容器 SQLite 部署

适合测试或小规模单机使用：

```bash
mkdir -p /opt/new-api-aipdd/data

docker run -d \
  --name new-api \
  --restart always \
  -p 3000:3000 \
  -v /opt/new-api-aipdd/data:/data \
  -e TZ=Asia/Shanghai \
  -e AIPDD_API_KEY="change-this-aipdd-api-key" \
  -e SESSION_SECRET="change-this-random-session-secret" \
  crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
```

查看日志：

```bash
docker logs -f new-api
```

## 更新镜像

Compose 部署：

```bash
cd /opt/new-api-aipdd
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker compose pull
docker compose up -d
docker image prune -f
```

单容器部署：

```bash
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker pull crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
docker stop new-api
docker rm new-api

docker run -d \
  --name new-api \
  --restart always \
  -p 3000:3000 \
  -v /opt/new-api-aipdd/data:/data \
  -e TZ=Asia/Shanghai \
  -e AIPDD_API_KEY="change-this-aipdd-api-key" \
  -e SESSION_SECRET="change-this-random-session-secret" \
  crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
```

## AIPDD 渠道如何添加

### 自动添加

推荐使用自动添加。只要容器环境变量中设置了：

```env
AIPDD_API_KEY=你的 AIPDD 上游 API Key
```

系统启动时会自动：

- 创建或同步渠道 `AIPDD`。
- 渠道类型设置为 `AIPDD`。
- Base URL 设置为 `https://api.aipdd.work`。
- 分组设置为 `default`。
- 将 `AIPDD_API_KEY` 作为上游密钥使用。
- 优先从 AIPDD 上游 catalog 获取模型列表、workflow 参数、端点类型和价格。
- 上游 catalog 获取失败时，回退到内置 AIPDD 默认模型列表。
- 创建或同步 AIPDD 模型目录元数据。

可选环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AIPDD_BASE_URL` | `https://api.aipdd.work` | AIPDD 上游地址 |
| `AIPDD_CATALOG_SYNC_ON_BOOT` | `true` | 容器启动时是否同步上游模型 catalog |
| `AIPDD_CATALOG_SYNC_TIMEOUT_SECONDS` | `10` | 启动同步超时时间 |

### 后台手动添加

如需手动添加，登录管理员后台：

1. 进入 `渠道` 或 `渠道管理`。
2. 点击 `添加渠道`。
3. 按下面填写：

| 字段 | 填写 |
| --- | --- |
| 名称 | `AIPDD` |
| 类型 | `AIPDD` |
| Base URL | `https://api.aipdd.work` |
| 密钥 | AIPDD 上游 API Key |
| 分组 | `default`，或你的业务分组 |
| 状态 | 启用 |
| 模型 | 见下方 AIPDD 模型列表，多个模型用英文逗号分隔 |

保存后，对该渠道执行测试。若模型列表为空，编辑渠道并填入下方模型列表。

## AIPDD 模型如何添加

系统启动时会自动同步 AIPDD 上游模型。同步成功后，渠道模型列表和模型目录会以 AIPDD 上游 `/v1/capabilities` 和 `/v1/models` 返回为准；新能力接口不可用时才回退旧版 `/scripts/admin/comfyui_workflow`，同步失败时才使用内置默认模型。

当前内置默认模型：

| 模型 | 能力 | 端点类型 | 调用接口 |
| --- | --- | --- | --- |
| `aipdd-flux-gguf` | Flux 图生图 | `image-generation` | `/v1/images/generations` |
| `aipdd-flux-gguf-t2i` | Flux 文生图 | `image-generation` | `/v1/images/generations` |
| `aipdd-wan2.2-wanx` | Wan2.2 图生视频 | `openai-video` | `/v1/videos` |
| `aipdd-wan2.2-animater` | Wan2.2 主体替换视频 | `openai-video` | `/v1/videos` |
| `aipdd-mimic-motion` | MimicMotion 动作迁移 | `openai-video` | `/v1/videos` |
| `aipdd-latentsync-1.5` | Latentsync 对口型 | `openai-video` | `/v1/videos` |
| `aipdd-indextts` | IndexTTS 声音复刻 | `audio-speech` | `/v1/audio/speech` |

渠道模型字段可直接填：

```text
aipdd-flux-gguf,aipdd-flux-gguf-t2i,aipdd-wan2.2-wanx,aipdd-wan2.2-animater,aipdd-mimic-motion,aipdd-latentsync-1.5,aipdd-indextts
```

如果模型目录中缺少模型，进入管理员后台：

1. 进入 `模型` 或 `模型管理`。
2. 点击 `添加模型`，或使用 `缺失模型` 功能补齐。
3. 模型名称填写上表中的模型名。
4. 供应商选择或新建 `AIPDD`。
5. 状态设置为启用。
6. 端点类型按上表选择。
7. 设置可见分组，至少包含用户所在分组。
8. 配置计费规则。

如果后台要求填写端点 JSON，可参考：

```json
{"image-generation":"/v1/images/generations"}
```

```json
{"openai-video":"/v1/videos"}
```

```json
{"audio-speech":"/v1/audio/speech"}
```

## 其他常用渠道和模型示例

添加其他渠道时，流程相同：先在渠道里添加上游，再在模型管理里确保模型目录、端点和计费规则存在。

### DoubaoVideo

适合 Seedance 视频模型：

| 字段 | 示例 |
| --- | --- |
| 渠道类型 | `DoubaoVideo` |
| Base URL | 使用火山/豆包上游视频 API 地址 |
| 密钥 | 上游 API Key |
| 模型 | 见下方 |

常见模型：

```text
doubao-seedance-1-0-pro-250528,doubao-seedance-1-0-lite-t2v,doubao-seedance-1-0-lite-i2v,doubao-seedance-1-5-pro-251215,doubao-seedance-2-0-260128,doubao-seedance-2-0-fast-260128
```

Seedance 2.0 模型支持参考素材输入；后台模型端点建议配置为 `openai-video`，调用 `/v1/videos`。

### VolcEngine

适合豆包聊天、Seedream 图片和部分 Seedance 模型：

| 字段 | 示例 |
| --- | --- |
| 渠道类型 | `VolcEngine` |
| Base URL | 使用火山引擎上游 API 地址 |
| 密钥 | 上游 API Key |
| 模型 | 按业务开放 |

常见模型：

```text
Doubao-pro-128k,Doubao-pro-32k,Doubao-pro-4k,Doubao-lite-128k,Doubao-lite-32k,Doubao-lite-4k,Doubao-embedding,doubao-seedream-4-0-250828,seedream-4-0-250828,doubao-seedance-1-0-pro-250528,seedance-1-0-pro-250528,doubao-seed-1-6-thinking-250715,seed-1-6-thinking-250715
```

端点类型按模型能力配置：

| 模型类型 | 端点类型 | 调用接口 |
| --- | --- | --- |
| 聊天模型 | `openai` | `/v1/chat/completions` |
| 图片模型 | `image-generation` | `/v1/images/generations` |
| 视频模型 | `openai-video` | `/v1/videos` |
| Embedding | `embeddings` | `/v1/embeddings` |

## 配置检查清单

上线前确认：

- `docker logs new-api` 没有 `AIPDD_API_KEY is required`。
- 管理员后台能看到 `AIPDD` 渠道，状态为启用。
- 渠道分组包含用户或令牌所在分组。
- 渠道模型列表包含要调用的模型名。
- 模型管理中对应模型状态为启用。
- 模型端点类型和调用接口一致。
- 模型计费规则已配置，避免调用时报 `扣费规则不存在`。
- 用户令牌拥有对应分组和模型权限。

## 常见问题

### 容器启动失败，日志提示 `AIPDD_API_KEY is required`

该镜像默认需要 AIPDD 引导密钥。给容器添加环境变量：

```env
AIPDD_API_KEY=你的 AIPDD 上游 API Key
```

如果临时不使用 AIPDD 内置模型，可以显式关闭引导检查：

```env
AIPDD_BOOTSTRAP_REQUIRED=false
```

### `model_not_found`

通常是渠道或模型目录未配置完整。检查：

- 渠道是否启用。
- 渠道模型字段是否包含该模型。
- 渠道分组是否包含用户分组。
- 模型管理中是否存在同名模型且状态启用。
- 模型端点类型是否匹配调用接口。

### `扣费规则不存在`

进入模型管理或系统设置中的模型定价区域，为该模型补齐计费类型和价格。

### 访问不了 3000 端口

检查：

```bash
docker compose ps
docker logs -f new-api
```

同时确认服务器防火墙、云安全组和反向代理已放行 `3000` 或对应代理端口。
