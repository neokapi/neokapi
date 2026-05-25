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
  page.html` / `kapi extract` pulls the translatable text to analyze.

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

## 3. Fix what's flagged

```bash
echo "$DRAFT" | kapi brand rewrite --pack marketing-blog --text - --json
```

Offline, this substitutes forbidden/competitor terms and reports the `changes`.
With `--ai` it rewrites tone and style holistically — diff `original` vs
`rewritten`. Re-run the check to confirm the score improved.

When the off-voice text is a file the user owns, **rewrite it in place** — git records
the change and is how they review and undo it. Don't leave a `.fixed` copy behind. If
the file has uncommitted edits, say so before overwriting, so unsaved work isn't lost.

## CI / quality gate

`--min-score` makes `check` exit non-zero (code `3`, distinct from an operational
error) when the score is below the threshold, while still printing the JSON:

```bash
kapi brand check RELEASE.md --pack professional-b2b --min-score 90 --json
```

To translate the on-brand result into other languages, bind the same profile and
see [localize.md](localize.md).
