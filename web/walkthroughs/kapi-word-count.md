---
id: kapi-word-count
audience: developer
target_doc: docs/walkthroughs/kapi-word-count.mdx
scenes:
  - id: word-count
    kind: terminal
    binary: kapi
    duration_budget_seconds: 10
    fixtures:
      - messages.json
    smoke_contract:
      - kapi stats messages.json
---

## Story

You need to estimate the cost of a translation before kicking off a vendor
job. `kapi stats` reads any supported format and outputs a quick breakdown —
blocks, words, characters, segments — so you can put a number on the bill.

## Scene 1 — stats (terminal)

Point `kapi stats` at a JSON message catalog and watch it report word, block,
and character counts across every dimension. The output is the kind of number
you paste into a quote.

## Closing

For multi-file projects, pass several files or a glob; the totals roll up.
