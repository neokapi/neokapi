---
sidebar_position: 2
title: Installation
---

# Installing Kapi CLI

Kapi is distributed as a single binary with no dependencies.

## macOS

### Homebrew (Recommended)

```bash
brew install gokapi/tap/kapi
```

### Binary Download

Download the latest release from [GitHub Releases](https://github.com/gokapi/gokapi/releases):

```bash
# Download for macOS (Apple Silicon)
curl -LO https://github.com/gokapi/gokapi/releases/latest/download/kapi-darwin-arm64.tar.gz
tar xzf kapi-darwin-arm64.tar.gz
sudo mv kapi /usr/local/bin/

# Download for macOS (Intel)
curl -LO https://github.com/gokapi/gokapi/releases/latest/download/kapi-darwin-amd64.tar.gz
tar xzf kapi-darwin-amd64.tar.gz
sudo mv kapi /usr/local/bin/
```

## Linux

### Binary Download

```bash
# Download for Linux (x86_64)
curl -LO https://github.com/gokapi/gokapi/releases/latest/download/kapi-linux-amd64.tar.gz
tar xzf kapi-linux-amd64.tar.gz
sudo mv kapi /usr/local/bin/

# Download for Linux (ARM64)
curl -LO https://github.com/gokapi/gokapi/releases/latest/download/kapi-linux-arm64.tar.gz
tar xzf kapi-linux-arm64.tar.gz
sudo mv kapi /usr/local/bin/
```

### Package Managers

Coming soon: apt, yum, snap packages.

## Windows

### Binary Download

Download the `.zip` file from [GitHub Releases](https://github.com/gokapi/gokapi/releases):

```powershell
# Download for Windows (x86_64)
curl -LO https://github.com/gokapi/gokapi/releases/latest/download/kapi-windows-amd64.zip
# Extract and add to PATH
```

### Package Managers

Coming soon: Chocolatey, Scoop packages.

## Go Install

If you have Go 1.23+ installed:

```bash
go install github.com/gokapi/gokapi/bowrain/cmd/kapi@latest
```

Make sure `~/go/bin` is in your `PATH`:

```bash
export PATH="$HOME/go/bin:$PATH"
```

## Docker

Run Kapi in a container:

```bash
docker run --rm -v $(pwd):/workspace ghcr.io/gokapi/kapi:latest flow list
```

Create an alias for convenience:

```bash
alias kapi='docker run --rm -v $(pwd):/workspace ghcr.io/gokapi/kapi:latest'
```

## Verify Installation

```bash
kapi --version
kapi --help
```

## Next Steps

- [Quick Start](/docs/getting-started/quickstart)
- [Project Walkthrough](/docs/getting-started/project-walkthrough)
- [Initialize a Project](/docs/kapi-cli/commands/init)
