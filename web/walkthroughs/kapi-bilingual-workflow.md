---
id: kapi-bilingual-workflow
audience: developer
target_doc: docs/walkthroughs/kapi-bilingual-workflow.mdx
scenes:
  - id: bilingual-workflow
    kind: terminal
    binary: kapi
    duration_budget_seconds: 90
    fixtures:
      - bilingual-project
    smoke_contract:
      - kapi extract -p kapi.yaml --no-memory
---

## Story

The "extract → translate → merge" round-trip is the canonical bilingual
workflow. Extract emits XLIFF per target locale (with content memory pre-fill); the
translator (or a vendor's tool) fills `<target>` elements; merge applies
those back onto the source files and absorbs the new pairs into the content memory
with full provenance.

## Scene 1 — bilingual-workflow (terminal)

A complete round-trip on a small fixture project: seed the content memory from a
corporate TMX, extract per-locale XLIFFs, simulate a translator filling
them in, merge back, and use `kapi memory audit` to trace exactly what the
merge wrote.

## Closing

`kapi memory audit --batch` is the receipt: every merge is provenance-tagged,
so you can prove what came from which translator return.
