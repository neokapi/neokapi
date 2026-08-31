---
sidebar_position: 5
title: Installation
slug: /installation
description: Bowrain runs as a server your team connects to. Use the hosted service at bowrain.cloud or run your own. Most routes into a workspace need nothing installed locally.
---

# Installation

Bowrain runs as a **server** your team connects to. Use the hosted service at
[bowrain.cloud](https://bowrain.cloud), or run your own (see
[For developers: self-hosting](/server/installation)).

Most routes into a workspace need **nothing installed**: content in a content
platform, a design tool, or a repository connects server-side; see
[Connectors](/server/connectors). This page covers the pieces you do install:
the desktop app, and the kapi CLI for the [developer
route](/server/connectors/kapi).

:::tip[Beta channel]
During a release-candidate phase the **beta channel** carries the freshest
build. The beta and stable packages install the same binary on different
update tracks. Pick one; they are mutually exclusive.
:::

## The web app

The web app is served by your Bowrain server; there is nothing to install.
Open [app.bowrain.cloud](https://app.bowrain.cloud) (or your own server's URL)
in a browser and sign in.

## Bowrain Desktop

A native cross-platform client that connects to the same server, with offline
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
Direct downloads for **Bowrain Desktop 1.2.0-rc13**:

**macOS** (Apple Silicon)
- **macOS arm64 (.dmg)** — [`bowrain-1.2.0-rc13-macOS-arm64.dmg`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc13/bowrain-1.2.0-rc13-macOS-arm64.dmg)

**Windows** (Authenticode-signed, portable zip)
- **Windows amd64** — [`bowrain-1.2.0-rc13-windows-amd64.zip`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc13/bowrain-1.2.0-rc13-windows-amd64.zip)
- **Windows arm64** — [`bowrain-1.2.0-rc13-windows-arm64.zip`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc13/bowrain-1.2.0-rc13-windows-arm64.zip)

**Linux**
- **Linux amd64 (tar.gz)** — [`bowrain-1.2.0-rc13-linux-amd64.tar.gz`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc13/bowrain-1.2.0-rc13-linux-amd64.tar.gz)
- **Linux arm64 (tar.gz)** — [`bowrain-1.2.0-rc13-linux-arm64.tar.gz`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc13/bowrain-1.2.0-rc13-linux-arm64.tar.gz)

Verify a download against [`checksums.txt`](https://github.com/neokapi/neokapi/releases/download/bowrain-v1.2.0-rc13/checksums.txt).
<!-- END:downloads-bowrain-desktop -->

## The kapi CLI (the developer route)

To connect a local codebase, install the bowrain plugin for the
[`kapi`](https://neokapi.github.io/kapi/get-started/installation)
CLI; there is no separate `bowrain` binary. Once installed, run every bowrain
command as `kapi <command>` (for example `kapi init`, `kapi push`, `kapi up`).
This is the [kapi connector](/server/connectors/kapi), one of several ways
content reaches Bowrain.

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
version, in CI or to hold a project on a known build, install by version or
pin it in the recipe's `plugins:` map (see
[plugins](/cli/commands/plugins)):

```bash
kapi plugin install bowrain@<version>
```

### Verify

```bash
kapi version
kapi plugin list
```

## Self-hosting

Prefer to run Bowrain yourself instead of using the hosted service? Installing
the server, the Docker images (`ghcr.io/neokapi/bowrain-server`, `-worker`,
`-web`), building from source, and configuration all live under
[For developers: self-hosting](/server/installation). Keep the server, worker,
and web images on the **same version tag**.

## Next steps

- [Quick start](/quickstart): get content in, from your systems or from a codebase
- [Keeping content caught up](/the-loop): how a project stays current
- [Connectors](/server/connectors): sync a CMS, design tool, or git host
- [Walkthrough](/walkthroughs/bowrain-getting-started): the kapi developer path
