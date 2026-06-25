---
sidebar_position: 8
title: Release Process
---

# Release Process

Bowrain releases on its **own track**, independent of the kapi CLI. It is
**tag-driven**: pushing a `bowrain-vX.Y.Z` tag to `neokapi/neokapi` triggers
[`.github/workflows/release-bowrain.yml`](https://github.com/neokapi/neokapi/blob/main/.github/workflows/release-bowrain.yml),
which builds and publishes everything except the Windows binaries — those are
produced as CI artifacts and signed locally on a Mac, then added to the
published release.

The kapi CLI and Kapi Desktop release separately on plain `vX.Y.Z` tags
(`release.yml`); the two tracks have non-overlapping tag prefixes and never
trigger each other. Because both build from the shared framework + `cli`
modules, cut a `bowrain-v*` release from a commit whose `kapi-bowrain` plugin is
protocol-compatible with a released kapi — they need not be the same commit, but
keep them close.

This page is the bowrain-focused overview. The full maintainer runbook —
signing identities, secrets, and the Windows step — lives in
[`RELEASE.md`](https://github.com/neokapi/neokapi/blob/main/RELEASE.md).

## Prerequisites

- The `neokapi/homebrew-tap` repository exists with a `Casks/` directory
- The `HOMEBREW_TAP_TOKEN` secret is configured in the `neokapi/neokapi` repository settings (a GitHub PAT with write access to `neokapi/homebrew-tap`)
- For the Windows signing step: a Mac with **SimplySign Desktop** logged in (see the maintainer runbook)

## Cutting a release

```bash
make release-bowrain v=2.1.0          # pre-flight + annotated tag bowrain-v2.1.0 + push → CI builds & publishes
gh run watch                          # follow the release workflow
make release-bowrain-windows v=2.1.0  # after CI: sign the Windows .exe's locally and finalize
```

A leading `v` is tolerated: `v=2.1.0` tags `bowrain-v2.1.0`. `make release-bowrain`
guards that the tree is clean, you are on `main` and in sync with `origin/main`,
and the tag does not already exist. `release-bowrain.yml` additionally gates on
the parity workflow having passed for the tagged commit.

## What happens automatically

The tag push triggers `release-bowrain.yml`, which:

1. **Builds and publishes** the Bowrain desktop app (DMG for macOS, ZIP for
   Windows, tarball for Linux) and the `kapi-bowrain` plugin — for all platforms
   (linux/darwin/windows, amd64/arm64) — and creates the GitHub release with
   notes and `checksums.txt`. (The `kapi` CLI itself ships on the kapi track.)
2. **Signs macOS artifacts** in CI — the desktop `.app`/DMG is Developer ID
   signed and notarized.
3. **Signs the `kapi-bowrain` plugin** tarballs with cosign/Sigstore (supply-chain
   trust for the plugin registry).
4. **Publishes Docker images** (bowrain-server, bowrain-worker, bowrain-web,
   bowrain-keycloak), updates the `bowrain-cli` formula and `bowrain` cask in
   `neokapi/homebrew-tap`, and registers the plugin in `manifest-plugins.json`
   so `kapi plugin install bowrain` resolves to this build.

Windows binaries are emitted as workflow **artifacts**; `make release-windows`
signs them locally (Authenticode via the Certum certificate through SimplySign,
which only runs on a logged-in Mac), uploads them to the release, and refreshes
`checksums.txt`. Until that step runs, the release has macOS/Linux assets but no
Windows assets yet.

> There is no standalone `bowrain` binary — all bowrain commands run as
> `kapi <command>` once the `kapi-bowrain` plugin is installed.

## Verifying a release

```bash
gh release view bowrain-v2.1.0
gh release view bowrain-v2.1.0 --json assets -q '.assets[].name'

brew update
brew install --cask neokapi/tap/bowrain
kapi version
```

On macOS, the downloaded DMG should open with no Gatekeeper warning
(`spctl -a -t install -vvv <App>.dmg`).

## Cleaning up a failed release

```bash
gh release delete bowrain-v2.1.0 --yes
git push origin :refs/tags/bowrain-v2.1.0
git tag -d bowrain-v2.1.0
```

## Release checklist

- [ ] All CI checks pass on `main`
- [ ] Version tag follows semver with the bowrain prefix (`bowrain-v2.1.0`)
- [ ] Tag is annotated (`git tag -a`, which `make release-bowrain` does for you)
- [ ] Release workflow completes all jobs
- [ ] `make release-bowrain-windows` has uploaded the signed Windows assets
- [ ] GitHub release has all expected assets
- [ ] `brew install --cask neokapi/tap/bowrain` works and `kapi version` is correct
