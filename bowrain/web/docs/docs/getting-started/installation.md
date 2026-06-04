---
sidebar_position: 2
title: Installation
slug: /installation
---

# Installation

Bowrain runs as a **server** your team connects to — from the web app, the desktop app, or, for a local codebase, the kapi CLI. Use the hosted service at [bowrain.cloud](https://bowrain.cloud), or run your own (see [For developers → Self-hosting](/server/installation)). Already have content in a CMS, Figma, or a git host? Connect those **server-side** — see [Connectors](/server/connectors); no install needed.

## The web app

The web editor is served by your Bowrain server — there is nothing to install. Open [bowrain.cloud](https://bowrain.cloud) (or your own server's URL) in a browser and sign in.

## Bowrain Desktop

A native cross-platform editor that connects to the same server, with offline support.

### Homebrew (macOS)

```bash
brew install --cask neokapi/tap/bowrain
```

### Binary Downloads

Download from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **macOS**: DMG (Apple Silicon)
- **Windows**: signed installer — `bowrain-X.Y.Z-windows-amd64-setup.exe` or `-arm64-setup.exe`
- **Linux**: tarball (amd64, arm64)

## Connect with kapi (the CLI plugin)

To sync a local codebase, install the bowrain plugin for the [`kapi`](https://neokapi.github.io/web/neokapi/docs/getting-started/installation) CLI — there is no separate `bowrain` binary. Once installed, run every bowrain command as `kapi <command>` (e.g. `kapi init`, `kapi push`, `kapi status`). This is the local-files/git connector — one of several ways content reaches Bowrain.

### Homebrew (macOS/Linux)

```bash
brew install neokapi/tap/bowrain-cli
```

### WinGet (Windows)

Install the `kapi` CLI, then add the bowrain plugin:

```powershell
winget install Neokapi.Kapi
kapi plugin install bowrain
```

### With kapi already installed

```bash
kapi plugin install bowrain
```

### Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/neokapi/neokapi/releases) page:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

### Verify

```bash
kapi version
```

## Kapi CLI (Standalone)

For standalone file processing without a server, install the [kapi CLI](https://neokapi.github.io/web/neokapi/docs/getting-started/installation) separately.

## Self-hosting

Prefer to run Bowrain yourself instead of using the hosted service? Installing the server, the Docker images, building from source, and configuration all live under [For developers → Self-hosting](/server/installation).

## Next Steps

- [Quick Start](/quickstart) — get content in from a connector or from kapi
- [Connectors](/server/connectors) — sync a CMS, design tool, or git host
- [Walkthrough](/walkthroughs/bowrain-getting-started) — the kapi developer path
