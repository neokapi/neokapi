---
id: kapi-pseudo-translate
audience: developer
target_doc: docs/walkthroughs/kapi-pseudo-translate.mdx
scenes:
  - id: pseudo-translate
    kind: terminal
    binary: kapi
    duration_budget_seconds: 20
    fixtures:
      - messages.json
    smoke_contract:
      - kapi pseudo-translate messages.json -o messages.fr.json
---

## Story

Pseudo-translation is the pre-flight check of the localization loop: prove the
UI is ready _before_ any real translation is bought. It expands every string
with locale-shaped accent characters and length padding, so truncation,
concatenation, and hardcoded-string bugs surface immediately on screen — no
API key, no waiting, no cost.

## Scene 1 — pseudo-translate (terminal)

The user opens a JSON message catalog (`messages.json`), runs
`kapi pseudo-translate ...` to generate a pseudo-locale
pseudo-translation, then inspects the output JSON to confirm the
expansion. The recording shows: source file → command → output file.

The narration that should appear next to this recording in the docs:
pseudo-translation expands every string with diacritical characters so
truncation, clipping, or missing-string bugs are immediately visible
when the UI re-renders. It is the pre-flight step of the development
loop, not the translation pipeline.

## Closing

When the UI holds up, `kapi translate` produces the real translations for
ad-hoc files, and `kapi up` catches a whole project up to its ship gates.
For deeper checks, see [Rule-based checks](/framework/checks/rule-checks).
