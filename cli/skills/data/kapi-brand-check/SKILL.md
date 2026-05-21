---
name: kapi-brand-check
description: Check whether text is on-brand — score it against a brand voice profile (tone, style, terminology) and list violations. Use after drafting or editing user-facing copy, marketing text, docs, UI strings, or any content that must match a brand voice. Triggers on "is this on brand", "check the voice/tone", "did I use forbidden/competitor terms", "brand compliance".
---

# kapi-brand-check

Scores text against a brand voice profile using the local `kapi` CLI — no network, no account. Returns a 0–100 brand compliance score plus specific findings (forbidden terms, competitor terms, tone/style issues).

## When to use

Right after you (the assistant) generate or edit any user-facing text — a paragraph, a slide, a README section, marketing copy, UI strings — and the user cares about staying on brand or consistent. Also when the user explicitly asks to check voice/tone/terminology.

## Prerequisites

- The `kapi` binary on PATH (`kapi version`).
- A brand voice profile. Use a built-in starter pack (`--pack`), a project YAML (`--profile-file`), or a stored profile (`--profile`). List options with `kapi brand profiles`.

## How to run

Pipe the text via stdin (preferred) and always pass `--json`:

```bash
echo "We utilize synergies to leverage our platform." \
  | kapi brand check --pack friendly-dtc --json
```

Other profile sources:

```bash
kapi brand check --text "$DRAFT" --profile-file ./brand.yaml --json
kapi brand check --text - --profile acme-marketing --json   # from the local store
```

Add `--ai` to also run an LLM tone/style/clarity check (needs a saved credential or `--api-key`); omit it for a fast, deterministic, offline vocabulary check.

## Output (JSON)

```json
{
  "profile": "Friendly DTC",
  "score": 95,
  "passed": true,
  "ai_checked": false,
  "dimensions": [{"dimension":"vocabulary","score":95,"penalty":5,"issues":1}, ...],
  "findings": [
    {"dimension":"vocabulary","severity":"major","message":"Forbidden term \"utilize\" found",
     "suggestion":"Use \"use\" instead","position":{"Start":3,"End":10},"original_text":"utilize"}
  ]
}
```

Read `score` (0–100) and `findings`. Each finding has a `severity` (minor/major/critical), the `original_text`, its `position`, and a `suggestion`. Present the issues to the user and offer to fix them (see `kapi-brand-fix`).

## CI / quality gate

`--min-score N` makes the command exit non-zero (code `3`, distinct from operational errors) when the score is below `N`, while still printing the JSON. Use it to gate content in scripts or CI:

```bash
echo "$DRAFT" | kapi brand check --pack professional-b2b --min-score 90 --json || echo "below brand threshold"
```

## Notes

- stdout is always clean JSON when `--json` is set; diagnostics go to stderr.
- The rule-based check is deterministic and offline. `--ai` adds nuance but needs credentials.
- To fix flagged text, hand off to `kapi-brand-fix`. To load the full guide into context before writing, use `kapi-brand-context`.
