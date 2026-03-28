---
sidebar_position: 3
title: Quick Start
---

# Quick Start

Get started with kapi in 2 minutes. No project setup or server needed.

## Pseudo-Translate a File

Generate pseudo-translations to test your UI for truncation and layout issues:

```bash
kapi pseudo-translate messages.json --target-lang qps -o messages-qps.json
```

Output: accented text like `[!!! Hëëëröööö Tîîîtlëëë !!!]` that exposes untranslated strings and layout problems.

## Count Words

Estimate translation costs:

```bash
kapi word-count messages.json
```

## Translate with AI

Translate a file using AI:

```bash
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr
```

## Explore Formats and Tools

```bash
# List all supported file formats
kapi formats

# List available processing tools
kapi tools

# List available flows
kapi flows

# List presets
kapi presets list
```

## Manage Terminology

```bash
# Import terms from CSV
kapi termbase import terms.csv --format csv -s en -t fr

# Look up terms in text
kapi termbase lookup "authentication module" -s en -t fr
```

## Next Steps

- [Kapi CLI Overview](/docs/kapi-cli/overview) — full command reference
- [Formats](/docs/kapi-cli/commands/formats) — supported file formats
- [Run Command](/docs/kapi-cli/commands/flow) — composed flows and tool commands
- [Features](/docs/features/formats) — detailed feature documentation
- [Bowrain Platform](/bowrain/introduction) — team collaboration with project management and server sync
