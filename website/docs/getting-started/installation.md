---
sidebar_position: 2
title: Installation
---

# Installation

Install the `kapi` CLI for standalone file processing.

## Homebrew (macOS/Linux)

```bash
brew install gokapi/tap/kapi
```

## Go Install

```bash
go install github.com/gokapi/gokapi/kapi/cmd/kapi@latest
```

## Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/gokapi/gokapi/releases) page:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## Building from Source

```bash
git clone https://github.com/gokapi/gokapi.git
cd gokapi
make build       # Build kapi CLI → bin/kapi
```

## Verify Installation

```bash
kapi version
```

## Bowrain Platform

For the Bowrain platform (Bowrain CLI, desktop app, server), see [Bowrain Installation](/bowrain/installation).
