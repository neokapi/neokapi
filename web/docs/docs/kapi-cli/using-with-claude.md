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

## Install the skill

Install the `kapi` skill into a project (or your user account):

```bash
kapi skills install                 # writes ./.claude/skills/kapi/
kapi skills install --target user   # ~/.claude/skills/kapi/, for every project
```

Claude Code discovers the skill in `.claude/skills/` and invokes it when a task
fits its description — you don't call it by name. The skill is one router that
loads the relevant section on demand (progressive disclosure), so it covers the
whole workflow without bloating context.

## What it does

The `kapi` skill drives the local CLI across the loop — know the brand, generate,
check, fix, publish:

| Capability | When Claude uses it | What it runs |
| --- | --- | --- |
| Brand voice | keep content on-brand: load the voice guide, score a draft, rewrite what drifts | `kapi brand guide` / `check` / `rewrite` |
| Localize | translate, enforce terminology, and round-trip into other languages and formats | `kapi run ai-translate-qa`, `kapi termbase`, `kapi extract` / `merge` |
| i18n setup | add i18n to a project | the kapi-react stack, or a stack's existing catalogs |
| Cloud governance (optional) | shared brand profiles, project sync, a reviewed termbase | the bowrain platform, one option for teams |

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
