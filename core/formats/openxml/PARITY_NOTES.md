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
| native (this package) | 185 | 0 | 140 | 0 | 45 |

## Recently cleared

- 830-2.docx, 830-6.docx — empty placeholder run preservation
  inside active complex fields. parseRunWithFieldState now emits a
  SubTypeFieldChar sentinel carrying the verbatim
  `<w:r><w:rPr>...</w:rPr></w:r>` payload when a run with no body
  chunks but a non-trivial rPr is encountered AND the parser is
  inside an active field (cfs.active). Empty placeholders OUTSIDE
  field state continue to be dropped — 830-6.docx para 5 is the
  canonical case where Okapi collapses the paragraph to
  `<w:p><w:pPr/></w:p>` without the placeholder.
- 830-2.docx, 830-6.docx text-run rPr — explicit-off
  `<w:rtl w:val="0"/>` is preserved on per-run rPr when the same rPr
  carries other non-default-valued siblings. Empirical match against
  upstream's reference output for 830-2.docx text runs (whose rPr
  authors rFonts/b/color/sz/szCs/highlight/u alongside rtl). The
  reordered-zip.docx case (rtl as the SOLE rPr child) continues to
  strip — that fixture's reference output emits `<w:r><w:t>` with no
  rPr at all, so the strip is correct when post-strip rPr would
  collapse. The synthesised paragraph style still drops rtl=0
  unconditionally (stripToggleMirrorsFromCommon) per upstream's
  830-2 reference where the synth NF974E24F-a1 has no rtl child.
- 848-nested-tables-with-revisions.docx — dropDeletedRows and
  dropEmptyTables now recurse into the body of each retained row /
  table so deleted nested rows and post-deletion empty inner tables
  get pruned. Per ECMA-376-1 §17.4.78 (CT_Row), §17.4.16 (CT_Cell),
  and §17.13.5.13 (deleted table row), nested tables are legal cell
  content and the row-deletion revision applies independently at
  every depth.
- Mauris.docx — cleared as a side effect of the rtl-preservation +
  empty-placeholder fixes (was diverging on the same rPr-strip path).

- WSO synthesis cluster — vanish promotion + paired explicit-off
  bCs/iCs preservation in synth rPr (8 fixtures: 948-1, vertAlign,
  830-1/3/5, content_category_test, lang, PageBreak):
  - `bCs`/`iCs` were unconditionally stripped from the synthesised
    pStyle's rPr on LTR paragraphs. The strip now respects the
    paired explicit-off rule (when both `<w:b val="0"/>` AND
    `<w:bCs val="0"/>` appear in the common, the `bCs` clearing
    override is preserved — it's needed to clear an inherited
    `bCs` from the parent style chain). Mirrors writer.go's
    stripToggleMirrorChildren which already implemented the
    pairing rule on per-run sidecar rPr. References: ECMA-376-1
    §17.3.2.16/.17, RunParser.canBeSkipped at RunParser.java:240-250.
  - `<w:vanish/>` was excluded from WSO promotion pending paragraph
    -style→run inheritance support in the native reader. allHidden()
    now consults `styleMap.effectiveProps(paraStyleID).vanish` so a
    paragraph whose vanish was promoted into a synthesised pStyle
    stays hidden on re-read (the inherited vanish keeps the
    allHidden guard firing). Edge case: when the source has no
    word/styles.xml, the synth pStyle's val is empty (upstream's
    StyleDefinitions.Empty.placedId() returns null), so the
    inheritance path can't recover stripped vanish — the run-strip
    pass now skips vanish in that case to keep TestRoundtripFormatted
    green. References: ECMA-376-1 §17.3.2.42, StyleOptimisation.java
    :96-129, WordDocument.java:335-337 styleOptimisationsFor() (only
    rStyle is on the WPML exclusion list).
  - `<w:br/>` sentinel runs now carry the source `<w:r>`'s rPr
    (was being reset to `runProps{}` at parse time). Per ECMA-376-1
    §17.3.2.1 (CT_R) every rPr child applies to the run regardless
    of its payload — vanish-bearing page-break runs (PageBreak.docx
    `<w:r><w:rPr><w:vanish/></w:rPr><w:br w:type="page"/></w:r>`)
    must round-trip with the vanish so WSO can lift it.
  - extractTxbxParagraph now applies the same allHidden guard as
    the outer parseParagraph branch so vanish-bearing textbox runs
    (Hidden_Textbox.docx) get routed through the hidden-text
    skeleton path instead of being extracted as translatable.
- `<w:cr/>` (ECMA-376-1 §17.3.3.4 carriage return) is captured via
  the existing  raw-run-markup sentinel so it round-trips
  inside the same envelope as the source `<w:r>`'s rPr (mirrors
  noBreakHyphen / softHyphen). Was being silently dropped via
  default-branch skipElement; MissingPara.docx authors `<w:cr/>`
  runs that previously vanished entirely.
- Source text containing private-use sentinel codepoints
  (U+E100..U+E10F) no longer gets misclassified as a synthetic
  `<w:tab/>` / `<w:drawing/>`. The buildBlock dispatch now uses
  EXACT match (`run.text == ""`) instead of `HasPrefix` for
  the single-char sentinels. Cleared OkapiMarkers.docx,
  1335-doc-properties.docx, 1440-default-formatting.docx — see
  commit "openxml: don't misclassify source text starting with
  private-use sentinel codepoints".
- `<w:br>` element attributes (`w:type="page"`, `w:type="column"`,
  `w:clear`) now survive round-trip in BOTH the canInline and the
  TypeBreak emit paths. Reader captures the full br element into
  `textRun.data`, buildBlock propagates as `Ph.Data`, writer
  prefers it over the literal `<w:br/>`. ECMA-376-1 §17.3.3.1.
- Cross-source-run br + text fusion: standalone `<w:br/>` source
  run followed by a separate text source-run with matching rPr
  now fuses into one `<w:r><w:br/><w:t>...</w:t></w:r>` per
  upstream RunMerger's rPr-only canMergeWith gate. The previous
  textSrcStart guard was over-conservative; removing it preserves
  the 1421-line-break.docx case (its third br is itself
  SubTypeBreakStandalone so step-3 closeRun fires regardless).
  Cleared OpenXmlRoundtripSoftLineBreaksDoNotTranslateTest
  CharacterStyle.docx.
- Cross-source-run br + raw markup fusion: a standalone br/tab
  followed by `<w:noBreakHyphen/>` or `<w:softHyphen/>` (Markup
  chunks emitted via flushMarkup, no break-induced run boundary)
  now fuses into one `<w:r>`. Mirrors upstream
  BlockTextUnitWriter.java:240-251 + 349-371. Cleared
  special-chars-and-linebreaks.docx.
- `<w:ruby>` (ECMA-376-1 §17.3.3.25) is now captured as opaque
  markup using the existing image-sentinel placeholder. Previously
  the dispatcher's default branch dropped the entire ruby subtree
  (translatable text inside `<w:rt>` and `<w:rubyBase>` was lost).
  Bridge keeps ruby content inline in its reference output, so
  verbatim capture matches the round-trip envelope. Cleared
  HelloWorld.docx, sample.docx, SampleRuby.docx.
- `<w:sdt>` envelopes around block-level paragraphs now survive
  round-trip. parseSDT writes `<w:sdt>` to the skeleton on entry,
  captures sdtPr / sdtEndPr raw and emits them inside the wrapper,
  recurses into nested `<w:sdt>` children, and emits
  `</w:sdtContent></w:sdt>` on close. Cleared watermark.docx.
- Inline drawing/pict/object/AlternateContent/ruby fused into
  preceding text-run envelope: a new SubTypeImageInline marks
  image Ph chunks that did NOT begin their source `<w:r>` (text
  preceded them inside the same `<w:r>`). The writer fuses such
  chunks into the still-open envelope when both sides have
  rPr-empty, mirroring `RunMerger`'s same-rPr fusion. Cleared
  gettysburg_en.docx (P3 source `<w:r><w:rPr/><w:t>N</w:t>
  <w:drawing>...</w:drawing></w:r>` + following text run).
- DML run-property strippable attributes (`lang`, `altLang`,
  `dirty`, `smtClean`, `err`, `noProof`) are now scrubbed from
  `<a:rPr>`/`<a:endParaRPr>`/`<a:defRPr>` start tags inside
  `<a:p>` paragraph bodies of captured `<w:drawing>` payloads.
  The strip is scoped to paragraph blocks (matching upstream
  Okapi's `BlockParser` / `RunParser` /
  `ParagraphBlockProperties.refine` invocation contexts) so
  list-style and table-style `<a:defRPr>` defaults inside
  `<a:lstStyle><a:defPPr>` are preserved verbatim. Mirrors
  upstream Okapi's `StrippableAttributes.DrawingRunProperties`
  (StrippableAttributes.java lines 67-100). Implemented in
  `writer.go::stripDMLRunPropertyAttrs`, applied during the
  WML post-skeleton flush in `postNonWSOForName`. Reduced
  divergence on `DrawingML_Test.docx` and `Hidden_Textbox.docx`.
- `AltContentEscaping.docx` — bare `<w:t>` inside `<mc:Choice>`
  now extracted via the new TEXT marker in
  `extractDrawingTranslations` (commit `397836ca`). The previous
  state preserved the wrapper verbatim so the `<w:t>` text never
  reached the translation pipeline.
- WSO recursion into `<w:txbxContent>` paragraphs
  (`optimizeNestedParagraphs`, commit `bfaf0b15`) brings several
  drawing-bearing fixtures (AlternateContent.docx,
  AlternateContentTest.docx) much closer to canon-equality —
  inner textbox paragraphs now synthesise their own pStyle and
  styleId numbering aligns with upstream's per-document
  IdGenerator stream.

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

### 1145-colors* — writer slow-path collapses adjacent text runs

**Fixtures**: `1145-colors.docx`, `1145-colors-aggressive.docx`.

**Symptom (post-Phase-5)**: native still divergent despite the
per-run rPr trilogy. Native emits ONE `<w:r>` per paragraph carrying
the FIRST source run's rPr, losing colors from the remaining N-1
source runs. WSO then over-synthesises a `NF974E24F-Normal1`
paragraph style for the second paragraph (whose collapsed run has
non-empty rPr `<w:color w:val="A50021"/>`), since WSO sees a single
run with non-empty rPr and `commonProps([single])` = the rPr itself.

**Investigation summary**:

The Phase-5 agent's diagnostic ("WSO synthesises an extra paragraph
style — that's a follow-up in style_optimization.go") was incorrect.
Per-run rPrs are NOT actually preserved on the wire. WSO_DEBUG dump
of the WSO INPUT for `1145-colors.docx` shows:

```
<w:p><w:pPr><w:rPr><w:color w:val="C00000"/></w:rPr></w:pPr>
  <w:r><w:rPr><w:color w:val="A50021"/></w:rPr>
    <w:t xml:space="preserve">Ƥàŕàĝŕàƥĥ ŵĩţĥ ŕēď ćōĺōŕś.</w:t>
  </w:r>
</w:p>
```

Single `<w:r>` with all six source colors collapsed into the first
run's rPr (A50021). Bridge reference emits 6 distinct `<w:r>`
elements with their own per-run colors.

**Cause**: `renderWMLBlock`'s slow path
(`writer.go` ~line 1273-1300) keeps `inRun=true` across adjacent
`r.Text` runs — only `PcOpen`/`PcClose`/`Ph` runs call `closeRun()`.
Two adjacent text runs with different `effectiveRPr(idx)` values
fold into the same `<w:r>` because the second run's chars hit the
`if !inRun` skip and append directly without a new `<w:r>` wrapper
or fresh `emitRPr(idx)`. The per-run rPr sidecar slot 1+ is
effectively unreachable for adjacent text-text run pairs.

**Why a WSO-only fix won't help**: even bailing on style synthesis
in `style_optimization.go` only removes the synthesised `pStyle`
entry from `styles.xml`. The collapsed `<w:r>` in `document.xml`
still loses 5 of 6 source colors — `document.xml` stays divergent
on the colors alone.

**Real fix (writer.go, OUT OF SCOPE for this WSO-only iteration)**:
in the text-emission case at the top of the `for _, r := range
runs` loop, close the open `<w:r>` and reset state when transitioning
between two consecutive `r.Text` runs whose `effectiveRPr(textRunIdx)`
or `effectiveRPr(textRunIdx+1)` would differ. The closeRun()
helper already exists; the fix is conditioning it on rPr-slot
divergence between adjacent text runs (not just on toggle/code
boundaries). Mirrors upstream Okapi `RunMerger.canRunPropertiesBeMerged`
(`RunMerger.java:156-229`) — adjacent runs only fuse when
`RunProperties.equals()`; heterogeneous-rPr text runs MUST surface
as separate `<w:r>` elements.

Ground source for the writer fix:
- ECMA-376-1 §17.3.2.1 `<w:r>` Run — each `<w:r>` is a self-contained
  formatting unit; runs with different rPr must be separate runs.
- `okapi/filters/openxml/RunBuilder.java` lines 73-188 — every
  source run keeps its full rPr.
- `okapi/filters/openxml/RunMerger.java` lines 156-229 — adjacent
  runs only fuse when `RunProperties.equals()`.

### Other outstanding clusters

(Not investigated in this iteration — left as-is from the wider
divergence list. Look at the per-fixture parity report for current
status.)
