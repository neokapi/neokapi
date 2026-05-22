---
sidebar_position: 2
title: Using kapi with Claude
---

# Using kapi with Claude

kapi connects to Claude — and other AI coding assistants — in two complementary
ways:

- **Agent skills** teach Claude Code when and how to call the `kapi` CLI. This is
  the primary path: it works offline, needs no server, and covers the loop from
  keeping generated content on-brand to publishing it in other languages.
- **An [MCP server](/kapi-cli/mcp)** exposes structured tools that any
  MCP-compatible assistant (Claude, Cursor, Windsurf, Copilot) can call.

This page covers the skills path. The two can be used together.

## Prerequisites

- The `kapi` CLI on your `PATH` — check with `kapi version`. See
  [installation](/getting-started/installation).
- For LLM-backed checks and translation, a saved AI provider credential
  (`kapi credentials add`). The rule-based checks need no credential.

## Install the skills

List the bundled skills and install them into a project:

```bash
kapi skills list
kapi skills install                 # writes ./.claude/skills/<name>/SKILL.md
kapi skills install --target user   # ~/.claude/skills, for every project
```

Claude Code discovers the files in `.claude/skills/` and invokes the matching
skill when a task fits its description. You don't call skills by name.

## What the skills do

The `kapi-*` skills drive the local CLI. Each is one coherent capability with a
distinct trigger; together they cover the loop — know the brand, generate, check,
fix, publish:

| Skill | When Claude uses it | What it runs |
| --- | --- | --- |
| `kapi-brand` | keep content on-brand: load the voice guide, score a draft, rewrite what drifts | `kapi brand guide` / `check` / `rewrite` |
| `kapi-localize` | translate, enforce terminology, and round-trip into other languages and formats | `kapi run ai-translate-qa`, `kapi termbase`, `kapi extract` / `merge` |
| `kapi-i18n` | add i18n to a project | the kapi-react stack, or a stack's existing catalogs |

The `bowrain` skill covers the governed platform workflow — shared brand profiles,
project sync, and a reviewed termbase — for teams using the bowrain platform.

## A worked session

A typical exchange in Claude Code:

> Write a short release note for the new export feature, on brand.

Claude loads the voice guide, drafts the note, then checks it:

```bash
kapi brand guide --pack marketing-blog
echo "$DRAFT" | kapi brand check --pack marketing-blog --text - --json
```

```json
{
  "profile": "Marketing Blog",
  "score": 82,
  "passed": true,
  "findings": [
    { "severity": "minor", "message": "Forbidden term \"utilize\" found", "suggestion": "Use \"use\" instead" }
  ]
}
```

It rewrites the flagged wording and shows you the result:

```bash
echo "$DRAFT" | kapi brand rewrite --pack marketing-blog --text - --json
```

When you ask for other languages, it translates and runs QA:

> Ship it in French and German.

```bash
kapi run ai-translate-qa -i RELEASE.md --target-lang fr
kapi run ai-translate-qa -i RELEASE.md --target-lang de
```

`--target-lang` is single-valued, so the assistant runs one command per locale.

## Gate it in CI

`kapi brand check --min-score` exits non-zero when the score is below the
threshold, so the same check the assistant runs in chat can gate a pull request:

```bash
kapi brand check RELEASE.md --pack marketing-blog --min-score 90 --json
```

The exit code is distinct from an operational error, so a CI step can tell a
failed gate apart from a crash.

## The plugin bundle

The skills are also packaged as a Claude Code plugin under
`packages/kapi-claude-plugin` for marketplace distribution. `kapi skills install`
is the simplest way to add them to a single project or to your user account.

## Related

- [MCP server](/kapi-cli/mcp) — structured tools for any MCP client.
- [Brand voice](/features/brand-voice) — the profile model the checks use.
- [Using kapi with React](/kapi-react/introduction) — zero-wrapper i18n.
