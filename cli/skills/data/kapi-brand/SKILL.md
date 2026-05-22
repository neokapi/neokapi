---
name: kapi-brand
description: Keep content on-brand with the kapi CLI — load a brand voice guide before writing, score drafted text against it (tone, style, terminology), and rewrite text that drifts off-voice. Use whenever user-facing copy, marketing text, docs, UI strings, or any content must match a brand voice. Triggers on "on brand", "check the voice/tone", "is this on brand", "rewrite in our voice", "forbidden/competitor terms", "brand compliance", "what's our voice".
---

# kapi-brand

Keeps generated content on-brand using the local `kapi` CLI — offline, no account.
One loop: load the voice guide before writing, score a draft, fix what drifts.

## Prerequisites

- The `kapi` binary on PATH (`kapi version`).
- A brand voice profile, from any of: a built-in starter pack (`--pack`), a
  git-shareable YAML (`--profile-file`), or the local store (`--profile`). List
  options with `kapi brand profiles`. Packs: `professional-b2b`, `friendly-dtc`,
  `technical-docs`, `marketing-blog`, `customer-support`.

## 1. Load the guide before writing

Pull the voice guide into context so the first draft is on-brand:

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

## How to apply

- Keep the English source text as the key — don't introduce message IDs.
- To translate the on-brand result into other languages, bind the same profile
  and hand off to `kapi-localize`.
- Profiles can also be managed in the local store: `kapi brand pack <name>`
  installs a starter pack; `kapi brand import <file.yaml>` imports one.
