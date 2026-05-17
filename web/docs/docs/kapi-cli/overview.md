---
sidebar_position: 1
title: Overview
---

# Kapi CLI

Kapi is a standalone command-line tool for file-based localization tasks — format conversion, pseudo-translation, word counting, quality checks, and AI translation. It operates directly on files without requiring a project, server, or configuration.

## What is Kapi?

Kapi is a standalone localization toolkit that:

- Processes files directly (no project initialization needed)
- Runs translation flows with AI, MT, TM, and QA tools
- Supports 15+ file formats with automatic detection
- Extends via gRPC plugins (including 40+ Okapi bridge filters)

## Key Commands

```bash
# List supported formats
kapi formats

# Count words in a file
kapi word-count messages.json

# Pseudo-translate for UI testing
kapi pseudo-translate messages.json --target-lang fr

# Translate with AI
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# Run a composed multi-tool flow
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

# List presets
kapi presets list

# Manage plugins
kapi plugins list
```

## When to Use Kapi

Use Kapi CLI when you:

- **Process individual files** — translate, pseudo-translate, count words
- **Need quick results** without project setup or configuration
- **Run in CI/CD** for automated file processing
- **Evaluate formats** — list supported formats, check file compatibility

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
