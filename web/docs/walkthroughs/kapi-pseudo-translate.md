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

You're shipping a feature with new strings and want to catch UI truncation
problems _before_ the strings come back from a real translation provider.
Pseudo-translation expands every string with locale-shaped accent characters
so issues surface immediately on screen — no API key, no waiting, no cost.

## Scene 1 — pseudo-translate (terminal)

The user opens a JSON message catalog (`messages.json`), runs
`kapi pseudo-translate ...` to generate a pseudo-locale (`qps`)
pseudo-translation, then inspects the output JSON to confirm the
expansion. The recording shows: source file → command → output file.

The narration that should appear next to this recording in the docs:
pseudo-translation expands every string with diacritical characters so
truncation, clipping, or missing-string bugs are immediately visible
when the UI re-renders. It's part of the development loop, not the
translation pipeline.

## Closing

For deeper QA, see [Quality checks](/docs/features/qa-checks). To run
pseudo-translation in the same flow as a TM-leverage step, see
[`kapi run`](/docs/kapi-cli/commands/flow).
