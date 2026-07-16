---
title: GitHub Actions
sidebar_label: GitHub Actions
---

# Use Case: kapi in GitHub Actions

This guide shows how to use kapi (with the bowrain plugin) in GitHub Actions workflows for automated translation, quality checks, and server sync.

## Overview

The [`setup-kapi`](https://github.com/neokapi/setup-kapi) GitHub Action installs kapi on any runner, with the bowrain plugin included by default. It handles platform detection, checksum verification, binary caching, and optional server authentication — so your workflow steps can focus on localization tasks.

## Setup

Add `neokapi/setup-kapi@v1` to your workflow:

```yaml
steps:
  - uses: actions/checkout@v4

  - uses: neokapi/setup-kapi@v1
```

The action downloads the correct binary for the runner platform (Linux, macOS, or Windows), verifies its SHA-256 checksum, and adds it to `PATH`. The built-in workflow token covers public release downloads, so no `token` input is required. On subsequent runs, the binary is restored from cache.

### Action Inputs

| Input        | Description                                                | Default  |
| ------------ | ---------------------------------------------------------- | -------- |
| `version`    | CLI version (e.g. `1.1.0` or `latest`)                     | `latest` |
| `plugins`    | Comma or newline-separated plugin refs to install, as the registry names them (`''` to install nothing) | `bowrain` |
| `auth-token` | Bowrain server JWT (exported as `BOWRAIN_AUTH_TOKEN`)      | `""`     |
| `server`     | Bowrain server URL (exported as `BOWRAIN_SERVER_URL`)      | `""`     |

### Action Outputs

| Output      | Description                      |
| ----------- | -------------------------------- |
| `version`   | Installed version (e.g. `1.1.0`) |
| `cache-hit` | Whether the plugin cache was hit |

## Recommended: Catch up with `kapi-action`

The simplest CI pattern uses two actions together:

- [`neokapi/setup-kapi`](https://github.com/neokapi/setup-kapi) — installs kapi (the bowrain plugin is included by default)
- [`neokapi/kapi-action`](https://github.com/neokapi/kapi-action) — runs a `kapi` command (here, `kapi up`) and commits translations

```yaml
name: Catch up translations

on:
  workflow_dispatch:
  push:
    branches: [main]
    paths:
      - "src/locales/en/**"

permissions:
  contents: write

jobs:
  up:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - uses: neokapi/kapi-action@v1
        id: up
        with:
          command: up

      - name: Summary
        if: steps.up.outputs.committed == 'true'
        run: echo "Translations committed at ${{ steps.up.outputs.commit-sha }}"
```

With `command: up` (the default), the action runs `kapi up` — the kapi loop on the server (push → catch up → pull) — then checks for changes, commits, and pushes. A run that **caught up** (`converged` — every gated scope cleared its ship gate) commits the produced translations; a run that **parked** (work remains that needs a person) commits what did catch up and annotates the parked locales; a **failed** run exits non-zero and commits nothing. It sets outputs you can use in subsequent steps:

| Output           | Description                                                        |
| ---------------- | ------------------------------------------------------------------ |
| `status`         | `success`, `no-changes`, or `failed`                               |
| `outcome`        | With `command: up`: `converged` or `parked`                        |
| `passes`         | With `command: up`: how many reconciliation passes the run took    |
| `parked-locales` | With `command: up`: comma-separated locales still short of their gate |
| `committed`      | `true` if a commit was created                                     |
| `commit-sha`     | SHA of the created commit                                          |

### kapi-action Inputs

| Input            | Default                                 | Description                              |
| ---------------- | --------------------------------------- | ---------------------------------------- |
| `command`        | `up`                                    | The `kapi` command to run                |
| `args`           | `""`                                    | Additional arguments                     |
| `project`        | `""`                                    | Path to the `.kapi` recipe (`-p` flag)   |
| `fail-on-parked` | `false`                                 | With `command: up`, fail the workflow when the run parks instead of committing partial progress |
| `commit`         | `true`                                  | Whether to commit changes                |
| `commit-message` | `chore: update translations via kapi`   | Commit message                           |
| `git-user-name`  | `Kapi Bot`                              | Git committer name                       |
| `git-user-email` | `bot@kapi.dev`                          | Git committer email                      |
| `paths`          | `""` (all changes)                      | Space-separated paths to stage for commit |

:::note
The workflow needs `permissions: contents: write` for the action to push commits.
:::

## Example: Ship Gate on Pull Request

Gate pull requests on the project's release bar whenever localization files
change. `kapi check --ship` runs the project's bound quality gates (brand,
terminology, QA) plus its ship/source coverage gates, and exits `3` — failing
the job — when any gate is unmet:

```yaml
name: Ship gate

on:
  pull_request:
    paths:
      - "src/locales/**"
      - "*.kapi"
      - ".kapi/**"

jobs:
  ship-gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1

      - name: Enforce the ship gates
        run: kapi check --ship
```

Ordinary builds never fail on target-language drift — a locale that is behind
is pending work, not an error. `check --ship` is the explicit, opt-in
enforcement point.

## Example: Push source on Push to Main

Send source changes to Bowrain Cloud when they land on `main`. Push is pure
transport; with `server.converge: on-push` the server catches the project up on its own clock.
Use `kapi up` instead of `kapi push` if you want CI to watch the run and commit
the results back:

```yaml
name: Sync Translations

on:
  push:
    branches: [main]
    paths:
      - "src/locales/**"

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - name: Push to Bowrain Cloud
        run: kapi push -m "Sync from CI (${GITHUB_SHA::7})"
```

The `auth-token` and `server` inputs export `BOWRAIN_AUTH_TOKEN` and `BOWRAIN_SERVER_URL` as environment variables, which the CLI picks up automatically.

## Example: Scheduled catch-up

Catch up on a schedule (e.g. nightly) to keep target locales up to date.
`kapi-action` runs `kapi up` and handles the commit, so no manual git plumbing
is needed:

```yaml
name: Nightly catch-up

on:
  schedule:
    - cron: "0 2 * * *" # 2 AM UTC

permissions:
  contents: write

jobs:
  up:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - uses: neokapi/kapi-action@v1
```

To translate specific files ad hoc instead of catching a project up, `kapi
translate` takes explicit inputs: `kapi translate src/locales/en/app.json
--target-lang fr` (an AI provider key such as `ANTHROPIC_API_KEY` must be set
for a CI run that produces translations).

## Example: Pull and Merge Server Changes

Pull translations from Bowrain Cloud and open a PR:

```yaml
name: Pull Translations

on:
  workflow_dispatch:
  schedule:
    - cron: "0 8 * * 1" # Monday 8 AM UTC

jobs:
  pull:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - name: Pull from Bowrain Cloud
        run: kapi pull

      - name: Create PR if changed
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          git diff --quiet && exit 0
          BRANCH="bowrain/pull-translations-$(date +%Y%m%d)"
          git checkout -b "${BRANCH}"
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add -A
          git commit -m "chore: pull translations from Bowrain Cloud"
          git push -u origin "${BRANCH}"
          gh pr create \
            --title "Pull translations from Bowrain Cloud" \
            --body "Automated pull of latest translations from Bowrain Cloud."
```

## Authentication

kapi supports two authentication methods in CI:

| Method                   | How                                    | Best For                                |
| ------------------------ | -------------------------------------- | --------------------------------------- |
| **Environment variable** | Set `BOWRAIN_AUTH_TOKEN`               | GitHub Actions (via `auth-token` input) |
| **Device flow**          | Run `kapi auth login` interactively | Local development                       |

The `auth-token` input on the setup action is the simplest approach — it exports the token as `BOWRAIN_AUTH_TOKEN`, which the CLI checks before looking for stored credentials.

### Generating a CI Token

Create an API token using kapi:

```bash
kapi auth login                               # authenticate with Bowrain Cloud
kapi auth token create --name "CI" --expire-days 90
```

The token (`bwt_...`) is shown once — store it immediately as a GitHub Actions secret:

```bash
gh secret set BOWRAIN_AUTH_TOKEN --repo your-org/your-repo
```

You can list and revoke tokens with `kapi auth token list` and `kapi auth token delete`.

## Plugins

The `plugins` input defaults to `bowrain` — the plugin that provides sync, push, and pull. List refs (as the registry names them) to add others alongside it, or pass `''` to install nothing:

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    plugins: |
      bowrain
      okapi-bridge
```

Plugins are cached between runs. The cache key includes a hash of the plugin list, so changes to the list trigger a fresh install.

## Pinning Versions

Pin the CLI version to avoid surprises from new releases:

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    version: "1.1.0"
```

Use `latest` (the default) for workflows where you always want the newest release.

## Related

- [CLI Overview](/cli/overview)
- [Flow Hooks](/cli/flows/hooks)
- [kapi up](/cli/commands/up) — run the kapi loop on the server (push → catch up → pull)
- [kapi push](/cli/commands/push) and [kapi pull](/cli/commands/pull)
- [kapi auth](/cli/commands/auth)
- [Source Language Preparation](/cli/use-cases/source-prep) — QA on source content in CI
