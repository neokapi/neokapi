# Bowrain Example: Docusaurus

[![Bowrain Sync](https://github.com/gokapi/bowrain-example-docusaurus/actions/workflows/bowrain-sync.yml/badge.svg)](https://github.com/gokapi/bowrain-example-docusaurus/actions/workflows/bowrain-sync.yml)
[![Translation Progress](https://img.shields.io/endpoint?url=https%3A%2F%2Fdev.bowrain.cloud%2Fapi%2Fv1%2Fbadges%2Fprojects%2FPROJECT_ID&label=bowrain)](https://dev.bowrain.cloud)
[![Locales](https://img.shields.io/endpoint?url=https%3A%2F%2Fdev.bowrain.cloud%2Fapi%2Fv1%2Fbadges%2Fprojects%2FPROJECT_ID%3Ftype%3Dlocales)](https://dev.bowrain.cloud)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow)](./LICENSE)

A complete example showing [Bowrain](https://dev.bowrain.cloud) localization for a [Docusaurus](https://docusaurus.io) site. This project is connected to the live Bowrain server at `dev.bowrain.cloud`.

## What's included

| File | Purpose |
|------|---------|
| `.bowrain/config.yaml` | Project config with file mappings and automation rules |
| `.bowrain/flows/qa-check.yaml` | QA flow (runs as pre-push gate) |
| `.github/workflows/bowrain-sync.yml` | GitHub Actions workflow for continuous translation |
| `i18n/en/code.json` | English source strings (24 keys) |
| `i18n/nb/code.json` | Norwegian translations (AI-generated) |
| `i18n/qps/code.json` | Pseudo-translations (for testing) |

## Quick start

```bash
# Clone and install
git clone https://github.com/gokapi/bowrain-example-docusaurus.git
cd bowrain-example-docusaurus
npm install

# Preview in English
npm start

# Preview pseudo-translated
npm start -- --locale qps

# Preview Norwegian
npm start -- --locale nb
```

## Bowrain workflow

```bash
# Initialize (if starting fresh)
bowrain init --anonymous --name "Acme Docs" --source en

# Pseudo-translate for layout testing
bowrain flow run pseudo-translate

# Sync with Bowrain Server (push → translate → pull)
bowrain sync --timeout 5m

# Check project status
bowrain status
```

## Automation

The `.bowrain/config.yaml` defines two automation rules:

- **`pre-push`**: Runs QA checks before content reaches the server
- **`post-push`**: Waits for AI translation and pulls results automatically

The GitHub Actions workflow (`.github/workflows/bowrain-sync.yml`) uses
[`gokapi/setup-bowrain`](https://github.com/gokapi/setup-bowrain) to sync
translations automatically on every push to `main`.

## Learn more

- [Bowrain Walkthrough](https://gokapi.github.io/docs/bowrain/walkthrough) — full tutorial using this project
- [Bowrain CLI](https://gokapi.github.io/docs/bowrain-cli/overview) — CLI reference
- [GitHub Actions](https://gokapi.github.io/docs/bowrain-cli/use-cases/github-actions) — CI/CD patterns

## License

MIT
