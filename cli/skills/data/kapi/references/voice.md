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
the project binds a voice profile (a `defaults.voice` recipe entry, or a profile
the store holds under a profile's own name), run `kapi voice check
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
  formality, and the words to use and avoid.
- **Samples** the user pastes or points at (a few on-brand paragraphs, past
  emails, docs): derive tone and word choices, and turn weak→strong pairs into
  `examples` (before / after).
- **A website** the user links: fetch a page or two (your web tool, or `curl`),
  read the live copy, and capture its voice. For a saved page, `kapi stats
  page.html` / `kapi extract` pulls the text to analyze.

Keep it concrete: 2–4 personality adjectives, a handful of words to avoid with
replacements, and 2–3 before/after examples beat a long abstract description.

Word rules are terms, and a voice profile holds none. Write them in the same
file under a top-level `terms:` list; importing the file moves them into the
project's terms:

```yaml
terms:
  - term: utilize
    replacement: use
  - term: Globex
    replacement: our platform
    competitor: true          # a competitor's name
  - term: simple
    advisory: true            # reports, never fails
  - replacement: sign in      # a preferred form: rejects nothing
```

A word rule fails the check unless it carries `advisory: true`, which makes it
only report. A rule whose `replacement` is a capitalised product name
(`term: QuickCast`, `replacement: Quickcast`) matches case-sensitively, so a
lower-case slug is left alone; set `case_sensitive` to override. Phrasing to
avoid that is a pattern rather than a term (`!{2,}`, a sentence opening with
"Just") goes under `style.prohibited_patterns` as a regex with a message. A
profile written with a `vocabulary:` list (`forbidden_terms`, `competitor_terms`,
`preferred_terms`) still loads; import converts it into terms and says so.

Then file the profile and verify:

```bash
kapi voice import voice.yaml                 # voice into the store, terms into the project's terms
kapi voice guide                             # confirm it renders as intended
echo "We utilize synergies." | kapi voice check --json
```

Bind the id it printed under `defaults.voice.profile` in the recipe. The scratch
`voice.yaml` has done its job; from then on **`kapi voice edit`** is how the
profile changes. It opens the stored profile in the user's editor as the same
YAML, validates what they save, and reads it back as one recorded change. The
document states the profile entire, so a section deleted in the editor is
deleted from the profile.

Show the user the rendered guide and a check on one of their own samples, then
refine from their feedback. For an inflected language, `kapi terms expand`
asks a model for each term's other surface forms (inflections, declensions) and
writes them onto the term, so the check matches them exactly
(`kapi terms expand --locale nb`; `--dry-run` prints what would be added, and
`--overwrite` asks again about terms that already declare forms). Once the
profile is bound in a project,
`kapi voice pointer` writes the section in `CLAUDE.md` (or an `AGENTS.md`
already at the root) that tells the next assistant the voice is held by kapi and
where to ask for it ([project.md](project.md)).

## 1. Load the guide before writing

```bash
kapi voice guide --pack marketing-blog
```

Apply the tone and style; follow the **Say this, not that** list (the terms in
force, never the forbidden or competitor ones). Then draft, and check the
result.

Inside a project, name the file you are about to edit: `kapi voice guide <file>`
answers for that file's own content. Before writing a comment, in a Go file or
any other, ask `kapi voice guide --comments <file>`: it answers for the point
the comments sit at and lists the comment limits in force there under "Code
comments"; keep each sentence and each comment within them.

## 2. Check a draft

Pipe text via stdin (or pass `--input-text "..."`); always pass `--json`:

```bash
echo "$DRAFT" | kapi voice check --pack marketing-blog --input-text - --json
```

Returns `passed`, `failing` (the count of failing findings), a 0–100 `score`
and `findings` (each with `fails`, `original_text`, `position`, `suggestion`). The rule-based check is deterministic and offline; add
`--ai` for an LLM tone/style/clarity pass (needs a saved credential).
`kapi voice check` applies the voice's patterns and the terms its file carries
(a pack's, or a file's `terms:`); the project's own terms are checked by
`kapi check <file>`, which reports every word rule as `terms.vocabulary`
(`Forbidden term "x" found`, `Competitor term "x" found`, `Retired term "x"
found`) and the voice's patterns as `voice.style` and its siblings.

## 3. Fix what's flagged: you rewrite, kapi checks

Rewrite the off-voice text on-brand **yourself**, route the change through kapi's
write verb, then re-check. kapi does not send content to a model to rewrite it:
`kapi voice rewrite` only substitutes the forbidden and competitor terms a voice
file carries with their approved replacements, deterministically and offline; it won't fix tone, style,
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

A recurring off-voice term is better fixed at the source: add a term so every
future draft is checked against it. That is just another `kind` in the
**same** `kapi apply` change-set: the content fix and the rule that justifies it
land together, atomically:

```jsonl
{"kind":"content","file":"blog-post.md","id":"p2","content_hash":"b74d…","text":"We use our infrastructure."}
{"kind":"term","op":"upsert","term":"utilize","locale":"en","status":"forbidden","replacement":"use"}
```

```bash
kapi apply changeset.jsonl
```

The `term` entry is written into the project's terms, and the change is
recorded, so `kapi context log` carries the one new rule and the next
`kapi check` enforces it. Add `"advisory":true` for a rule that reports
without failing, or `"competitor":true` for a competitor's name. The entry
requires a kapi project. A change-set has no `voice` kind; `kapi apply` refuses
one and names the `term` form. (An approved term is a `term` entry too; see
[create.md](create.md).)

### Offline term substitution

`kapi voice rewrite` swaps the forbidden and competitor terms a voice file
carries (a pack's terms, a file's `terms:`) for their approved replacements:
deterministic, offline, no model. It reads text from
`--input-text` or stdin and prints the rewrite, reporting each `change`:

```bash
echo "$DRAFT" | kapi voice rewrite --pack marketing-blog --input-text - --json
```

A rule that names no replacement, and a match on a declared inflected form of a
term, are left in place and reported under `skipped`: one entry per rule with
`term`, `list` (`forbidden` or `competitor`), `fails`, `scope`, `matched`
(the spellings in the text), `count` and `reason` (`no_replacement` or
`inflected_form`). The exit code stays 0. Read `skipped` before trusting an
unchanged `rewritten`: empty means there was nothing to fix, entries mean
violations were found and left for you. Rewrite those yourself and verify with
`kapi voice check`.

It only changes the terms the voice file carries; it won't fix tone, style, or
phrasing. For those, rewrite the text yourself with the voice guide as context
and apply through `kapi apply`.

## CI / quality gate

`check` exits `3` (distinct from an operational error) when at least one
finding fails, while still printing the JSON. The score is reported and does
not decide the exit code:

```bash
kapi voice check RELEASE.md --pack professional-b2b --json
```

To translate the on-brand result into other languages, bind the same profile and
see [translate.md](translate.md).
