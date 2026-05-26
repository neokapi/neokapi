---
sidebar_position: 1
title: Kapi CLI
description: kapi is a standalone command-line tool for format-aware localization and brand guardrails — AI translation, TM leverage, terminology enforcement, QA, and pseudo-translation across native and Okapi bridge formats.
keywords: [kapi, CLI, localization, brand guardrails, AI translation, pseudo-translate, word count, formats, tools]
---

# Kapi CLI

Kapi is a standalone command-line tool that keeps content **on-brand and
terminologically consistent**, then **localizes it into every language and
format**. It operates directly on files without requiring a project, server, or
configuration, and runs offline by default.

## What is Kapi?

Kapi does two jobs from one engine:

- **Brand guardrails for AI output** — load a brand voice profile, score text
  0–100, and rewrite content that drifts off-voice. Wire it into your AI coding
  assistant over [MCP](/reference/mcp) so generation stays on-brand.
- **Format-aware localization** — AI translation, MT, TM leverage, terminology
  enforcement, QA, and pseudo-translation across native localization, document,
  data, subtitle, and office formats, with more through the okapi-bridge and
  automatic format detection (see the [format reference](/formats)).

It processes files directly (no project initialization needed) and extends via
plugins, including the Okapi bridge to the Java filters.

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
kapi pseudo-translate messages.json

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
kapi plugin list
```

## Output Locations

Tools that write files (translation, pseudo-translation, source transforms)
choose an output path automatically, so most commands need no `-o`:

- **Locale-additive formats** (KLF) are updated **in place** — the target
  locale is added to the same file.
- **Other formats** (JSON, XLIFF, …) write a new file in a **locale-aware**
  location: the source locale in the input path is swapped for the target
  (`locales/en/app.json → locales/fr/app.json`), or, when the path carries no
  locale, the file lands under a `{lang}/` directory beside the input
  (`messages.json → fr/messages.json`).

Override the destination with `-o <path|template>` (placeholders: `{dir}`,
`{name}`, `{lang}`, `{ext}`) or `--output-dir DIR` to root outputs at
`DIR/{lang}/`. See the [command reference](/commands) for every flag.

## When to Use Kapi

Use Kapi CLI when you:

- **Keep AI output on-brand** — score and fix content against a voice profile
- **Localize content** — translate, pseudo-translate, count words, run QA
- **Need quick results** without project setup or configuration
- **Run in CI/CD** — gate a build on a brand score or QA check
- **Work offline** — a single binary with embedded TM and termbase

For a visual interface, use [Kapi Desktop](/kapi/desktop/overview) — the GUI companion for building flows, managing plugins, and running tools with live progress.

## Installation

See [Installation](/kapi/get-started/installation) for Homebrew and binary downloads.

## Project Files

Kapi can optionally use Kapi project files to save workflow configurations. Use `-p` to reference a project:

```bash
# Run a flow from a Kapi project
kapi run translate -p myproject.kapi

# Override project defaults with CLI flags
kapi run translate -p myproject.kapi --target-lang de
```

See [Kapi Projects](/reference/project-file) for the full format reference.

## Next Steps

- [Brand Voice](/framework/brand-voice) — profiles, scoring, and enforcement
- [Using Kapi with AI Assistants](/reference/mcp) — wire kapi into Claude Code, Cursor, and more
- [Formats](/commands?id=formats)
- [Run Command](/commands?id=run)
- [Pseudo-Translation](/commands?id=pseudo-translate)
- [Word Count](/commands?id=word-count)
- [Terminology](/commands?id=termbase)
- [Translation Memory](/commands?id=tm)
- [Plugins](/commands?id=plugin)

### Use Cases

- [Terminology QA](/kapi/recipes/terminology-qa) — catch terminology mistakes in translated files
- [Pre-translate with TM](/kapi/recipes/pre-translate-with-tm) — combine TM and your glossary for consistent translations
