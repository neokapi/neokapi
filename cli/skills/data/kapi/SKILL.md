---
name: kapi
description: Read, edit, and check the content inside any file format with the kapi CLI, then keep it on-brand and ship it in other languages. kapi parses formats an editor can't open directly — Word, PowerPoint, JSON, XLIFF, Markdown, HTML, YAML, config — into one content model; reads, searches, and rewrites the text in place and writes it back byte-for-byte with the format-aware toolbox (kcat/kgrep/ksed); runs deterministic and AI checks you loop on until they pass; keeps content on-brand against a voice profile; and localizes with terminology enforcement and multi-format publishing. Covers reading/searching/rewriting the text inside any supported format, creating a brand voice profile (from what you already know, sample content, or a linked website), brand checks (score a draft 0-100, rewrite off-voice text), translation with terminology enforcement and multi-format publishing, and adding i18n to a project. Use for any task involving reading or editing the content of documents your editor can't open directly (Word, PowerPoint, JSON, XLIFF), brand voice/tone, consistent terminology, translating or localizing content, or internationalizing a project. Triggers on "read/search/grep a document", "what does this .docx/.pptx say", "find and replace across .docx/.json/.xliff files", "rewrite text in a Word doc", "edit the content of a file my editor can't open", "check this content", "create/set up a brand voice", "on brand", "brand voice/tone", "forbidden/competitor terms", "translate", "localize to fr/de/ja", "multilingual", "glossary", "terminology", "internationalize", "add i18n", "publish in another language".
---

# kapi

kapi is an open, format-aware content engine you drive from the command line. It
parses any format it understands — Word, PowerPoint, JSON, XLIFF, Markdown, HTML,
YAML, config — into one content model, then reads, searches, edits, and checks
the text inside it and writes it back byte-for-byte. On top of that engine it
keeps AI-generated content on-brand and consistent, and ships it in other
languages and formats — offline, through the local `kapi` CLI. You do the
writing, editing, and translating; kapi handles the formats and the guardrails
(brand voice, terminology) and round-trips the result.

## Decide the scope first

Before reaching for a command, judge whether this is a one-off or ongoing work:

- **Ad hoc** — one file or a snippet, a one-time read, check, or edit,
  exploration, one or no target language. Just run the command; no setup. kapi
  works without a project.
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

- **Read, search, or rewrite content in any format** — print the prose of a file
  you can't open directly (Word, PowerPoint, JSON, XLIFF…), search it for a
  phrase, or apply a find-and-replace that leaves keys, tags, and styles intact,
  using the format-aware toolbox (`kcat`/`kgrep`/`ksed`). See
  [references/toolbox.md](references/toolbox.md).
- **Edit content in any format** — read a file's blocks (`kapi inspect --jsonl`),
  rewrite the text yourself, and write it back through the one write verb
  (`kapi apply`) — structure and inline codes preserved, no model. The
  deliberate, block-by-block edit loop (use `ksed` for a regex substitution). See
  [references/edit.md](references/edit.md).
- **Create / author content** — when you're writing the document, not editing a
  fixed source: author in a generative format, let kapi parse it as the first
  check, then gate on brand + terminology and revise. See
  [references/create.md](references/create.md).
- **Keep content on-brand** — create a brand voice profile, load its guide before
  writing, score a draft (0–100), and fix off-voice text yourself — routed
  through `kapi apply`. (`kapi brand rewrite` swaps forbidden/competitor terms
  offline; for tone and phrasing, rewrite the text yourself against the guide.)
  See [references/brand.md](references/brand.md).
- **Translate, enforce terminology, publish** — translate content into other
  languages and round-trip it back into its original format, with a glossary for
  consistency. Translate it yourself, but route it **through kapi** (extract →
  translate → merge) and then verify — don't hand-translate files and write them
  back, or terminology, placeholders, and format go unchecked. A provider is only
  needed for unattended runs. See [references/localize.md](references/localize.md).

  Across all of these, do the writing/editing/translating yourself and route it
  through kapi — don't reach for a provider. The provider-backed modes
  (`kapi translate`, the optional `--ai` checks) are for unattended runs only;
  kapi never sends content to a model to rewrite it.
- **Add i18n to a project** — set up the kapi-react stack for React apps, or plug
  kapi into the catalogs another stack already uses. See
  [references/i18n.md](references/i18n.md).

## Prerequisites

- The `kapi` binary on PATH (`kapi version`).
- No AI provider credential is required when you write, edit, or translate the
  text yourself within kapi's guardrails — including editing through `kapi apply`,
  which applies your edits with no model. A saved credential
  (`kapi credentials add`) is only needed for kapi to call a provider directly —
  unattended translation (`kapi translate`) or the optional `--ai` checks. The
  rule-based brand and terminology checks need no credential.

The English source text is always the key — don't introduce message IDs.
