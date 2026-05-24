---
sidebar_position: 4
title: Installation
---

# Installation

Once you've [tried kapi in the browser](/get-started/try-it), install the `kapi`
CLI to run it locally and against your own files — a single self-contained
binary that runs offline by default.

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

## Add a provider credential (optional)

The rule-based commands — pseudo-translate, word-count, brand checks against a
profile file — need no credential. For LLM-backed translation, QA, and review,
save a provider key once under a name you'll reference in flows:

```bash
kapi credentials add my-openai --provider openai --api-key sk-…
kapi credentials list       # see what's saved
```

Credentials live in your OS keychain. See the
[Quick Start](/get-started/quickstart) for what to run next.

## Kapi

For a visual interface, install Kapi alongside the CLI:

```bash
brew install --cask neokapi/tap/kapi-desktop
```

Or download the DMG/ZIP from [GitHub Releases](https://github.com/neokapi/neokapi/releases). See the [Kapi overview](/desktop/overview) for details.
