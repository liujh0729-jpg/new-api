---
name: new-api-docker-deploy
description: Deploy and update New API on a pre-provisioned Docker server over SSH using Alibaba Cloud ACR images, Docker Compose, PostgreSQL, and Redis, while preserving the manually registered AIPDD delivery-site identity and archiving deployment runs. Use when the user asks for a first deployment or update, supplies a site-scoped AIPDD API key and registered externalInstanceId, optionally changes New API's ServerAddress without configuring reverse proxy or HTTPS, resets the AIPDD channel, reconciles authenticated AIPDD prices, or synchronizes VIP1-VIP5 group pricing. Keep initialization separate from updates; require one immutable AIPDD_INSTANCE_ID across key rotations, generate credentials and root only on first deployment, preserve data and secrets during updates, and never publish application images to Docker Hub.
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
- AIPDD periodic catalog sync: keep `AIPDD_CATALOG_SYNC_INTERVAL_MINUTES=0` during deployment-managed price preservation or full reconciliation. Enable a positive interval only when the user explicitly accepts New API's built-in **partial** price synchronization behavior.

## Select exactly one deployment mode

Determine the mode before generating files or secrets. If the user did not specify the mode, inspect the deployment directory and ask:

- **First deployment / initialization**: use only when the target has no complete New API deployment. Generate all secrets, create `.env` and Compose files, create the PostgreSQL/Redis data stores, call `/api/setup`, and create the initial `root` administrator.
- **Update**: use only when the target already has a working New API deployment. Preserve the existing `.env`, PostgreSQL and Redis volumes, users, channels, administrator credentials, domain setting, and AIPDD key. Pull the ACR application image and recreate only the application container. Never call `/api/setup`, generate replacement secrets, delete data, or reset the administrator during an update.

Treat a directory containing only some deployment files or data as a partial/unknown state. Stop and ask before changing it; do not classify it as a first deployment automatically.

Treat the following as required inputs. Ask for any missing value before connecting:

1. Server host or IP address.
2. SSH username.
3. SSH password. Never put it in a command, URL, temporary file, or log.
4. A site-scoped AIPDD API key created by an administrator from the manually registered delivery site. A normal unbound user Key is invalid for NewAPI runtime traffic. During an update, reuse the Key in remote `.env`; ask for a replacement only when explicitly rotating it or when it is unusable. A replacement Key must belong to the same site.
5. The site's registered `externalInstanceId` UUID for first deployment. During an update, recover it from `<deployment-dir>/.aipdd-instance-id` and `AIPDD_INSTANCE_ID`; if both exist they must match. Accept `DEPLOY_INSTANCE_ID` only as an explicit recovery value for a legacy deployment, and require it to equal the site's registered UUID.

Optionally ask for SSH port, deployment directory, public port, and the New API application domain. Use port `22`, `/opt/new-api`, and `6070` when omitted. A supplied domain means only setting New API’s `ServerAddress` option; it does not authorize a reverse proxy, HTTPS, certificate, DNS, or firewall change. Ask these three decisions independently in either deployment mode and never infer any `yes` from a general request to deploy:

1. Before any destructive AIPDD action, ask exactly: “是否强制覆盖 AIPDD 渠道？这会删除现有 AIPDD 渠道并按当前 AIPDD API Key 重建。”
2. Ask exactly: “是否覆盖 AIPDD 模型价格？选择是会根据当前 API Key 的实时认证目录执行一次完整重建，包括 Seedance 的分辨率价格矩阵、LTX 等按秒模型、LLM 与按次模型的本地价格和计费模式；非 AIPDD 价格不变。New API 自带的目录同步只会自动改写部分 Token Market 按秒价格，不能替代这次完整重建。”
3. Ask exactly: “是否同步 VIP 分组价格和用户分组关联？选择是会把 VIP1=0.78、VIP2=0.80、VIP3=0.85、VIP4=0.90、VIP5=0.95 合并到全局分组，将这五个 VIP 从全局用户可选分组中移除，为同名用户分组建立移除 default 的专属价格规则，并把 VIP1-VIP5 追加到所有 AIPDD 渠道；不会自动修改任何现有用户的所属分组，其他分组、渠道设置和模型原价不变。”

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

## Strict AIPDD site identity

Treat the manually registered delivery site as the only runtime identity source:

- Persist its `externalInstanceId` in `<deployment-dir>/.aipdd-instance-id` with mode `600` and in `.env` as `AIPDD_INSTANCE_ID`.
- Pass `AIPDD_INSTANCE_ID` into the `new-api` container. The file, `.env`, container environment, and AIPDD site record must contain the same canonical UUID.
- Never derive the instance UUID from `AIPDD_API_KEY`, and never replace it during Key rotation. Rotating a Key changes only `AIPDD_API_KEY`.
- On an update, never invent a random UUID. If the protected file is absent, recover only from an existing matching `.env` value or an explicit `DEPLOY_INSTANCE_ID` verified against the registered site. Stop on missing or conflicting identity.
- For a first deployment, require the administrator's manual site and registration-token flow to have associated this UUID before site runtime calls. If the site is not registered and `ACTIVE`, stop and ask for that prerequisite; do not auto-create or auto-bind a site.

The deployed NewAPI image must send `X-AIPDD-Instance-ID`, `X-AIPDD-Order-ID`, and `X-AIPDD-Attempt-ID` on Chat, Shared Task, and Seedance creation requests. It should also send numeric `X-AIPDD-NewAPI-User-ID` and `X-AIPDD-NewAPI-Token-ID` for audit. A logical order keeps its order ID across retries; each actual retry gets a new attempt ID, while a network resend of the same attempt reuses it.

## 部署档案与凭据上报

Use `scripts/report_deployment.py` from a trusted local environment for every deployment. The default API base URL is `https://api.aipdd.work`; override it only with `--base-url`. Authentication comes from local `AIPDD_API_KEY` or a protected `--key-file`, never from an argument value. Payload JSON comes from stdin or a mode-`600` `--payload` file. Temporary payload/key files must be created with mode `600` before secrets are written and deleted immediately after the call. Use `--dry-run` only for a redacted, no-network preview.

The CLI surface is:

```bash
python .agents/skills/new-api-docker-deploy/scripts/report_deployment.py \
  --stage instance|credentials|deployment-start|deployment-finish \
  [--payload protected.json] [--instance-id UUID] \
  [--base-url https://api.aipdd.work] [--key-file protected-key] \
  [--timeout 15] [--dry-run]
```

`--instance-id` is required for `instance` and `credentials`; deployment stages take `deploymentId` from the payload. Exit `0` means success, `2` means local validation failed, and `3` means the network/API call failed. Never print a payload, key, HTTP response body, or dry-run preview unless the preview is redacted.

For every run, generate a new deployment UUID and one UTC `startedAt`. Resolve the registered site UUID from `<deployment-dir>/.aipdd-instance-id` before containers are started or updated:

```python
from pathlib import Path
from report_deployment import resolve_instance_id

instance_id = resolve_instance_id(Path("/opt/new-api"), create_if_missing=False)
```

The helper validates an existing UUID. Create the file on a first deployment only from the UUID associated through the manual site registration-token flow. On update, recover a missing file only from a matching `.env` or explicit registered value. Never generate a new identity during an update and never replace a valid existing instance ID.

Report in this order:

1. **Before first-deployment containers start:** verify that the site-scoped Key and registered Instance UUID belong to the same manually registered site, upsert the compatibility `instance` archive, then send `deployment-start` with `schemaVersion=1` and `run.status=running`.
2. **Before an update pulls or recreates the application:** upsert `instance`, then send `deployment-start` with `run.mode=update` and `run.status=running`. For update mode, inspect the currently running application container first and set `release.previousImageDigest` to its content digest (`sha256:...`, from `RepoDigests` or Image ID). Upstream rejects update archives without this field. Include the same `previousImageDigest` on `deployment-finish`.
3. **After a new root administrator is successfully created:** send `credentials` with `mode=initial` and only these generated/configured entries: `admin_password` with `username=root`, `postgres_password`, `redis_password`, `session_secret`, `crypto_secret`, and `ssh_password` with its SSH username. Do not include the AIPDD API key. On update, do not send credentials unless the user explicitly authorized rotation; then use `mode=update` and include only credential types changed in this run.
4. **After verification or a terminal deployment failure:** send `deployment-finish` for the same deployment UUID with one terminal status: `succeeded`, `failed`, `rolled_back`, `rollback_failed`, or `abandoned`. Include `finishedAt`, `durationMs`, all three decisions, available AIPDD results, verification, recovery, and a sanitized error summary when applicable.

The instance payload contains only `instanceLabel`, `serverIp`, `sshPort`, `sshUsername`, `sshPassword`, `domain`, `publicUrl`, and `deploymentDirectory`. Credential items contain only `type`, optional `username`, and `secret`. Deployment payloads must use exactly the DTO fields accepted by `NewApiDeploymentDtos.UpsertRequest`: top-level `schemaVersion`, `deploymentId`, `instance`, `run`, `release`, `decisions`, `aipdd`, `verification`, `recovery`, and `error`, with only their documented nested DTO fields. Unknown fields are rejected. Keep password fields only in protected payload material.

Reporting is best-effort and is not part of New API deployment health. A reporting failure must not turn an otherwise healthy deployment into a deployment failure or trigger rollback. Record every failed reporting stage, continue safe deployment work, and make the final handoff explicitly say that the deployment archive and/or credentials were not recorded and name each failed stage. Do not claim an archive exists unless its API call succeeded.

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

Before generating Compose files or starting containers, resolve or create the protected `.aipdd-instance-id`, upsert the instance record, and submit `deployment-start` as specified in **部署档案与凭据上报**. If either report fails, record the failed stage and continue the deployment; do not weaken any deployment safety check.

### 2A. Update path

Use this path only after the remote inspection confirms a complete deployment and the user selected **Update**. Do not execute the first-deployment sections below.

Before pulling the new image, ask three independent questions and record the answers:

1. **是否覆盖 AIPDD 渠道？** Default to **否**. **是** means delete all existing AIPDD channels and rebuild the managed AIPDD channel from the current AIPDD key; this is destructive and requires the existing administrator login.
2. **是否覆盖 AIPDD 模型价格？** Default to **否**. **是** authorizes the complete price-reconciliation procedure below: fetch the live authenticated catalog, replace pricing entries belonging to the previous and current AIPDD model sets, and automatically write `ModelPrice`, `billing_setting.billing_expr`, `billing_setting.task_pricing`, and `billing_setting.billing_mode` while preserving non-AIPDD entries. Seedance pricing must be written as a `by_resolution` matrix. Require `pricingBasis=display`, platform settlement fields `displayAmountAwcoinPerSecond` / `displayVideoInputAwcoinPerSecond`, and New API sale fields `suggestedRetailAwcoinPerSecond` / `suggestedRetailVideoInputAwcoinPerSecond`; reject catalogs that omit any of them. New API `task_pricing` must be built only from the suggested-retail fields (Excel 对比原生价 / MSRP). Never use `displayAmount*`, legacy modality fields, `byokAmountAwcoinPerSecond`, or `byokVideoInputAwcoinPerSecond` as New API's sale price. Convert every AIPDD AWCoin amount with the RMB-anchored factor `AWCoin × rmbPerAwcoin ÷ site USDExchangeRate` (read `USDExchangeRate` only; do not change it) so that site display RMB equals AIPDD authoritative suggested retail. Never use catalog `usdPerAwcoin` for New API sale prices. A non-Seedance `per_unit` capability is supported only when `chargeConfig.unit=second`; it must be written as legacy-shaped flat `task_pricing` whose unit price uses the same RMB-anchored conversion. `priceVariants` and every local-price or legacy `ModelPrice` fallback are unsupported for an approved reconciliation. Also ensure display currency is RMB (`DisplayInCurrencyEnabled=true`, `general_setting.quota_display_type=CNY`). **否** preserves all current local AIPDD pricing options, including any legacy flat Seedance pricing, does not call the catalog sync endpoint, and requires the application to restore runtime capabilities read-only from the same-origin last-known-good snapshot. In either case keep `AIPDD_CATALOG_SYNC_INTERVAL_MINUTES=0` unless the user separately and explicitly accepts recurring **partial** synchronization: current New API periodically imports only eligible `token_market_media` + `per_second` display prices and does not perform this complete reconciliation. Never claim that an update migrated all pricing when overwrite was declined or when only built-in catalog sync ran.
3. **是否同步 VIP 分组价格和用户分组关联？** Default to **否**. **是** authorizes only the idempotent VIP synchronization procedure below: force the five fixed global ratios, remove VIP1-VIP5 from global `UserUsableGroups`, add `-:default` under each same-name user group in `group_ratio_setting.group_special_usable_group`, and append the five group names to every AIPDD channel while preserving all existing channel groups and every non-group channel field. It does not authorize any model-price write, channel deletion, user-record reassignment, `GroupGroupRatio`, or top-up-ratio change.

Do not infer any answer from the request to update the application. If the user does not explicitly answer all three decisions, stop before changing the deployment.

1. Resolve the protected registered instance UUID. If the file is missing, recover it only from a matching `AIPDD_INSTANCE_ID` or verified `DEPLOY_INSTANCE_ID`; never generate one. Capture the running `new-api` image digest as `release.previousImageDigest`, upsert the instance, and submit `deployment-start` before pulling or recreating containers. Generate a fresh deployment UUID for this update. Record reporting failures but do not classify them as deployment failures.
2. Back up the existing Compose file and `.env` to the protected timestamped backup directory. Do not print either file.
3. Read the current Compose file without exposing secrets. Confirm that the application image points to the expected ACR repository. If it points elsewhere, show the image name and ask before changing it.
4. If the ACR repository is private, authenticate with the user-provided least-privilege registry account interactively. Never put registry credentials in the Compose file or command arguments.
5. Preserve every existing `.env` value except the deployment-controlled AIPDD sync toggles and a missing identity entry. Require an existing `AIPDD_INSTANCE_ID` to equal `.aipdd-instance-id`; append it only when absent:

   ```dotenv
   AIPDD_CATALOG_SYNC_ON_BOOT=<true only for a required one-time first-deployment/channel bootstrap; otherwise false>
   AIPDD_CATALOG_SYNC_INTERVAL_MINUTES=<0 unless recurring partial synchronization was separately approved>
   AIPDD_CHANNEL_OVERWRITE_ON_BOOT=<true only for the approved channel rebuild, otherwise false>
   AIPDD_INSTANCE_ID=<the unchanged registered externalInstanceId UUID>
   ```

   If Compose hard-codes either toggle or the instance UUID, change it to read the corresponding `.env` variable. If the `new-api` service does not expose `AIPDD_INSTANCE_ID`, add it next to `AIPDD_API_KEY`. Keep the backup and never print the resulting secret file.
6. Pull and recreate only the application service:

   ```sh
   docker compose pull new-api
   docker compose up -d --no-build --no-deps new-api
   docker compose ps
   ```

   After recreation, require `printenv AIPDD_INSTANCE_ID` inside the application container to equal the protected file. This UUID is not a secret, but do not print the surrounding environment.

7. Do not run `/api/setup`, do not generate new passwords, and do not change PostgreSQL or Redis services unless the user explicitly requested a dependency upgrade. If a dependency upgrade is requested, back up first and handle it as a separate approved change.
8. Confirm `/api/status` and the application logs. Preserve the existing administrator password and all existing application data.
9. If the user requests an application-domain change, AIPDD channel overwrite, AIPDD price overwrite, or VIP group-price synchronization during the update, use the authenticated existing administrator account and the separate procedures below. Do not treat an image update as permission to perform any of these operations.

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
      - "6070:6070"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    environment:
      SQL_DSN: postgresql://root:${POSTGRES_PASSWORD}@postgres:5432/new-api
      REDIS_CONN_STRING: redis://:${REDIS_PASSWORD}@redis:6379
      SESSION_SECRET: ${SESSION_SECRET}
      CRYPTO_SECRET: ${CRYPTO_SECRET}
      AIPDD_API_KEY: ${AIPDD_API_KEY}
      AIPDD_INSTANCE_ID: ${AIPDD_INSTANCE_ID}
      AIPDD_BOOTSTRAP_REQUIRED: "true"
      AIPDD_CATALOG_SYNC_ON_BOOT: ${AIPDD_CATALOG_SYNC_ON_BOOT}
      AIPDD_CATALOG_SYNC_INTERVAL_MINUTES: ${AIPDD_CATALOG_SYNC_INTERVAL_MINUTES}
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
AIPDD_INSTANCE_ID=<registered delivery-site externalInstanceId UUID>
AIPDD_CATALOG_SYNC_ON_BOOT=<true only for a required one-time first-deployment/channel bootstrap; reset false after reconciliation>
AIPDD_CATALOG_SYNC_INTERVAL_MINUTES=0
AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false
```

Set `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=true` only when the user explicitly confirmed force overwrite. Transfer both files over the protected SSH session, then run `chmod 600 .env` remotely. Do not use `docker compose config` after secrets are present because it expands them.

For a first deployment, an approved AIPDD price overwrite must complete the authenticated reconciliation after the root administrator and managed AIPDD channel exist. That reconciliation must write Seedance prices as `by_resolution` and every supported `per_unit/second` capability as flat per-second `task_pricing`; a successful catalog fetch or channel bootstrap alone is not sufficient. If price overwrite is declined, report that catalog-derived local prices were not initialized and do not claim that the new duration pricing rules are active. Independently, when VIP group-price synchronization is approved, run it only after the root administrator and final managed AIPDD channel exist; if channel overwrite is also approved, rebuild the channel first and synchronize its groups afterward.

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

Immediately after successful root creation, submit the `credentials` initial report described in **部署档案与凭据上报**. Build it in a protected mode-`600` payload file or pass it through stdin, include only the six specified credential types, and delete the payload immediately afterward. A credentials-reporting failure does not invalidate successful root creation; record the `credentials` stage failure for the final handoff and retain the generated values for the one-time credential block.

Log in with `POST /api/user/login` using a cookie jar held in a protected temporary file. Check the response before making any admin channel request. If 2FA or a secure-verification challenge is required, stop and ask the user; do not bypass it.

If the user supplied an application domain, set New API’s own domain setting after login. Do not edit Compose, add a proxy, or configure TLS. Normalize a bare domain such as `api.example.com` to `http://api.example.com:6070`; preserve an explicit `http://` URL only when it points to the selected public port. Use the authenticated root cookie jar to call. On an update, perform this same operation only when the user explicitly requested a domain change and has supplied valid existing administrator credentials:

```http
PUT /api/option/
Content-Type: application/json

{"key":"ServerAddress","value":"http://api.example.com:6070"}
```

Call `GET /api/option/` afterward and verify that the `ServerAddress` option matches the normalized URL. This is a database-backed New API setting and survives container restarts. Do not set `MATERIAL_PUBLIC_BASE_URL` unless the user separately asks to configure public material URLs.

### 6. Handle the AIPDD channel, model-price, and VIP group choices in either mode

When force overwrite is **not** confirmed:

- Do not delete existing AIPDD channels. Do not modify them unless the independent VIP group-price synchronization was confirmed; that procedure may change only their `group` field.
- On first deployment, leave `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false`.
- On update, set `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false` for this update and preserve all other `.env` values.
- Verify the existing channel state with the authenticated admin API and report if no usable AIPDD channel exists.

When price overwrite is **not** confirmed, set both `AIPDD_CATALOG_SYNC_ON_BOOT=false` and `AIPDD_CATALOG_SYNC_INTERVAL_MINUTES=0`, do not call `POST /api/channel/<id>/aipdd/sync`, do not write any AIPDD model-pricing option, and report that current AIPDD prices were preserved. `AIPDD_CATALOG_SYNC_ON_BOOT=false` alone is insufficient: on the master node, a configured `AIPDD_API_KEY` still starts the background task, whose default interval is five minutes. With both sync paths disabled, the application must still activate the existing same-origin `a_ip_dd_catalog_snapshots` payload in memory without a remote request or database price write. Before updating, record the enabled AIPDD duration-model names exposed by `GET /api/pricing`. After restart, require every recorded model—especially every Seedance `by_resolution` model—to remain exposed with the same billing mode and active resolution set. If any disappears, roll back to the previous application image and do not claim success. A missing, invalid, or different-origin snapshot is a deployment blocker when the current installation contains AIPDD duration-priced models. The independently approved VIP procedure may still write only `GroupRatio`, `UserUsableGroups`, `group_ratio_setting.group_special_usable_group`, and AIPDD channel `group` fields. When price overwrite **is** confirmed, use boot sync only when a first-deployment or approved channel bootstrap requires one initial live catalog; otherwise keep it disabled and use the authenticated manual endpoint in the bounded reconciliation below. Keep periodic sync disabled throughout. Reset `AIPDD_CATALOG_SYNC_ON_BOOT=false` after a successful bootstrap/reconciliation so a later container restart cannot partially rewrite the reconciled plan. The environment toggle and the sync response's `updated_prices` field are not proof of complete price replacement: current New API automatically writes only eligible Token Market duration entries during catalog sync; Seedance, LLM, per-call, and other AIPDD sale-price rules still require the reconciliation plan below.

#### Built-in AIPDD catalog sync boundary

Treat New API's built-in catalog synchronization and this Skill's complete price reconciliation as different operations:

- Boot synchronization is controlled by `AIPDD_CATALOG_SYNC_ON_BOOT`. Periodic synchronization runs independently on the master node whenever `AIPDD_API_KEY` is configured and `AIPDD_CATALOG_SYNC_INTERVAL_MINUTES>0`; the code default is five minutes. A non-positive interval disables the periodic task.
- The manual backend trigger is `POST /api/channel/<managed-aipdd-id>/aipdd/sync`. Some frontend builds intend to expose it as `渠道管理 → AIPDD → … → 上游更新`, but do not rely on that menu: builds whose fetchable-channel list omits type `58` hide the action even though the backend endpoint exists.
- A successful sync atomically refreshes the authenticated revisioned catalog, model/capability metadata, the managed channel, and the same-origin snapshot. On live-fetch failure it may return a snapshot result; `used_snapshot=true` is never fresh enough for a requested price overwrite.
- `updated_prices` counts only built-in changes made for eligible available `token_market_media` capabilities using `pricingModel=per_second`, `currency=awcoin`, and `pricingBasis=display`. Those entries are converted with `display AWCoin × rmbPerAwcoin ÷ USDExchangeRate` and written to `billing_setting.task_pricing` / `billing_setting.billing_mode`. It does not certify that every AIPDD model price was rebuilt.
- Built-in sync does not replace the complete deployment reconciliation for Seedance suggested-retail matrices, LLM tiered expressions, per-call prices, or the complete previous/current AIPDD model ownership cleanup. Use the helper procedure only after the user explicitly approved price overwrite.

When VIP group-price synchronization is **not** confirmed, do not run its helper, do not write `GroupRatio`, `UserUsableGroups`, or `group_ratio_setting.group_special_usable_group`, and do not modify channel group associations. Report that the current group configuration was preserved.

#### Automatically reconcile AIPDD prices after confirmation

Run this only after the administrator login succeeds. Treat the confirmation as authorization to replace AIPDD-owned entries in the five pricing options below, not as authorization to change non-AIPDD prices or user/group ratios unless the independent VIP synchronization decision was also explicitly confirmed.

Before the live catalog fetch, require `AIPDD_CATALOG_SYNC_INTERVAL_MINUTES=0` in the running container so the periodic partial importer cannot race with the backup, plan generation, writes, or verification. Leave it at `0` after reconciliation unless the user explicitly accepts that future periodic runs update only the eligible Token Market duration subset rather than rerunning this complete plan. After successful reconciliation, also persist `AIPDD_CATALOG_SYNC_ON_BOOT=false` unless the user explicitly accepted the same partial behavior on future restarts.

1. Before catalog synchronization, save these items in a timestamped, mode-`600` backup without printing them:
   - the current AIPDD channel model list from `GET /api/channel/?p=1&page_size=100&type=58`;
   - the exact current values of `ModelPrice`, `ModelRatio`, `billing_setting.billing_expr`, `billing_setting.task_pricing`, `billing_setting.billing_mode`, `USDExchangeRate`, `DisplayInCurrencyEnabled`, and `general_setting.quota_display_type` from `GET /api/option/`.
2. Call `POST /api/channel/<managed-aipdd-id>/aipdd/sync`. Require `success=true`, a non-empty revision, and `used_snapshot=false`. A fallback snapshot is not fresh enough for an approved overwrite, especially after an API-key change; stop before writing options if a live authenticated catalog was not fetched.
3. Export only the saved catalog JSON payload from the newest `a_ip_dd_catalog_snapshots` row. This is the actual GORM table name for the acronym-heavy model on the SQLite deployment. Discover the equivalent table name from the target database schema before querying if the deployment uses another database or an older schema. Do not export the channel key, `.env`, cookies, or any database-wide dump. Store the catalog, option response, and previous-model list in protected temporary JSON files. The option export passed to the helper must include a positive `USDExchangeRate` (read-only; never modify it during reconciliation). The catalog `awcoinRate` must include a positive `rmbPerAwcoin`; missing either value is a hard abort before any option write.
4. Ensure the site displays prices in RMB before applying the plan. If needed, set `DisplayInCurrencyEnabled=true` and `general_setting.quota_display_type=CNY` through authenticated `PUT /api/option/` calls. Do **not** change `USDExchangeRate`. Re-fetch and require both display options match; if either write fails, stop before price writes.
5. Run the bundled offline helper from a trusted local environment:

   ```bash
   python .agents/skills/new-api-docker-deploy/scripts/build_aipdd_pricing_options.py \
     --catalog catalog.json \
     --options options-before.json \
     --managed-models aipdd-models-before.json \
     --output pricing-plan.json
   ```

   The helper removes only entries owned by the previous/current AIPDD model sets, then rebuilds all AIPDD prices with the RMB-anchored conversion `storedUSD = AWCoin × rmbPerAwcoin ÷ site USDExchangeRate` (catalog `usdPerAwcoin` is unused for sale prices so that `storedUSD × USDExchangeRate` equals AIPDD authoritative RMB / suggested retail):
   - non-duration AIPDD tasks as catalog-derived per-call `ModelPrice` values;
   - AIPDD LLMs as `tiered_expr` rules using RMB-anchored prompt/completion USD prices;
   - Seedance/per-second capabilities as `task_pricing`, with `unit=second`, a `by_resolution` tier for every catalog resolution, a per-tier no-reference-video price, an automatic per-tier `same`/`custom` reference-video policy, `group_ratio_policy=none` on the `480p` tier, and `billing_mode=task_pricing`;
   - non-Seedance `per_unit/second` capabilities such as LTX as flat `task_pricing`, with `unit=second`, `no_reference_video_unit_price=chargeConfig.amount × rmbPerAwcoin ÷ site USDExchangeRate`, `reference_video_policy=same`, and `billing_mode=task_pricing`.

   The Seedance matrix must use the dual-price AIPDD contract: platform settlement via `displayAmount*` and New API sale via `suggestedRetail*`. Normalize each catalog resolution key with `trim + lowercase`, reject empty keys, keys longer than 128 characters, duplicates after normalization, and a `targetResolution` that does not match its normalized map key. Require `pricingBasis=display`. For every resolution, require positive `displayAmountAwcoinPerSecond`, positive `displayVideoInputAwcoinPerSecond`, positive `suggestedRetailAwcoinPerSecond`, positive `suggestedRetailVideoInputAwcoinPerSecond`, and positive `defaultFramesPerSecond`. Require positive `defaultDurationSeconds` except that a model name containing the version tokens `Seedance 2.5` may use the exact `-1` auto-duration sentinel; match the tokens case-insensitively with whitespace, hyphen, underscore, or dot separators, and continue rejecting zero and every other negative value. The sentinel is validation-only: do not replace it with a fixed duration or use it in sale-price selection. Convert only the suggested-retail AWCoin rates with the RMB-anchored factor into `task_pricing` USD/second. Legacy modality fields, platform `displayAmount*` fields, and BYOK fields must never participate in sale-price selection. Emit `reference_video_policy=same` when both suggested-retail prices are equal, otherwise emit `custom` and its explicit price. Emit `group_ratio_policy=none` for `480p` so that all customers use the native multiplier `1`; omit it for other resolutions unless a future explicit rule requires otherwise. Never aggregate a maximum across different resolutions. Do not alias `2k/1440p` or `4k/2160p`, and do not read `priceVariants`, `minimumAwcoin`, an existing `ModelPrice`, a previous flat `task_pricing` value, or any other fallback source. A missing, zero, invalid, or structurally incompatible required field must abort plan generation before any option write.
6. Inspect the helper summary, not the complete option bodies. Require every current catalog model to appear exactly once in the per-call, task-pricing, or tiered-expression lists; require `summary.price_conversion` to equal `rmb_anchored`; and require `task_pricing_contract` to state that Seedance required suggested retail prices for New API sale, still required AIPDD display settlement fields, fixed the `480p` group ratio at `1`, rejected legacy catalog pricing for its `by_resolution` matrix, converted prices as `AWCoin × rmbPerAwcoin ÷ site USDExchangeRate`, while `per_unit/second` capabilities used flat USD/second task pricing, with no legacy `ModelPrice` fallback. If catalog validation, a positive price, a model ID, a resolution tier, a supported duration unit, or any required current-contract field is missing, make no option writes.
7. Apply `pricing-plan.json.updates` through authenticated `PUT /api/option/` calls in the emitted order. This writes task pricing and expressions first and enables billing modes last. Require HTTP 200 and `success=true` for every write. Never put the cookie or complete JSON request in a command line or ordinary log.
8. If any write or verification fails, apply every item in `pricing-plan.json.rollback` in the emitted order, verify the original option values were restored, and report the failure. Do not leave `billing_mode=task_pricing` without a matching valid task-pricing object.
9. Re-fetch `GET /api/option/` and require all five pricing values to equal the generated plan, and require `DisplayInCurrencyEnabled=true`, `general_setting.quota_display_type=CNY`, and an unchanged positive `USDExchangeRate`. Reject any reconciled Seedance entry that contains root-level `no_reference_video_unit_price`, `reference_video_policy`, or `reference_video_unit_price`; require `unit=second`, a non-empty `by_resolution`, an exact normalized tier-key match with the authenticated catalog, and `group_ratio_policy=none` on every `480p` tier. For every reconciled `per_unit/second` entry, reject `by_resolution`, require a positive root `no_reference_video_unit_price`, `reference_video_policy=same`, `unit=second`, and absence from `ModelPrice`. Spot-check at least one Seedance resolution tier: `storedUSD × USDExchangeRate` must equal `suggestedRetailAwcoinPerSecond × rmbPerAwcoin` (and the video-input counterpart when `reference_video_policy=custom`). Then call `GET /api/pricing` and verify every model in `TaskPricingRequiredModels` has `billing_mode=task_pricing` and a valid positive task-pricing object. Seedance must additionally expose `task_pricing_resolutions` equal to the active catalog/configuration intersection; flat duration models must remain visible without inventing resolution options. This read-only validation must replace a paid model request.
10. Delete all temporary catalog, option, model-list, plan, cookie, and request files after verification. Report the catalog revision, that prices are RMB-anchored, and counts by pricing mode, but do not print the full price maps or any secret.

When force overwrite **is** confirmed, delete and rebuild only AIPDD channels:

1. Use the authenticated cookie jar to request `GET /api/channel/?p=1&page_size=100&type=58`. `58` is the project’s `ChannelTypeAIPDD`.
2. Record only the channel IDs and names needed for the operation; never request or print channel keys.
3. Ask for a final confirmation immediately before the first deletion if the user’s confirmation was not explicit for this exact action.
4. Send `DELETE /api/channel/<id>` for every returned AIPDD channel. Do not delete other channel types, disabled non-AIPDD channels, database volumes, or application data.
5. On first deployment, ensure `.env` contains `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=true`. On update, back up `.env`, change only this setting temporarily or as explicitly approved, then run `docker compose restart new-api`.
6. Poll `/api/status`, then query the AIPDD channel list again. Confirm that the startup bootstrap created a fresh managed AIPDD channel. If price overwrite was confirmed, run the complete automatic reconciliation procedure after the fresh channel exists; if it was declined, confirm that catalog synchronization was not run and existing prices were preserved. If VIP group-price synchronization was confirmed, run it only after this rebuild and any approved model-price reconciliation have completed.

After a successful channel rebuild, set `AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false` for future restarts and preserve the user’s separate model-price and VIP group-price choices. Do not leave a one-time destructive overwrite enabled by accident.

The environment toggle alone is insufficient: it updates one existing AIPDD channel but does not clear additional AIPDD channels. If any AIPDD deletion fails, stop the destructive workflow, preserve the remaining data, and report the channel ID and error without claiming that overwrite completed.

#### Idempotently synchronize VIP group prices after confirmation

Run this only when the independent VIP decision was explicitly confirmed and the administrator login succeeds. It is valid whether AIPDD model-price overwrite was accepted or declined. When model-price overwrite was also confirmed, complete and verify model-price reconciliation first. When channel overwrite was also confirmed, wait until the final managed AIPDD channel exists. The only authorized mutations are `GroupRatio`, `UserUsableGroups`, `group_ratio_setting.group_special_usable_group`, and the `group` field of channels whose type is exactly `58`.

The fixed contract is:

- `VIP1=0.78`
- `VIP2=0.80`
- `VIP3=0.85`
- `VIP4=0.90`
- `VIP5=0.95`

Do not modify `GroupGroupRatio`, `TopupGroupRatio`, user records, subscription plans, non-AIPDD channels, channel keys, channel models, model mappings, model prices, billing modes, or task-pricing matrices. Existing non-VIP `UserUsableGroups` entries and all unrelated special usable-group rules must be preserved. VIP1-VIP5 must not remain globally user-selectable. For each VIP group, preserve an existing `-:default` description or create it when absent. Re-running the procedure against already synchronized state must produce no writes.

1. Fetch the exact current `GroupRatio`, `UserUsableGroups`, and `group_ratio_setting.group_special_usable_group` values from `GET /api/option/`. Fetch all AIPDD channel pages from `GET /api/channel/?p=<page>&page_size=100&type=58` until the reported total is exhausted. Require at least one AIPDD channel; do not claim success when there is nothing to associate.
2. Save the three exact option values and the redacted AIPDD channel list in a timestamped mode-`600` backup. Never call the channel-key endpoint and never print option bodies or channel payloads.
3. Store the option response and a single combined `{"items":[...]}` AIPDD channel document in protected temporary files, then run the bundled offline helper from a trusted local environment:

   ```bash
   python .agents/skills/new-api-docker-deploy/scripts/build_vip_group_sync_plan.py \
     --options options-before.json \
     --channels aipdd-channels-before.json \
     --output vip-group-plan.json
   ```

4. Inspect only `vip-group-plan.json.summary`. Require `fixed_groups` to equal the five-value contract above, `private_user_groups` to equal VIP1-VIP5, `private_rule` to equal `-:default`, and `contract` to state that the VIP groups are not global user-selectable, same-name user groups receive the private rule, and unrelated groups, channels, keys, models, users, and model prices are preserved. Reject any plan containing an option key other than `GroupRatio`, `UserUsableGroups`, or `group_ratio_setting.group_special_usable_group`, any channel type other than `58`, a duplicate channel ID, or a merged channel group longer than 64 characters.
5. Apply `option_updates` in order. Immediately before each authenticated `PUT /api/option/` call, re-fetch that option and require its parsed JSON map to equal the item’s `previous_value`; if it changed after plan generation, stop and regenerate the plan instead of overwriting concurrent administrator work. Send only `key` and `value`. Require HTTP 200 and `success=true` after every write.
6. For each `channel_updates` item, immediately re-fetch `GET /api/channel/<id>` and require that its ID and type are unchanged and its current `group` exactly equals `previous_group`. Use the complete redacted channel object returned by that request, change only its `group` field to the planned value, and send it to `PUT /api/channel/`. The redacted empty `key` is intentionally not updated by the backend; do not fetch, copy, or send the real key. Require `success=true` and then re-fetch the channel to verify the exact merged group.
7. Re-fetch `GET /api/option/` and require all three managed option maps to equal the plan: the five ratios are exact, VIP1-VIP5 are absent from global `UserUsableGroups`, and each same-name user group has `-:default` in `group_ratio_setting.group_special_usable_group`. Query every AIPDD channel again and require it to contain each VIP group exactly once while preserving every original group in its original relative order.
8. If any option write, channel write, or verification fails, stop forward changes. Roll back changed channels in `channel_rollback` order by re-fetching each latest channel, requiring its current group to equal `expected_group`, changing only `group` to the rollback value, and sending the complete redacted object to `PUT /api/channel/`. Then apply `option_rollback` in order. Verify all original option values and channel group strings were restored. Report any rollback failure without exposing payloads.
9. Delete all temporary option, channel, plan, cookie, and request files after verification. Report only the five fixed ratios, changed option names, changed channel count, and whether the operation was a no-op.

### 7. Verify and hand off

Verify all of the following:

- `docker compose ps` shows the three services running; PostgreSQL is healthy.
- `GET /api/status` succeeds from the server.
- When an application domain was configured, `http://<domain>:6070/api/status` succeeds and the `ServerAddress` option matches; do not claim HTTPS. Otherwise report that the port-only mode is active.
- Application logs do not contain a missing `AIPDD_API_KEY` or database/Redis connection error.
- `.aipdd-instance-id`, `.env`, and the running container expose the same registered `AIPDD_INSTANCE_ID`; no Key-derived or newly randomized identity was used.
- A read-only site authorization probe using the protected site Key and Instance UUID reaches `GET /api/finance/v1/settlements/<nonexistent-probe-order>`. Accept only an authenticated “order not found” result; treat missing or invalid headers (400), an invalid Key (401), and an unbound, cross-site, inactive-site, or mismatched Instance response (403) as deployment failures. Never put the Key in a command argument or log.
- The image is attempt-aware: Chat, Shared Task, and Seedance creation requests use Instance, Order, and Attempt headers. Do not run a paid model call solely for this verification; use automated contract-test/build evidence or an already authorized smoke request.
- In first-deployment mode, the root administrator was created on the new database. In update mode, `/api/setup` was not called and the existing administrator was preserved.
- All three requested decisions match the user’s answers.
- `AIPDD_CATALOG_SYNC_ON_BOOT=false` is persisted after price preservation or successful full reconciliation, unless future partial boot synchronization was separately accepted.
- `AIPDD_CATALOG_SYNC_INTERVAL_MINUTES` matches the authorized behavior: `0` when prices were preserved or fully reconciled, and positive only when recurring partial Token Market price synchronization was separately accepted.
- In either mode, report the three decisions separately: channel overwrite `是/否`, AIPDD model-price overwrite `是/否`, and VIP group-price synchronization `是/否`.
- When AIPDD price overwrite was confirmed, report that Seedance was verified as `by_resolution` matrix pricing, every `per_unit/second` model was verified as flat per-second task pricing, display currency is RMB, `USDExchangeRate` was left unchanged, a Seedance spot-check confirmed `storedUSD × USDExchangeRate == AWCoin × rmbPerAwcoin`, and whether recurring partial Token Market synchronization remains disabled or was explicitly enabled; when overwrite was declined, report that the existing pricing shape was preserved and may still be legacy pricing and that both boot and periodic remote synchronization were disabled.
- When AIPDD price overwrite was declined, verify that the same-origin catalog snapshot was activated read-only and that the complete pre-update AIPDD duration-model set remains present in `GET /api/pricing`; a healthy container alone is insufficient.
- When price overwrite was confirmed, every `TaskPricingRequiredModels` entry has a valid positive task-pricing object and `billing_mode=task_pricing`; Seedance entries have a non-empty `by_resolution` matrix, `per_unit/second` entries use the flat shape and do not remain in `ModelPrice`, display options are `DisplayInCurrencyEnabled=true` and `general_setting.quota_display_type=CNY`, and catalog sync alone does not satisfy this check.
- When VIP group-price synchronization was confirmed, `GroupRatio` contains the five exact fixed ratios, VIP1-VIP5 are absent from global `UserUsableGroups`, every same-name user group has a `-:default` private rule, no existing user record was reassigned, and every AIPDD channel contains VIP1-VIP5 exactly once. When it was declined, report that group ratios, usable-group rules, and channel group associations were preserved.

After these checks, submit `deployment-finish` with the same deployment UUID, the actual terminal status, the three decisions, collected verification results, recovery/rollback results, and a sanitized error object when applicable. Submit a terminal report even when the New API deployment failed, provided `deployment-start` succeeded. If any reporting call failed, do not change the deployment status or initiate rollback because of that reporting failure; list the exact failed reporting stage in the final response.

Do not claim success if health checks, admin initialization, or requested AIPDD synchronization failed. State which stage failed and preserve the deployment directory for diagnosis.

## Final response format

After successful first deployment, provide the server address, deployment directory, service status, the three independent AIPDD/VIP decision results, and this block. Use `http://<domain>:6070` when the New API application domain was configured; otherwise use `http://<server>:6070`. Do not claim HTTPS. Do not include the AIPDD API key:

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

When the credentials report succeeded, also state: `凭据已归档到 AIPDD 部署站点页面；以上凭据仍请立即保存到密码管理器。` This does not replace the “请立即保存” warning or authorize omitting the one-time credential block. If it failed, state that credentials were not archived and name the failed `credentials` stage.

For an update, do not print or regenerate credentials. Report that `.env`, the database, the root password, and existing data were preserved, and include the separate results for channel overwrite, AIPDD model-price overwrite, and VIP group-price synchronization. In either mode, report whether the instance, deployment-start, credentials (when applicable), and deployment-finish records succeeded; if any failed, state that the corresponding archive/credentials were not recorded without calling the deployment itself failed.
