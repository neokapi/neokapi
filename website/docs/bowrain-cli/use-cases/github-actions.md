---
title: GitHub Actions
sidebar_label: GitHub Actions
---

# Use Case: Bowrain CLI in GitHub Actions

This guide shows how to use the Bowrain CLI in GitHub Actions workflows for automated translation, quality checks, and server sync.

## Overview

The [`setup-bowrain`](https://github.com/gokapi/setup-bowrain) GitHub Action installs the Bowrain CLI on any runner. It handles platform detection, checksum verification, binary caching, and optional server authentication — so your workflow steps can focus on localization tasks.

## Setup

Add `gokapi/setup-bowrain@v1` to your workflow:

```yaml
steps:
  - uses: actions/checkout@v4

  - uses: gokapi/setup-bowrain@v1
    with:
      token: ${{ secrets.GOKAPI_REGISTRY_TOKEN }}
```

The action downloads the correct binary for the runner platform (Linux, macOS, or Windows), verifies its SHA-256 checksum, and adds it to `PATH`. On subsequent runs, the binary is restored from cache.

### Action Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `version` | CLI version (e.g. `0.5.0` or `latest`) | `latest` |
| `token` | GitHub token with read access to `gokapi/gokapi` releases | — |
| `auth-token` | Bowrain server JWT (exported as `BOWRAIN_AUTH_TOKEN`) | `""` |
| `server` | Bowrain server URL (exported as `BOWRAIN_SERVER_URL`) | `""` |
| `plugins` | Comma or newline-separated plugin refs to install | `""` |

### Action Outputs

| Output | Description |
|--------|-------------|
| `version` | Installed version (e.g. `0.5.0`) |
| `cache-hit` | Whether the plugin cache was hit |

## Example: Translation on Pull Request

Run AI translation and quality checks whenever localization files change:

```yaml
name: Translate

on:
  pull_request:
    paths:
      - "src/locales/**"
      - ".bowrain/**"

jobs:
  translate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: gokapi/setup-bowrain@v1
        with:
          token: ${{ secrets.GOKAPI_REGISTRY_TOKEN }}

      - name: Run translation flow
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: bowrain flow run ai-translate

      - name: Run QA checks
        run: bowrain flow run qa-check
```

## Example: Server Sync on Push to Main

Automatically push translations to Bowrain Server when changes land on `main`:

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

      - uses: gokapi/setup-bowrain@v1
        with:
          token: ${{ secrets.GOKAPI_REGISTRY_TOKEN }}
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://bowrain.example.com

      - name: Push to Bowrain Server
        run: bowrain push -m "Sync from CI (${GITHUB_SHA::7})"
```

The `auth-token` and `server` inputs export `BOWRAIN_AUTH_TOKEN` and `BOWRAIN_SERVER_URL` as environment variables, which the CLI picks up automatically.

## Example: Scheduled Translation

Run translation flows on a schedule (e.g. nightly) to keep target locales up to date:

```yaml
name: Nightly Translation

on:
  schedule:
    - cron: "0 2 * * *"  # 2 AM UTC

jobs:
  translate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: gokapi/setup-bowrain@v1
        with:
          token: ${{ secrets.GOKAPI_REGISTRY_TOKEN }}

      - name: Run translation flow
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: bowrain flow run ai-translate

      - name: Commit translations
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add src/locales/
          git diff --cached --quiet || git commit -m "chore: update translations"
          git push
```

## Example: Pull and Merge Server Changes

Pull translations from Bowrain Server and open a PR:

```yaml
name: Pull Translations

on:
  workflow_dispatch:
  schedule:
    - cron: "0 8 * * 1"  # Monday 8 AM UTC

jobs:
  pull:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: gokapi/setup-bowrain@v1
        with:
          token: ${{ secrets.GOKAPI_REGISTRY_TOKEN }}
          auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
          server: https://bowrain.example.com

      - name: Pull from Bowrain Server
        run: bowrain pull

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
          git commit -m "chore: pull translations from Bowrain Server"
          git push -u origin "${BRANCH}"
          gh pr create \
            --title "Pull translations from Bowrain Server" \
            --body "Automated pull of latest translations from Bowrain Server."
```

## Authentication

The Bowrain CLI supports two authentication methods in CI:

| Method | How | Best For |
|--------|-----|----------|
| **Environment variable** | Set `BOWRAIN_AUTH_TOKEN` | GitHub Actions (via `auth-token` input) |
| **Device flow** | Run `bowrain auth login` interactively | Local development |

The `auth-token` input on the setup action is the simplest approach — it exports the token as `BOWRAIN_AUTH_TOKEN`, which the CLI checks before looking for stored credentials.

### Generating a CI Token

1. Log in to your Bowrain Server dashboard
2. Navigate to **Settings > API Tokens**
3. Create a token with the required scopes
4. Store it as a GitHub Actions secret (e.g. `BOWRAIN_AUTH_TOKEN`)

## Plugins

Install plugins by listing them in the `plugins` input:

```yaml
- uses: gokapi/setup-bowrain@v1
  with:
    token: ${{ secrets.GOKAPI_REGISTRY_TOKEN }}
    plugins: |
      okapi-filters
      custom-tool
```

Plugins are cached between runs. The cache key includes a hash of the plugin list, so changes to the list trigger a fresh install.

## Pinning Versions

Pin the CLI version to avoid surprises from new releases:

```yaml
- uses: gokapi/setup-bowrain@v1
  with:
    token: ${{ secrets.GOKAPI_REGISTRY_TOKEN }}
    version: "0.5.0"
```

Use `latest` (the default) for workflows where you always want the newest release.

## Related

- [Bowrain CLI Overview](/docs/bowrain-cli/overview)
- [Flow Hooks](/docs/bowrain-cli/flows/hooks)
- [bowrain push](/docs/bowrain-cli/commands/push) and [bowrain pull](/docs/bowrain-cli/commands/pull)
- [bowrain auth](/docs/bowrain-cli/commands/auth)
