# Keep content in voice

Score and fix content against a voice profile with the local `kapi` CLI,
offline, with no account. One loop: load the voice guide before writing, score a
draft, fix what drifts.

## Profiles

A profile comes from any of: a built-in pack (`--pack`), a git-shareable
YAML (`--profile-file`), or the local store (`--profile`). List options with
`kapi voice profiles`. Packs: `professional-b2b`, `friendly-dtc`,
`technical-docs`, `marketing-blog`, `customer-support`.

**Inside a project, the profile is part of the context; don't pass a flag.** When
the project binds a voice profile (a `defaults.voice` recipe entry, or a
`.kapi/voice.yaml`, or a `voice.yaml` at the project root), run `kapi voice check
<file>` and `kapi voice guide` with **no**
`--profile`/`--profile-file`/`--pack`: kapi resolves the project's voice. Pass a
flag only for a one-off outside a project, or to override the bound profile. See
[project.md](project.md).

## Create a profile

If the user has no profile yet, draft one for them: you (the assistant) do the
analysis; the CLI gives you the schema and stores the result.

```bash
kapi voice new -o voice.yaml                         # commented template to fill in
kapi voice new --pack marketing-blog -o voice.yaml   # or start from a close pack
```

Fill in `voice.yaml` from whatever signal is available:

- **What you already know** about the product/company from this conversation or
  the repo (README, marketing copy, existing UI strings): infer personality,
  formality, and preferred/forbidden terms.
- **Samples** the user pastes or points at (a few on-brand paragraphs, past
  emails, docs): derive tone and vocabulary, and turn weak→strong pairs into
  `examples` (before / after).
- **A website** the user links: fetch a page or two (your web tool, or `curl`),
  read the live copy, and capture its voice. For a saved page, `kapi stats
  page.html` / `kapi extract` pulls the text to analyze.

Keep it concrete: 2–4 personality adjectives, a handful of forbidden/competitor
terms with replacements, and 2–3 before/after examples beat a long abstract
description. Each vocabulary rule carries a `severity` (`minor`, `major`, or
`critical`); `minor` only warns, the others fail the check. Phrasing to avoid
that is a pattern rather than a term (`!{2,}`, a sentence opening with "Just")
goes under `style.prohibited_patterns` as a regex with a message.

For an inflected language, `kapi voice expand` asks a model for each vocabulary
term's other surface forms (inflections, declensions) and writes them into the
profile as `forms:`, so the check matches them exactly:

```bash
kapi voice expand --profile-file voice.yaml --language nb   # write forms for Norwegian
kapi voice expand --profile-file voice.yaml --dry-run       # print what would be added
```

Rules that already carry forms are left alone unless `--overwrite` is given; the
result is authoring-time work you review in the diff. Then save and verify:

```bash
kapi voice import voice.yaml                 # into the local store
kapi voice guide --profile-file voice.yaml   # confirm it renders as intended
echo "We utilize synergies." | kapi voice check --profile-file voice.yaml --json
```

Show the user the rendered guide and a check on one of their own samples, then
refine the YAML from their feedback. Once the profile is bound in a project,
`kapi voice pointer` writes the section in `AGENTS.md` (or an existing
`CLAUDE.md`) that tells the next assistant the voice is held by kapi and where to
ask for it ([project.md](project.md)).

## 1. Load the guide before writing

```bash
kapi voice guide --pack marketing-blog
```

Apply the tone, style, and preferred terms; never use the forbidden or competitor
terms (use the listed replacements). Then draft, and check the result.

## 2. Check a draft

Pipe text via stdin (or pass `--input-text "..."`); always pass `--json`:

```bash
echo "$DRAFT" | kapi voice check --pack marketing-blog --input-text - --json
```

Returns a 0–100 `score` and `findings` (each with `severity`, `original_text`,
`position`, `suggestion`). The rule-based check is deterministic and offline; add
`--ai` for an LLM tone/style/clarity pass (needs a saved credential).

## 3. Fix what's flagged: you rewrite, kapi checks

Rewrite the off-voice text on-brand **yourself**, route the change through kapi's
write verb, then re-check. kapi does not send content to a model to rewrite it:
`kapi voice rewrite` only substitutes forbidden/competitor terms with their
approved replacements, deterministically and offline; it won't fix tone, style,
or phrasing. For those, rewrite the text yourself with the voice guide as
context. Load it first:

```bash
kapi voice guide                       # the voice to follow: your context
kapi terms lookup "<term>" -t en     # the approved wording for a flagged term
```

Rewrite each flagged block, then apply your edits with `kapi apply`, the one
write verb. It writes the file in place through the faithful round-trip
(structure and inline codes preserved) and rejects an edit that drifted or would
corrupt markup. See [edit.md](edit.md) for the `content`-entry shape, the guards,
and the diff/in-place flags:

```bash
kapi inspect blog-post.md --jsonl > blocks.jsonl
# You rewrite the off-voice blocks' "text" on-brand (keeping the <x id="…"/> tags)
# and save them as content entries to edits.jsonl. Then:
kapi apply edits.jsonl --diff           # preview, then drop --diff to apply
```

When the off-voice text is a file the user owns, apply in place: git records the
change and is how they review and undo it. Don't leave a `.fixed` copy behind. If
the file has uncommitted edits, say so before overwriting, so unsaved work isn't
lost. Re-run the check to confirm the score improved.

### Fix the rule, not just the draft

A recurring off-voice term is better fixed at the source: add a vocabulary rule
so every future draft is checked against it. That is just another `kind` in the
**same** `kapi apply` change-set: the content fix and the rule that justifies it
land together, atomically:

```jsonl
{"kind":"content","file":"blog-post.md","id":"p2","content_hash":"b74d…","text":"We use our infrastructure."}
{"kind":"voice","op":"add-rule","list":"forbidden","term":"utilize","replacement":"use","severity":"minor"}
```

```bash
kapi apply changeset.jsonl
```

The `voice` entry is written into the project's committed voice profile
YAML (the `defaults.voice.profile_file` the recipe binds), and the existing
import compiles it into the local voice store. `git diff` shows the one new rule;
the next `kapi voice check` / `kapi check --ship` enforces it. `list` is `forbidden`,
`competitor`, or `preferred`; the entry requires a `.kapi` project. (Add an
approved term instead with a `term` entry; see [create.md](create.md).)

### Offline term substitution

`kapi voice rewrite` swaps forbidden and competitor terms for their approved
replacements: deterministic, offline, no model. It reads text from
`--input-text` or stdin and prints the rewrite, reporting each `change`:

```bash
echo "$DRAFT" | kapi voice rewrite --pack marketing-blog --input-text - --json
```

A rule that names no replacement, and a match on a declared inflected form of a
term, are left in place and reported under `skipped`: one entry per rule with
`term`, `list` (`forbidden` or `competitor`), `severity`, `scope`, `matched`
(the spellings in the text), `count` and `reason` (`no_replacement` or
`inflected_form`). The exit code stays 0. Read `skipped` before trusting an
unchanged `rewritten`: empty means there was nothing to fix, entries mean
violations were found and left for you. Rewrite those yourself and verify with
`kapi voice check`.

It only changes the terms the profile defines; it won't fix tone, style, or
phrasing. For those, rewrite the text yourself with the voice guide as context
and apply through `kapi apply`.

## CI / quality gate

`--min-score` makes `check` exit non-zero (code `3`, distinct from an operational
error) when the score is below the threshold, while still printing the JSON:

```bash
kapi voice check RELEASE.md --pack professional-b2b --min-score 90 --json
```

To translate the on-brand result into other languages, bind the same profile and
see [translate.md](translate.md).
