---
name: bowrain-brand-governance
description: Use an organization's GOVERNED brand voice — shared, versioned brand profiles hosted on a Bowrain server — to load the official voice guide, score content compliance (persisted for trends), and rewrite in voice. Use when a team needs one authoritative brand voice across people and projects, compliance tracking, or approval workflows, rather than a local profile. Triggers on "our org's brand voice", "workspace brand profile", "track brand compliance", "governed rewrite".
---

# bowrain-brand-governance

The governed, multi-user counterpart to the local `kapi-brand-*` skills. Brand voice profiles live on a Bowrain server (shared, versioned, with compliance score history and correction-learning) and are reached over the Bowrain MCP server / REST API.

## When to use

A team wants a single source of truth for brand voice across many people and projects, with persisted compliance scores, trends, and human approval — not a local one-off profile. For solo/offline work, use the local `kapi-brand-context` / `kapi-brand-check` / `kapi-brand-fix` skills instead; they share the same vocabulary, so this is a frictionless upgrade.

## Prerequisites

- The `kapi-bowrain` plugin installed (`kapi plugins install bowrain-cli`) and authenticated (`kapi login`, or `BOWRAIN_AUTH_TOKEN` in CI).
- The Bowrain MCP server configured as an MCP server for your assistant, OR use the plugin's commands.

## Governed brand tools (via the Bowrain MCP server)

- `list_profiles` — list the workspace's brand voice profiles.
- `get_voice_guide` — render the official guide (locale/channel overrides applied) to inject into context.
- `score_brand_compliance` — score text; the score is persisted for trend reporting.
- `check_vocabulary` — fast forbidden/competitor term check against the workspace vocabulary.
- `suggest_corrections` / `rewrite_in_voice` — governed rewrite; corrections feed the learning loop.
- Resources: `brand://profiles/{id}`, `brand://profiles/{id}/vocabulary`, `brand://terminology/{workspace}`.
- Prompts: `write_in_voice`, `rewrite_in_voice`, `check_draft`.

## How to apply

1. `get_voice_guide` to load the org voice before writing.
2. After drafting, `score_brand_compliance` to get a tracked score and findings.
3. `rewrite_in_voice` to fix issues; corrections are logged and improve future suggestions.
4. Compliance gates and trends are managed server-side (see the project's automations). For a local, offline equivalent, fall back to `kapi brand check --min-score`.
