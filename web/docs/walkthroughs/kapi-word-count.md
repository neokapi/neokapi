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
      - kapi word-count fixtures/messages.json
---

## Story

You need to estimate the cost of a translation before kicking off a vendor
job. `kapi word-count` reads any supported format and outputs a quick
breakdown so you can put a number on the bill.

## Scene 1 — word-count (terminal)

Point `kapi word-count` at a JSON message catalog and watch it report
words, characters, and segment counts. The output is the kind of number
you paste into a quote.

## Closing

For multi-file projects, point at a directory; the totals roll up.
