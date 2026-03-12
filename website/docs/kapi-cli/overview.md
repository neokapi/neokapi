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

# Run a translation flow
kapi flow run ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# List available tools
kapi tools

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

For project management, server sync, and team collaboration, use the [Bowrain CLI](/bowrain/cli/overview) instead.

## Installation

```bash
# macOS/Linux
brew install neokapi/tap/kapi

# Go install
go install github.com/neokapi/neokapi/kapi/cmd/kapi@latest

# Binary downloads: https://github.com/neokapi/neokapi/releases
```

## Next Steps

- [Formats](/docs/kapi-cli/commands/formats)
- [Flow Command](/docs/kapi-cli/commands/flow)
- [Pseudo-Translation](/docs/kapi-cli/commands/pseudo-translate)
- [Word Count](/docs/kapi-cli/commands/word-count)
- [Terminology](/docs/kapi-cli/commands/termbase)
- [Translation Memory](/docs/kapi-cli/commands/tm)
- [Plugins](/docs/kapi-cli/commands/plugins)

### Use Cases

- [Terminology QA](/docs/kapi-cli/use-cases/terminology-qa) — catch terminology mistakes in translated files
- [Pre-Translation with Terminology](/docs/kapi-cli/use-cases/terminology-pretranslation) — combine TM, AI, and your glossary for consistent translations
