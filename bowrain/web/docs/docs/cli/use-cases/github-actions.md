---
title: GitHub Actions
sidebar_label: GitHub Actions
---

# Use Case: kapi in GitHub Actions

This guide shows how to use kapi (with the bowrain plugin) in GitHub Actions workflows for automated translation, quality checks, and server sync.

## Overview

The [`setup-kapi`](https://github.com/neokapi/setup-kapi) GitHub Action installs kapi on any runner and, through its `plugins` input, the bowrain plugin (`kapi-bowrain`). It handles platform detection, checksum verification, binary caching, and optional server authentication — so your workflow steps can focus on localization tasks.

## Setup

Add `neokapi/setup-kapi@v1` to your workflow:

```yaml
steps:
  - uses: actions/checkout@v4

  - uses: neokapi/setup-kapi@v1
    with:
      plugins: kapi-bowrain
```

The action downloads the correct binary for the runner platform (Linux, macOS, or Windows), verifies its SHA-256 checksum, and adds it to `PATH`. The built-in workflow token covers public release downloads, so no `token` input is required. On subsequent runs, the binary is restored from cache.

### Action Inputs

| Input        | Description                                                | Default  |
| ------------ | ---------------------------------------------------------- | -------- |
| `version`    | CLI version (e.g. `0.5.0` or `latest`)                     | `latest` |
| `plugins`    | Comma or newline-separated plugin refs to install          | `""`     |
| `auth-token` | Bowrain server JWT (exported as `BOWRAIN_AUTH_TOKEN`)      | `""`     |
| `server`     | Bowrain server URL (exported as `BOWRAIN_SERVER_URL`)      | `""`     |

### Action Outputs

| Output      | Description                      |
| ----------- | -------------------------------- |
| `version`   | Installed version (e.g. `0.5.0`) |
| `cache-hit` | Whether the plugin cache was hit |

## Recommended: Full Sync with `kapi-action`

The simplest CI pattern uses two actions together:

- [`neokapi/setup-kapi`](https://github.com/neokapi/setup-kapi) — installs kapi and the bowrain plugin (`kapi-bowrain`)
- [`neokapi/kapi-action`](https://github.com/neokapi/kapi-action) — runs a `kapi` command (here, `kapi sync`) and commits translations

```yaml
name: Sync Translations

on:
  workflow_dispatch:
  push:
    branches: [main]
    paths:
      - "src/locales/en/**"

permissions:
  contents: write

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          plugins: kapi-bowrain
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - uses: neokapi/kapi-action@v1
        id: sync
        with:
          command: sync

      - name: Summary
        if: steps.sync.outputs.committed == 'true'
        run: echo "Translations committed at ${{ steps.sync.outputs.commit-sha }}"
```

With `command: sync`, the action runs `kapi sync` (push → wait → pull), checks for changes, commits, and pushes. It sets outputs you can use in subsequent steps:

| Output       | Description                          |
| ------------ | ------------------------------------ |
| `status`     | `success`, `no-changes`, or `failed` |
| `committed`  | `true` if a commit was created       |
| `commit-sha` | SHA of the created commit            |

### kapi-action Inputs

| Input            | Default                                | Description                              |
| ---------------- | -------------------------------------- | ---------------------------------------- |
| `command`        | `run`                                  | The `kapi` command to run (use `sync`)   |
| `args`           | `""`                                   | Additional arguments                     |
| `project`        | `""`                                   | Path to the `.kapi` recipe (`-p` flag)   |
| `commit`         | `true`                                 | Whether to commit changes               |
| `commit-message` | `chore: sync translations via Bowrain` | Commit message                          |
| `git-user-name`  | `Bowrain Bot`                          | Git committer name                      |
| `git-user-email` | `bot@bowrain.cloud`                    | Git committer email                     |
| `paths`          | `i18n/ docs/ blog/`                    | Space-separated paths to stage for commit |

:::note
The workflow needs `permissions: contents: write` for the action to push commits.
:::

## Example: Translation on Pull Request

Run AI translation and quality checks whenever localization files change:

```yaml
name: Translate

on:
  pull_request:
    paths:
      - "src/locales/**"
      - "*.kapi"
      - ".kapi/**"

jobs:
  translate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          plugins: kapi-bowrain

      - name: Run translation flow
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: kapi ai-translate

      - name: Run QA checks
        run: kapi qa-check
```

## Example: Server Sync on Push to Main

Automatically push translations to Bowrain Cloud when changes land on `main`:

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
          plugins: kapi-bowrain
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://dev.bowrain.cloud

      - name: Push to Bowrain Cloud
        run: kapi push -m "Sync from CI (${GITHUB_SHA::7})"
```

The `auth-token` and `server` inputs export `BOWRAIN_AUTH_TOKEN` and `BOWRAIN_SERVER_URL` as environment variables, which the CLI picks up automatically.

## Example: Scheduled Translation

Run translation flows on a schedule (e.g. nightly) to keep target locales up to date:

```yaml
name: Nightly Translation

on:
  schedule:
    - cron: "0 2 * * *" # 2 AM UTC

jobs:
  translate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          plugins: kapi-bowrain

      - name: Run translation flow
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: kapi ai-translate

      - name: Commit translations
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add src/locales/
          git diff --cached --quiet || git commit -m "chore: update translations"
          git push
```

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
          plugins: kapi-bowrain
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

Install plugins by listing them in the `plugins` input. The bowrain plugin (`kapi-bowrain`) is required for sync, push, and pull; add any others alongside it:

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    plugins: |
      kapi-bowrain
      okapi-filters
      custom-tool
```

Plugins are cached between runs. The cache key includes a hash of the plugin list, so changes to the list trigger a fresh install.

## Pinning Versions

Pin the CLI version to avoid surprises from new releases:

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    plugins: kapi-bowrain
    version: "0.5.0"
```

Use `latest` (the default) for workflows where you always want the newest release.

## Related

- [CLI Overview](/cli/overview)
- [Flow Hooks](/cli/flows/hooks)
- [kapi sync](/cli/commands/sync) — push + wait + pull in one command
- [kapi push](/cli/commands/push) and [kapi pull](/cli/commands/pull)
- [kapi auth](/cli/commands/auth)
- [Source Language Preparation](/cli/use-cases/source-prep) — QA on source content in CI
</content>
</invoke>
