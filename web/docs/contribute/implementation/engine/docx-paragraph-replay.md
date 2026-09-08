---
sidebar_position: 3
title: Docx paragraph replay
description: "Implementation note for E-02: how a WordprocessingML round trip puts an unchanged paragraph back as the bytes its author wrote, what a rendered paragraph still keeps of them, and what the corpus and the parity slice measure."
keywords: [docx, WordprocessingML, paragraph replay, byte fidelity, skeleton, OpenXML, parity, neokapi]
---

# Docx paragraph replay

An untranslated round trip through `core/formats/openxml` returns 150 of the
185 docx fixtures upstream Okapi ships byte for byte, beside 86 of 87 xlsx and
86 of 86 pptx. This note describes the mechanism that does it for
WordprocessingML, what it declines to replay, and what the corpus test and the
parity slice measure. Parent AD:
[E-02: The format system](/contribute/architecture/engine/e-02-format-system).
The store mechanics it builds on are in
[Skeleton store and streaming](/contribute/implementation/engine/skeleton-store).

The writer projects an extracted paragraph from the model, so a paragraph that
is rebuilt is re-serialised: its start-tag attributes, the self-closing forms
inside it, the character references, the whitespace a producer indented it
with, the runs the reader merged, and the run properties it rebuilt all come
out in the writer's spelling. Replay keeps the source's spelling wherever
nothing changed.

## What a rebuilt paragraph loses

Measured over the 185 fixtures before replay existed, with each differing
paragraph normalised one step at a time until both sides agreed. The first
column counts fixtures in which the mechanism is observable, the second
counts paragraphs, and the third counts paragraphs where that mechanism alone
explains the whole difference.

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

The self-closing row counts the markup a paragraph carries rather than the
paragraph itself: a drawing's `<mc:AlternateContent>` subtree travels through
the model and out through Go's `xml` encoder, so every empty element in the
anchor came back as a start and end pair. The residual row is complex fields
whose result runs the writer reconstructs, `<w:lastRenderedPageBreak>`, `<w:t>`
splits the run merger cannot express, and a `<w:sdtEndPr>` dropped from a
structured document tag.

Two things had to change together for any fixture to come back whole. The
writer's skippable-element strip ran over whole parts after assembly,
`word/styles.xml` included, so 179 fixtures lost bytes in a part the reader
never touched; it now reaches rendered content only (`renderBlock`), and a
separate pass, `stripWMLRevisionElements`, still removes revision markup from
whole parts because accepting revisions is a decision about the document. With
that in place, replay is what moves the count.

## The mechanism

### The reader records the span

`rawxml.go` gives the reader what it needs: `d.Offset()` is where the token
the decoder last returned begins, `d.Pin(off)` holds the source window open
across a subtree, and `d.Range(from, to)` returns the bytes between two
offsets. `parseParagraph` pins the `<w:p>` start on entry, keeps the raw
start tag, and takes the paragraph's span at the end element, so it holds the
exact source bytes: the start-tag attributes, the self-closing forms, the
character-reference spellings and the whitespace.

The reader brackets the paragraph's skeleton with marker refs, the device the
ODF writer already uses for part boundaries (`skelPartStartPrefix` in
`reader.go`), so no new skeleton entry type is needed and a writer that does
not know the markers takes its ordinary path. `wml_replay.go` names them:

- `@@SKEL_PARA@@` and `@@SKEL_PARA_BLOCK@@`, each followed by the paragraph's
  span, precede the paragraph's skeleton. The second says the skeleton is the
  paragraph's frame around one block ref, which is what lets the writer replay
  the paragraph's unchanged children into a rendered block.
- `@@SKEL_REGION_END@@` closes the region after the paragraph;
  `@@SKEL_REGION_END_SPAN@@` followed by a span closes a region holding
  several paragraphs; `@@SKEL_REGION_SKIP@@` closes it with no replay.
- `@@SKEL_REGION_START@@` opens a region ahead of a paragraph the reader holds
  back until it knows where a complex field ends, so the region also covers
  the skeleton written between that paragraph and the next while it waited,
  and the replayed bytes land in source order.

Every emission path in `parseParagraph`, and the two flushes in `wml.go` for
paragraphs the reader defers, write the same markers; `replayParagraphBegin`
and `replayRegionEnd` hold the state. A paragraph that is dropped, or merged
into its neighbour, makes the region it belongs to ineligible, because the
region's span would put it back.

The rendered paragraph reopens with the source's start tag (`wmlParagraphOpenTag`
drops the slash of a self-closing tag, since the writer closes the element
itself), so `w:rsidR`, `w14:paraId` and `w14:textId` survive a translation too.

### The writer decides on the render

The writer decides the way `smlSourceContent` decides for a shared string:
the test is the content about to be emitted. `wmlReplayState` marks the part
buffer at the region's start and renders every block inside as usual. A block
is unchanged when its rendering for the target equals its rendering from the
source runs, which `renderBlockFromSource` produces with `Writer.fromSource`
set so `preferredRuns` answers with the source, nested textbox blocks
included. At the region's end, if no block differed, the buffer is truncated
back to the mark and a placeholder stands in for the span.

The second render happens only for a block whose rendering can differ:
`changedBlocks` names every block with target runs for the locale and,
transitively, every block whose payload carries a drawing marker for one, so
an untranslated round trip renders each block once and a translated one
renders each translated paragraph twice. A target that says what the source
said, codes included, replays; a target with the same text and no formatting
is a change.

A tool such as `kapi sed` with no target locale edits a block's source runs
in place, and a rendering from an edited source is its own comparison. The
reader therefore stamps every block it emits with a digest of the source
runs (`source_fingerprint.go`, the `openxml-source-fingerprint` annotation),
and a writer replays nothing over a block whose runs no longer match it:
the docx region renders such a block whole, and the SpreadsheetML and
DrawingML source forms are held back the same way. A block that reaches the
writer without the stamp is treated as edited, so a path that drops
annotations loses replay rather than an edit.

The spans wait as placeholders (`<!--kapi-replay-N-->`) until the part's
post-passes have run, the revision strip and the run fusions in `postWML`
among them, and `restoreWMLReplays` puts them back last. No pass rewrites
bytes the source wrote.

### A changed paragraph keeps what did not change

For a paragraph whose block rendered differently, `replayUnchangedChildren`
walks three lists of the paragraph's direct children: the source span's,
the source rendering's and the target rendering's. `wmlParagraphChildren`
lists the children the reader renders and skips the ones it never reads
(`<w:pPr>`, proofing marks, permission ranges, Word's `_GoBack` bookmark). When
the three lists have the same length and a child has the same name in all
three and rendered the same from source and target, the source's bytes for it
go into the output; otherwise the rendered child does. A run the reader
merged with its neighbour, or a revision wrapper it unwrapped, breaks the
alignment or the name match and is rendered, which is what keeps a wrapper's
bytes out of an accepted document. A translated paragraph therefore keeps the
bytes of its unchanged drawing runs, bookmarks, comment ranges and field
markers, and renders its text.

### What is not replayed

**Tracked changes.** With `AutomaticallyAcceptRevisions` set, which is the
default, the reader's pre-pass drops what a revision deletes, the paragraph
parser unwraps what it inserts, and the writer strips the markers. A replayed
span would put the revision back, so `paragraphReplayable` renders any
paragraph holding a revision wrapper, a paragraph-mark marker, a
property-change snapshot or a move-range marker while acceptance is on. With
acceptance off the same paragraph replays.

**Fields.** A complex field can straddle paragraph boundaries, and the reader
holds a paragraph back while an extractable field is still open at its end
(`partFieldStraddle`). The region stays open while a field is open across a
paragraph's end, so the paragraphs a field crosses are replayed together or
rendered together; `pullLeadingFldCharEndIntoPrevParagraph` never sees a
replayed paragraph beside a rendered one.

**`xml:space`.** ECMA-376-1 §17.3.3.31 leaves whitespace handling in `<w:t>` to
XML, and a consumer may collapse a leading or trailing space where the
attribute is missing. The writer adds it, and `wmlTextIsSpaceSafe` renders a
paragraph rather than replaying it over that repair; SpreadsheetML has the
same rule in `smlTextIsSpaceSafe`. XML whitespace is space, tab, carriage
return and line feed: a narrow no-break space or a line separator at the edge
of a `<w:t>` is content, and such a paragraph replays.

**Nested content.** A textbox paragraph inside a drawing, a `<m:nor/>` prose
span inside an equation and a drawing's alt text are blocks of their own.
Each is a ref inside the host paragraph's region, or a marker in the host
block's payload, and is compared like any other; a translated textbox renders
its host paragraph.

**Core properties.** `docProps/core.xml` keeps its byte order mark: Go's
decoder reports it as character data in the prolog, so `parseCoreProperties`
holds it back from the decoder and writes it to the skeleton as it was.

## The corpus test

`TestWMLParagraphReplay_CorpusIsByteIdentical` in
`wml_paragraph_replay_test.go` asserts every docx fixture in `okapi-testdata`
goes back byte for byte on an untranslated round trip, with the exclusions
named in `docxReplayExclusions` and a reason per group;
`TestWMLParagraphReplay_ExclusionsAreStillNeeded` fails for a fixture that no
longer needs its entry.

| Group | Fixtures | Reason |
| --- | ---: | --- |
| Revision acceptance | 30 | A paragraph holding revision markup is rendered, and the accepted document differs from the source by design |
| DrawingML paragraphs in chart and diagram parts | 3 | Rebuilt from the model, and a DOCX package takes Okapi's `DrawingRunProperties` attribute strip on write |
| A trailing self-closing core property | 1 | `parseCoreProperties` drops it as Okapi's Jericho-based parser does |
| The `xml:space` repair | 1 | `952-1.docx` holds a `<w:t>` with a trailing space and no `xml:space="preserve"` |

`TestByteFidelity_CorpusUntouchedParts` states the weaker contract for parts
the reader extracted nothing from; its exclusions are the same predicates
applied to a part.

## The parity slice

The openxml chain in `cli/parity/roundtrip/coverage_test.go` compares native
output with upstream Okapi's after canonicalisation. Replay restores what
Okapi's filter drops, so the canonicaliser applies the same omissions to both
sides:

- `OpenXMLEffectiveRPr{StripSkippableElements: true}` drops the skippable
  properties and the containers they empty before the §17.7 cascade runs,
  so a paragraph mark one side dropped is not filled with effective
  formatting on the other.
- `XMLCanonical{StripWMLSkippableElements: true}` drops `<w:lang>`,
  `<w:noProof>`, `<w:bidiVisual>`, proofing marks, permission ranges,
  `<w:lastRenderedPageBreak>`, the `_GoBack` bookmark and its end, `<w:bCs>`
  and `<w:iCs>` on a run with no complex-script text, every empty `<w:rPr>`,
  `<w:pPr>` and `<w:sdtEndPr>`, and every run left with nothing but its
  properties.
- `XMLCanonical{MergeAdjacentWMLRuns: true}` fuses adjacent runs with equal
  properties and the text elements inside them, the shape Okapi's `RunMerger`
  writes.

Over the 185 docx fixtures, native output at the canonical tier moved from 146
before either change to 149 with the strip confined and 155 with replay; the
bridge stays byte-equal on all 185. The slice has no tier floor for openxml
native, so the tier table in a verbose run is the measurement.

## Cost

The write half of a round trip on the five largest fixtures, measured with
`go test -bench` on one machine before and after replay (milliseconds per
write, median of three runs of five). "Translated" brackets every text run.

| Fixture | Size | Before, untranslated | Before, translated | After, untranslated | After, translated |
| --- | ---: | ---: | ---: | ---: | ---: |
| `apissue.docx` | 988 KB | 4.2 | 4.3 | 2.5 | 6.4 |
| `large-attribute.docx` | 925 KB | 5.3 | 5.1 | 23.7 | 5.5 |
| `content_category_test.docx` | 706 KB | 1.4 | 1.4 | 1.3 | 1.5 |
| `delTextAmp.docx` | 554 KB | 3.1 | 3.0 | 2.7 | 3.1 |
| `Hangs.docx` | 279 KB | 11.3 | 11.4 | 7.1 | 23.5 |

An untranslated write renders each block once and replays, and is faster than
before on four of the five. A translated write carries the second render and
the child alignment for every translated paragraph, which costs half as much
again on `apissue.docx` and twice as much on `Hangs.docx`, whose paragraphs
hold VML shapes the source render has to expand too. Allocation per write is
unchanged.

`large-attribute.docx` is the exception in the untranslated column, and the
time is deflate rather than replay. Its one drawing carries a megabyte of
base64 in an `o:gfxdata` attribute with an `&#xA;` reference every 76
characters; the source spelling deflates in 21.6 ms against 0.9 ms for the
same attribute with raw newlines, which is what the writer used to produce.
An XML parser normalises a raw newline in an attribute value to a space
(XML 1.0 §3.3.3), so the source spelling is the one that keeps the shape.

The skeleton grows by one span per paragraph, which is the size of the
paragraph-bearing parts once more.
