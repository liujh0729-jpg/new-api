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

如果使用 AIPDD 内置任务模型，还需要准备 AIPDD_API_KEY。容器启动后会根据该 Key 自动创建或同步名为 AIPDD 的渠道和模型目录。

## 3. 阿里云 ACR 镜像部署（推荐）

当前使用的三个 ACR 公网镜像地址：

~~~text
crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
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
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
    container_name: new-api
    restart: unless-stopped
    command: --log-dir /app/logs
    ports:
      - "3000:3000"
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
      test: ["CMD-SHELL", "wget -q -O - http://localhost:3000/api/status | grep -o '\"success\":\\s*true' || exit 1"]
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
curl http://127.0.0.1:3000/api/status
~~~

看到成功响应后，用浏览器访问 http://服务器IP:3000。首次访问会进入初始化向导，按页面提示创建管理员账号。

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
  -t crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest .
docker push crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
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
docker pull crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest

docker run -d \
  --name new-api \
  --restart unless-stopped \
  -p 3000:3000 \
  -v /opt/new-api/data:/data \
  -v /opt/new-api/logs:/app/logs \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET='请替换为固定随机字符串' \
  -e CRYPTO_SECRET='请替换为固定随机字符串' \
  -e AIPDD_API_KEY='请替换为AIPDD上游Key' \
  crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
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
| SQLITE_PATH | SQLite 数据库路径；容器部署时建议放在 /data 下 |
| REDIS_CONN_STRING | Redis 连接字符串；多实例、共享缓存和任务轮询建议启用 |
| SESSION_SECRET | 会话密钥；生产环境必须固定 |
| CRYPTO_SECRET | 加密密钥；使用 Redis 或多实例时必须固定并保持一致 |
| AIPDD_API_KEY | AIPDD 上游 API Key；使用 AIPDD 内置任务模型时必填 |
| AIPDD_BASE_URL | AIPDD 上游地址，默认 https://api.aipdd.work |
| AIPDD_CATALOG_SYNC_ON_BOOT | 是否在启动时同步 AIPDD 模型目录 |
| AIPDD_CHANNEL_OVERWRITE_ON_BOOT | 是否允许启动同步覆盖 AIPDD 渠道配置；手动维护渠道时可设为 false |
| MATERIAL_PUBLIC_BASE_URL | 本地素材的公网访问地址；异步任务或文件上传时按部署域名配置 |
| ERROR_LOG_ENABLED | 是否记录错误日志 |
| BATCH_UPDATE_ENABLED | 是否启用批量更新 |
| NODE_NAME | 节点名称；多容器部署时用于审计日志识别 |
| STREAMING_TIMEOUT | 流式请求无响应超时时间，单位秒 |

PostgreSQL Compose 网络内的连接示例：

~~~text
postgresql://root:数据库密码@postgres:5432/new-api
~~~

外部 MySQL 连接示例：

~~~text
root:数据库密码@tcp(mysql.example.com:3306)/new-api?parseTime=true
~~~

不要把 localhost 写进 New API 容器的外部数据库配置；在容器内，localhost 指向 New API 容器本身。

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
        proxy_pass http://127.0.0.1:3000;
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
  -t crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest .
docker push crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
~~~

如果需要回滚，切换到上一个 Git 提交或镜像标签，再重新启动。

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

### 10.2 日志提示 AIPDD_API_KEY is required

当前镜像启用了 AIPDD 引导检查。处理方式：

1. 在 .env 中填写有效的 AIPDD_API_KEY，然后执行 docker compose up -d。
2. 如果不使用 AIPDD，设置 AIPDD_BOOTSTRAP_REQUIRED=false 和 AIPDD_CATALOG_SYNC_ON_BOOT=false。

### 10.3 端口 3000 已被占用

检查端口：

~~~bash
sudo ss -lntp | grep ':3000'
~~~

可以修改宿主机端口映射，例如：

~~~yaml
ports:
  - "8080:3000"
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
- [ ] AIPDD_API_KEY 已配置，或已明确关闭 AIPDD 引导。
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
