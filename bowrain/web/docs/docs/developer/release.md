---
sidebar_position: 8
title: Release Process
---

# Release Process

## Prerequisites

- The `neokapi/homebrew-tap` repository exists with a `Casks/` directory
- The `HOMEBREW_TAP_TOKEN` secret is configured in the `neokapi/neokapi` repository settings (a GitHub PAT with write access to `neokapi/homebrew-tap`)

## Creating a Release

1. **Ensure `main` is clean and CI passes:**

   ```bash
   git checkout main
   git pull
   gh run list --workflow=ci.yml --limit=1
   ```

2. **Tag the release:**

   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```

3. **Monitor the release workflow:**

   ```bash
   gh run list --workflow=release.yml
   gh run watch
   ```

## What Happens Automatically

The tag push triggers `.github/workflows/release.yml`, which runs these jobs:

1. **Build + publish** — builds the `kapi` CLI for all platforms (linux/darwin/windows, amd64/arm64), creates the GitHub release with notes, publishes checksums, and updates the Homebrew formulae in `neokapi/homebrew-tap`

2. **Build Bowrain** (matrix: linux/amd64, linux/arm64, windows/amd64, windows/arm64, darwin/universal) — builds the Bowrain desktop app for each platform. Each entry packages its artifact (DMG for macOS, ZIP for Windows, tarball for Linux) and uploads to the GitHub release

3. **Update Homebrew Cask** — downloads the macOS DMG, computes SHA256, updates `Casks/bowrain.rb` in `neokapi/homebrew-tap`

## Verifying a Release

```bash
gh release view v0.1.0
gh release view v0.1.0 --json assets -q '.assets[].name'

brew update
brew install --cask neokapi/tap/kapi
kapi version

brew install --cask neokapi/tap/bowrain
```

## Troubleshooting

### Release build fails

- Check that `HOMEBREW_TAP_TOKEN` is set and has write access to `neokapi/homebrew-tap`
- Ensure the tag follows semver: `v1.2.3`

### Bowrain build fails

- **macOS**: Ensure Wails CLI is compatible with the Go version
- **Linux**: The action auto-detects the Ubuntu version and installs the correct WebKit dev package
- **Windows**: The action handles Go and Wails setup automatically

### Cleaning up a failed release

```bash
gh release delete v0.1.0 --yes
git push origin :refs/tags/v0.1.0
git tag -d v0.1.0
```

## Release Checklist

- [ ] All CI checks pass on `main`
- [ ] Version tag follows semver (`v0.1.0`, `v1.0.0`, etc.)
- [ ] Tag is annotated (`git tag -a`, not lightweight)
- [ ] Release workflow completes all jobs
- [ ] GitHub release has all expected assets
- [ ] `brew install --cask neokapi/tap/kapi` works
- [ ] `kapi version` shows the correct version
