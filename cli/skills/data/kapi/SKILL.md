---
name: kapi
description: Read, edit and check document content with the kapi CLI, applying scoped writing guidance and terms through format-aware tools. Use for content work in Word, PowerPoint, JSON, XLIFF, Markdown and other supported formats; retrieving or updating a project's voice and terminology; and multilingual content or i18n workflows. The agent writes the wording; kapi supplies context, document editing and declared checks.
---

# kapi

kapi is an open, format-aware content engine you drive from the command line. It
parses any format it understands (Word, PowerPoint, JSON, XLIFF, Markdown, HTML,
YAML, config) into one content model, then reads, searches, edits, and checks
the text inside it and writes edits through the format's writer. You do the
writing, editing and translating. `kapi formats --json` reports whether a format
is editable and supports a faithful round-trip; check those capabilities before
relying on preservation.

## What kapi holds for you

**kapi holds this project's content context** in a form you can read: the terms,
the voice and the rules it actually goes by. Communication is contextual: a
legal notice is not a help article, and the same fact is written differently in a
changelog, a migration guide and a support reply. Without that context you are
guessing at house style; with it you can ask rather than guess.

Two jobs follow from that, and they are the ones to reach for first:

- **Context discovery**: when the project has no context yet, or its context has
  drifted from what the material actually says. Point kapi at what already
  exists (the repo, the published site, whatever style guide there is) and
  propose the profile, the terminology and the checks for the user to correct.
  They review a draft instead of authoring one.

  The **second visit** is the same loop: a surface appeared that nobody
  declared, a product was renamed, the register drifted. Read the drift
  (`kapi ls --untracked` names content no collection governs), propose the
  delta, and apply only what the user approves. A refresh is a change-set,
  never a rewrite of files they have not seen. See
  [references/context-discovery.md](references/context-discovery.md).
- **Context retrieval**: before you write or rewrite anything, ask. There are
  two questions and never a store, because which store holds the answer is not
  something you should have to know:

  ```bash
  kapi context docs/guide.md          # what applies HERE, at this location
  kapi context --profile marketing    # the same, for a profile, with no file
  kapi context search widget          # what do we know about THIS word
  kapi context search "sign in" --json
  ```

  `kapi context <path>` answers for the place a file sits: the point it resolved
  to, the voice in force with its full guidance, and the terms bound there. Read
  that one document before you touch the file. `kapi context search` answers
  for a word or a phrase, across every store the project binds. The usage
  count on each term is as of the last `kapi up`: a term you just added shows
  no uses until the next run, so run `kapi up` before you trust the number.

  Over MCP the same two are `context://<path>` (a resource you *read*, not a
  tool you call; `context://profile/<name>` gives the by-name form, and
  `?format=json` the structured shape) and the `context_search` tool. Both
  say plainly when a store was unreachable rather than returning a confident
  empty result, and both state what scope they answered from.

  Retrieve first, then write; a check that fails afterwards is the expensive way
  to learn the same fact.
- **Context freshness**: a retrieved answer is a snapshot, and the project's
  context moves while you work: a colleague approves a terminology decision, a
  `kapi up` run brings one down. Two surfaces tell you, and both only tell you:

  ```bash
  kapi status                       # the governance line: in sync, or what moved
  ```

  A `context_search` answer carries a note when the context, the terms or the
  decisions moved since you last read them here. **Read the notes, and when one
  says the context moved, ask again before you continue.** The wording you had
  settled on was chosen against a context that has since changed, and everything
  you write from here on inherits the stale answer.

  Neither surface resolves anything: what a moved context means for work already
  done is a judgement. Re-read, reconsider what you have written, and say what
  changed rather than silently rewriting.

A rule can be scoped, so *what applies here* has a real answer that differs by
file and by surface: the old name may be permitted in the migration guide and
nowhere else. When a check flags something that looks correct in context, that is
worth surfacing to the user rather than silently rewriting.

## Decide the scope first

Before reaching for a command, judge whether this is a one-off or ongoing work:

- **Ad hoc**: one file or a snippet, a one-time read, check, or edit,
  exploration, one or no target language. Just run the command; no setup. kapi
  works without a project.
- **Project**: many files or a whole app; the same target locales repeatedly; a
  voice profile or terminology that must stay consistent; recurring work (CI,
  re-translate on change); content memory you want to reuse. Bind that
  context **once** in a kapi project, then issue plain requests. kapi applies
  the project's locales, content, voice profile, and terms with no flags.

If a kapi project already exists (kapi walks up from the cwd to find it), use
it. If the task is project-shaped and there's no project, offer to set one up;
don't impose a project on a genuine one-off. See
[references/project.md](references/project.md).

## Edit and check content

Retrieve `kapi context <path>` before editing. For block edits, inspect the file
and read [references/edit.md](references/edit.md) for inline-code and drift guards:

```bash
kapi inspect content/en/page.json --jsonl
```

Write a JSONL change-set with one entry per changed block. Copy its `file`, `id`
and full `content_hash` from inspection; supply `kind: "content"` and the new
wording in `text`. Preserve the inspected inline tokens. This illustrates the
entry shape; replace the example ID and hash with the inspected values:

```json
{"kind":"content","file":"content/en/page.json","id":"tu2","content_hash":"<full inspected hash>","text":"Your new wording"}
```

Save the change-set outside protected project files when the task restricts
which files may change. The argument to `apply` is the change-set file:

```bash
kapi apply edits.jsonl --diff
kapi apply edits.jsonl
```

For a larger Markdown, HTML or DOCX section, use `kapi inspect FILE --sections`.
Select the heading by title/path, then use its returned ID and document snapshot
in one entry: `{"kind":"section","file":"FILE","id":"<heading ID>","snapshot":"<snapshot>","text":"<Markdown body>"}`.
This replaces the body and descendants while preserving the heading. Preview
with `kapi apply edits.jsonl --diff --json` to inspect the native block range and
immutable offset plan, then apply. Re-inspect after each structural edit; IDs
and snapshots belong to the inspected version. Unsupported structures fail
without changing the file. Retrieve the file's context before authoring and
run the usual checks after saving.


Check the saved content. Project voice and terms resolve from its path:

```bash
kapi check content/en/page.json --json
```

Read `execution.contexts` to confirm the effective voice selection, profile and
channel, then read the findings and `execution.analyzers`. Fix relevant findings within
the requested scope and re-check. Unsupported semantic guidance still needs
review against the retrieved context; a passing score covers only the checks
that ran. If the same finding persists or contradicts the governing guidance,
report the unresolved issue instead of repeatedly rewriting unrelated text.

Use `kapi check --ship --json` when verifying the project's release gates,
including translation and coverage policy. See
[references/project.md](references/project.md). The optional Claude Code Stop
hook enforces these project gates when installed.

## Then read the section that matches the task

- **Read, search, or rewrite content in any format**: print the prose of a file
  you can't open directly (Word, PowerPoint, JSON, XLIFF…), search it for a
  phrase, apply a find-and-replace that leaves keys, tags, and styles intact, or
  compare two versions block by block (`kdiff`), using the format-aware toolbox
  (`kcat`/`kgrep`/`ksed`/`kdiff`). See
  [references/toolbox.md](references/toolbox.md).
- **Edit content in any format**: read a file's blocks (`kapi inspect --jsonl`),
  rewrite the text yourself, and write it back through the one write verb
  (`kapi apply`), which preserves structure and inline codes and uses no model.
  The deliberate, block-by-block edit loop (use `ksed` for a regex
  substitution). See
  [references/edit.md](references/edit.md).
- **Create / author content**: when you're writing the document rather than
  editing a fixed source. Author in a generative format, let kapi parse it as
  the first check, then gate on voice + terminology and revise. See
  [references/create.md](references/create.md).
- **Keep content in voice**: retrieve the voice guidance before writing, check
  a draft's supported constraints, and fix the wording yourself through `kapi apply`.
  (`kapi voice rewrite` swaps forbidden/competitor terms offline and lists
  under `skipped` the ones it matched and could not swap; for tone and
  phrasing, rewrite the text yourself against the guide.) See
  [references/voice.md](references/voice.md).
- **Discover or refresh a project's context**: assemble it from the user's
  repo, site, and materials (voice profile + terminology seed + the checks that
  enforce both), review it with the user, and bind it in a project; a complete
  journey in one language, with a Bowrain server as an optional last step. Then
  the refresh flow for the second visit, which diffs new material against the
  bound context and lands it as an approve-then-apply change-set. See
  [references/context-discovery.md](references/context-discovery.md).
- **Translate, enforce terminology, publish**: translate content into other
  languages and round-trip it back into its original format, with a terms store
  for consistency. Translate it yourself, but route it **through kapi** (extract →
  translate → merge) and then verify. Hand-translating files and writing them
  back leaves terminology, placeholders, and format unchecked. A provider is only
  needed for unattended runs. See [references/translate.md](references/translate.md).

  In the agent edit loop, supply the wording yourself and use kapi's local
  format and check tools. Provider-backed translation and model-backed analysis
  are separate operations; use them when the task calls for them and their
  provider use is authorized.
- **Add i18n to a project / choose an i18n framework**: detect the stack,
  recommend the lowest-toil setup for it (every known framework carries a
  **Toil Index** grade from T0 "add and forget" to T4 "you're on your own"),
  set up the neokapi-i18n stack for React apps, or plug kapi into the catalogs
  another stack already uses, with the specific tools that make that stack
  maintainable. See [references/i18n.md](references/i18n.md).

**Advanced:** the porcelain verbs compose a lower layer you can drive directly.
`kapi exec <tool>` runs one registry tool with nothing around it, `kapi run <flow>`
runs one named flow for one pass, and `kapi extract`/`kapi merge` carry the
translator hand-off. Reach for them only when a task needs exactly one tool or one
custom pipeline; the layer model is
[Understanding the CLI layers](https://neokapi.github.io/kapi/direct-execution-layer).

## Prerequisites

- The `kapi` binary on PATH (`kapi version`).
- No AI provider credential is required when you write, edit, or translate the
  text yourself within kapi's guardrails, including editing through `kapi apply`,
  which applies your edits with no model. A saved credential
  (`kapi credentials add`) is only needed for kapi to call a provider directly:
  unattended translation (`kapi translate`) or the optional `--ai` checks. The
  rule-based voice and terminology checks need no credential.

In kapi's own stacks (neokapi-i18n, KBF) the English source text is always the
key. Never introduce message IDs. When plugging into another stack's catalogs,
follow that stack's key idiom instead (see references/i18n.md).
