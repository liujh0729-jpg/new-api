---
name: new-api-docker-deploy
description: Build and update the New API Docker service from Alibaba Cloud ACR, then synchronize its managed AIPDD channel, published models, and prices. Use for production New API releases and AIPDD catalog refreshes.
---

# New API Docker Deploy

Deploy the application and leave it with a verified, current AIPDD catalog. The normal workflow always synchronizes the managed AIPDD channel, its published models, and their prices after the application becomes healthy.

## Scope

- Preserve the database, environment secrets, administrator credentials, and unrelated channels.
- Do not prompt for or modify independent Seedance, membership, VIP, subscription, or user-group configuration.
- Do not delete an AIPDD channel during an ordinary update.
- Treat forced AIPDD channel replacement as a repair operation requiring explicit user authorization.

## Entrypoint

From the repository root, use:

```powershell
.\bin\deploy-acr-server.ps1
```

The entrypoint builds and tests the image, publishes the timestamped, `latest`, and `aipdd` tags, then runs `scripts/run_remote_update.py` locally to update the server over SSH.

Supply secrets through process environment variables or the script's secure prompts:

```powershell
$env:ACR_PASSWORD = "..."
$env:DEPLOY_SERVER_PASSWORD = "..."
$env:DEPLOY_ADMIN_PASSWORD = "..."
.\bin\deploy-acr-server.ps1
```

Use `-DryRun` to inspect the local release plan. Use `-ForceAipddChannelOverwrite` only when the user explicitly requests deletion and reconstruction of the managed AIPDD channel.

## Update contract

The remote updater must:

1. Verify Docker, Compose, the deployment files, expected ACR repository, and registered AIPDD instance UUID.
2. Verify that the deployed site's AIPDD Key belongs to that UUID without creating a paid task.
3. Back up the Compose and environment files without printing their contents.
4. Disable automatic catalog scheduling in the deployment environment so releases remain the synchronization boundary:

   ```dotenv
   AIPDD_CATALOG_SYNC_ON_BOOT=false
   AIPDD_CATALOG_SYNC_INTERVAL_MINUTES=0
   AIPDD_CHANNEL_OVERWRITE_ON_BOOT=false
   ```

5. Log in to ACR without placing the password in a command argument, pull the configured image, and recreate only the `new-api` service.
6. Require a healthy `/api/status` response and verify the running container's AIPDD instance UUID.
7. Authenticate as the existing New API administrator.
8. Call the managed channel's AIPDD synchronization endpoint and require a live authenticated catalog response. A fallback snapshot is not sufficient for a successful deployment sync.
9. Read the synchronized channel model list and update prices only for those models.
10. Verify the resulting options and public pricing response before reporting success.

Never print, persist in reports, or leave behind ACR passwords, SSH credentials, administrator passwords, cookies, or AIPDD Keys except where the deployment reporting contract explicitly requires protected credential storage.

## AIPDD pricing

Use `scripts/build_aipdd_pricing_options.py` to generate a reversible option plan from:

- the authenticated catalog snapshot;
- the post-sync option values;
- the AIPDD channel model lists from before and after synchronization.

```sh
python .agents/skills/new-api-docker-deploy/scripts/build_aipdd_pricing_options.py \
  --catalog catalog.json \
  --options options-after-sync.json \
  --managed-models aipdd-models-before.json \
  --current-models aipdd-models-after-sync.json \
  --output pricing-plan.json
```

The planner owns only AIPDD models exposed by the synchronized channel:

- preserve built-in `token_market_media/per_second` task pricing written by catalog synchronization and validate its resolution matrix;
- write supported `per_unit/second` capabilities as flat task pricing;
- write per-call prices to `ModelPrice`;
- write LLM prices as tiered expressions;
- remove stale prices only from the previous/current managed AIPDD model set;
- convert manual AWCoin prices with `AWCoin × rmbPerAwcoin ÷ USDExchangeRate`;
- leave `USDExchangeRate`, display settings, and unrelated options unchanged.

Apply the emitted updates in order. If a write or verification fails, apply every rollback item and verify restoration.

## Completion

Report success only when:

- the application and supporting services are healthy;
- the image update completed;
- the AIPDD sync used the live catalog;
- the managed channel model set was verified;
- every planned price and billing mode matches the resulting options;
- deployment identity and preserved-state checks passed.

Deployment reporting is best-effort. A reporting outage must be surfaced separately and must not turn an otherwise healthy release into an application rollback.

For a new installation, create protected Compose and environment files, initialize the database and administrator, register the immutable AIPDD site UUID, and then run the same catalog and pricing reconciliation described above.
