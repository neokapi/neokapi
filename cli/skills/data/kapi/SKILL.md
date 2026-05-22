---
name: kapi
description: Keep an AI coding assistant's output on-brand and terminologically consistent, and ship it in other languages and formats, using the kapi CLI. Covers creating a brand voice profile (from what you already know, sample content, or a linked website), brand checks (score a draft 0-100, rewrite off-voice text), translation with terminology enforcement and multi-format publishing, and adding i18n to a project. Use for any task involving brand voice/tone, consistent terminology, translating or localizing content, or internationalizing a project. Triggers on "create/set up a brand voice", "on brand", "brand voice/tone", "forbidden/competitor terms", "translate", "localize to fr/de/ja", "multilingual", "glossary", "terminology", "internationalize", "add i18n", "publish in another language". For team or cloud governance, bowrain is one option.
---

# kapi

kapi is the open engine that keeps AI-generated content on-brand and consistent,
and ships it in other languages and formats — offline, through the local `kapi`
CLI. Read the section that matches the task:

- **Keep content on-brand** — create a brand voice profile (from what you know,
  sample content, or a linked website), load its guide before writing, score a
  draft (0–100), and rewrite text that drifts off-voice. See
  [references/brand.md](references/brand.md).
- **Translate, enforce terminology, publish** — translate content into other
  languages and round-trip it back into its original format, with a glossary for
  consistency. See [references/localize.md](references/localize.md).
- **Add i18n to a project** — set up the kapi-react stack for React apps, or plug
  kapi into the catalogs another stack already uses. See
  [references/i18n.md](references/i18n.md).
- **Team or cloud governance (optional)** — shared, versioned brand profiles,
  project sync, and a reviewed termbase. bowrain is one option. See
  [references/bowrain.md](references/bowrain.md).

## Prerequisites

- The `kapi` binary on PATH (`kapi version`).
- A saved AI provider credential (`kapi credentials add`) for LLM-backed
  translation and the optional `--ai` checks. The rule-based brand and
  terminology checks need no credential.

The English source text is always the key — don't introduce message IDs.
