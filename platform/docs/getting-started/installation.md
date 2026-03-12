---
sidebar_position: 2
title: Installation
slug: /installation
---

# Installation

## Bowrain CLI

### Homebrew (macOS/Linux)

```bash
brew install gokapi/tap/bowrain-cli
```

### Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/gokapi/gokapi/releases) page:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

### Go Install

```bash
go install github.com/gokapi/gokapi/bowrain-cli/cmd/bowrain@latest
```

### Verify

```bash
bowrain version
```

## Bowrain Desktop

### Homebrew (macOS)

```bash
brew install --cask gokapi/tap/bowrain
```

### Binary Downloads

Download from [GitHub Releases](https://github.com/gokapi/gokapi/releases):
- macOS (universal DMG)
- Linux (amd64, arm64)
- Windows (amd64, arm64)

## Bowrain Server

### Docker (Recommended)

```bash
docker pull ghcr.io/gokapi/bowrain-server:latest
docker run -p 8080:8080 ghcr.io/gokapi/bowrain-server:latest
```

For production deployments, see [Self-Hosting](/bowrain/server/self-hosting).

### Building from Source

```bash
git clone https://github.com/gokapi/gokapi.git
cd gokapi
make build-bowrain      # Bowrain CLI → bin/bowrain
make build-server      # Bowrain Server → bin/bowrain-server
make build-all         # All binaries
```

## Kapi CLI (Standalone)

For standalone file processing without a server, install the [kapi CLI](/docs/getting-started/installation) separately.

## Next Steps

- [Quick Start](/bowrain/quickstart)
- [Walkthrough](/bowrain/walkthrough)
