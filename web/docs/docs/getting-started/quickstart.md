---
sidebar_position: 3
title: Quick Start
---

# Quick Start

Get started with kapi in 2 minutes. No project setup or server needed.

## Keep AI output on-brand

Print a brand voice guide you can paste into your AI assistant's context, or
inject automatically via [MCP](/kapi-cli/mcp). Start from a built-in starter
pack:

```bash
kapi brand guide --pack friendly-dtc
```

Score a draft against a profile. `--min-score` makes it a CI gate — kapi exits
with code 3 when the score is below the threshold:

```bash
kapi brand check --profile-file brand.yaml --min-score 80 release-notes.md
```

Input can be a file argument, `--text "…"`, or piped via stdin. Fix off-voice
content — forbidden terms, competitor mentions, jargon:

```bash
kapi brand rewrite --profile-file brand.yaml --text "Leverage our solution to drive synergy."
```

Five starter packs ship built in: `professional-b2b`, `friendly-dtc`,
`technical-docs`, `marketing-blog`, and `customer-support`. See
[Brand Voice](/features/brand-voice).

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

Translate a file using AI, then run a QA check in the same flow:

```bash
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr
```

When a brand voice profile is bound on the flow, its guide is injected into the
translation prompt — so output is on-brand at generation time.

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

## Use it inside your AI assistant

Expose brand and terminology tools over MCP so your assistant stays on-brand
while it writes:

```bash
kapi mcp
```

See [Using Kapi with AI Assistants](/kapi-cli/mcp) for Claude Code, Cursor, and
Windsurf setup.

## Next Steps

- [Brand Voice](/features/brand-voice) — profiles, scoring, and enforcement
- [Kapi CLI Overview](/kapi-cli/overview) — full command reference
- [Formats](/commands?id=formats) — supported file formats
- [Run Command](/commands?id=run) — composed flows and tool commands
- [Features](/features/formats) — detailed feature documentation
