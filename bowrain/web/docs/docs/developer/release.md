---
sidebar_position: 8
title: Release Process
---

# Release Process

Bowrain and kapi share one **tag-driven** release: pushing a `vX.Y.Z` tag to
`neokapi/neokapi` triggers
[`.github/workflows/release.yml`](https://github.com/neokapi/neokapi/blob/main/.github/workflows/release.yml),
which builds and publishes everything except the Windows binaries — those are
produced as CI artifacts and signed locally on a Mac, then added to the
published release.

This page is the bowrain-focused overview. The full maintainer runbook —
signing identities, secrets, and the Windows step — lives in
[`RELEASE.md`](https://github.com/neokapi/neokapi/blob/main/RELEASE.md).

## Prerequisites

- The `neokapi/homebrew-tap` repository exists with a `Casks/` directory
- The `HOMEBREW_TAP_TOKEN` secret is configured in the `neokapi/neokapi` repository settings (a GitHub PAT with write access to `neokapi/homebrew-tap`)
- For the Windows signing step: a Mac with **SimplySign Desktop** logged in (see the maintainer runbook)

## Cutting a release

```bash
make release v=1.3.4          # pre-flight + annotated tag + push → CI builds & publishes
gh run watch                  # follow the release workflow
make release-windows v=1.3.4  # after CI: sign the Windows .exe's locally and finalize
```

A leading `v` is tolerated: `v=1.3.4` and `v=v1.3.4` both tag `v1.3.4`.
`make release` guards that the tree is clean, you are on `main` and in sync with
`origin/main`, and the tag does not already exist. `release.yml` additionally
gates on the parity workflow having passed for the tagged commit.

## What happens automatically

The tag push triggers `release.yml`, which:

1. **Builds and publishes** the Bowrain desktop app (DMG for macOS, ZIP for
   Windows, tarball for Linux), the `kapi` CLI, and the `kapi-bowrain` plugin —
   for all platforms (linux/darwin/windows, amd64/arm64) — and creates the
   GitHub release with notes and `checksums.txt`.
2. **Signs macOS artifacts** in CI — the desktop `.app`/DMG and the CLI binaries
   are Developer ID signed and notarized.
3. **Signs the `kapi-bowrain` plugin** tarballs with cosign/Sigstore (supply-chain
   trust for the plugin registry).
4. **Publishes Docker images** (bowrain-server, bowrain-worker), updates the
   Homebrew casks in `neokapi/homebrew-tap`, and updates the plugin registry.

Windows binaries are emitted as workflow **artifacts**; `make release-windows`
signs them locally (Authenticode via the Certum certificate through SimplySign,
which only runs on a logged-in Mac), uploads them to the release, and refreshes
`checksums.txt`. Until that step runs, the release has macOS/Linux assets but no
Windows assets yet.

> There is no standalone `bowrain` binary — all bowrain commands run as
> `kapi <command>` once the `kapi-bowrain` plugin is installed.

## Verifying a release

```bash
gh release view v1.3.4
gh release view v1.3.4 --json assets -q '.assets[].name'

brew update
brew install --cask neokapi/tap/bowrain
brew install --cask neokapi/tap/kapi
kapi version
```

On macOS, the downloaded DMG should open with no Gatekeeper warning
(`spctl -a -t install -vvv <App>.dmg`).

## Cleaning up a failed release

```bash
gh release delete v1.3.4 --yes
git push origin :refs/tags/v1.3.4
git tag -d v1.3.4
```

## Release checklist

- [ ] All CI checks pass on `main`
- [ ] Version tag follows semver (`v1.3.4`)
- [ ] Tag is annotated (`git tag -a`, which `make release` does for you)
- [ ] Release workflow completes all jobs
- [ ] `make release-windows` has uploaded the signed Windows assets
- [ ] GitHub release has all expected assets
- [ ] `brew install --cask neokapi/tap/bowrain` works and `kapi version` is correct
