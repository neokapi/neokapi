---
sidebar_position: 4
title: Installation
description: Install the kapi CLI on macOS, Linux, or Windows via Homebrew, WinGet, or a direct binary download. Offline by default; no configuration needed to start.
keywords: [kapi, install, homebrew, winget, binary download, macos, linux, windows]
---

# Installation

neokapi ships two artifacts you can install independently:

- the **`kapi` CLI** — a single self-contained binary that runs offline by
  default and operates directly on your files;
- **Kapi Desktop** — the visual companion app, which bundles the CLI.

The two sections below cover each. If you only want the command line, you need
the first section alone.

## Install the Kapi CLI

Once you've [tried kapi in the browser](/kapi/get-started/quickstart), install
the binary to run it locally against your own files.

### Homebrew (macOS/Linux)

```bash
brew install neokapi/tap/kapi-cli
```

### WinGet (Windows)

```powershell
winget install Neokapi.Kapi
```

### Binary Downloads

Pre-built binaries for all platforms are available on the [GitHub Releases](https://github.com/neokapi/neokapi/releases) page:

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

### From source (Go developers)

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

### Verify the install

```bash
kapi version
```

### Add a provider credential (optional)

The rule-based commands — pseudo-translate, word-count, brand checks against a
profile file — need no credential. For LLM-backed translation, QA, and review,
save a provider key once under a name you'll reference in flows:

```bash
kapi credentials add my-openai --provider openai --api-key sk-…
kapi credentials list       # see what's saved
```

Credentials live in your OS keychain. See the
[Quick Start](/kapi/get-started/quickstart) for what to run next.

## Install Kapi Desktop

Kapi Desktop is the visual companion to the CLI. Each package below installs the
`kapi` CLI as a dependency, so a single install covers both. See the
[Kapi Desktop overview](/kapi/desktop/overview) for what it does.

### macOS (Homebrew)

```bash
brew install --cask neokapi/tap/kapi
```

### Windows (installer)

Download and run the signed installer from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **amd64**: `kapi-desktop-X.Y.Z-windows-amd64-setup.exe`
- **arm64**: `kapi-desktop-X.Y.Z-windows-arm64-setup.exe`

The installer is Authenticode-signed and registers a Start-menu entry and uninstaller.

### Manual download (macOS, Linux)

Download the latest release from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **macOS**: `kapi-desktop-X.Y.Z-macOS-arm64.dmg` (Apple Silicon)
- **Linux**: `kapi-desktop-X.Y.Z-linux-amd64.tar.gz` or `kapi-desktop-X.Y.Z-linux-arm64.tar.gz`
