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

`kapi inspect` reads a file into addressable blocks with text, a content hash and
a structural role. A person or an assistant uses those records to construct a
change-set. `kapi apply` checks each entry's content hash before writing through
the format's writer; it rejects an entry if the source has changed.

## Scene 1 — rewrite-content (terminal)

Inspect `release-notes.md` with `kapi inspect --jsonl`, preview a two-entry
change-set with `kapi apply edits.jsonl --diff`, apply it with `kapi apply`,
and check the result with `kapi check`. These steps require no AI provider.

## Closing

The writer preserves the surrounding structure and inline codes when applying
text edits. The same inspection and change-set workflow is available across
supported writable formats.
