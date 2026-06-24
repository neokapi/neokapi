# Keep content on-brand

Score and fix content against a brand voice profile with the local `kapi` CLI —
offline, no account. One loop: load the voice guide before writing, score a
draft, fix what drifts.

## Profiles

A profile comes from any of: a built-in starter pack (`--pack`), a git-shareable
YAML (`--profile-file`), or the local store (`--profile`). List options with
`kapi brand profiles`. Packs: `professional-b2b`, `friendly-dtc`,
`technical-docs`, `marketing-blog`, `customer-support`.

**Inside a project, the profile is part of the context — don't pass a flag.** When
the project binds a brand voice (a `defaults.brand_voice` recipe entry, or a
`brand.yaml` / `.kapi/brand.yaml` at the project root), run `kapi brand check
<file>`, `kapi brand rewrite <file>`, and `kapi brand guide` with **no**
`--profile`/`--profile-file`/`--pack` — kapi resolves the project's voice. Pass a
flag only for a one-off outside a project, or to override the bound profile. See
[project.md](project.md).

## Create a profile

If the user has no profile yet, draft one for them — you (the assistant) do the
analysis; the CLI gives you the schema and stores the result.

```bash
kapi brand new -o brand.yaml                         # commented template to fill in
kapi brand new --pack marketing-blog -o brand.yaml   # or start from a close pack
```

Fill in `brand.yaml` from whatever signal is available:

- **What you already know** about the product/company from this conversation or
  the repo (README, marketing copy, existing UI strings) — infer personality,
  formality, and preferred/forbidden terms.
- **Samples** the user pastes or points at (a few on-brand paragraphs, past
  emails, docs) — derive tone and vocabulary, and turn weak→strong pairs into
  `examples` (before / after).
- **A website** the user links — fetch a page or two (your web tool, or `curl`),
  read the live copy, and capture its voice. For a saved page, `kapi word-count
  page.html` / `kapi extract` pulls the text to analyze.

Keep it concrete: 2–4 personality adjectives, a handful of forbidden/competitor
terms with replacements, and 2–3 before/after examples beat a long abstract
description. Then save and verify:

```bash
kapi brand import brand.yaml                 # into the local store
kapi brand guide --profile-file brand.yaml   # confirm it renders as intended
echo "We utilize synergies." | kapi brand check --profile-file brand.yaml --json
```

Show the user the rendered guide and a check on one of their own samples, then
refine the YAML from their feedback.

## 1. Load the guide before writing

```bash
kapi brand guide --pack marketing-blog
```

Apply the tone, style, and preferred terms; never use the forbidden or competitor
terms (use the listed replacements). Then draft, and check the result.

## 2. Check a draft

Pipe text via stdin (or `--text "..."`); always pass `--json`:

```bash
echo "$DRAFT" | kapi brand check --pack marketing-blog --text - --json
```

Returns a 0–100 `score` and `findings` (each with `severity`, `original_text`,
`position`, `suggestion`). The rule-based check is deterministic and offline; add
`--ai` for an LLM tone/style/clarity pass (needs a saved credential).

## 3. Fix what's flagged — you rewrite, kapi checks

You are a capable writer, so the default fix path is the same one translation
uses: **kapi doesn't call a second model when you're in the loop.** Load the
brand voice as context, rewrite the off-voice text on-brand yourself, route the
change through kapi's one write verb, then re-check.

```bash
kapi brand guide                       # the voice to follow — your context
kapi termbase lookup "<term>" -t en     # the approved wording for a flagged term
```

Rewrite each flagged block, then apply your edits through the faithful
round-trip — `kapi apply` (the one write verb) or `kapi rewrite --edits` (the
provider-free mode of rewrite). Both write the file in place, preserve structure
and inline codes, and reject an edit that drifted or would corrupt markup. See
[edit.md](edit.md) for the `content`-entry shape, the guards, and the diff/in-place
flags:

```bash
kapi inspect blog-post.md --jsonl | rewrite-the-flagged-blocks > edits.jsonl
kapi apply edits.jsonl --diff           # preview, then drop --diff to apply
```

When the off-voice text is a file the user owns, apply in place — git records the
change and is how they review and undo it. Don't leave a `.fixed` copy behind. If
the file has uncommitted edits, say so before overwriting, so unsaved work isn't
lost. Re-run the check to confirm the score improved.

### Fix the rule, not just the draft

A recurring off-voice term is better fixed at the source: add a vocabulary rule
so every future draft is checked against it. That is just another `kind` in the
**same** `kapi apply` change-set — the content fix and the rule that justifies it
land together, atomically:

```jsonl
{"kind":"content","file":"blog-post.md","id":"p2","content_hash":"b74d…","text":"We use our infrastructure."}
{"kind":"brand","op":"add-rule","list":"forbidden","term":"utilize","replacement":"use","severity":"minor"}
```

```bash
kapi apply changeset.jsonl
```

The `brand` entry is written into the project's committed brand voice profile
YAML (the `defaults.brand_voice.profile_file` the recipe binds), and the existing
import compiles it into the local brand store. `git diff` shows the one new rule;
the next `kapi brand check` / `kapi verify` enforces it. `list` is `forbidden`,
`competitor`, or `preferred`; the entry requires a `.kapi` project. (Add an
approved term instead with a `term` entry — see [create.md](create.md).)

### Unattended fallback: let kapi call a provider

When **no assistant is in the loop** (CI, a batch job), kapi can rewrite via a
configured provider — the second-model path, gated behind a saved credential:

```bash
echo "$DRAFT" | kapi brand rewrite --pack marketing-blog --text - --ai --json
```

Offline (no `--ai`), `kapi brand rewrite` only substitutes forbidden/competitor
terms deterministically and reports the `changes`. With `--ai` it rewrites tone
and style holistically. This is the fallback for unattended runs; when you are in
the loop, prefer rewriting yourself and applying through `kapi apply`.

## CI / quality gate

`--min-score` makes `check` exit non-zero (code `3`, distinct from an operational
error) when the score is below the threshold, while still printing the JSON:

```bash
kapi brand check RELEASE.md --pack professional-b2b --min-score 90 --json
```

To translate the on-brand result into other languages, bind the same profile and
see [localize.md](localize.md).
