# Release Process

This document describes how to create a release of gokapi.

## Prerequisites

- The `gokapi/homebrew-tap` repository exists with a `Casks/` directory
- The `HOMEBREW_TAP_TOKEN` secret is configured in the `gokapi/gokapi` repository settings (a GitHub PAT with write access to `gokapi/homebrew-tap`)
- GoReleaser configuration is in `.goreleaser.yaml`

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

The tag push triggers `.github/workflows/release.yml`, which runs these jobs in sequence:

1. **GoReleaser** — builds the `kapi` CLI for all platforms (linux/darwin/windows, amd64/arm64), creates the GitHub release with changelog, publishes checksums, and updates the Homebrew formula in `gokapi/homebrew-tap`

2. **Build Bowrain (macOS)** — builds a universal macOS binary, packages it as a DMG, uploads to the GitHub release

3. **Update Homebrew Cask** — downloads the DMG, computes SHA256, updates `Casks/bowrain.rb` in `gokapi/homebrew-tap`

4. **Build Bowrain (Windows)** — builds the Windows executable, creates a ZIP, uploads to the release

5. **Build Bowrain (Linux)** — installs GTK/WebKit deps, builds the Linux binary, creates a tarball, uploads to the release

## Verifying a Release

```bash
# Check the GitHub release
gh release view v0.1.0

# List release assets
gh release view v0.1.0 --json assets -q '.assets[].name'

# Test Homebrew CLI install
brew update
brew install gokapi/tap/kapi
kapi version

# Test Homebrew Cask install (macOS)
brew install --cask gokapi/tap/bowrain
```

## Testing Locally

Use GoReleaser's snapshot mode to test the build locally without publishing:

```bash
goreleaser release --snapshot --clean
```

This builds all artifacts in `dist/` without creating a GitHub release or updating Homebrew.

## Troubleshooting

### GoReleaser fails

- Check that `HOMEBREW_TAP_TOKEN` is set and has write access to `gokapi/homebrew-tap`
- Verify `.goreleaser.yaml` is valid: `goreleaser check`
- Ensure the tag follows semver: `v1.2.3`

### Bowrain build fails

- **macOS**: Ensure Wails CLI is compatible with the Go version
- **Linux**: The `libgtk-3-dev` and `libwebkit2gtk-4.1-dev` packages must be available
- **Windows**: Check that Node.js and Go are on PATH

### Homebrew cask update fails

- The cask job waits up to 5 minutes for the DMG asset to appear on the release
- If the macOS build is slow, the cask update may time out — re-run the job

### Cleaning up a failed release

```bash
# Delete the release
gh release delete v0.1.0 --yes

# Delete the tag remotely and locally
git push origin :refs/tags/v0.1.0
git tag -d v0.1.0
```

Then fix the issue and re-tag.

## Release Checklist

- [ ] All CI checks pass on `main`
- [ ] Version tag follows semver (`v0.1.0`, `v1.0.0`, etc.)
- [ ] CHANGELOG or commit messages reflect what's in the release
- [ ] Tag is annotated (`git tag -a`, not lightweight)
- [ ] Release workflow completes all jobs
- [ ] GitHub release has all expected assets:
  - `kapi_*_linux_amd64.tar.gz`
  - `kapi_*_linux_arm64.tar.gz`
  - `kapi_*_darwin_amd64.tar.gz`
  - `kapi_*_darwin_arm64.tar.gz`
  - `kapi_*_windows_amd64.zip`
  - `bowrain-*-macOS-universal.dmg`
  - `bowrain-*-windows-amd64.zip`
  - `bowrain-*-linux-amd64.tar.gz`
  - `checksums.txt`
- [ ] `brew install gokapi/tap/kapi` works
- [ ] `kapi version` shows the correct version
