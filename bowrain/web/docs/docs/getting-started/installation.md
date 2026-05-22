---
sidebar_position: 2
title: Installation
slug: /installation
---

# Installation

## Bowrain CLI

Bowrain's CLI is the **`kapi-bowrain` plugin** for the [`kapi`](https://neokapi.github.io/web/neokapi/docs/getting-started/installation) CLI — there is no separate `bowrain` binary. Once installed, run every bowrain command as `kapi <command>` (e.g. `kapi init`, `kapi push`, `kapi status`). The Homebrew formula below depends on `kapi` and registers the plugin for you.

### Homebrew (macOS/Linux)

```bash
brew install neokapi/tap/bowrain-cli
```

### With kapi already installed

```bash
kapi plugins install bowrain
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

## Bowrain Desktop

### Homebrew (macOS)

```bash
brew install --cask neokapi/tap/bowrain
```

### Binary Downloads

Download from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

- macOS (universal DMG)
- Linux (amd64, arm64)
- Windows (amd64, arm64)

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

## Kapi CLI (Standalone)

For standalone file processing without a server, install the [kapi CLI](https://neokapi.github.io/web/neokapi/docs/getting-started/installation) separately.

## Next Steps

- [Quick Start](/quickstart)
- [Walkthrough](/walkthroughs/bowrain-getting-started)
