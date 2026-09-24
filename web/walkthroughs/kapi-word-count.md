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

`kapi stats` reports block, word, character and segment counts for supported
formats. These counts help estimate translation work and cost.

## Scene 1 — stats (terminal)

Run `kapi stats` on a JSON message catalog and inspect the reported counts.

## Closing

Pass several files or a glob to obtain combined totals for a larger project.
