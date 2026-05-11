# OpenXML parity divergence — working notes

Snapshot of the native OpenXML reader/writer divergence against the
okapi reference engine in `cli/parity/roundtrip` (suite
`TestRoundTrip_Coverage/openxml`).

This file is a working note for the next iteration of OpenXML parity
work. It is **not** an architecture decision and **not** user-facing
docs.

## Status

| Engine | Total | byte | canon | sem | div |
|---|---:|---:|---:|---:|---:|
| bridge (okapi-bridge) | 185 | 185 | 0 | 0 | 0 |
| native (this package) | 185 | 0 | 53 | 0 | 132 |

## Outstanding clusters

### Heterogeneous per-run rPr (1083-* fixtures, plus a long tail)

**Fixtures**: `1083-date-and-hyperlink-instructions.docx`,
`1083-empty-and-hyperlink-instructions.docx`,
`1083-hyperlink-and-date-instructions.docx`,
`1083-hyperlink-and-empty-instructions.docx`. Likely a wider tail —
any paragraph whose source runs carry HETEROGENEOUS `<w:rPr>` (e.g.
some runs with `<w:rStyle>`, others without) hits this.

**Symptom**: A paragraph contains 2+ text-bearing source runs whose
`<w:rPr>` differ — typical case is a HYPERLINK complex field where
the displayed runs (after `<w:fldChar separate/>`) carry
`<w:rStyle val="Hyperlink"/>` and the surrounding runs do not. On
write, every run loses its distinctive `<w:rPr>` (the `rStyle`
disappears from the displayed runs).

**Cause**: The native pipeline collapses per-source-run rPr into a
**paragraph-wide common subset** at parse time
(`commonRPrChildren` in `source_rpr.go`, stashed under
`openxmlSourceRPrAnnotationKey`). The writer prepends this single
common rPr to every emitted `<w:r>`. When source runs have
heterogeneous rPr the intersection is empty (or strictly smaller than
any individual run's rPr), so distinctive children like `rStyle` are
lost on every output run.

Concretely for `1083-empty-and-hyperlink-instructions.docx`:

```
Source para 1:
  <w:r><w:rPr><w:lang/></w:rPr><w:t>A Text</w:t></w:r>
  <w:r ... fldChar begin/instrText/separate ... />            (field markup)
  <w:r><w:rPr><w:rStyle Hyperlink/><w:lang/></w:rPr><w:t> </w:t></w:r>
  <w:r><w:rPr><w:rStyle Hyperlink/><w:lang/></w:rPr><w:t>with</w:t></w:r>

commonRPrChildren = ∅ (run "A Text" lacks rStyle, lang is stripped)

Native output: every emitted <w:r> for translatable text has NO rPr,
so "with" loses its rStyle=Hyperlink.

Upstream Okapi output: each run keeps its own rPr verbatim.
```

**Upstream contract** (`okapi/filters/openxml/RunBuilder.java` 73-188,
`RunMerger.java` 156-229): every source run keeps its **full** rPr.
RunMerger only fuses adjacent runs whose `RunProperties.equals(...)`
(line 167). Heterogeneous-rPr paragraphs surface multiple `<w:r>`
elements, each with its own rPr. `WordStyleOptimisation`
(`StyleOptimisation.java` 96-237) lifts a paragraph's common rPr
**into a synthesised pStyle** rather than dropping it from the runs;
the per-run rPr stays put.

**Why current approach falls short**: `commonRPrChildren` is the
right input for **WSO**'s synthesised-pStyle path, but the writer
ALSO uses it as the per-run rPr — there's no separate
"per-run distinctive rPr" channel. Pseudo-translation preserves
1:1 source→target text-run correspondence
(`core/tools/pseudo.go` 327-359), so the model retains enough run
identity to support per-run rPr on write — what's missing is a
storage + emission channel.

**Fix sketch**: introduce a per-run rPr sidecar:

1.  Reader stashes a list of per-run rPr (rPrChildren XML strings),
    aligned with the text-bearing runs in source order, under a new
    annotation (e.g. `openxmlPerRunRPrAnnotationKey`). The list is
    ordered the same as `block.Source[0].Runs` text-bearing entries.
2.  Writer walks `preferredRuns(block)` and tracks a text-run index
    that increments on each `model.Run.Text` it emits. For each run,
    it consults the per-run rPr sidecar at that index and emits
    `<w:rPr>{per-run rPr}</w:rPr>` instead of (or in addition to,
    minus the common subset) the current paragraph-wide
    `sourceRPr`.
3.  When the per-run sidecar is **identical for every run** the
    writer can collapse to the existing `sourceRPr` path (and WSO
    can still lift it into a pStyle). When it differs, each
    distinct run gets its own rPr.
4.  WSO (`style_optimization.go`) already bypasses paragraphs whose
    runs carry exclusion properties (`rStyle`, `vanish`); the
    sidecar interacts cleanly because WSO operates on the writer's
    output, not on the model.

**Cost estimate**: medium. Reader changes are localised
(`buildBlock` populates the sidecar from `runs []textRun`). Writer
changes touch `renderWMLBlock` (and possibly the SML/DML fast
paths) to consume the sidecar. Pseudo-translation already preserves
per-run identity; AI/MT translation may not — needs verification on
real-translation flows.

**Why it isn't done in this commit**: out of scope for the time-box
(30-min investigation). The "opaque-chunk dragging" hypothesis from
de1848ab's note turned out to mis-characterise the problem — it's
not about cross-paragraph field state, it's about per-run rPr loss
that happens to be most visible on complex-field fixtures because
HYPERLINK display text always carries `rStyle`.

### Other outstanding clusters

(Not investigated in this iteration — left as-is from the wider
divergence list. Look at the per-fixture parity report for current
status.)
