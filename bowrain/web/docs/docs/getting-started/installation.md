---
sidebar_position: 2
title: Installation
slug: /installation
---

# Installation

Bowrain runs as a **server** your team connects to — from the web app, the
desktop app, or, for a local codebase, the kapi CLI. Use the hosted service at
[bowrain.cloud](https://bowrain.cloud), or run your own (see
[For developers → Self-hosting](/server/installation)). Already have content in
a CMS, Figma, or a git host? Connect those **server-side** — see
[Connectors](/server/connectors); no install needed.

:::tip[Beta channel]
During a release-candidate phase the **beta channel** carries the freshest
build. The beta and stable packages install the same binary on different
update tracks — pick one; they are mutually exclusive.
:::

## The web app

The web editor is served by your Bowrain server — there is nothing to install.
Open [bowrain.cloud](https://bowrain.cloud) (or your own server's URL) in a
browser and sign in.

## Bowrain Desktop

A native cross-platform editor that connects to the same server, with offline
support.

### Homebrew (macOS)

```bash
brew install --cask neokapi/tap/bowrain@beta   # beta channel
brew install --cask neokapi/tap/bowrain        # stable
```

### Direct downloads

The links below always point at the release named in them; they are regenerated
on every release from the assets actually attached to it.

<!-- BEGIN:downloads-bowrain-desktop -->
Direct downloads for **Bowrain Desktop 1.2.0-rc8**:

**macOS** (Apple Silicon)
- **macOS arm64 (.dmg)** — [`bowrain-1.2.0-rc8-macOS-arm64.dmg`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc8/bowrain-1.2.0-rc8-macOS-arm64.dmg)

**Windows** (Authenticode-signed, portable zip)
- **Windows amd64** — [`bowrain-1.2.0-rc8-windows-amd64.zip`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc8/bowrain-1.2.0-rc8-windows-amd64.zip)
- **Windows arm64** — [`bowrain-1.2.0-rc8-windows-arm64.zip`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc8/bowrain-1.2.0-rc8-windows-arm64.zip)

**Linux**
- **Linux amd64 (tar.gz)** — [`bowrain-1.2.0-rc8-linux-amd64.tar.gz`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc8/bowrain-1.2.0-rc8-linux-amd64.tar.gz)
- **Linux arm64 (tar.gz)** — [`bowrain-1.2.0-rc8-linux-arm64.tar.gz`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc8/bowrain-1.2.0-rc8-linux-arm64.tar.gz)

Verify a download against [`checksums.txt`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc8/checksums.txt).
<!-- END:downloads-bowrain-desktop -->

## Connect with kapi (the CLI plugin)

To sync a local codebase, install the bowrain plugin for the
[`kapi`](https://neokapi.github.io/web/neokapi/kapi/get-started/installation)
CLI — there is no separate `bowrain` binary. Once installed, run every bowrain
command as `kapi <command>` (e.g. `kapi init`, `kapi push`, `kapi up`). This is
the local-files/git connector — one of several ways content reaches Bowrain.

### Homebrew (macOS/Linux)

Installs the kapi CLI together with the bowrain plugin:

```bash
brew install neokapi/tap/bowrain-cli-beta   # beta channel
brew install neokapi/tap/bowrain-cli        # stable
```

### WinGet (Windows)

Install the `kapi` CLI, then add the bowrain plugin:

```powershell
winget install Neokapi.KapiCli
kapi plugin install bowrain
```

### With kapi already installed

```bash
kapi plugin install bowrain
```

Plugin builds are attached to the same GitHub release and indexed in the plugin
registry; kapi verifies their signatures on install. To pin a specific plugin
version — in CI, or to hold a project on a known build — install by version or
pin it in the recipe's `plugins:` map (see
[plugins](/cli/commands/plugins)):

```bash
kapi plugin install bowrain@<version>
```

### Verify

```bash
kapi version
kapi plugins list
```

## Self-hosting

Prefer to run Bowrain yourself instead of using the hosted service? Installing
the server, the Docker images (`ghcr.io/neokapi/bowrain-server`, `-worker`,
`-web`), building from source, and configuration all live under
[For developers → Self-hosting](/server/installation). Keep the server, worker,
and web images on the **same version tag**.

## Next steps

- [Quick start](/quickstart) — get content in, from your systems or from a codebase
- [The kapi loop on Bowrain](/the-loop) — how a connected project stays caught up
- [Connectors](/server/connectors) — sync a CMS, design tool, or git host
- [Walkthrough](/walkthroughs/bowrain-getting-started) — the kapi developer path
