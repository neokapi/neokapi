# kapi — Claude Code plugin

Keep your AI coding assistant on-brand and terminologically consistent, and publish content in every language and format. The kapi skills use the local [kapi](https://github.com/neokapi/neokapi) CLI; the bowrain skills use the bowrain platform.

## What's inside

This plugin bundles Agent Skills that teach Claude Code to use kapi across the full loop:

> KNOW-BRAND → GENERATE → CHECK → FIX → PUBLISH → GOVERN

**kapi skills** (local CLI, offline):

- `kapi-brand-context` — load a brand voice guide into context before writing.
- `kapi-brand-check` — score text against a brand voice profile (0–100 + findings).
- `kapi-brand-fix` — rewrite text to fix forbidden/competitor terms and tone.
- `kapi-terminology` — build/look up/enforce a glossary (CSV/JSON/TBX).
- `kapi-translate` — brand-aware multilingual translation across many formats.
- `kapi-publish` — round-trip localization deliverables (DOCX/XLSX/PPTX/…).

**bowrain skills** (governed platform):

- `bowrain-brand-governance` — shared, versioned brand profiles + compliance scoring.
- `bowrain-project` — push/pull/sync content with a Bowrain server.
- `bowrain-terminology` — shared termbase with a review workflow.

## Prerequisites

Install the `kapi` CLI (e.g. `brew install neokapi/tap/kapi`). The `bowrain-*`
skills additionally need the `kapi-bowrain` plugin and a Bowrain account.

## Generated, not hand-edited

The `skills/` directory is generated from the kapi binary
(`kapi skills export --dir skills`) so it stays byte-identical with the CLI's
embedded copies. Run `make plugin-bundle` at the repo root to regenerate.
