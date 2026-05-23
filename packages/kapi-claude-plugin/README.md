# kapi — Claude Code plugin

Keep your AI coding assistant on-brand and terminologically consistent, and publish content in every language and format. Bundles one `kapi` Agent Skill that drives the local [kapi](https://github.com/neokapi/neokapi) CLI.

## What's inside

A single `kapi` skill — a router `SKILL.md` that loads the relevant reference on
demand (progressive disclosure) — covering the loop:

> KNOW-BRAND → GENERATE → CHECK → FIX → PUBLISH

- **Brand voice** — load a voice guide, score a draft (0–100 + findings), rewrite what drifts.
- **Localize** — translate, enforce terminology, and round-trip into other languages and formats.
- **i18n setup** — add i18n to a project (the kapi-react stack, or a stack's existing catalogs).
- **Cloud governance (optional)** — shared brand profiles, project sync, and a reviewed termbase; the bowrain platform is one option for teams.

## The verify hook

The plugin also registers a `Stop` hook (`hooks/hooks.json` → `kapi hook stop`).
Inside a `.kapi` project, it runs the project's `kapi verify` gates — brand
voice, terminology, and translation QA — when the assistant tries to finish. If
a gate is failing it keeps the assistant working, with the findings to fix,
rather than letting it stop on off-brand or broken output. It fails open: outside
a project, or if verify cannot run, the assistant stops normally. The skill makes
the verify loop the default; this hook makes it a guarantee.

## Prerequisites

Install the `kapi` CLI (e.g. `brew install neokapi/tap/kapi`). The optional
cloud-governance path additionally needs the `kapi-bowrain` plugin and a bowrain
account.

## Generated, not hand-edited

The `skills/` directory is generated from the kapi binary
(`kapi skills export --dir skills`) so it stays byte-identical with the CLI's
embedded copies (the canonical source, `cli/skills/data/`). It is **gitignored**
and produced at release time — run `make plugin-bundle` at the repo root to
generate it locally.
