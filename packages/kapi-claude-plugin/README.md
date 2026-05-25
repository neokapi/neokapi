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

## The hooks

The plugin registers two Claude Code hooks (`hooks/hooks.json`), both scoped to
the `.kapi` project in the session's working directory and both **fail-open** —
outside a project, or if they cannot run, the assistant proceeds normally.

- **`Stop` → `kapi hook stop`.** When the assistant tries to finish, it runs the
  project's `kapi verify` gates — brand voice, terminology, and translation QA —
  and, if a gate is failing, keeps the assistant working with the findings to fix
  rather than letting it stop on off-brand or broken output. The skill makes the
  verify loop the default; this hook makes it a guarantee.
- **`PreToolUse` (Edit/Write/MultiEdit) → `kapi hook pre-edit`.** Before a file
  is written, it checks whether that file is a path the project *generates* as a
  translation target (a `kapi merge` output). If so it **denies** the edit and
  tells the assistant to route the change through the round-trip
  (`kapi extract` → translate → `kapi merge` → `kapi verify`) or edit the source
  instead. A hand-edited target is overwritten on the next merge and skips the
  terminology, placeholder, and brand-voice gates; this turns the skill's
  "don't hand-translate files" guidance into a hard rule. Source files and files
  the project does not generate are never blocked.

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
