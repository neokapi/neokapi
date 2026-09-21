---
name: kapi
description: Use when writing or editing any prose that ships from a repository or project, including documentation, README and guide text, UI strings, error messages, release notes, marketing and product copy, and code comments; and when reading, searching or editing content inside Word, PowerPoint, JSON, XLIFF, Markdown, HTML, YAML and other formats. Ask kapi what wording applies at the file you are about to change, record what you notice about how the project writes, and check what you changed before reporting it done. Also for a project's voice profile and terms, and for translation, multilingual content and i18n work. You write the wording; kapi supplies the context, the document editing and the declared checks.
---

# kapi

kapi is an open, format-aware content engine you drive from the command line. It
parses any format it understands into one content model, answers what wording
applies at a given file, writes your edits back through the format's writer, and
runs the project's declared checks. You do the writing, editing and translating.

A project's **context** is what it has recorded about how it writes: the voice
in force at a location, the terms bound there, wording already approved, and the
decisions behind all of it. Most projects have recorded little of it. Four
habits keep you working from that context and growing it as you go.

## 1. Before you write, ask what applies here

```bash
kapi context docs/guide.md          # what applies HERE, at this location
kapi context search widget          # what do we know about THIS word
```

Over MCP: read the `context://<project-relative-path>` resource, and call
`context_search`.

Read the answer before you touch the file. It gives the point the file resolved
to, the voice in force with its guidance, the terms bound there, and the
`candidates` nobody has decided on yet. Build on the candidates and report none
of them as a rule in force: a check reports each one and no check fails on one.

An answer with `coverage: empty` is an answer. It says the project has recorded
nothing at that point, and its notes say what is worth noticing while you work.
Detail, including per-call `project` and what makes an answer go stale, is in
[references/context.md](references/context.md).

## 2. While you read the project, record what you notice

```bash
kapi context observe "the docs address the reader as you" --seen-in docs/guide.md
kapi context propose utilise --use use --seen-in docs/guide.md --quote "Utilise the editor"
```

Over MCP: `context_observe` and `context_propose`.

Record as you read, one fact per call, rather than saving it for the end of the
task. Observe states no rule and changes no check. Propose states a rule about
one word and records it as a **candidate**, which advises until a person
confirms it.

`--seen-in` and `--quote` are what let a person judge the entry later, and
proposing requires them.

## 3. When the person changes your wording, record the correction

```bash
kapi context correct "sign in" "log in" --seen-in web/src/auth.tsx --propose
```

Over MCP: `context_correct`.

The judgement has already been made, which makes a correction the cheapest
context there is. `--propose` records the rule it implies with it, so the next
use of the old wording is reported.

[references/growing-context.md](references/growing-context.md) covers the whole
mechanism, including the discovery session for a project that has recorded
nothing at all.

## 4. Before you report done, check what you changed

```bash
kapi check --diff-against HEAD --json
```

Over MCP: `check_file` on each file you changed.

Only the content blocks the change touched are checked. Repair what it reports
and run the same command again. Exit 4 means the check did not run and is never
a pass. [references/check.md](references/check.md) covers the report, the
warnings, and the other ways to name a change (`--staged`, `--diff-range`,
`--diff-file`, `--ship`).

Then end your report with what this session recorded and how to review it:

```bash
kapi context log --session <id>     # or --limit 10 for what you just recorded
```

Over MCP: call `context_session_summary` and use the sentence it returns.
Deciding is the user's: there is no tool for confirming, discarding, reverting
or widening on the agent surface, and an agent that tries the command is
refused.

## Then read the reference that matches the task

- **Ask what applies, and notice when it moves**:
  [references/context.md](references/context.md).
- **Grow the context**: the three everyday calls, the discovery session for a
  project with nothing recorded, and the refresh for the second visit:
  [references/growing-context.md](references/growing-context.md).
- **Check content**: the report, exit codes, warnings, release gates:
  [references/check.md](references/check.md).
- **Edit content in any format**: read a file's blocks (`kapi inspect --jsonl`),
  rewrite the text yourself, write it back through the one write verb (`kapi
  apply`), which preserves structure and inline codes and uses no model. Also
  repairing a finding in a code comment:
  [references/edit.md](references/edit.md).
- **Read, search or rewrite in place**: print the prose of a file you cannot
  open directly, search it, apply a find-and-replace that leaves keys, tags and
  styles intact, or compare two versions block by block
  (`kcat`/`kgrep`/`ksed`/`kdiff`): [references/toolbox.md](references/toolbox.md).
- **Create / author content**: when you are writing the document rather than
  editing a fixed source: [references/create.md](references/create.md).
- **Keep content in voice**: retrieve the guidance, check a draft, fix the
  wording yourself: [references/voice.md](references/voice.md).
- **Translate, enforce terminology, publish**: route your own translation
  through kapi (extract, translate, merge) and verify, with a terms store for
  consistency: [references/translate.md](references/translate.md).
- **Set up or run a project**, what a recipe binds, prerequisites, and the layers
  under the porcelain: [references/project.md](references/project.md).
- **Add i18n to a project / choose an i18n framework**: detect the stack and
  recommend the lowest-toil setup for it:
  [references/i18n.md](references/i18n.md).

## Credentials, and message keys

Provider-backed translation and model-backed analysis are separate operations
from the local format and check tools. Use them when the task calls for them and
their provider use is authorized; everything above needs no credential.

In kapi's own stacks (neokapi-i18n, KBF) the English source text is always the
key. Never introduce message IDs. When plugging into another stack's catalogs,
follow that stack's key idiom instead ([references/i18n.md](references/i18n.md)).
