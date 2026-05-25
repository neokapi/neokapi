# MIF corpus — provenance

Real-world Adobe FrameMaker Maker Interchange Format (`.mif`) files vendored
verbatim from the Okapi Framework's MIF filter test resources. Each file is
byte-identical to its pinned source. MIF is a whitespace- and statement-fragile
text container, so the corpus contract is **semantic**, not byte-exact (Okapi's
own `RoundTripComparison` is event/semantic-stable, not byte-stable):
`corpus_test.go` asserts an untouched read→write→re-read preserves the
translatable surface (every paragraph Block, in order, with identical source
text), plus a pseudo-translation invariant on a large clean document.

No permissively-licensed **non-Okapi** real-world MIF corpus exists, so every
file here is an Okapi Framework MIF filter fixture (Apache-2.0). The Adobe
FrameMaker tutorial fixtures shipped alongside them (`Ch03_GettingStarted.mif`,
`Ch08_Measurements.mif`, `Ch10_MeasurementRef.mif`, `Ch12_Cleaning.mif`,
`Ch13_Safety.mif`) are **deliberately excluded** — that is Adobe tutorial
content under Adobe copyright, not redistributable under Apache-2.0.

Four of the six files round-trip cleanly under the strict semantic contract.
Two (`okapi-Test01.mif`, `okapi-TestMarkers.mif`) exercise the tracked
**#558/#509 `<Marker>`-split round-trip gap** and are handled honestly without
weakening the corpus — see **Known gaps** below.

## Provenance

| File | Upstream repo | License | Commit |
| --- | --- | --- | --- |
| `okapi-Test01.mif` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-Test03.mif` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-TestMarkers.mif` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-TestFootnote.mif` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-TestParaLines.mif` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |
| `okapi-1187_crlf.mif` | Okapi Framework, Apache-2.0, gitlab.com/okapiframework/Okapi @ `509d8f567c03` | Apache-2.0 | `509d8f567c03` |

All files are drawn from `okapi/filters/mif/src/test/resources/`.

## Feature coverage

- **`okapi-Test01.mif`** (~209 KB, FrameMaker 9) — a broad FrameMaker document:
  body pages, tables, tabs and spaces inside strings, paragraph/character
  catalogs, variables, headings, and **index/cross-reference markers between
  paragraph text fragments**. The densest single fixture; its marker-between-
  text paragraphs trigger the #558/#509 gap (see below).
- **`okapi-Test03.mif`** (~151 KB, FrameMaker 9) — a large structured document
  that round-trips cleanly; exercises catalogs and body-page flows at scale.
- **`okapi-TestMarkers.mif`** (~208 KB, FrameMaker 9) — index markers and
  hyperlink/link markers embedded between paragraph text fragments. The canonical
  marker fixture; triggers the #558/#509 gap (see below).
- **`okapi-TestFootnote.mif`** (~207 KB, FrameMaker 9) — footnote bodies as
  translatable content within a large document. Round-trips cleanly; used as the
  pseudo-translation invariant fixture (`corpus_invariants_test.go`).
- **`okapi-TestParaLines.mif`** (~6.7 KB, FrameMaker 11) — multiple `<ParaLine>`
  elements merging into single paragraph units; the focused multi-paraline
  fixture.
- **`okapi-1187_crlf.mif`** (~436 B, FrameMaker 9) — consecutive empty
  `<ParaLine>`s merged across CRLF line endings; the focused empty-paraline /
  line-ending fixture.

## Known gaps

`okapi-Test01.mif` and `okapi-TestMarkers.mif` contain paragraphs that
interleave a `<Marker>` (an inline code) **between two `<String>` text
fragments**, e.g. in `Test01.mif`:

```
<String `First sentence. Second '>
<Marker
 <MType 2>
 ...
> # end of Marker
<String `sentence.'>
```

The native reader extracts these correctly (one paragraph Block with the marker
modelled as an inline code). The round-trip **merge**, however, is not yet
event-stable: the per-`<String>`-run skeleton-ref machine
(`findStringPositions` in `reader.go`) and the per-inline-code Block split model
disagree on how to route a marker-separated paragraph back into its `<String>`
slots, so a strict read→write→re-read diverges by exactly the marker-split
paragraphs (Test01: 221→220 blocks; TestMarkers: 217→216). This is the tracked
**#558 (native MIF audit) / #509 (bridge MIF Char Tab)** event-instability gap —
the same per-paragraph skeleton-store rework noted in `extraction_test.go`.

These two files are **not silently skipped and the contract is not weakened**.
`TestCorpusMarkerSplitExtractionStable` asserts the achievable contracts for
them — non-empty deterministic extraction, the writer reconstructs without
error, and re-reading the writer output does **not panic** (guarding the
historical `findStringPositions` index-out-of-range crash) — then skips only the
strict read→write→re-read equality with this citation. The four clean files
carry the full strict semantic round-trip.

## Exact fetch commands

Commit-pinned, fetched from the pinned Okapi GitLab raw URL.

```sh
# Okapi Framework — Apache-2.0 @ 509d8f567c03
OK=509d8f567c03
OKBASE="https://gitlab.com/okapiframework/Okapi/-/raw/$OK/okapi/filters/mif/src/test/resources"
curl -sSL -o okapi-Test01.mif        "$OKBASE/Test01.mif"
curl -sSL -o okapi-Test03.mif        "$OKBASE/Test03.mif"
curl -sSL -o okapi-TestMarkers.mif   "$OKBASE/TestMarkers.mif"
curl -sSL -o okapi-TestFootnote.mif  "$OKBASE/TestFootnote.mif"
curl -sSL -o okapi-TestParaLines.mif "$OKBASE/TestParaLines.mif"
curl -sSL -o okapi-1187_crlf.mif     "$OKBASE/1187_crlf.mif"
```
