# New API Docker 部署文档

> 本文面向使用 Docker 或 Docker Compose 部署 New API 的管理员，覆盖阿里云 ACR 镜像部署、源码构建、SQLite 单容器部署、PostgreSQL + Redis 生产部署、升级、备份和故障排查。
>
> 当前仓库的 Compose 模板默认使用 PostgreSQL、Redis 和 AIPDD 自动引导。生产环境请先替换所有示例密码、会话密钥和上游 API Key。

## 1. 部署方式选择

| 方式 | 适合场景 | 数据库 | 推荐度 |
| --- | --- | --- | --- |
| Docker Compose + PostgreSQL + Redis | 生产环境、多人使用、需要稳定升级 | PostgreSQL | 推荐 |
| 单容器 Docker + SQLite | 个人使用、测试、小规模内网 | SQLite | 适合轻量部署 |
| 源码构建 Compose | 需要修改 Go/前端代码或使用本地分支 | PostgreSQL | 开发 / 定制 |
| 宝塔 Docker | 已经使用宝塔面板管理服务器 | SQLite 或外部数据库 | 可选 |

无论采用哪种方式，都必须持久化数据库数据。容器删除或重建不会自动保留未挂载的数据。

## 2. 前置要求

### 2.1 安装 Docker

确认 Docker 和 Compose 可用：

~~~bash
docker --version
docker compose version
~~~

建议使用 Docker Compose v2，即 docker compose 命令，而不是旧版 docker-compose。

服务器还需要：

- 能访问 Docker Hub 和所使用的上游 AI 服务。
- 对外开放 HTTP/HTTPS 端口；数据库和 Redis 不建议暴露到公网。
- 至少 1 核 2 GB 内存；模型任务较多时按实际并发增加资源。
- 一个用于存放 Compose 文件、.env、数据和日志的部署目录。

### 2.2 准备密钥

生产环境至少准备以下密钥：

~~~bash
openssl rand -hex 32
~~~

生成的随机字符串可分别用于 SESSION_SECRET 和 CRYPTO_SECRET。服务运行后不要随意修改：

- SESSION_SECRET 变化会导致现有登录会话失效。
- CRYPTO_SECRET 变化可能导致已保存的加密配置、共享缓存或多节点数据无法解密。

如果使用 AIPDD 内置任务模型，还需要提供 AIPDD 上游 API Key。**密钥与上游地址都以数据库为准**：密钥依次取 AIPDD 渠道、首次设置向导里保存的值、本环境变量，上游地址依次取渠道的 `base_url`、`AIPDD_BASE_URL`、内置默认 `https://api.aipdd.work`；模型目录与默认定价同步都从这里取值，运行期改渠道配置即刻生效，不必重启。`AIPDD_API_KEY` 只作为容器首次启动的兜底预置项，运行期不再必填。

## 3. 阿里云 ACR 镜像部署（推荐）

当前使用的三个 ACR 公网镜像地址：

~~~text
crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest
crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15
crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest
~~~

生产部署和版本升级统一使用 ACR。ACR 中的 `new-api-aipdd:latest` 是当前应用发布源；不要再将应用发布到旧外部镜像仓库，也不要让生产服务器从外部镜像仓库拉取这三个运行时镜像。

### 3.1 创建部署目录

~~~bash
sudo mkdir -p /opt/new-api
sudo chown -R "$USER":"$USER" /opt/new-api
cd /opt/new-api
mkdir -p data logs
~~~

### 3.2 创建 .env

在 /opt/new-api/.env 中填写实际值：

~~~env
POSTGRES_PASSWORD=请替换为强密码
REDIS_PASSWORD=请替换为强密码
AIPDD_API_KEY=请替换为AIPDD上游Key
SESSION_SECRET=请替换为固定随机字符串
CRYPTO_SECRET=请替换为固定随机字符串
~~~

不要把 .env 提交到 Git，也不要把它直接发送到工单或聊天工具。

### 3.3 创建生产 Compose 文件

将下面内容保存为 /opt/new-api/docker-compose.yml：

~~~yaml
services:
  new-api:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest
    container_name: new-api
    restart: unless-stopped
    command: --log-dir /app/logs
    ports:
      - "6070:6070"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    environment:
      SQL_DSN: postgresql://root:${POSTGRES_PASSWORD}@postgres:5432/new-api
      REDIS_CONN_STRING: redis://:${REDIS_PASSWORD}@redis:6379
      AIPDD_API_KEY: ${AIPDD_API_KEY}
      AIPDD_CATALOG_SYNC_ON_BOOT: "true"
      AIPDD_CHANNEL_OVERWRITE_ON_BOOT: "true"
      SESSION_SECRET: ${SESSION_SECRET}
      CRYPTO_SECRET: ${CRYPTO_SECRET}
      TZ: Asia/Shanghai
      ERROR_LOG_ENABLED: "true"
      BATCH_UPDATE_ENABLED: "true"
      NODE_NAME: new-api-node-1
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started
    networks:
      - new-api-network
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O - http://localhost:6070/api/status | grep -o '\"success\":\\s*true' || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3

  postgres:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15
    container_name: new-api-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: root
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: new-api
    volumes:
      - pg_data:/var/lib/postgresql/data
    networks:
      - new-api-network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U root -d new-api"]
      interval: 5s
      timeout: 5s
      retries: 10

  redis:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest
    container_name: new-api-redis
    restart: unless-stopped
    command: ["redis-server", "--requirepass", "${REDIS_PASSWORD}"]
    networks:
      - new-api-network

volumes:
  pg_data:

networks:
  new-api-network:
    driver: bridge
~~~

### 3.4 校验并启动

如果 ACR 仓库需要认证，先在部署服务器登录 ACR。然后执行以下命令；不要把展开后的 Compose 配置输出公开，因为其中可能包含密码：

~~~bash
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker compose pull
docker compose up -d
~~~

查看容器状态和日志：

~~~bash
docker compose ps
docker compose logs -f --tail=200 new-api
~~~

检查健康接口：

~~~bash
curl http://127.0.0.1:6070/api/status
~~~

看到成功响应后，用浏览器访问 http://服务器IP:6070。首次访问会进入初始化向导，按页面提示创建管理员账号。

## 4. 使用仓库源码构建并更新 ACR 镜像

如果需要使用当前仓库代码发布新版本，先在开发机或 CI 构建并推送到 ACR，再让部署服务器从 ACR 拉取。不要在部署服务器上构建或使用 `docker compose up -d --build`：

~~~bash
git clone https://github.com/QuantumNous/new-api.git
cd new-api
mkdir -p data logs
~~~

启动前在当前目录准备环境变量：

~~~bash
export POSTGRES_PASSWORD='请替换为强密码'
export REDIS_PASSWORD='请替换为强密码'
export AIPDD_API_KEY='请替换为AIPDD上游Key'
export SESSION_SECRET='请替换为固定随机字符串'
export CRYPTO_SECRET='请替换为固定随机字符串'
~~~

也可以把这些变量写入项目根目录的 .env，但不要提交该文件。

构建并推送应用镜像：

~~~bash
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker build --platform linux/amd64 \
  -t crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest .
docker push crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest
~~~

然后在部署服务器更新：

~~~bash
docker compose pull
docker compose up -d
~~~

源码构建会使用 Bun 构建 web/default 前端，再使用 Go 编译后端，并把前端静态文件复制到最终镜像。

## 5. 单容器 SQLite 部署

单容器适合测试、个人使用或小规模内网部署。生产环境如果有多人并发、多个实例或大量任务，优先使用 PostgreSQL + Redis。

~~~bash
mkdir -p /opt/new-api/data /opt/new-api/logs
cd /opt/new-api
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker pull crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest

docker run -d \
  --name new-api \
  --restart unless-stopped \
  -p 6070:6070 \
  -v /opt/new-api/data:/data \
  -v /opt/new-api/logs:/app/logs \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET='请替换为固定随机字符串' \
  -e CRYPTO_SECRET='请替换为固定随机字符串' \
  -e AIPDD_API_KEY='请替换为AIPDD上游Key' \
  crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest
~~~

SQLite 数据库和本地素材会保存在 /opt/new-api/data。/data 挂载是必须的，否则删除容器后数据可能丢失。

如果暂时不使用 AIPDD 内置模型，可以显式关闭 AIPDD 引导：

~~~text
AIPDD_BOOTSTRAP_REQUIRED=false
AIPDD_CATALOG_SYNC_ON_BOOT=false
AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false
~~~

## 6. 环境变量说明

| 变量 | 说明 |
| --- | --- |
| PORT | 服务监听端口，默认 3000；通常只修改宿主机端口映射即可 |
| TZ | 容器时区，建议设置为 Asia/Shanghai |
| SQL_DSN | PostgreSQL 或 MySQL 连接字符串；不设置时使用 SQLite |
| SQLITE_PATH | SQLite 数据库路径；容器部署时建议放在 /data 下。注意它覆盖的是整串 DSN，一旦显式设置，默认自带的 WAL 与 busy_timeout 参数就不再生效（见下方说明） |
| REDIS_CONN_STRING | Redis 连接字符串；多实例、共享缓存和任务轮询建议启用 |
| SESSION_SECRET | 会话密钥；生产环境必须固定 |
| CRYPTO_SECRET | 加密密钥；使用 Redis 或多实例时必须固定并保持一致 |
| AIPDD_API_KEY | AIPDD 上游 API Key 的兜底值；密钥以数据库为准（AIPDD 渠道 → 首次设置向导 → 本变量），运行期改渠道密钥无需重启 |
| AIPDD_BASE_URL | AIPDD 上游地址的兜底值，同样以数据库为准（渠道自身的 base_url → 本变量 → 内置默认 https://api.aipdd.work） |
| AIPDD_CATALOG_SYNC_ON_BOOT | 是否在启动时同步 AIPDD 模型目录；不设置时只要配了密钥就同步 |
| AIPDD_CATALOG_SYNC_INTERVAL_MINUTES | AIPDD 模型目录定时同步间隔（分钟），默认 5；设为 0 禁用。定时任务始终启动，每一拍自行判断是否已有密钥，所以向导里后补的密钥不用等重启 |
| AIPDD_CATALOG_SYNC_TIMEOUT_SECONDS | 单次 AIPDD 模型目录同步超时（秒），默认 10 |
| AIPDD_CHANNEL_OVERWRITE_ON_BOOT | 是否允许启动同步覆盖 AIPDD 渠道配置；手动维护渠道时可设为 false |
| AIPDD_PRICING_MARKUP | AIPDD 默认定价的零售毛利倍率，默认 1.10（批发价上加 10%）；作用于目录同步写入的文本模型分档表达式与任务模型价格，填 1 即平价销售 |
| MATERIAL_PUBLIC_BASE_URL | 本地素材的公网访问地址；异步任务或文件上传时按部署域名配置 |
| ERROR_LOG_ENABLED | 是否记录错误日志 |
| BATCH_UPDATE_ENABLED | 是否启用批量更新 |
| NODE_NAME | 节点名称；多容器部署时用于审计日志识别 |
| STREAMING_TIMEOUT | 流式请求无响应超时时间，单位秒 |
| LOG_MAX_FILES | 日志目录保留的 `oneapi-*.log` 文件个数，默认 30；填 0 或负数表示不清理历史日志 |

PostgreSQL Compose 网络内的连接示例：

~~~text
postgresql://root:数据库密码@postgres:5432/new-api
~~~

外部 MySQL 连接示例：

~~~text
root:数据库密码@tcp(mysql.example.com:3306)/new-api?parseTime=true
~~~

不要把 localhost 写进 New API 容器的外部数据库配置；在容器内，localhost 指向 New API 容器本身。

### 6.1 SQLite 的 WAL 与并发写

不设置 `SQL_DSN` 时，本系统用 SQLite，默认 DSN 已经带上两个参数：

~~~text
one-api.db?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)
~~~

- `busy_timeout(30000)`：多个写请求撞车时排队等 30 秒，而不是直接报 `database is locked`。
- `journal_mode(WAL)`：读不阻塞写、写不阻塞读，中转高并发下更稳。

注意三件事：

1. 只有**默认路径**才有这两个参数。显式设置 `SQLITE_PATH` 会替换整串 DSN，想要 WAL 就得自己把参数写回去，例如 `SQLITE_PATH=/data/one-api.db?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)`。
2. WAL 要求数据库文件在**本地文件系统**上。放在 NFS、网络盘或部分容器挂载卷上会损坏数据库，这种情况请改用 `SQL_DSN` 连 MySQL 或 PostgreSQL。
3. 开启 WAL 后，数据库目录里会多出 `one-api.db-wal`、`one-api.db-shm` 两个附属文件。备份时必须和主文件一起拷、一起删，只拷 `.db` 会丢最近一段写入。

## 7. 反向代理和 HTTPS

生产环境建议只开放 80/443，由 Nginx、Caddy 或 Traefik 转发到 New API 的 3000 端口。数据库和 Redis 不需要对公网开放。

Nginx 示例：

~~~nginx
server {
    listen 443 ssl http2;
    server_name api.example.com;

    client_max_body_size 100m;
    proxy_read_timeout 600s;
    proxy_send_timeout 600s;

    location / {
        proxy_pass http://127.0.0.1:6070;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
    }
}
~~~

配置公网域名后，如果使用本地素材或异步任务，还应设置：

~~~env
MATERIAL_PUBLIC_BASE_URL=https://api.example.com
~~~

### 7.1 配置微信支付 API v3 Native 充值

微信支付官方渠道使用普通商户的 Native 支付，不依赖 Epay。启用前需要在微信支付商户平台准备：

- 公众号或开放平台的 AppID，以及已经完成绑定的微信支付商户号。
- 商户 API 证书文件 `apiclient_cert.pem`。
- 与证书匹配的商户私钥文件 `apiclient_key.pem`。
- 32 字节的 APIv3 Key（这是文本，不是文件）。
- 微信支付公钥 ID（以 `PUB_KEY_ID_` 开头）和对应的 `pub_key.pem` 文件。

管理员进入系统设置中的支付设置，按“商户身份 → 商户证书 → APIv3 与微信支付公钥 → 验证并启用”的顺序填写。后端只保存文件内容的 AES-256-GCM 密文及证书序列号、指纹等非敏感元数据；接口不会回显私钥、APIv3 Key 或公钥原文。

部署时必须显式设置至少 32 个字符的固定 `CRYPTO_SECRET`。应像数据库密码一样备份它，并确保所有实例完全一致。不要在已经保存微信支付凭据后更换或丢失该值，否则历史密文无法解密，需要重新上传全部凭据。

微信支付通知地址固定为：

~~~text
https://你的域名/api/wechat-pay/notify
~~~

系统根据回调地址或站点地址生成该 URL，并要求公网 HTTPS。反向代理必须原样转发请求体和 `Wechatpay-*` 请求头，不要给该路径增加登录、验证码、页面跳转或响应体改写。通知处理会先限制请求体大小，再使用微信支付公钥验签和 APIv3 Key 解密；只有商户号、AppID、币种、金额和订单号全部匹配时才会结算。

保存配置后，先运行页面上的 ¥0.01 链路测试并扫码支付。测试单使用独立订单表，只将配置标记为已验证，绝不会给任何用户增加余额。测试成功后再打开“启用微信支付”；官方渠道启用时，充值页默认隐藏 Epay 的 `wxpay` 选项，管理员可在高级开关中选择同时展示。

当前版本不自动处理退款后的余额回收。在微信支付商户平台手动退款后，管理员还需要核对用户消耗情况并人工调整余额；不要直接把退款通知当作充值成功通知处理。

## 8. 日常运维

查看状态：

~~~bash
docker compose ps
docker stats
docker compose logs --tail=200 new-api
~~~

重启应用：

~~~bash
docker compose restart new-api
~~~

停止服务但保留卷：

~~~bash
docker compose down
~~~

不要在排查问题时直接使用 docker compose down -v，该命令会删除 Compose 管理的数据库卷。

### 8.1 更新 ACR 镜像

~~~bash
cd /opt/new-api
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker compose pull
docker compose up -d
docker image prune -f
~~~

更新前建议先备份数据库。应用版本更新必须先在开发机或 CI 构建并推送 ACR，再在服务器执行上述命令；不要回写或更新旧外部镜像仓库。

### 8.2 从源码发布新版本到 ACR

~~~bash
git pull
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker build --platform linux/amd64 \
  -t crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest .
docker push crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/ap-new-api-community:latest
~~~

如果需要回滚，切换到上一个 Git 提交或镜像标签，再重新启动。

### 8.3 渠道故障告警

本部署**不自动禁用渠道**：全局开关 `AutomaticDisableChannelEnabled` 默认关闭，且 AIPDD 渠道的 `auto_ban` 已置为 `0`。原因是只有单条 AIPDD 渠道，一旦被自动禁用，`default` 分组下没有任何备份渠道，整站会直接不可用；恢复还依赖人工或渠道测试任务。

代价是坏渠道不会被系统摘掉，因此 NewAPI 增加了独立于该开关的告警：当某次上游错误已达到「本应自动禁用」的标准（默认命中 401、`channel:` 类错误、或命中禁用关键字）却没有禁用渠道时，会写一条系统日志并向 root 用户发送通知，同一渠道 10 分钟内只通知一次。

告警要有出口才会送达：root 用户需要配置通知方式（邮箱需同时配置 SMTP，或在用户设置里填 Webhook / Bark / Gotify）。未配置时只会落到服务器日志，形如：

~~~text
[SYS] 通道「AIPDD」（#1）错误已达禁用标准，但未自动禁用，原因：status_code=401, invalid api key
~~~

如果日后新增第二条渠道做互备，可以重新打开自动禁用；届时建议同时确认 `AutomaticDisableStatusCodeRanges` 是否要包含 5xx——默认只含 401，上游 503（号池耗尽）不会触发禁用。

## 9. 备份和恢复

### 9.1 PostgreSQL

在 Compose 项目目录执行：

~~~bash
mkdir -p backups
docker compose exec -T postgres pg_dump -U root new-api > backups/new-api-$(date +%F-%H%M).sql
~~~

恢复前先停止 New API 写入：

~~~bash
docker compose stop new-api
docker compose exec -T postgres psql -U root -d new-api < backups/new-api-YYYY-MM-DD-HHMM.sql
docker compose start new-api
~~~

### 9.2 SQLite

SQLite 数据位于宿主机挂载的 data 目录。建议停机后复制整个目录：

~~~bash
docker stop new-api
tar -czf new-api-data-$(date +%F-%H%M).tar.gz -C /opt/new-api data
docker start new-api
~~~

Redis 主要用于缓存、限流和任务协作，不是业务数据库的替代品。备份重点是 PostgreSQL 或 SQLite，以及 /data 中的素材文件。

## 10. 常见问题

### 10.1 set POSTGRES_PASSWORD 或 set REDIS_PASSWORD

这是 Compose 变量校验失败，通常是 .env 不在 docker-compose.yml 同一目录、变量名拼写错误、执行目录不正确或变量为空。

~~~bash
pwd
ls -la .env docker-compose.yml
docker compose config
~~~

### 10.2 日志提示 an AIPDD API key is required

只有显式要求密钥的部署（`AIPDD_BOOTSTRAP_REQUIRED=true`）才会在缺少密钥时报错，实际日志文案是：

~~~text
an AIPDD API key is required; enter it in the setup wizard, save it on the AIPDD channel, or set AIPDD_API_KEY before starting the container so AIPDD models can be bootstrapped
~~~

处理方式（按密钥的解析顺序，数据库优先）：

1. 已完成首次设置的话，直接在名为 AIPDD 的渠道里保存密钥，或在首次设置向导里填写——保存即生效，不必重启容器。
2. 首次启动前就想预置，则在 .env 中填写 AIPDD_API_KEY，再执行 docker compose up -d。
3. 如果不使用 AIPDD，设置 AIPDD_BOOTSTRAP_REQUIRED=false 和 AIPDD_CATALOG_SYNC_ON_BOOT=false。

向导成功建库、建管理员之后，如果 AIPDD 模型目录同步或 AIPDD 渠道创建失败，**不会**把这次初始化判为失败——系统会照常可用，只是提示「初始化完成，但有警告」，并把失败项（`aipdd_catalog_sync` / `aipdd_channel_setup`）和一段已脱敏的原因显示出来。出现这种提示时，管理员补好密钥后在 AIPDD 渠道点「同步上游定价」即可，不必重新初始化；后台管理员账号、登录和已有数据都不受影响。

### 10.3 端口 3000 已被占用

检查端口：

~~~bash
sudo ss -lntp | grep ':6070'
~~~

可以修改宿主机端口映射，例如：

~~~yaml
ports:
  - "8080:6070"
~~~

容器内部仍然监听 3000，访问地址改为 http://服务器IP:8080。

### 10.4 容器重建后数据消失

确认 SQLite 是否映射 ./data:/data、PostgreSQL 是否保留 pg_data 卷，以及是否误执行了 docker compose down -v。

~~~bash
docker volume ls
docker compose config --volumes
~~~

### 10.5 登录后频繁失效

确认所有实例使用相同的 SESSION_SECRET 和 CRYPTO_SECRET，并检查反向代理是否正确传递 Host、X-Forwarded-Proto 等请求头。

### 10.6 流式响应中断或异步任务超时

优先检查反向代理读取超时、STREAMING_TIMEOUT、容器 CPU/内存，以及上游服务是否仍在处理任务。

## 11. 上线前检查清单

- [ ] Docker 和 Docker Compose 版本满足要求。
- [ ] SESSION_SECRET 和 CRYPTO_SECRET 已设置为固定随机值。
- [ ] PostgreSQL / SQLite 数据已挂载或使用持久化卷。
- [ ] Redis 和数据库密码已修改，且没有暴露到公网。
- [ ] AIPDD 密钥已配置（AIPDD 渠道、首次设置向导或 AIPDD_API_KEY 任一即可），或已明确关闭 AIPDD 引导。
- [ ] 已通过 /api/status 检查应用健康状态。
- [ ] 已完成首次管理员初始化。
- [ ] 生产环境已配置 HTTPS 和反向代理。
- [ ] 已配置定期数据库和素材备份。
- [ ] 已用普通用户 API Key 完成一次真实请求或任务测试。

## 12. 相关文件

- [Dockerfile](../Dockerfile)：生产镜像构建文件。
- [docker-compose.yml](../docker-compose.yml)：源码构建 + PostgreSQL + Redis Compose 模板。
- [docker-compose.dev.yml](../docker-compose.dev.yml)：前端开发和后端本地构建环境。
- [环境变量示例](../.env.example)：项目环境变量参考。
- [管理员用户手册](user-guide/admin-user-manual.zh_CN.md)：后台渠道、模型和价格配置。
- [AIPDD 部署补充说明](dockerhub-aipdd-deploy.zh_CN.md)：ACR 镜像和 AIPDD 自动同步细节。
