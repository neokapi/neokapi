# Format Ops Learnings

Brief, factual, dated lessons from format-ops ritual runs: failure modes, fix
patterns, model quirks ([format-ops.md §2, §5](./format-ops.md)). Every ritual
run ends with a reflection note here; proposed prompt/rubric/process edits go
to the ledger's `pending[]` queue, never silently applied. The
`process-health` ritual prunes this file; pruned entries move to the Archive
section at the bottom, not deleted.

## Entry format

```markdown
### YYYY-MM-DD — <one-line lesson> (<ritual or phase>)

What happened, what it cost, and the mechanism that now prevents it.
Evidence: file:line / TestName / transcript pointer / diff.
```

## Learnings

### 2026-06-12 — Hand-edited registries need independent probes (design review, bootstrap)

The design review of the Editor axis found that a committed, hand-edited
integrations index is self-declaration, not measurement: the HEAD survey
showed exactly this failure mode already live — `bowrain/connector/wordpress.go`
labels its content `Format: "html"` while the html DataFormat reader never
runs, and figma's read-only-ness exists only as a runtime
`errors.New("figma publish not yet supported…")` in `Publish`, not as a
declared capability, so declared depth is undiscoverable without executing
(docs/internals/research/format-ops/eval/editor-integrations.md, "Missing for
determinism"). The fix is structural, not editorial: the rubric pins
**floor = min(declared, probed)** with a deterministic probe per level
([format-maturity.md §2.3](./format-maturity.md)), `integrations.yaml` sits on
the change-control list so a score-improving change may not edit it, and
`TestIntegrationsIndex` (`core/formats/maturity_test.go`) rejects evidence
paths that do not resolve on HEAD. The bootstrap seeding immediately exercised
the lesson: openxml's preview coverage is probeable only as shape-keyed
`STRUCTURE_RULES` detectors (no format id in the index), so the entry records
the probe gap in its notes instead of claiming a corroborated E1.
Generalization: any hand-edited registry that feeds a score (support.yaml,
vocabulary.yaml evidence cells, corpus.yaml origins) gets an independent
deterministic check, or its claims degrade to unknown.

## Archive

None yet.
