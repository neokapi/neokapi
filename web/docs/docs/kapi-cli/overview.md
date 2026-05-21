---
sidebar_position: 1
title: Overview
---

# Kapi CLI

Kapi is a standalone command-line tool that keeps content **on-brand and
terminologically consistent**, then **localizes it into every language and
format**. It operates directly on files without requiring a project, server, or
configuration, and runs offline by default.

## What is Kapi?

Kapi does two jobs from one engine:

- **Brand governance for AI output** — load a brand voice profile, score text
  0–100, and rewrite content that drifts off-voice. Wire it into your AI coding
  assistant over [MCP](/kapi-cli/mcp) so generation stays on-brand.
- **Format-aware localization** — AI translation, MT, TM leverage, terminology
  enforcement, QA, and pseudo-translation across 30+ native formats plus the
  Okapi bridge filters, with automatic format detection (see the
  [Format Reference](/formats)).

It processes files directly (no project initialization needed) and extends via
crash-isolated gRPC plugins, including the Okapi bridge to the Java filters.

## Key Commands

```bash
# --- Brand voice ---
# Print a brand voice guide to inject into your AI assistant
kapi brand guide --pack friendly-dtc

# Score text against a profile; --min-score gates CI (exit code 3)
kapi brand check --profile-file brand.yaml --min-score 80 release-notes.md

# Rewrite off-voice content
kapi brand rewrite --profile-file brand.yaml --text "Leverage our solution"

# Serve brand + terminology tools to your AI assistant over MCP
kapi mcp

# --- Localization ---
# List supported formats
kapi formats

# Count words in a file
kapi word-count messages.json

# Pseudo-translate for UI testing
kapi pseudo-translate messages.json --target-lang fr

# Translate with AI
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# Run a composed multi-tool flow (brand-voice-aware when a profile is bound)
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr

# List available tools and flows
kapi tools
kapi flows

# Manage terminology
kapi termbase import terms.csv --format csv -s en -t fr
kapi termbase lookup "authentication module" -s en -t fr

# Manage translation memory
kapi tm import translations.tmx --name project-tm -s en -t fr
kapi tm lookup "Welcome to our platform" -s en -t fr

# List presets and plugins
kapi presets list
kapi plugins list
```

## When to Use Kapi

Use Kapi CLI when you:

- **Keep AI output on-brand** — score and fix content against a voice profile
- **Localize content** — translate, pseudo-translate, count words, run QA
- **Need quick results** without project setup or configuration
- **Run in CI/CD** — gate a build on a brand score or QA check
- **Work offline** — a single binary with embedded TM and termbase

For a visual interface, use [Kapi](/kapi-desktop/overview) — the GUI companion for building flows, managing plugins, and running tools with live progress.

## Installation

```bash
# macOS/Linux
brew install neokapi/tap/kapi

# Go install
go install github.com/neokapi/neokapi/kapi/cmd/kapi@latest

# Binary downloads: https://github.com/neokapi/neokapi/releases
```

## Project Files

Kapi can optionally use Kapi project files to save workflow configurations. Use `-p` to reference a project:

```bash
# Run a flow from a Kapi project
kapi run translate -p myproject.kapi

# Override project defaults with CLI flags
kapi run translate -p myproject.kapi --target-lang de
```

See [Kapi Project Files](/kapi-desktop/project-file) for the full format reference.

## Kapi

[Kapi](/kapi-desktop/overview) is the GUI companion for kapi. It provides:

- Visual flow editor for building tool pipelines
- Live flow execution with progress visualization
- Plugin browser and installer
- AI credential management with OS keychain storage
- File browser with format auto-detection

Install via Homebrew:

```bash
brew install --cask neokapi/tap/kapi-desktop
```

## Next Steps

- [Brand Voice](/features/brand-voice) — profiles, scoring, and enforcement
- [Using Kapi with AI Assistants](/kapi-cli/mcp) — wire kapi into Claude Code, Cursor, and more
- [Formats](/kapi-cli/commands/formats)
- [Run Command](/kapi-cli/commands/flow)
- [Pseudo-Translation](/kapi-cli/commands/pseudo-translate)
- [Word Count](/kapi-cli/commands/word-count)
- [Terminology](/kapi-cli/commands/termbase)
- [Translation Memory](/kapi-cli/commands/tm)
- [Plugins](/kapi-cli/commands/plugins)

### Use Cases

- [Terminology QA](/kapi-cli/use-cases/terminology-qa) — catch terminology mistakes in translated files
- [Pre-Translation with Terminology](/kapi-cli/use-cases/terminology-pretranslation) — combine TM, AI, and your glossary for consistent translations
