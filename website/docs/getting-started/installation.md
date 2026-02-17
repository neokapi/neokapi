---
sidebar_position: 2
title: Installation
---

# Installation

## Homebrew (macOS/Linux)

Install the `kapi` CLI via Homebrew:

```bash
brew install gokapi/tap/kapi
```

Install the Bowrain desktop app (macOS):

```bash
brew install --cask gokapi/tap/bowrain
```

## Go Install

If you have Go installed:

```bash
go install github.com/gokapi/gokapi/bowrain/cmd/kapi@latest
```

## Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/gokapi/gokapi/releases) page:

- **kapi CLI** — Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64)
- **Bowrain desktop app** — macOS (universal DMG), Linux (amd64), Windows (amd64)

## Verify Installation

```bash
kapi version
```

## Building from Source

```bash
git clone https://github.com/gokapi/gokapi.git
cd gokapi
make build       # Build kapi CLI → bin/kapi
make build-all   # Build all binaries
```

For the Bowrain desktop app:

```bash
cd bowrain/apps/bowrain
wails3 build
```
