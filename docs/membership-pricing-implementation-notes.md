# Membership pricing implementation notes

This log records implementation-time findings that refine the agreed membership pricing map.

## Agreed invariants

- Membership is independent from NewAPI routing groups.
- Final model consumption is `base amount * group multiplier * membership multiplier`.
- Membership applies to every model-billing mode and model-call surcharge, but not recharge, refunds, manual adjustments, subscriptions, provider cost, or violation fees.
- A normalized public model name containing `apseedance` at resolved resolution `480p` applies membership multiplier `1.0` while retaining the group multiplier.
- Model names and public relay request fields do not change.
- Accepted requests freeze membership pricing so later membership edits cannot reprice in-flight work.
- The subscription product is not part of this implementation.

## Deviations discovered during implementation

- The legacy task-pricing `group_ratio_policy` JSON field remains readable so existing configuration does not fail validation, but it no longer bypasses global group pricing. New CSV imports do not emit it.
- The Seedance CSV parser accepts legacy extra VIP columns for operator convenience, but ignores them. CSV preview/apply changes only task base pricing and billing mode.
- The explicit legacy VIP migration clears stale `upgrade_group` references from any pre-existing subscription records while moving VIP users and tokens to `default`. It does not create, renew, or resolve membership from subscriptions.
- A zero group multiplier remains a supported free-pricing configuration. Membership cannot turn that free price into a charge.
- Existing quota rounding/truncation rules are preserved for `NORMAL` users; membership is inserted as an additional multiplier without changing each billing path's established integer conversion.
- Anonymous model-square responses are restricted to the `default` pricing group and the `NORMAL` membership. Authenticated users may compare only their authorized groups, unless token groups are locked to the user's own group.

## Implemented surfaces

- Persistent membership levels and auditable user grants, with dynamic codes, PPM multipliers, ranks, effective windows, revocation, archiving, cache invalidation, and highest-rank resolution.
- Explicit legacy VIP migration preflight and apply endpoints.
- Frozen membership snapshots across token, fixed-price, tiered-expression, asynchronous task, Seedance, AIPDD, audio/realtime, and model-call surcharge billing.
- Membership details in consumption logs and task billing snapshots.
- Admin level management, per-user grant/history management, profile membership card, and model-square “your price” calculation/group selector.

## Local acceptance verification

- Started the compiled Go backend against an isolated SQLite database and the Rsbuild development server directly on Windows; Docker was not used and the workspace's existing `one-api.db` was not opened.
- Verified through real HTTP requests that anonymous pricing resolves to `default + NORMAL`, a manually granted `VIP1` resolves in the self/profile/pricing APIs, revocation immediately returns to `NORMAL`, and legacy migration preflight is available.
- Created overlapping `VIP1` and `VIP2` grants to verify highest-rank resolution, then verified cache invalidation after grant revocation, multiplier update, and level archiving. Invalid multipliers above `1_000_000` PPM were rejected.
