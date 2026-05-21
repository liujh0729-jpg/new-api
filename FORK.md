# Fork Maintenance

This repository is maintained as a downstream fork of `QuantumNous/new-api`.

## Upstream

- Upstream repository: https://github.com/QuantumNous/new-api
- Upstream remote name: `upstream`
- Upstream push URL: `DISABLED`
- Fork remote name: `origin`
- License: GNU Affero General Public License v3.0

## Current Baseline

- Local `main` at setup time: `ba474393fbb91ad7bb66e6c9693da8d0550eedfb`
- Fetched `upstream/main` at setup time: `18282e610ddf3c8c39732fe84e50ded2cf6dcc7f`
- Status at setup time: local `main` is 15 commits behind `upstream/main`

## Branch Policy

- `main`: stable public branch for this fork.
- `feature/*`: feature development branches.
- `sync-upstream/*`: temporary branches used to merge upstream changes before they land on `main`.

Avoid unrelated refactors in files that frequently change upstream. Keep fork-specific behavior isolated behind configuration, new modules, or small integration points where practical.

## Sync Workflow

Fetch upstream:

```bash
git fetch upstream
```

Create a temporary sync branch:

```bash
git checkout main
git checkout -b sync-upstream/YYYY-MM-DD
git merge upstream/main
```

After resolving conflicts, verify backend and frontend builds:

```bash
go test ./...
cd web/default
bun run build
```

When verification passes, merge the sync branch back to `main` and push:

```bash
git checkout main
git merge sync-upstream/YYYY-MM-DD
git push origin main
```

## Attribution and Notices

This fork preserves the upstream license, notices, author attribution, and original project link required by `LICENSE` and `NOTICE`.

Modified versions must not misrepresent the origin of the software. Changes made by this fork should be documented in commit history, release notes, or this file.
