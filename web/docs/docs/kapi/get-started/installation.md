---
sidebar_position: 4
title: Installation
description: Install the kapi CLI on macOS, Linux, or Windows via Homebrew or a direct binary download. Offline by default; no configuration needed to start.
keywords: [kapi, install, homebrew, binary download, macos, linux, windows]
---

# Installation

Once you've [tried kapi in the browser](/kapi/get-started/quickstart), install the `kapi`
CLI to run it locally and against your own files — a single self-contained
binary that runs offline by default.

## Homebrew (macOS/Linux)

```bash
brew install neokapi/tap/kapi-cli
```

## Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/neokapi/neokapi/releases) page:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## From source (Go developers)

Install the latest with Go:

```bash
go install github.com/neokapi/neokapi/kapi/cmd/kapi@latest
```

Or build the repository:

```bash
git clone https://github.com/neokapi/neokapi.git
cd neokapi
make build       # Build kapi CLI → bin/kapi
```

## Verify Installation

```bash
kapi version
```

## Add a provider credential (optional)

The rule-based commands — pseudo-translate, word-count, brand checks against a
profile file — need no credential. For LLM-backed translation, QA, and review,
save a provider key once under a name you'll reference in flows:

```bash
kapi credentials add my-openai --provider openai --api-key sk-…
kapi credentials list       # see what's saved
```

Credentials live in your OS keychain. See the
[Quick Start](/kapi/get-started/quickstart) for what to run next.

## Kapi Desktop

For a visual interface, install Kapi Desktop alongside the CLI:

```bash
brew install --cask neokapi/tap/kapi
```

Or download the DMG/ZIP from [GitHub Releases](https://github.com/neokapi/neokapi/releases). See the [Kapi Desktop overview](/kapi/desktop/overview) for details.
