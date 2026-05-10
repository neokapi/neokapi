# MIF parity divergence — cluster analysis

Snapshot of the native MIF reader/writer divergence against the okapi
reference engine in `cli/parity/roundtrip` (suite
`TestRoundTrip_Coverage/mif`, all 41 upstream fixtures under
`integration-tests/okapi/src/test/resources/mif`).

This file is a working note for the next iteration of MIF parity work.
It is **not** an architecture decision and **not** user-facing docs.

## Status

| Engine | Total | byte | canon | sem | div |
|---|---:|---:|---:|---:|---:|
| bridge (okapi-bridge) | 41 | 41 | 0 | 0 | 0 |
| native (this package) | 41 | **17** | 0 | 0 | 24 |

Cleared 17 of 41 fixtures so far through five commits that added
extraction for FrameMaker translatable surfaces the original reader
walked past:

| Fix | Commits |
|---|---|
| `<Page>` + `<AFrames>` → `<Frame>` → `<TextLine><String>` extraction | f0d6f22c |
| `^[A-Z]:` codeFinder rule for PgfNumFormat (catalog) | 764adb70 |
| Inline `<Para><Pgf><PgfNumFormat>` overrides | c87f4218 |
| `<Marker><MText>` for Index + Hypertext markers | 404c1ba9 |
| codeFinder `\x{NNNN}` escapes + walk into `<FNote>` | c68c016b |

Remaining clusters break down as follows.

## Divergence clusters (still open)

### Cluster C — `<Char HardReturn>` writer elision (4 fixtures)

**Affects**: `1187_crlf.mif`, `1188_crlf.mif`, `987.mif`, `Test01.mif`.

**Symptom**: When `extractHardReturnsAsText: true` (default) the reader
correctly inlines `<Char HardReturn>` into the block source as `\n`,
but the writer's skeleton refs don't elide the matching `<Char
HardReturn>` line — so the output carries BOTH the pseudo-translated
text containing `\n` AND the leftover `<Char HardReturn>` skeleton
line. okapi's `MIFFilter.writeParagraph` rebuilds the paragraph from
the textual model on output so `<Char>` statements that were inlined
as text never reappear.

**Fix shape**: Extend `findStringPositions` so each ref's
`endOffset` swallows trailing `<Char HardReturn>` lines that
contributed to the block text. Or: expand the ref to cover the entire
ParaLine close + Char tail when the merged text contains `\n`.

### Cluster F — `<Char Cent>` / `<Char Pound>` glyph-to-String rewrite (1 fixture)

**Affects**: `Test01-v8.mif`.

**Symptom**: Source has `<Char Cent>`. Reference rewrites this to
`<String '¢'>` and merges it into adjacent text. Native preserves the
`<Char Cent>` skeleton line.

**Fix shape**: Same family as Cluster C. The reader already maps Char
glyph names to characters in `extractParaTextImpl`; the writer needs
to elide the original `<Char>` skeleton lines and emit the merged
text as `<String>`.

### Cluster G — Marker structural rewrite (4 fixtures)

**Affects**: `938-1.mif`, `990-marker.mif`, `990-ref-format-1.mif`,
`TestMarkers.mif`.

**Symptom**: Marker text is now correctly extracted and translated
(commit 404c1ba9). The remaining diff is a Char-handling rewrite at
`<Char HardSpace>` adjacent to the marker — okapi inlines `<Char
HardSpace>` into a `<String ' '>` and re-orders the resulting
String/Marker/String sequence. Same family as Cluster C/F.

### Cluster H — Multi-Font runs split per font (6 fixtures)

**Affects**: `938-2.mif`, `990-ref-format-2.mif`, `991.mif`,
`ImportedText.mif`, `Test03.mif`, `Test04.mif`.

**Symptom**: When a paragraph contains multiple `<String>` runs
separated by `<Font>` style changes (different font, weight, language,
etc.), okapi splits the translated text along the font boundaries:

  source: `<Font ...A><String 'normal '><Font ...B><String 'bold '><Font ...A><String 'normal'>`
  ref:    same shape, with each String pseudo-translated independently
  got:    one merged `<String 'ńōŕmàĺ ƀōĺď ńōŕmàĺ'>` losing the Font runs

**Root cause**: `extractParaTextImpl` concatenates ALL String values in
ALL ParaLines into one block, losing the Font-run structure entirely.
The `findStringPositions` writer-side stringIdx>0 elision then
collapses the multi-String shape into a single String on output.

**Fix shape**: Treat Font changes as inline-code boundaries and emit
the para text with `<Font>...</Font>` placeholders that survive the
pseudo-translate transform. The skeleton store already supports
multi-string refs (the existing `expandToEnclosingParaLine` widens
secondary refs); extend it so the FONT skeleton lines between
String refs are preserved.

### Cluster K — FNote/Para `>` close-line rewrite (1 fixture)

**Affects**: `TestFootnote.mif`.

**Symptom**: Footnote text now correctly translates (commit c68c016b).
The remaining diff is okapi's structural rewrite of the ParaLine
close: source has bare `>`, reference always emits `> # end of
ParaLine`. Native preserves source-faithful skeleton.

**Fix shape**: This is okapi-side cosmetic. Either match okapi
(rewrite all bare `>` closes to `> # end of <Tag>` form), or accept
this as a permanent canonical-only diff.

### Cluster L — XRef structural rewrite (3 fixtures)

**Affects**: `Test02-v9.mif`, `TestEncoding-v9.mif`,
`TestEncoding-v10.mif`.

**Symptom**: `<XRef>` (cross-reference) blocks contain `<XRefDef>`
strings and surrounding `<String>` runs. okapi appears to expand
referent text-units inline in some cases, restructuring the XRef.

**Fix shape**: Implement XRef extraction (currently treated as opaque
inline code via the codeFinder rule). Mirrors okapi
`MIFFilter.java:1146-1153` (XRef inline tracking).

### Cluster M — `<Char><AFrame>` inline anchor splitting (1 fixture)

**Affects**: `902-3.mif`.

**Symptom**: Source has `<String 'Paragraph 2.'>` followed by an
`<AFrame 1>` placeholder. okapi splits the String at the AFrame
boundary into `<String 'Paragraph'><AFrame 1><String ' 2.'>`. Native
treats AFrame as opaque skeleton between two unrelated Strings.

**Fix shape**: Same family as Cluster H — multi-run para text
preservation.

### Cluster N — "Custom format:" inline PgfNumFormat (subsumed by inline-format work)

**Affects**: previously `904.mif` — now byte-equal after commit
c87f4218. Cluster retired.

### ?-other (3 fixtures)

**Affects**: `893.mif`, `895.mif`,
`896-autonumber-building-blocks.mif`.

These fixtures have multiple late-stage divergences that need
individual investigation. `893.mif` has an okapi quirk where some
`<String 'P:Body'>` cells preserve `P:` as code and others don't
(unlike Test01.mif which always pseudo-translates `P` → `Ƥ`); the
context-detection logic for the leading-letter rule isn't obvious
from the okapi source.

## Triage suggestion for the next iteration

The single highest-value next investment is **Cluster H** (multi-Font
runs in paragraphs) — six fixtures, and the same machinery would also
unblock Cluster M (AFrame inline splitting) and contribute to
Cluster L (XRef inline restructuring). It requires extending the
para-text extraction model to preserve `<Font>` boundaries as inline
codes, which means revisiting both `extractParaTextImpl` and the
`findStringPositions` ref scheme.

The Char-handling clusters (C, F, partial G) are a coherent
sub-project: rewrite the writer/skeleton so any `<Char Foo>`
statement whose value was inlined into the para text gets elided
from the skeleton output.

Cluster K is okapi-side cosmetic and should be accepted as a
permanent canonical-only diff.

## Files touched in this iteration

- `core/formats/mif/reader.go` — Page/AFrames/TextLine extraction,
  inline PgfNumFormat extraction, Marker/MText extraction,
  applyCodeFinderWithExtras helper, FNote in isMIFContainer.
- `core/formats/mif/config.go` — codeFinder rules use Go's `\x{NNNN}`
  unicode escape syntax instead of the never-compiling `\u…` form.
- `core/formats/mif/PARITY_NOTES.md` — this file.
