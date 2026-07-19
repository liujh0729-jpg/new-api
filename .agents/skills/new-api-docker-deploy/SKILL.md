---
name: new-api-docker-deploy
description: Deploy and update New API on a pre-provisioned Docker server over SSH using Alibaba Cloud ACR images for the application, PostgreSQL, and Redis with Docker Compose. Use when the user provides server SSH credentials and an AIPDD API key, asks for a first deployment or an update, optionally sets New API's own ServerAddress to a supplied domain without configuring a reverse proxy or HTTPS, asks to reset or synchronize the AIPDD channel, confirms that AIPDD local prices may be overwritten and reconciled from the authenticated catalog, or requests idempotent VIP1-VIP5 group-price synchronization with same-name private user-group linkage. Keep first-deployment initialization separate from updates; generate credentials and create the root account only on first deployment; updates preserve .env, databases, users, and channels, pull the ACR image, and never publish updated application images to Docker Hub.
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

Optionally ask for SSH port, deployment directory, public port, and the New API application domain. Use port `22`, `/opt/new-api`, and `6070` when omitted. A supplied domain means only setting New API’s `ServerAddress` option; it does not authorize a reverse proxy, HTTPS, certificate, DNS, or firewall change. Ask these three decisions independently in either deployment mode and never infer any `yes` from a general request to deploy:

1. Before any destructive AIPDD action, ask exactly: “是否强制覆盖 AIPDD 渠道？这会删除现有 AIPDD 渠道并按当前 AIPDD API Key 重建。”
2. Ask exactly: “是否覆盖 AIPDD 模型价格？选择是会根据当前 API Key 的实时认证目录重建 AIPDD 本地价格规则，包括 Seedance 的分辨率价格矩阵、LTX 等按秒模型的单位价格和计费模式；非 AIPDD 价格不变。”
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

Before pulling the new image, ask three independent questions and record the answers:

1. **是否覆盖 AIPDD 渠道？** Default to **否**. **是** means delete all existing AIPDD channels and rebuild the managed AIPDD channel from the current AIPDD key; this is destructive and requires the existing administrator login.
2. **是否覆盖 AIPDD 模型价格？** Default to **否**. **是** authorizes the complete price-reconciliation procedure below: fetch the live authenticated catalog, replace pricing entries belonging to the previous and current AIPDD model sets, and automatically write `ModelPrice`, `billing_setting.billing_expr`, `billing_setting.task_pricing`, and `billing_setting.billing_mode` while preserving non-AIPDD entries. Seedance pricing must be written as a `by_resolution` matrix. Require `pricingBasis=display`, `displayAmountAwcoinPerSecond`, and `displayVideoInputAwcoinPerSecond`; reject catalogs that omit any of them. Never use legacy modality fields, `byokAmountAwcoinPerSecond`, or `byokVideoInputAwcoinPerSecond` as New API's sale price. A non-Seedance `per_unit` capability is supported only when `chargeConfig.unit=second`; it must be written as legacy-shaped flat `task_pricing` whose unit price is the catalog AWCoin amount converted to USD/second. `priceVariants` and every local-price or legacy `ModelPrice` fallback are unsupported for an approved reconciliation. **否** preserves all current local AIPDD pricing options, including any legacy flat Seedance pricing, does not call the catalog sync endpoint, and requires the application to restore runtime capabilities read-only from the same-origin last-known-good snapshot. Never claim that an update migrated pricing when overwrite was declined.
3. **是否同步 VIP 分组价格和用户分组关联？** Default to **否**. **是** authorizes only the idempotent VIP synchronization procedure below: force the five fixed global ratios, remove VIP1-VIP5 from global `UserUsableGroups`, add `-:default` under each same-name user group in `group_ratio_setting.group_special_usable_group`, and append the five group names to every AIPDD channel while preserving all existing channel groups and every non-group channel field. It does not authorize any model-price write, channel deletion, user-record reassignment, `GroupGroupRatio`, or top-up-ratio change.

Do not infer any answer from the request to update the application. If the user does not explicitly answer all three decisions, stop before changing the deployment.

1. Back up the existing Compose file and `.env` to the protected timestamped backup directory. Do not print either file.
2. Read the current Compose file without exposing secrets. Confirm that the application image points to the expected ACR repository. If it points elsewhere, show the image name and ask before changing it.
3. If the ACR repository is private, authenticate with the user-provided least-privilege registry account interactively. Never put registry credentials in the Compose file or command arguments.
4. Preserve every existing `.env` value except the two decisions represented by environment toggles:

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
8. If the user requests an application-domain change, AIPDD channel overwrite, AIPDD price overwrite, or VIP group-price synchronization during the update, use the authenticated existing administrator account and the separate procedures below. Do not treat an image update as permission to perform any of these operations.

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
AIPDD_CATALOG_SYNC_ON_BOOT=<true only when price overwrite is confirmed, otherwise false>
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

When price overwrite is **not** confirmed, set `AIPDD_CATALOG_SYNC_ON_BOOT=false`, do not call `POST /api/channel/<id>/aipdd/sync`, do not write any AIPDD model-pricing option, and report that current AIPDD prices were preserved. This flag disables remote refresh and database reconciliation only; the application must still activate the existing same-origin `a_ip_dd_catalog_snapshots` payload in memory. Before updating, record the enabled AIPDD duration-model names exposed by `GET /api/pricing`. After restart, require every recorded model—especially every Seedance `by_resolution` model—to remain exposed with the same billing mode and active resolution set. If any disappears, roll back to the previous application image and do not claim success. A missing, invalid, or different-origin snapshot is a deployment blocker when the current installation contains AIPDD duration-priced models. The independently approved VIP procedure may still write only `GroupRatio`, `UserUsableGroups`, `group_ratio_setting.group_special_usable_group`, and AIPDD channel `group` fields. When price overwrite **is** confirmed, set it to `true` and execute the complete procedure below. The environment toggle and the sync response's `updated_prices` field are not proof of price replacement: catalog synchronization intentionally preserves local pricing in current versions.

When VIP group-price synchronization is **not** confirmed, do not run its helper, do not write `GroupRatio`, `UserUsableGroups`, or `group_ratio_setting.group_special_usable_group`, and do not modify channel group associations. Report that the current group configuration was preserved.

#### Automatically reconcile AIPDD prices after confirmation

Run this only after the administrator login succeeds. Treat the confirmation as authorization to replace AIPDD-owned entries in the five pricing options below, not as authorization to change non-AIPDD prices or user/group ratios unless the independent VIP synchronization decision was also explicitly confirmed.

1. Before catalog synchronization, save these items in a timestamped, mode-`600` backup without printing them:
   - the current AIPDD channel model list from `GET /api/channel/?p=1&page_size=100&type=58`;
   - the exact current values of `ModelPrice`, `ModelRatio`, `billing_setting.billing_expr`, `billing_setting.task_pricing`, and `billing_setting.billing_mode` from `GET /api/option/`.
2. Call `POST /api/channel/<managed-aipdd-id>/aipdd/sync`. Require `success=true`, a non-empty revision, and `used_snapshot=false`. A fallback snapshot is not fresh enough for an approved overwrite, especially after an API-key change; stop before writing options if a live authenticated catalog was not fetched.
3. Export only the saved catalog JSON payload from the newest `a_ip_dd_catalog_snapshots` row. This is the actual GORM table name for the acronym-heavy model on the SQLite deployment. Discover the equivalent table name from the target database schema before querying if the deployment uses another database or an older schema. Do not export the channel key, `.env`, cookies, or any database-wide dump. Store the catalog, option response, and previous-model list in protected temporary JSON files.
4. Run the bundled offline helper from a trusted local environment:

   ```bash
   python .agents/skills/new-api-docker-deploy/scripts/build_aipdd_pricing_options.py \
     --catalog catalog.json \
     --options options-before.json \
     --managed-models aipdd-models-before.json \
     --output pricing-plan.json
   ```

   The helper removes only entries owned by the previous/current AIPDD model sets, then rebuilds:
   - non-duration AIPDD tasks as catalog-derived per-call `ModelPrice` values;
   - AIPDD LLMs as `tiered_expr` rules using the catalog's USD-converted prompt/completion prices;
   - Seedance/per-second capabilities as `task_pricing`, with `unit=second`, a `by_resolution` tier for every catalog resolution, a per-tier no-reference-video price, an automatic per-tier `same`/`custom` reference-video policy, `group_ratio_policy=none` on the `480p` tier, and `billing_mode=task_pricing`;
   - non-Seedance `per_unit/second` capabilities such as LTX as flat `task_pricing`, with `unit=second`, `no_reference_video_unit_price=chargeConfig.amount * usdPerAwcoin`, `reference_video_policy=same`, and `billing_mode=task_pricing`.

   The Seedance matrix must use the strict display-price AIPDD contract. Normalize each catalog resolution key with `trim + lowercase`, reject empty keys, keys longer than 128 characters, duplicates after normalization, and a `targetResolution` that does not match its normalized map key. Require `pricingBasis=display`. For every resolution, require positive `displayAmountAwcoinPerSecond`, positive `displayVideoInputAwcoinPerSecond`, positive `defaultDurationSeconds`, and positive `defaultFramesPerSecond`. Legacy modality fields and BYOK fields must never participate in sale-price selection. Emit `reference_video_policy=same` when both display prices are equal, otherwise emit `custom` and its explicit price. Emit `group_ratio_policy=none` for `480p` so that all customers use the native multiplier `1`; omit it for other resolutions unless a future explicit rule requires otherwise. Never aggregate a maximum across different resolutions. Do not alias `2k/1440p` or `4k/2160p`, and do not read `priceVariants`, `minimumAwcoin`, an existing `ModelPrice`, a previous flat `task_pricing` value, or any other fallback source. A missing, zero, invalid, or structurally incompatible required field must abort plan generation before any option write.
5. Inspect the helper summary, not the complete option bodies. Require every current catalog model to appear exactly once in the per-call, task-pricing, or tiered-expression lists, and require `task_pricing_contract` to state that Seedance required explicit display fields, fixed the `480p` group ratio at `1`, and rejected legacy catalog pricing for its `by_resolution` matrix, while `per_unit/second` capabilities used flat USD/second task pricing, with no legacy `ModelPrice` fallback. If catalog validation, a positive price, a model ID, a resolution tier, a supported duration unit, or any required current-contract field is missing, make no option writes.
6. Apply `pricing-plan.json.updates` through authenticated `PUT /api/option/` calls in the emitted order. This writes task pricing and expressions first and enables billing modes last. Require HTTP 200 and `success=true` for every write. Never put the cookie or complete JSON request in a command line or ordinary log.
7. If any write or verification fails, apply every item in `pricing-plan.json.rollback` in the emitted order, verify the original option values were restored, and report the failure. Do not leave `billing_mode=task_pricing` without a matching valid task-pricing object.
8. Re-fetch `GET /api/option/` and require all five values to equal the generated plan. Reject any reconciled Seedance entry that contains root-level `no_reference_video_unit_price`, `reference_video_policy`, or `reference_video_unit_price`; require `unit=second`, a non-empty `by_resolution`, an exact normalized tier-key match with the authenticated catalog, and `group_ratio_policy=none` on every `480p` tier. For every reconciled `per_unit/second` entry, reject `by_resolution`, require a positive root `no_reference_video_unit_price`, `reference_video_policy=same`, `unit=second`, and absence from `ModelPrice`. Then call `GET /api/pricing` and verify every model in `TaskPricingRequiredModels` has `billing_mode=task_pricing` and a valid positive task-pricing object. Seedance must additionally expose `task_pricing_resolutions` equal to the active catalog/configuration intersection; flat duration models must remain visible without inventing resolution options. This read-only validation must replace a paid model request.
9. Delete all temporary catalog, option, model-list, plan, cookie, and request files after verification. Report the catalog revision and counts by pricing mode, but do not print the full price maps or any secret.

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
- In first-deployment mode, the root administrator was created on the new database. In update mode, `/api/setup` was not called and the existing administrator was preserved.
- All three requested decisions match the user’s answers.
- In either mode, report the three decisions separately: channel overwrite `是/否`, AIPDD model-price overwrite `是/否`, and VIP group-price synchronization `是/否`.
- When AIPDD price overwrite was confirmed, report that Seedance was verified as `by_resolution` matrix pricing and every `per_unit/second` model was verified as flat per-second task pricing; when it was declined, report that the existing pricing shape was preserved and may still be legacy pricing.
- When AIPDD price overwrite was declined, verify that the same-origin catalog snapshot was activated read-only and that the complete pre-update AIPDD duration-model set remains present in `GET /api/pricing`; a healthy container alone is insufficient.
- When price overwrite was confirmed, every `TaskPricingRequiredModels` entry has a valid positive task-pricing object and `billing_mode=task_pricing`; Seedance entries have a non-empty `by_resolution` matrix, `per_unit/second` entries use the flat shape and do not remain in `ModelPrice`, and catalog sync alone does not satisfy this check.
- When VIP group-price synchronization was confirmed, `GroupRatio` contains the five exact fixed ratios, VIP1-VIP5 are absent from global `UserUsableGroups`, every same-name user group has a `-:default` private rule, no existing user record was reassigned, and every AIPDD channel contains VIP1-VIP5 exactly once. When it was declined, report that group ratios, usable-group rules, and channel group associations were preserved.

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

For an update, do not print or regenerate credentials. Report that `.env`, the database, the root password, and existing data were preserved, and include the separate results for channel overwrite, AIPDD model-price overwrite, and VIP group-price synchronization.
