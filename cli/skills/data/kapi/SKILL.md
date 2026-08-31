---
name: kapi
description: Hold and apply a project's content context — the terms, voice and rules it goes by — and read, edit, check and translate the content inside any file format with the kapi CLI. kapi parses formats an editor can't open directly — Word, PowerPoint, JSON, XLIFF, Markdown, HTML, YAML — into one content model; reads, searches, and compares the text (kcat/kgrep/ksed/kdiff); edits it in place through a faithful round-trip you drive with `kapi inspect` + `kapi apply` (structure and inline codes preserved, no second model); answers what applies to a given piece of content and checks against it, looping until it passes; and translates into other languages with terminology enforcement and multi-format publishing. Use when the task involves reading or editing the content of a document the editor can't open (.docx/.pptx/.json/.xliff), authoring or rewriting in-voice copy, voice profile/tone, forbidden/competitor terms, consistent terminology or a glossary, checking content, asking what voice or terminology applies to a file or surface, discovering or setting up a project's context ("set up my brand", "create a starter pack", "discover our context"), refreshing a context that already exists ("refresh our brand context", "we renamed a product", "our style guide changed", "which content is not covered by our setup?"), connecting or onboarding a project to Bowrain, translating or localizing (to fr/de/ja…), making a project multilingual, adding or setting up i18n, internationalizing an existing app, choosing an i18n library or framework (React, Next.js, Vue, Angular, Svelte, Flutter, iOS, Android, Rails, Django, Go…), or finding hardcoded strings that should be translatable.
---

# kapi

kapi is an open, format-aware content engine you drive from the command line. It
parses any format it understands — Word, PowerPoint, JSON, XLIFF, Markdown, HTML,
YAML, config — into one content model, then reads, searches, edits, and checks
the text inside it and writes it back byte-for-byte. You do the writing, editing,
and translating; kapi handles the formats and the guardrails and round-trips the
result.

## What kapi holds for you

**kapi holds this project's content context** — the terms, the voice and the
rules it actually goes by — in a form you can read. Communication is contextual:
a legal notice is not a help article, and the same fact is written differently in
a changelog, a migration guide and a support reply. Without that context you are
guessing at house style; with it you can ask rather than guess.

Two jobs follow from that, and they are the ones to reach for first:

- **Context discovery** — when the project has no context yet, or its context has
  drifted from what the material actually says. Point kapi at what already
  exists — the repo, the published site, whatever style guide there is — and
  propose the profile, the terminology and the checks for the user to correct.
  They review a draft instead of authoring one.

  The **second visit** is the same loop: a surface appeared that nobody
  declared, a product was renamed, the register drifted. Read the drift
  (`kapi ls --untracked` names content no collection governs), propose the
  delta, and apply only what the user approves — a refresh is a change-set,
  never a rewrite of files they have not seen. See
  [references/context-discovery.md](references/context-discovery.md).
- **Context retrieval** — before you write or rewrite anything, ask. There are
  two questions and never a store, because which store holds the answer is not
  something you should have to know:

  ```bash
  kapi context docs/guide.md          # what applies HERE, at this location
  kapi context --profile marketing    # the same, for a profile, with no file
  kapi context search widget          # what do we know about THIS word
  kapi context search "sign in" --json
  ```

  `kapi context <path>` answers for the place a file sits: the point it resolved
  to, the voice in force with its full guidance, and the terms bound there — one
  document to read before you touch the file. `kapi context search` answers for
  a word or a phrase, across every store the project binds.

  Over MCP the same two are `context://<path>` (a resource you *read*, not a
  tool you call — `context://profile/<name>` for the by-name form, and
  `?format=json` for the structured shape) and the `context_search` tool. Both
  say plainly when a store was unreachable rather than returning a confident
  empty result, and both state what scope they answered from.

  Retrieve first, then write; a check that fails afterwards is the expensive way
  to learn the same fact.
- **Context freshness** — a retrieved answer is a snapshot, and the project's
  context moves while you work: a colleague approves a terminology decision, a
  `kapi up` run brings one down. Two surfaces tell you, and both only tell you:

  ```bash
  kapi status                       # the governance line: in sync, or what moved
  ```

  A `context_search` answer carries a note when the context, the terms or the
  decisions moved since you last read them here. **Read the notes, and when one
  says the context moved, ask again before you continue** — the wording you had
  settled on was chosen against a context that has since changed, and everything
  you write from here on inherits the stale answer.

  Neither surface resolves anything: what a moved context means for work already
  done is a judgement. Re-read, reconsider what you have written, and say what
  changed rather than silently rewriting.

A rule can be scoped, so *what applies here* has a real answer that differs by
file and by surface — the old name may be permitted in the migration guide and
nowhere else. When a check flags something that looks correct in context, that is
worth surfacing to the user rather than silently rewriting.

## Decide the scope first

Before reaching for a command, judge whether this is a one-off or ongoing work:

- **Ad hoc** — one file or a snippet, a one-time read, check, or edit,
  exploration, one or no target language. Just run the command; no setup. kapi
  works without a project.
- **Project** — many files or a whole app; the same target locales repeatedly; a
  voice profile or terminology that must stay consistent; recurring work (CI,
  re-translate on change); content memory you want to reuse. Bind that
  context **once** in a kapi project, then issue plain requests — kapi applies
  the project's locales, content, voice profile, and terms with no flags.

If a kapi project already exists (kapi walks up from the cwd to find it), use
it. If the task is project-shaped and there's no project, offer to set one up;
don't impose a project on a genuine one-off. See
[references/project.md](references/project.md).

## Verify before you call it done

**The task is not done until `kapi check --ship` passes.** Writing or translating the
files is not the finish line — a clean verify is. Don't trust a single pass of your
own output: in a project, run `kapi check --ship` after writing or translating content. It
checks the work against the project's gates — voice profile score, terminology, and
translation QA (placeholders intact, nothing left untranslated) — and prints the
specific findings. Fix what it flags and run it again, until it passes (exit 0). kapi
is the gate; keep iterating until it's green. (The kapi Claude Code plugin also wires
this in as a Stop hook, so a failing gate keeps you working automatically.)

```bash
kapi check --ship --json        # whole project; or: kapi check --ship <files> [--gate voice|terminology|qa]
```

## Then read the section that matches the task

- **Read, search, or rewrite content in any format** — print the prose of a file
  you can't open directly (Word, PowerPoint, JSON, XLIFF…), search it for a
  phrase, apply a find-and-replace that leaves keys, tags, and styles intact, or
  compare two versions block by block (`kdiff`), using the format-aware toolbox
  (`kcat`/`kgrep`/`ksed`/`kdiff`). See
  [references/toolbox.md](references/toolbox.md).
- **Edit content in any format** — read a file's blocks (`kapi inspect --jsonl`),
  rewrite the text yourself, and write it back through the one write verb
  (`kapi apply`) — structure and inline codes preserved, no model. The
  deliberate, block-by-block edit loop (use `ksed` for a regex substitution). See
  [references/edit.md](references/edit.md).
- **Create / author content** — when you're writing the document, not editing a
  fixed source: author in a generative format, let kapi parse it as the first
  check, then gate on voice + terminology and revise. See
  [references/create.md](references/create.md).
- **Keep content in voice** — retrieve the voice guidance before writing, score a
  draft (0–100), and fix off-voice text yourself — routed through `kapi apply`.
  (`kapi voice rewrite` swaps forbidden/competitor terms offline; for tone and
  phrasing, rewrite the text yourself against the guide.) See
  [references/voice.md](references/voice.md).
- **Discover or refresh a project's context** — assemble it from the user's
  repo, site, and materials (voice profile + terminology seed + the checks that
  enforce both), review it with the user, and bind it in a project; a complete
  journey in one language, with a Bowrain server as an optional last step. Then
  the refresh flow for the second visit, which diffs new material against the
  bound context and lands it as an approve-then-apply change-set. See
  [references/context-discovery.md](references/context-discovery.md).
- **Translate, enforce terminology, publish** — translate content into other
  languages and round-trip it back into its original format, with a terms store
  for consistency. Translate it yourself, but route it **through kapi** (extract →
  translate → merge) and then verify — don't hand-translate files and write them
  back, or terminology, placeholders, and format go unchecked. A provider is only
  needed for unattended runs. See [references/translate.md](references/translate.md).

  Across all of these, do the writing/editing/translating yourself and route it
  through kapi — don't reach for a provider. The provider-backed modes
  (`kapi translate`, the optional `--ai` checks) are for unattended runs only;
  kapi never sends content to a model to rewrite it.
- **Add i18n to a project / choose an i18n framework** — detect the stack,
  recommend the lowest-toil setup for it (every known framework carries a
  **Toil Index** grade from T0 "add and forget" to T4 "you're on your own"),
  set up the neokapi-i18n stack for React apps, or plug kapi into the catalogs
  another stack already uses — with the specific tools that make that stack
  maintainable. See [references/i18n.md](references/i18n.md).

**Advanced:** the porcelain verbs compose a lower layer you can drive directly —
`kapi exec <tool>` runs one registry tool with nothing around it, `kapi run <flow>`
runs one named flow for one pass, and `kapi extract`/`kapi merge` carry the
translator hand-off. Reach for them only when a task needs exactly one tool or one
custom pipeline; the layer model is
[Understanding the CLI layers](https://neokapi.github.io/kapi/direct-execution-layer).

## Prerequisites

- The `kapi` binary on PATH (`kapi version`).
- No AI provider credential is required when you write, edit, or translate the
  text yourself within kapi's guardrails — including editing through `kapi apply`,
  which applies your edits with no model. A saved credential
  (`kapi credentials add`) is only needed for kapi to call a provider directly —
  unattended translation (`kapi translate`) or the optional `--ai` checks. The
  rule-based voice and terminology checks need no credential.

In kapi's own stacks (neokapi-i18n, KBF) the English source text is always the
key — don't introduce message IDs. When plugging into another stack's catalogs,
follow that stack's key idiom instead (see references/i18n.md).
