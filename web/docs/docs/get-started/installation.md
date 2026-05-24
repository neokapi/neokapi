---
sidebar_position: 2
title: Installation
---

# Installation

Install the `kapi` CLI to keep AI output on-brand and localize content — a
single self-contained binary that runs offline by default.

## Homebrew (macOS/Linux)

```bash
brew install neokapi/tap/kapi
```

## Go Install

```bash
go install github.com/neokapi/neokapi/kapi/cmd/kapi@latest
```

## Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/neokapi/neokapi/releases) page:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## Building from Source

```bash
git clone https://github.com/neokapi/neokapi.git
cd neokapi
make build       # Build kapi CLI → bin/kapi
```

## Verify Installation

```bash
kapi version
```

## Kapi

For a visual interface, install Kapi alongside the CLI:

```bash
brew install --cask neokapi/tap/kapi-desktop
```

Or download the DMG/ZIP from [GitHub Releases](https://github.com/neokapi/neokapi/releases). See the [Kapi overview](/desktop/overview) for details.
