# kapi — Claude Code plugin

Keep your AI coding assistant on-brand and terminologically consistent, and publish content in every language and format. The kapi skills use the local [kapi](https://github.com/neokapi/neokapi) CLI; the bowrain skills use the bowrain platform.

## What's inside

This plugin bundles Agent Skills that teach Claude Code to use kapi across the full loop:

> KNOW-BRAND → GENERATE → CHECK → FIX → PUBLISH → GOVERN

**kapi skills** (local CLI, offline):

- `kapi-brand` — keep content on-brand: load the voice guide, score a draft (0–100 + findings), rewrite what drifts.
- `kapi-localize` — translate, enforce terminology, and round-trip into other languages and formats.
- `kapi-i18n` — add i18n to a project (the kapi-react stack, or a stack's existing catalogs).

**bowrain skill** (governed platform):

- `bowrain` — project sync, shared/versioned brand profiles with compliance scoring, and a reviewed termbase.

## Prerequisites

Install the `kapi` CLI (e.g. `brew install neokapi/tap/kapi`). The `bowrain`
skill additionally needs the `kapi-bowrain` plugin and a bowrain account.

## Generated, not hand-edited

The `skills/` directory is generated from the kapi binary
(`kapi skills export --dir skills`) so it stays byte-identical with the CLI's
embedded copies (the canonical source, `cli/skills/data/`). It is **gitignored**
and produced at release time — run `make plugin-bundle` at the repo root to
generate it locally.
