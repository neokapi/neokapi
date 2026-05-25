---
name: kapi
description: Keep an AI coding assistant's output on-brand and terminologically consistent, and ship it in other languages and formats, using the kapi CLI. Covers creating a brand voice profile (from what you already know, sample content, or a linked website), brand checks (score a draft 0-100, rewrite off-voice text), translation with terminology enforcement and multi-format publishing, and adding i18n to a project. Use for any task involving brand voice/tone, consistent terminology, translating or localizing content, or internationalizing a project. Triggers on "create/set up a brand voice", "on brand", "brand voice/tone", "forbidden/competitor terms", "translate", "localize to fr/de/ja", "multilingual", "glossary", "terminology", "internationalize", "add i18n", "publish in another language". For team or cloud governance, bowrain is one option.
---

# kapi

kapi is the open engine that keeps AI-generated content on-brand and consistent,
and ships it in other languages and formats — offline, through the local `kapi`
CLI. You do the writing and translating; kapi handles the formats and the
guardrails (brand voice, terminology) and round-trips the result.

## Decide the scope first

Before reaching for a command, judge whether this is a one-off or ongoing work:

- **Ad hoc** — one file or a snippet, a one-time check, exploration, one or no
  target language. Just run the command; no setup. kapi works without a project.
- **Project** — many files or a whole app; the same target locales repeatedly; a
  brand voice or glossary that must stay consistent; recurring work (CI,
  re-translate on change); translation memory you want to reuse. Bind that
  context **once** in a `.kapi` project, then issue plain requests — kapi applies
  the project's locales, content, brand voice, and glossary with no flags.

If a `.kapi` project already exists (kapi walks up from the cwd to find it), use
it. If the task is project-shaped and there's no project, offer to set one up;
don't impose a project on a genuine one-off. See
[references/project.md](references/project.md).

## Verify before you call it done

**The task is not done until `kapi verify` passes.** Writing or translating the
files is not the finish line — a clean verify is. Don't trust a single pass of your
own output: in a project, run `kapi verify` after writing or translating content. It
checks the work against the project's gates — brand voice score, terminology, and
translation QA (placeholders intact, nothing left untranslated) — and prints the
specific findings. Fix what it flags and run it again, until it passes (exit 0). kapi
is the gate; keep iterating until it's green. (The kapi Claude Code plugin also wires
this in as a Stop hook, so a failing gate keeps you working automatically.)

```bash
kapi verify --json        # whole project; or: kapi verify <files> [--brand|--terms|--qa]
```

## Then read the section that matches the task

- **Keep content on-brand** — create a brand voice profile, load its guide before
  writing, score a draft (0–100), and rewrite text that drifts off-voice. See
  [references/brand.md](references/brand.md).
- **Translate, enforce terminology, publish** — translate content into other
  languages and round-trip it back into its original format, with a glossary for
  consistency. Translate it yourself, but route it **through kapi** (extract →
  translate → merge) and then verify — don't hand-translate files and write them
  back, or terminology, placeholders, and format go unchecked. A provider is only
  needed for unattended runs. See [references/localize.md](references/localize.md).
- **Add i18n to a project** — set up the kapi-react stack for React apps, or plug
  kapi into the catalogs another stack already uses. See
  [references/i18n.md](references/i18n.md).
- **Team or cloud governance (optional)** — shared, versioned brand profiles,
  project sync, and a reviewed termbase. bowrain is one option. See
  [references/bowrain.md](references/bowrain.md).

## Prerequisites

- The `kapi` binary on PATH (`kapi version`).
- No AI provider credential is required when you translate or rewrite the text
  yourself within kapi's guardrails. A saved credential (`kapi credentials add`)
  is only needed for kapi to call a provider directly — unattended translation
  (`kapi ai-translate`) or the optional `--ai` checks. The rule-based brand and
  terminology checks need no credential.

The English source text is always the key — don't introduce message IDs.
