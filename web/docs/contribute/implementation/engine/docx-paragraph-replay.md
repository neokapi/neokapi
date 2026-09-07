---
sidebar_position: 3
title: Docx paragraph replay
description: "Implementation note for E-02: the measured reasons a WordprocessingML round trip rewrites bytes it did not need to touch, and a design that replays an unchanged paragraph's source span instead of rebuilding it from the model."
keywords: [docx, WordprocessingML, paragraph replay, byte fidelity, skeleton, SkeletonOriginal, OpenXML, parity, neokapi]
---

# Docx paragraph replay

An untranslated round trip through `core/formats/openxml` returns 86 of 87 xlsx
fixtures and 62 of 86 pptx fixtures byte for byte, and 1 of 185 docx. This note
measures why, and designs the change that closes most of the gap. Parent AD:
[E-02: The format system](/contribute/architecture/engine/e-02-format-system).
The store mechanics it builds on are in
[Skeleton store and streaming](/contribute/implementation/engine/skeleton-store).

The short answer is two mechanisms rather than one. The WordprocessingML writer
rebuilds every extracted paragraph from the model, so an unchanged paragraph is
re-serialised. Separately, a writer-side normaliser removes skippable elements
from whole parts, including parts the reader never extracted from. Each one
alone leaves every fixture differing; both have to be addressed for any fixture
to come back whole.

## The measured baseline

Every one of the 185 docx fixtures upstream Okapi ships was read and written
back with no translation applied, using the same reader and writer path as
`skeletonRoundtripBytes` in `core/formats/openxml/validity_test.go`. Each ZIP
entry that differed was split into its outermost `<w:p>` subtrees and the
material between them, and both sides were normalised one step at a time until
they agreed. The step that first produced agreement names the mechanism.
Paragraphs were aligned by index within a part; where the paragraph count
itself changed, the part is counted under paragraph sequence rather than
attributed further. The measurement scripts live outside the repo, and the
figures below are reproducible from the fixture corpus alone.

`887.docx` already returns byte for byte. The other 184 differ.

### Where the bytes differ

| Place | Fixtures |
| --- | --- |
| Inside a `<w:p>` in a paragraph-bearing part | 184 |
| `word/styles.xml`, from the skippable-element strip | 179 |
| Inside a `<w:p>`, from the same strip | 118 |
| `word/document.xml`, outside any paragraph, from revision acceptance | 18 |
| `word/document.xml`, outside any paragraph, other | 8 |
| `word/document.xml`, outside any paragraph, from the strip | 9 |
| A second styles part the same predicate covers, from the strip | 2 |
| `docProps/core.xml` | 2 |
| `word/charts/chart1.xml` | 2 |
| `word/diagrams/data1.xml` | 1 |

`word/styles.xml` is the single most common place a docx loses bytes, and no
paragraph is involved: `stripWMLSkippableElements` in `writer.go` removes
`<w:lang>` and `<w:noProof>` from every part `shouldStripWMLLang` names, then
collapses the `<w:rPr>` and `<w:pPr>` containers those elements emptied. The
strip mirrors upstream Okapi's `RunSkippableElements`, it runs over the
assembled part after the skeleton is filled in, and it is already excluded by
name from `TestByteFidelity_CorpusUntouchedParts` for exactly this reason.

The predicate covers the main document's parts and the parallel
WordprocessingML package ECMA-376-1 §17.12.7 defines, so two fixtures lose
bytes in both styles parts:

```
word/styles.xml
word/glossary/styles.xml
```

### Mechanisms inside a paragraph

The first column counts fixtures in which the mechanism is observable at all.
The second counts paragraphs. The third counts paragraphs where that mechanism
is the cheapest one that explains the whole difference, so the three columns
answer prevalence, volume, and sufficiency in turn.

| Mechanism | Fixtures | Paragraphs | Sufficient |
| --- | --- | --- | --- |
| `<w:t>` attributes rewritten (`xml:space`) | 154 | 1058 | 39 |
| `<w:p>` start attributes dropped (`w:rsid*`, `w14:paraId`, `w14:textId`) | 153 | 1758 | 0 |
| Self-closing form expanded | 147 | 824 | 4 |
| Skippable-element strip | 121 | 345 | 28 |
| Run boundaries merged | 102 | 488 | 92 |
| Run properties rebuilt from the model | 75 | 481 | 15 |
| Inter-element whitespace collapsed | 51 | 115 | 0 |
| Residual past all of the above | 54 | 132 | 54 |

Two rows deserve a note. The `<w:p>` start attributes row is sufficient for no
paragraph on its own because the writer emits a literal `<w:p>` into the
skeleton (`wml_paragraph.go` lines 962, 1050, 1236, 1336) and discards the
source start tag, so every paragraph that loses its attributes also loses
something else in the same breath. The self-closing row counts the embedded
markup a paragraph carries rather than the paragraph itself: a footer holding a
`<mc:AlternateContent>` drawing comes back with `<wp:simplePos x="0" y="0"/>`
written as `<wp:simplePos x="0" y="0"></wp:simplePos>` for every empty element
in the anchor, because that subtree travels through the model and out through
Go's `xml` encoder. The skeleton's own pass-through already replays source
bytes; a paragraph's interior does not.

The residual row breaks down into a complex field whose result runs the writer
reconstructs (12 fixtures), a `<w:lastRenderedPageBreak>` the model does not
carry (7), a `<w:t>` split the run merger cannot express (17), a `<w:sdtEndPr>`
dropped from a structured document tag (2), and a handful of DrawingML
paragraphs inside charts and SmartArt.

### What each lever is worth

| Change | Fixtures byte-identical, of 185 |
| --- | --- |
| Today | 1 |
| Paragraph replay alone | 1 |
| Paragraph replay with the skippable-element strip confined to extracted content | 161 |

Paragraph replay on its own moves nothing, because 179 of the 184 differing
fixtures also lose bytes in `word/styles.xml` and the remaining 5 lose bytes to
the same normaliser inside a paragraph. Confining the strip on its own moves
nothing either, because all 184 differ inside a paragraph. Together they reach
161.

The 24 that stay differing are 18 whose paragraph sequence changes under
revision acceptance, 8 whose `word/document.xml` loses table rows or tables to
the same acceptance passes, 2 whose `docProps/core.xml` loses a UTF-8 byte order
mark, 2 chart parts and 1 SmartArt part whose DrawingML paragraphs go through
the pptx-side rebuild. The sets overlap.

## The design

### Replay an unchanged paragraph whole

`rawxml.go` already provides everything the reader needs. `d.Offset()` reports
where the token the decoder last returned begins, `d.Pin(off)` holds the source
window open across a subtree, and `d.From(off)` returns the bytes from that
offset through the end of the token last returned. Pinning at the `<w:p>` start
in `parsePart` and taking `From` when `parseParagraph` returns gives the
paragraph's exact source span, including its start-tag attributes, its
self-closing forms, its character-reference spellings and the whitespace a
producer indented it with.

The writer decides whether to use it the way `smlSourceContent` decides for a
shared string: the test is the content about to be emitted, not the presence of
a target. A paragraph is unchanged when the markup the writer is about to
produce for it equals the markup the same path produces from its blocks' source
runs. A target that says what the source said replays; a target that says
something else is rendered, because its runs are not the source's runs and the
layout between them has nowhere to go.

Storing only the source span, and letting the writer render the paragraph a
second time from the source runs to test it, follows the SpreadsheetML
precedent and keeps the skeleton small. The alternative is the ODF and HTML
contract, `SkeletonOriginal`, which carries an `EncodeSkeletonPair(rendered,
original)` payload so the writer compares against a stored form instead of
re-rendering. That costs roughly double the skeleton bytes for a docx, since
almost every paragraph qualifies. Both are listed as a decision point below.

### The skeleton shape

A paragraph currently appears in the skeleton as literal `<w:p>` text, the
re-serialised `<w:pPr>`, the run envelopes, a `SkeletonRef` for the block, and
literal `</w:p>`. The ref stands for the run content only, so an entry that
replaces the ref cannot replace the frame around it.

The reader brackets the paragraph with two marker refs instead, the device the
ODF writer already uses for part boundaries (`skelPartStartPrefix` and
`skelPartEndPrefix` in `core/formats/odf/writer.go`). The writer records the
current length of the part buffer at the start marker together with the pending
source span, renders the paragraph exactly as it does today, and at the end
marker compares what it produced with the source-run render. On a match it
truncates the buffer back to the mark and writes the source span. Buffering is
bounded by one paragraph, and a writer that does not recognise the markers
takes its ordinary path, so a skeleton written by a newer reader still writes
through an older writer.

### A changed paragraph

Replaying a whole paragraph answers the untranslated round trip. A translated
paragraph needs the same faithfulness for everything the translation did not
touch, which is the frame around the runs and the runs whose text is unchanged.

The reader records three more spans per paragraph: the `<w:p>` start tag, the
`<w:pPr>` subtree, and one span per source run. The writer then replays the
start tag and the paragraph properties verbatim, and for each model run either
replays its source span or renders it, projecting run properties through
`runPropsProjection` in `run_projection.go`. Recovering the start tag alone
returns the 1758 paragraphs across 153 fixtures that lose `w:rsidR`,
`w14:paraId` and `w14:textId` today, and it returns them for translated
documents as well as untranslated ones.

Per-run spans need a model run to map back to a source run. The writer already
carries per-run sidecars (`perRunRPr`, `perRunSrcStart`) with an alignment
guard that drops the sidecar when the counts disagree, because `mergeRuns` can
coalesce source runs whose non-toggle properties differ. The span sidecar takes
the same guard: when the writer cannot align model runs to source runs one to
one, it renders every run in the paragraph. The spans travel as an attribute on
the run's opening code, beside `smlRPr` and `dmlRPr` in `runprops.go`, rather
than in the code's `Data`, because the HTML export echoes any `Data` beginning
with `<`.

### What lives inside a paragraph

**Tracked changes.** With `AutomaticallyAcceptRevisions` set, which is the
default, `parsePart` rewrites the part bytes before the streaming parser sees
them: `dropMoveFromRanges`, `dropDeletedRows` and `dropEmptyTables` remove
moved-from spans, deleted rows and the tables those rows emptied. The writer
then strips revision property changes and paragraph marks. A replayed span
would reinstate all of it. Replay is therefore suppressed for any paragraph the
acceptance pre-pass touched and for any paragraph holding a revision element
while acceptance is on. These are the 18 fixtures whose paragraph sequence
changes.

**Comments, footnotes and endnotes.** `<w:commentRangeStart>`,
`<w:commentRangeEnd>` and `<w:footnoteReference>` are position markers with no
text of their own, and their targets live in `word/comments.xml`,
`word/footnotes.xml` and `word/endnotes.xml`. Replay keeps them exactly where
the source put them. Those parts hold their own paragraphs and get the same
treatment, which is why the measurement shows differences in them today.

**Fields.** A complex field runs from `<w:fldChar w:fldCharType="begin"/>`
through `<w:instrText>`, a separate marker, the result runs, and an end marker,
and it can straddle paragraph boundaries; `wml.go` keeps the state in
`partCfs` for that reason. 18 fixtures hold a field that crosses a paragraph
boundary. Replaying every paragraph in such a span reproduces the field
verbatim and needs no state machine, but replaying some and rendering others
leaves a boundary the writer's extractability logic never agreed to. Replay is
therefore decided for the whole span: if any paragraph the field crosses is
changed, none of them is replayed.

**Bookmarks.** `<w:bookmarkStart>` and `<w:bookmarkEnd>` are direct paragraph
children that some paths drop. A replayed paragraph keeps them, which is a
parity question rather than a correctness one.

**Hyperlinks.** `<w:hyperlink r:id="rId7">` carries a relationship id that the
writer rewrites when the target changes, and `wml_hyperlink.go` preserves every
other attribute verbatim already. Replay must not cover a paragraph whose
hyperlink relationship changed, which is a level-two concern; an untranslated
round trip never rewrites one.

### Composition with the skeleton store

Nothing new is needed in `core/format/skeleton.go`. Marker refs are ordinary
`SkeletonRef` entries, and `SkeletonOriginal` is already defined, already
carried in the entry stream, and already consumed by the ODF and HTML writers.
The store stays append-only and streaming-safe: a paragraph's span is written
before its ref, so a streaming reader and writer sharing the store keep their
existing ordering contract.

## Cost and risk

### Files

| File | Change |
| --- | --- |
| `core/formats/openxml/wml.go` | Pin and unpin the source window around `parseParagraph`; emit the paragraph markers |
| `core/formats/openxml/wml_paragraph.go` | Capture the paragraph span, the start tag and the `<w:pPr>` span; suppress replay for revision and field-straddling paragraphs |
| `core/formats/openxml/writer.go` | Paragraph mark and truncate in the skeleton loop; the source-run render gate; the `xml:space` safety gate |
| `core/formats/openxml/wml_run.go`, `runprops.go` | Per-run source spans and their alignment guard |
| `core/formats/openxml/byte_fidelity_test.go` | Drop the `word/styles.xml` exclusion once the strip is confined |
| `cli/parity/roundtrip/normalizers.go` | A canonicaliser option for the skippable-element strip and one for adjacent-run merging |

### The corpus test that gates it

`TestWMLParagraphReplay_CorpusIsByteIdentical`, modelled on
`TestSMLSourceForm_CorpusIsByteIdentical` in `sml_source_form_test.go`: every
docx fixture in `okapi-testdata` goes back byte for byte on an untranslated
round trip, with a named exclusion map carrying a reason per fixture. The
target is 161 of 185, with the 24 exclusions named and grouped.

`TestByteFidelity_CorpusUntouchedParts` already states the weaker contract for
parts the reader extracted nothing from. Removing its `word/styles.xml`
exclusion is the assertion that gates the strip work on its own.

### The parity slice

The parity tier compares neokapi output with upstream Okapi output after
canonicalisation, and the openxml chain in
`cli/parity/roundtrip/coverage_test.go` already folds most of what replay would
restore. `OpenXMLEffectiveRPr` resolves both sides' run properties per
ECMA-376-1 §17.7, and the `XMLCanonical` pass that follows sets `SortAttrs`,
`SortChildElements`, `StripRevisionIDs`, `StripXMLSpacePreserve` and
`StripNamespaceDecls`, while re-emitting both sides through `encoding/xml`,
which normalises self-closing form and inter-element whitespace. Paragraph
attributes, `xml:space`, self-closing form, whitespace and run-property
placement are therefore invisible to the parity comparison, and replaying them
costs nothing there.

Two things the canonicaliser does not fold. Okapi's `RunMerger` fuses adjacent
runs whose properties are equal, and a replayed paragraph keeps the source's
split; 102 fixtures carry a run-count difference today. Okapi's
`RunSkippableElements` drops `<w:lang>` and `<w:noProof>`, and a confined strip
keeps them; 179 fixtures carry that difference in `word/styles.xml` alone. Both
are cosmetic under the spec, both would otherwise need per-fixture annotations
in `parity-annotations.yaml`, and both have the same cheap remedy: two more
`XMLCanonical` options applied to both sides, shaped like `StripRevisionIDs`
and like the `MergeAdjacentCSRs` option IDML already uses.

### Failure modes

**A replayed paragraph beside a rebuilt one.** The skippable-element strip runs
over the assembled part rather than over each paragraph, so a replayed
paragraph and a rebuilt one in the same part go through the same pass and stay
consistent for `<w:lang>`. What does survive is run splitting: a replayed
paragraph keeps the source's runs while its rebuilt neighbour has merged them.
Both forms are valid WordprocessingML and render identically under ECMA-376-1
§17.3.2, so the exposure is cosmetic drift within one document.

**`xml:space`.** Six fixtures hold a `<w:t>` whose text has leading or trailing
whitespace and which declares no `xml:space="preserve"`: `1200-1.docx`,
`952-1.docx`, `AlternateContentTest.docx`, `Escapades.docx`,
`StartsWithLineSeparator.docx` and `special-chars-and-linebreaks.docx`.
Replaying those bytes hands a consumer text it is entitled to trim.
SpreadsheetML settled this by repairing rather than replaying
(`smlTextIsSpaceSafe`, with `948-3.xlsx` excluded from the corpus test by
name), and WordprocessingML takes the same gate and the same six named
exclusions.

**Namespace prefixes.** A replayed span carries whatever prefixes were bound
where it was read. The writer assembles a part from skeleton text that already
holds the root element's declarations, so a span replayed into its own part
resolves. Replay is part-local for that reason, and the cross-format export
path, which builds from the event stream rather than the skeleton, is
untouched.

## Recommendation

Do the work, in two changes on one branch, and take the parity decision first
because it gates both.

**Change one: confine the skippable-element strip.** `stripWMLSkippableElements`
stops running over parts the reader extracts nothing from, and stops running
over paragraph interiors the reader does extract. Gated by removing the
`word/styles.xml` exclusion from `TestByteFidelity_CorpusUntouchedParts`, and
by a new `XMLCanonical` option that applies the strip to both sides of the
parity comparison. Measurable output: 179 fixtures stop differing in
`word/styles.xml`. Package-level count stays at 1, which is expected.

**Change two: paragraph span replay.** The reader records the paragraph span and
the marker refs, the writer gates on the source-run render, and the corpus test
asserts the package. Measurable output: 1 to 161 of 185.

**Later, if the remaining 24 matter.** Revision acceptance is the largest group
at 18 and is already a configuration toggle, so the question there is the
default rather than the mechanism. The chart and SmartArt parts (3) belong with
the DrawingML paragraph work that issues 2532 through 2535 already describe.
The `docProps/core.xml` byte order mark (2) is a two-line fix in the writer.
The 8 body-markup fixtures overlap the revision group almost entirely.

Level two, replaying the frame of a changed paragraph, is worth doing in the
same pass as change two for the reader work, since the spans it needs are
recorded at the same point. Its payoff is not visible in this corpus, which
measures untranslated round trips, and would need a translated-corpus
measurement of its own.

### Decisions to take

1. **Does neokapi keep Okapi's skippable-element strip?** Confining it is what
   makes 179 fixtures reachable and is the larger half of the work's value. It
   moves neokapi's `word/styles.xml` output away from Okapi's, which the parity
   canonicaliser can absorb with one option applied to both sides. The
   alternative is to accept that a docx never round-trips whole.
2. **Are source run boundaries replayed or merged?** Replay keeps the source
   split in 102 fixtures where Okapi merges. Either a canonicaliser option
   cancels it or those fixtures carry annotations.
3. **Source span alone, or the `SkeletonOriginal` pair?** The span alone follows
   `smlSourceContent` and costs a second render per paragraph on write. The
   pair follows ODF and HTML and roughly doubles a docx skeleton. The span
   alone is the recommendation, with the pair as the fallback if the second
   render shows up in a profile.
4. **Does revision acceptance stay on by default?** 18 fixtures cannot return
   byte for byte while it is, and the flag is already reachable through the
   format configuration.
