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

Pseudo-translation helps identify truncation, concatenation and hardcoded-string
problems before translation begins. It adds accented characters and length
padding to strings without calling an external provider.

## Scene 1 — pseudo-translate (terminal)

Open `messages.json`, run `kapi pseudo-translate ...`, and inspect the expanded
strings in the output JSON. Loading that file in the application makes missing
strings and layout problems easier to identify.

## Closing

After checking the layout, use `kapi translate` for individual files or `kapi up`
for a project. See [Rule-based checks](/framework/checks/rule-checks) for further
content checks.
