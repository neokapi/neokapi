---
sidebar_position: 1
title: Overview
---

# Kapi Desktop

Kapi Desktop is the visual companion for the [kapi CLI](/docs/kapi-cli/overview). It provides a native desktop interface for building localization flows, running tools, managing plugins, and storing AI credentials — all without writing YAML or remembering CLI flags.

## Features

- **Flow editor** — Build multi-tool pipelines visually. Chain AI translation, quality checks, pseudo-translation, and more into reusable flows.
- **Flow runner** — Execute flows with live progress visualization: per-file status, node highlighting, and timing.
- **Tool runner** — Run individual tools on files with dynamic configuration forms generated from tool schemas.
- **Plugin manager** — Browse the plugin registry, install, update, and manage plugins from a UI.
- **Credential vault** — Store AI provider API keys (Anthropic, OpenAI, Ollama) securely in your OS keychain.
- **Project files** — Save workflows as portable `.kapi` files you can share via git or open with double-click.

## Installation

### macOS (Homebrew)

```bash
brew install --cask neokapi/tap/kapi-desktop
```

This also installs the `kapi` CLI as a dependency.

### Manual Download

Download the latest release from [GitHub Releases](https://github.com/neokapi/neokapi/releases):

- **macOS**: `KapiDesktop-vX.Y.Z-arm64.dmg` (Apple Silicon) or `KapiDesktop-vX.Y.Z-amd64.dmg` (Intel)
- **Windows**: `KapiDesktop-vX.Y.Z-windows-amd64.zip`
- **Linux**: `KapiDesktop-vX.Y.Z-linux-amd64`

## Quick Start

1. **Launch Kapi Desktop** and click **New Project**
2. Set your source language (e.g., `en-US`) and target languages
3. Go to **Flows** and create a flow with the tools you need
4. Go to **Credentials** and add your AI provider API key
5. Select your flow, pick input files, and click **Run**

Your workflow is saved as a `.kapi` file that you can reopen, share, or run from the CLI:

```bash
kapi run translate -p myproject.kapi
```

## Kapi Desktop vs Bowrain Desktop

| | Kapi Desktop | Bowrain Desktop |
|---|---|---|
| Purpose | Standalone file processing | Platform-connected editing |
| Server | None required | Bowrain Server |
| Projects | `.kapi` files | `.bowrain/` directories |
| Collaboration | Share `.kapi` files via git | Real-time multi-user editing |
| Automation | Local flows | Server-side hooks + automation |
| Install | `brew install --cask kapi-desktop` | `brew install --cask bowrain` |

Kapi Desktop is for developers who want powerful localization tools without a server. When you need team collaboration, connector integrations, or server-side automation, upgrade to [Bowrain](/bowrain/).

## Next Steps

- [.kapi Project Files](/docs/kapi-desktop/project-file) — Full format reference
- [Getting Started](/docs/kapi-desktop/getting-started) — Step-by-step walkthrough
- [Kapi CLI](/docs/kapi-cli/overview) — Command-line reference
