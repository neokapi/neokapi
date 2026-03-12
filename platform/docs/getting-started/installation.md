---
sidebar_position: 2
title: Installation
slug: /installation
---

# Installation

## Bowrain CLI

### Homebrew (macOS/Linux)

```bash
brew install neokapi/tap/bowrain-cli
```

### Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/neokapi/neokapi/releases) page:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

### Go Install

```bash
go install github.com/neokapi/neokapi/bowrain-cli/cmd/bowrain@latest
```

### Verify

```bash
bowrain version
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

For production deployments, see [Self-Hosting](/bowrain/server/self-hosting).

### Building from Source

```bash
git clone https://github.com/neokapi/neokapi.git
cd neokapi
make build-bowrain      # Bowrain CLI → bin/bowrain
make build-server      # Bowrain Server → bin/bowrain-server
make build-all         # All binaries
```

## Kapi CLI (Standalone)

For standalone file processing without a server, install the [kapi CLI](/docs/getting-started/installation) separately.

## Next Steps

- [Quick Start](/bowrain/quickstart)
- [Walkthrough](/bowrain/walkthrough)
