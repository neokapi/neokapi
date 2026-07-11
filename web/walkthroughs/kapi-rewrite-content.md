---
id: kapi-rewrite-content
audience: developer
target_doc: docs/kapi/recipes/rewrite-content.mdx
scenes:
  - id: rewrite-content
    kind: terminal
    binary: kapi
    duration_budget_seconds: 40
    fixtures: []
    smoke_contract:
      - kapi apply edits.jsonl --diff
      - kapi apply edits.jsonl
---

## Story

Editing content in any format is the inspect → apply pipeline. `kapi inspect`
parses a file into one anchored record per block — the text, a stable content
hash, and its structural role — so a person or an AI can address exactly the
blocks to change. `kapi apply` is the one write verb: each change-set entry is
one reviewed change, bound to its block's content hash, landed through the
byte-faithful round-trip. If the file drifted since inspection, the entry
refuses to land.

## Scene 1 — rewrite-content (terminal)

Inspect `release-notes.md` with `kapi inspect --jsonl`, preview a two-entry
change-set with `kapi apply edits.jsonl --diff`, land it with `kapi apply`,
and confirm the result still passes `kapi check`. No AI provider is required
by any step; when an AI authors the edit, it does so against the same anchors.

## Closing

Format, structure, and inline codes round-trip untouched — only the leaf text
changes. The same pipeline scales to any format kapi reads, from Markdown to
DOCX.
