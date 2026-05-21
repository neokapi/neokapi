---
name: kapi-brand-fix
description: Rewrite text so it complies with a brand voice profile — swap forbidden/competitor terms for the approved ones and (with --ai) adjust tone and style. Use after kapi-brand-check flags issues, or whenever the user asks to "make this on brand", "fix the wording", "rewrite in our voice". Triggers on "rewrite on brand", "fix flagged terms", "apply our voice".
---

# kapi-brand-fix

Rewrites text to match a brand voice profile. Offline by default (deterministic term substitution); add `--ai` for full tone/style rewriting via an LLM.

## When to use

After `kapi-brand-check` reports findings, or when the user asks to bring existing text on brand. For new content, prefer writing on-brand from the start with `kapi-brand-context`.

## How to run

```bash
echo "We utilize synergies to facilitate growth." \
  | kapi brand rewrite --pack friendly-dtc --json
```

```bash
kapi brand rewrite --text "$DRAFT" --profile-file ./brand.yaml --json        # offline term swaps
kapi brand rewrite --text "$DRAFT" --pack marketing-blog --ai --json          # LLM rewrite (needs credentials)
```

## Output (JSON)

```json
{
  "profile": "Friendly DTC",
  "ai_rewrite": false,
  "original": "We utilize synergies to facilitate growth.",
  "rewritten": "We use synergies to facilitate growth.",
  "changes": [{"from":"utilize","to":"use","count":1}]
}
```

Without `--ai`: only forbidden/competitor terms that declare a replacement are substituted; `changes` lists exactly what changed. With `--ai`: the model rewrites the whole passage to the brand voice guide; `changes` is empty (the rewrite is holistic) — diff `original` vs `rewritten` to show the user.

## How to apply

Show the user the `rewritten` text and the `changes`. If they accept, replace the original. Re-run `kapi-brand-check` on the result to confirm the score improved.
