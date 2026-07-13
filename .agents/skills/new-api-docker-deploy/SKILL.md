---
name: new-api-docker-deploy
description: Deploy and update New API on a pre-provisioned Docker server over SSH using Alibaba Cloud ACR images for the application, PostgreSQL, and Redis with Docker Compose. Use when the user provides server SSH credentials and an AIPDD API key, asks for a first deployment or an update, optionally sets New API's own ServerAddress to a supplied domain without configuring a reverse proxy or HTTPS, or asks to reset or synchronize the AIPDD channel. Keep first-deployment initialization separate from updates; generate credentials and create the root account only on first deployment; updates preserve .env, databases, users, and channels, pull the ACR image, and never publish updated application images to Docker Hub.
---

# New API Docker 自动部署（阿里云 ACR）

Use this skill to deploy or update the project on a server that already has Docker and Docker Compose. The default deployment is Docker Compose + PostgreSQL 15 + Redis, using the public Alibaba Cloud ACR images below. ACR is the source of truth for runtime images; do not pull the application, PostgreSQL, or Redis images from Docker Hub during deployment. Do not clone the repository or build the image on the deployment server.

Public ACR image addresses:

- Application: `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest`
- PostgreSQL: `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15`
- Redis: `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest`

## 镜像发布与升级规则

When the project version changes, update the application image in ACR before running a remote update. Do not push a new version to the legacy DockerHub application repository or use Docker Hub as the deployment source. Build and publish from a trusted development or CI environment:

```bash
docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker build --platform linux/amd64 \
  -t crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest .
docker push crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
```

If the PostgreSQL or Redis dependency version changes, mirror the exact required tag into the corresponding ACR repository and update the Compose file before deployment. The remote deployment must only run `docker compose pull` and `docker compose up -d` against the ACR addresses above; do not use `--build` on the deployment server. If the ACR repository is private, authenticate on the deployment server with a least-privilege registry account; public anonymous pull may be used only when explicitly enabled in ACR.

## Scope and defaults

Apply these defaults unless the user explicitly changes them:

- Docker image: `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest`
- Deployment directory: `/opt/new-api`
- Public application port: `6070`
- PostgreSQL image: `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15`
- Redis image: `crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest`
- PostgreSQL database/user: `new-api` / `root`
- Admin username: `root`
- Time zone: `Asia/Shanghai`
- Domain setting: do not change unless the user supplies a domain
- Reverse proxy, HTTPS, certificates, DNS, and firewall configuration: always out of scope

## Select exactly one deployment mode

Determine the mode before generating files or secrets. If the user did not specify the mode, inspect the deployment directory and ask:

- **First deployment / initialization**: use only when the target has no complete New API deployment. Generate all secrets, create `.env` and Compose files, create the PostgreSQL/Redis data stores, call `/api/setup`, and create the initial `root` administrator.
- **Update**: use only when the target already has a working New API deployment. Preserve the existing `.env`, PostgreSQL and Redis volumes, users, channels, administrator credentials, domain setting, and AIPDD key. Pull the ACR application image and recreate only the application container. Never call `/api/setup`, generate replacement secrets, delete data, or reset the administrator during an update.

Treat a directory containing only some deployment files or data as a partial/unknown state. Stop and ask before changing it; do not classify it as a first deployment automatically.

Treat the following as required inputs. Ask for any missing value before connecting:

1. Server host or IP address.
2. SSH username.
3. SSH password. Never put it in a command, URL, temporary file, or log.
4. AIPDD upstream API key for first deployment. During an update, reuse the key already stored in the remote `.env`; ask for a new key only when the user explicitly requests an AIPDD key change or the existing deployment has no usable key. Keep it masked and do not echo it in the final report.

Optionally ask for SSH port, deployment directory, public port, and the New API application domain. Use port `22`, `/opt/new-api`, and `6070` when omitted. A supplied domain means only setting New API’s `ServerAddress` option; it does not authorize a reverse proxy, HTTPS, certificate, DNS, or firewall change. Before any destructive AIPDD action, ask exactly whether to force-overwrite AIPDD channels: “是否强制覆盖 AIPDD 渠道？这会删除现有 AIPDD 渠道并按当前 AIPDD API Key 重建。” Never infer a yes from a general request to deploy.

When a domain is supplied, validate it as a hostname and ask the user to ensure its A/AAAA record points to this server. Do not edit DNS. With no reverse proxy, the public URL must include the application port: `http://<domain>:6070`. Accept a bare hostname or an `http://` URL; reject an `https://` value unless the user explicitly understands that TLS is not being configured by this skill.

## Secret policy

On first deployment only, generate these values locally with a cryptographically secure generator, for example `openssl rand -hex 32` or Python `secrets.token_urlsafe(32)`:

- `POSTGRES_PASSWORD`
- `REDIS_PASSWORD`
- `SESSION_SECRET`
- `CRYPTO_SECRET`
- the initial admin password

Generate each value independently. Do not use timestamps, usernames, project names, `12345678`, or one secret for multiple purposes. Preserve the generated admin password and all four infrastructure secrets until the final handoff. During an update, never regenerate or rotate any of these values unless the user explicitly requests a separate secret-rotation operation.

Keep `AIPDD_API_KEY` only in protected secret material. Do not place any secret in an SSH command argument, command URL, shell history, ordinary progress message, `docker compose config` output, or unredacted logs. Use a protected temporary file or an interactive transfer, set remote `.env` permissions to `600`, and delete local temporary secret files after transfer. Do not print the contents of `.env`.

At the end of a first deployment, output a clearly labeled credential block containing the generated values and the admin username. Output the AIPDD key as `已配置（不回显）`; only display it if the user explicitly asks for the sensitive value to be shown. For an update, do not output or regenerate credentials; state that existing credentials were preserved.

## Deployment workflow

### 1. Establish a safe SSH session

Use an SSH-capable connector or a PTY-backed `ssh`/`scp` session so the password is entered interactively. Do not use `sshpass`, inline passwords, or a command such as `ssh user:password@host`. If the host-key fingerprint is unknown, show it and ask the user to confirm before accepting it.

Run these read-only preflight checks on the server:

```sh
docker --version
docker compose version
docker info
df -h
ss -ltn 2>/dev/null | grep ':6070 ' || true
```

Stop and report if Docker or `docker compose` is unavailable, the daemon is unhealthy, disk space is critically low, or port `6070` is occupied. Do not silently choose another port.

### 2. Detect and protect an existing deployment

Check whether the target directory already contains `docker-compose.yml`, `.env`, `data/`, or a running `new-api` container. If it does, treat it as an existing deployment:

- Select **Update** only when the user explicitly intends an update/redeploy.
- Never overwrite the existing `.env` or database by default.
- Before an upgrade, copy the existing compose file and `.env` into a timestamped `backups/` directory with restrictive permissions.
- Never run `docker compose down -v`, delete `data/`, delete the PostgreSQL volume, or remove unrelated containers.

For a new deployment, create the directory and data paths:

```sh
mkdir -p /opt/new-api/data /opt/new-api/logs /opt/new-api/backups
chmod 700 /opt/new-api /opt/new-api/data /opt/new-api/logs /opt/new-api/backups
```

Substitute the user-selected deployment directory when it is not `/opt/new-api`.

### 2A. Update path

Use this path only after the remote inspection confirms a complete deployment and the user selected **Update**. Do not execute the first-deployment sections below.

Before pulling the new image, ask two independent questions and record the answers:

1. **是否覆盖 AIPDD 渠道？** Default to **否**. **是** means delete all existing AIPDD channels and rebuild the managed AIPDD channel from the current AIPDD key; this is destructive and requires the existing administrator login.
2. **是否覆盖 AIPDD 模型价格？** Default to **否**. **是** means enable the AIPDD catalog sync so upstream model catalog and AIPDD-authoritative pricing are applied; it may add, remove, or update AIPDD catalog models and prices, but must not delete non-AIPDD channels. **否** means preserve current AIPDD model prices and do not call the catalog sync endpoint.

Do not infer either answer from the request to update the application. If the user does not explicitly answer, stop before changing the deployment.

1. Back up the existing Compose file and `.env` to the protected timestamped backup directory. Do not print either file.
2. Read the current Compose file without exposing secrets. Confirm that the application image points to the expected ACR repository. If it points elsewhere, show the image name and ask before changing it.
3. If the ACR repository is private, authenticate with the user-provided least-privilege registry account interactively. Never put registry credentials in the Compose file or command arguments.
4. Preserve every existing `.env` value except these two explicit update decisions:

   ```dotenv
   AIPDD_CATALOG_SYNC_ON_BOOT=<true when price overwrite is confirmed, otherwise false>
   AIPDD_CHANNEL_OVERWRITE_ON_BOOT=<true only for the approved channel rebuild, otherwise false>
   ```

   If the Compose file currently hard-codes either setting, change it to read the corresponding `.env` variable. Keep the backup and never print the resulting secret file.
5. Pull and recreate only the application service:

   ```sh
   docker compose pull new-api
   docker compose up -d --no-build --no-deps new-api
   docker compose ps
   ```

6. Do not run `/api/setup`, do not generate new passwords, and do not change PostgreSQL or Redis services unless the user explicitly requested a dependency upgrade. If a dependency upgrade is requested, back up first and handle it as a separate approved change.
7. Confirm `/api/status` and the application logs. Preserve the existing administrator password and all existing application data.
8. If the user requests an application-domain change or AIPDD channel overwrite during the update, use the authenticated existing administrator account and the separate procedures below. Do not treat an image update as permission to perform either operation.

### 3. First deployment: generate and transfer deployment files

Create the following compose file, substituting the selected directory and public port. Keep passwords in `.env`; do not interpolate literal secret values into the compose file.

```yaml
services:
  new-api:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/new-api-aipdd:latest
    container_name: new-api
    restart: unless-stopped
    command: --log-dir /app/logs
    ports:
      - "6070:3000"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    environment:
      SQL_DSN: postgresql://root:${POSTGRES_PASSWORD}@postgres:5432/new-api
      REDIS_CONN_STRING: redis://:${REDIS_PASSWORD}@redis:6379
      SESSION_SECRET: ${SESSION_SECRET}
      CRYPTO_SECRET: ${CRYPTO_SECRET}
      AIPDD_API_KEY: ${AIPDD_API_KEY}
      AIPDD_BOOTSTRAP_REQUIRED: "true"
      AIPDD_CATALOG_SYNC_ON_BOOT: ${AIPDD_CATALOG_SYNC_ON_BOOT}
      AIPDD_CHANNEL_OVERWRITE_ON_BOOT: ${AIPDD_CHANNEL_OVERWRITE_ON_BOOT}
      ERROR_LOG_ENABLED: "true"
      BATCH_UPDATE_ENABLED: "true"
      NODE_NAME: new-api-node-1
      TZ: Asia/Shanghai
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started

  postgres:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/postgres:15
    container_name: new-api-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: root
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: new-api
      TZ: Asia/Shanghai
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U root -d new-api"]
      interval: 5s
      timeout: 5s
      retries: 20

  redis:
    image: crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com/aipdd/redis:latest
    container_name: new-api-redis
    restart: unless-stopped
    command: ["redis-server", "--appendonly", "yes", "--requirepass", "${REDIS_PASSWORD}"]
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

Use a `.env` file with this shape and generated values:

```dotenv
POSTGRES_PASSWORD=<generated>
REDIS_PASSWORD=<generated>
SESSION_SECRET=<generated>
CRYPTO_SECRET=<generated>
AIPDD_API_KEY=<user-provided>
AIPDD_CATALOG_SYNC_ON_BOOT=true
AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false
```

Set `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=true` only when the user explicitly confirmed force overwrite. Transfer both files over the protected SSH session, then run `chmod 600 .env` remotely. Do not use `docker compose config` after secrets are present because it expands them.

### 4. First deployment: pull and start the ACR deployment

Run from the deployment directory:

```sh
# Required only when the ACR repository is private.
# docker login crpi-3iiuxr617jsmyl60.cn-hangzhou.personal.cr.aliyuncs.com
docker compose pull
docker compose up -d
docker compose ps
```

Poll for a bounded period, such as 120 seconds. Confirm that PostgreSQL is healthy, Redis is running, and the application responds locally:

```sh
curl -fsS --max-time 10 http://127.0.0.1:6070/api/status
```

If pulling fails, report the image/tag and Docker error. If the app fails, inspect only redacted or tail log output; never expose environment variables or API keys.

### 5. First deployment: initialize the root administrator

On a fresh database, call `POST /api/setup` with the generated admin password. Use a protected temporary JSON file or an equivalent mechanism that does not put the password in shell history or command output:

```json
{
  "username": "root",
  "password": "<generated-admin-password>",
  "confirmPassword": "<generated-admin-password>",
  "SelfUseModeEnabled": false,
  "DemoSiteEnabled": false
}
```

Call the endpoint against `http://127.0.0.1:6070/api/setup`, check that the response reports success, then delete the temporary request file. If setup reports that an administrator already exists, do not reset that account; ask whether the user wants to keep the existing administrator and report that the generated password is not valid for it.

Log in with `POST /api/user/login` using a cookie jar held in a protected temporary file. Check the response before making any admin channel request. If 2FA or a secure-verification challenge is required, stop and ask the user; do not bypass it.

If the user supplied an application domain, set New API’s own domain setting after login. Do not edit Compose, add a proxy, or configure TLS. Normalize a bare domain such as `api.example.com` to `http://api.example.com:6070`; preserve an explicit `http://` URL only when it points to the selected public port. Use the authenticated root cookie jar to call. On an update, perform this same operation only when the user explicitly requested a domain change and has supplied valid existing administrator credentials:

```http
PUT /api/option/
Content-Type: application/json

{"key":"ServerAddress","value":"http://api.example.com:6070"}
```

Call `GET /api/option/` afterward and verify that the `ServerAddress` option matches the normalized URL. This is a database-backed New API setting and survives container restarts. Do not set `MATERIAL_PUBLIC_BASE_URL` unless the user separately asks to configure public material URLs.

### 6. Handle the AIPDD channel choice in either mode

When force overwrite is **not** confirmed:

- Do not delete or modify existing AIPDD channels.
- On first deployment, leave `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false`.
- On update, set `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false` for this update and preserve all other `.env` values.
- Verify the existing channel state with the authenticated admin API and report if no usable AIPDD channel exists.

In update mode, when price overwrite is **not** confirmed, set `AIPDD_CATALOG_SYNC_ON_BOOT=false`, do not call `POST /api/channel/<id>/aipdd/sync`, and report that current AIPDD prices were preserved. When price overwrite **is** confirmed, set it to `true`, confirm that catalog synchronization completed, and report the returned `updated_prices` count without printing pricing secrets.

When force overwrite **is** confirmed, delete and rebuild only AIPDD channels:

1. Use the authenticated cookie jar to request `GET /api/channel/?p=1&page_size=100&type=58`. `58` is the project’s `ChannelTypeAIPDD`.
2. Record only the channel IDs and names needed for the operation; never request or print channel keys.
3. Ask for a final confirmation immediately before the first deletion if the user’s confirmation was not explicit for this exact action.
4. Send `DELETE /api/channel/<id>` for every returned AIPDD channel. Do not delete other channel types, disabled non-AIPDD channels, database volumes, or application data.
5. On first deployment, ensure `.env` contains `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=true`. On update, back up `.env`, change only this setting temporarily or as explicitly approved, then run `docker compose restart new-api`.
6. Poll `/api/status`, then query the AIPDD channel list again. Confirm that the startup bootstrap created a fresh managed AIPDD channel. If price overwrite was confirmed, also confirm catalog synchronization succeeded; if it was declined, confirm that catalog synchronization was not run and existing prices were preserved.

After a successful channel rebuild, set `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false` for future restarts and preserve the user’s separate price-sync choice. Do not leave a one-time destructive overwrite enabled by accident.

The environment toggle alone is insufficient: it updates one existing AIPDD channel but does not clear additional AIPDD channels. If any AIPDD deletion fails, stop the destructive workflow, preserve the remaining data, and report the channel ID and error without claiming that overwrite completed.

### 7. Verify and hand off

Verify all of the following:

- `docker compose ps` shows the three services running; PostgreSQL is healthy.
- `GET /api/status` succeeds from the server.
- When an application domain was configured, `http://<domain>:6070/api/status` succeeds and the `ServerAddress` option matches; do not claim HTTPS. Otherwise report that the port-only mode is active.
- Application logs do not contain a missing `AIPDD_API_KEY` or database/Redis connection error.
- In first-deployment mode, the root administrator was created on the new database. In update mode, `/api/setup` was not called and the existing administrator was preserved.
- The requested AIPDD overwrite result matches the user’s answer.
- In update mode, report both decisions separately: channel overwrite `是/否` and AIPDD price overwrite `是/否`.

Do not claim success if health checks, admin initialization, or requested AIPDD synchronization failed. State which stage failed and preserve the deployment directory for diagnosis.

## Final response format

After successful first deployment, provide the server address, deployment directory, service status, AIPDD overwrite result, and this block. Use `http://<domain>:6070` when the New API application domain was configured; otherwise use `http://<server>:6070`. Do not claim HTTPS. Do not include the AIPDD API key:

```text
=== New API 部署凭据（请立即保存）===
管理员用户名：root
管理员密码：<generated-admin-password>
POSTGRES_PASSWORD：<generated>
REDIS_PASSWORD：<generated>
SESSION_SECRET：<generated>
CRYPTO_SECRET：<generated>
AIPDD_API_KEY：已配置（不回显）
====================================
```

Warn the user that these values are shown once and should be stored in a password manager. Never paste `.env` or raw Docker logs into the final response.

For an update, do not print or regenerate credentials. Report that `.env`, the database, the root password, and existing data were preserved, and include the separate results for channel overwrite and AIPDD price overwrite.
