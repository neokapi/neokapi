# Keep content on-brand

Score and fix content against a brand voice profile with the local `kapi` CLI —
offline, no account. One loop: load the voice guide before writing, score a
draft, fix what drifts.

## Profiles

A profile comes from any of: a built-in starter pack (`--pack`), a git-shareable
YAML (`--profile-file`), or the local store (`--profile`). List options with
`kapi brand profiles`. Packs: `professional-b2b`, `friendly-dtc`,
`technical-docs`, `marketing-blog`, `customer-support`.

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

## CI / quality gate

`--min-score` makes `check` exit non-zero (code `3`, distinct from an operational
error) when the score is below the threshold, while still printing the JSON:

```bash
kapi brand check RELEASE.md --pack professional-b2b --min-score 90 --json
```

To translate the on-brand result into other languages, bind the same profile and
see [localize.md](localize.md).
