---
sidebar_position: 2
title: Installation
slug: /installation
---

# Installation

Bowrain runs as a **server** your team connects to — from the web app, the desktop app, or, for a local codebase, the kapi CLI. Already have content in a CMS, Figma, or a git host? Connect those **server-side** — see [Connectors](/server/connectors); no install needed.

## Bowrain Server

### Docker (Recommended)

```bash
docker pull ghcr.io/neokapi/bowrain-server:latest
docker run -p 8080:8080 ghcr.io/neokapi/bowrain-server:latest
```

For production deployments, see [Self-Hosting](/server/self-hosting).

### Building from Source

```bash
git clone https://github.com/neokapi/neokapi.git
cd neokapi
make build                   # kapi CLI → bin/kapi
make build-bowrain-plugin    # kapi-bowrain plugin → bin/kapi-bowrain
make build-server            # Bowrain Server → bin/bowrain-server
make build-all               # All binaries
```

The **web editor** is served by `bowrain-server` — there is no separate install; open the server URL in a browser.

## Bowrain Desktop

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

## Next Steps

- [Quick Start](/quickstart) — get content in from a connector or from kapi
- [Connectors](/server/connectors) — sync a CMS, design tool, or git host
- [Walkthrough](/walkthroughs/bowrain-getting-started) — the kapi developer path
