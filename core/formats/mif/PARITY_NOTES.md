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
| native (this package) | 41 | **26** | 0 | 0 | 15 |

Cleared 26 of 41 fixtures so far through six commits that added
extraction for FrameMaker translatable surfaces the original reader
walked past:

| Fix | Commits |
|---|---|
| `<Page>` + `<AFrames>` → `<Frame>` → `<TextLine><String>` extraction | f0d6f22c |
| `^[A-Z]:` codeFinder rule for PgfNumFormat (catalog) | 764adb70 |
| Inline `<Para><Pgf><PgfNumFormat>` overrides | c87f4218 |
| `<Marker><MText>` for Index + Hypertext markers | 404c1ba9 |
| codeFinder `\x{NNNN}` escapes + walk into `<FNote>` | c68c016b |
| Multi-Font run splitting (Cluster H + L/M side-effects) | (this iteration) |

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

### Cluster H — Multi-Font runs split per font (RESOLVED — 6 + 3 side-effects)

**Was Affecting**: `938-2.mif`, `990-ref-format-2.mif`, `991.mif`,
`ImportedText.mif`, `Test03.mif`, `Test04.mif` (direct), plus
`902-3.mif`, `938-1.mif`, `990-ref-format-1.mif` (assist via
inline-code boundary handling).

**Symptom**: When a paragraph contains multiple `<String>` runs
separated by `<Font>` style changes (different font, weight, language,
etc.), okapi splits the translated text along the font boundaries:

  source: `<Font ...A><String 'normal '><Font ...B><String 'bold '><Font ...A><String 'normal'>`
  ref:    same shape, with each String pseudo-translated independently
  got:    one merged `<String 'ńōŕmàĺ ƀōĺď ńōŕmàĺ'>` losing the Font runs

**Resolution**: `extractParaRuns` walks each ParaLine and emits one
`paraTextRun` per text segment between inline-code boundaries
(`<Font>`, `<Marker>`, `<AFrame>`, `<XRef>...<XRefEnd>`, `<TextInset>`
… anything that isn't `<String>`/`<Char>`). Each non-empty run
becomes its own translatable Block; the inline-code statements stay in
skeleton text between block refs. Mirrors okapi's processPara +
readUntilText (MIFFilter.java:636-805 + 1027-1175): okapi's `default`
branch in readUntilText flips `significant=true` for any tag that
isn't ParaLine/Pgf/String/Char/Marker, which closes the running
TextFragment and starts a new one when text resumes.

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

Cluster H is now resolved. The remaining 15 divergent fixtures break
down into two coherent sub-projects:

**Char-handling (Cluster C, F, partial G)**: 6+ fixtures
(`1187_crlf.mif`, `1188_crlf.mif`, `987.mif`, `Test01.mif`,
`Test01-v8.mif`, plus `990-marker.mif`/`TestMarkers.mif` for the
HardSpace-before-Marker special case). The fix is uniform: rewrite
the writer/skeleton so any `<Char Foo>` statement whose value was
inlined into the para text gets elided from the skeleton output, and
adjacent `<Char>` statements get folded into the surrounding `<String>`
on output.

**Footnote/FNote ParaLine close cosmetic (Cluster K-like)**:
`Test02-v9.mif`, `TestEncoding-v9.mif`, `TestEncoding-v10.mif`,
`TestFootnote.mif`. Diff is a uniform 15-byte difference at the
ParaLine/Para close inside `<FNote>`: native preserves source-faithful
`>\n   > # end of Para`, okapi rewrites to `\n   > # end of ParaLine\n>
# end of Para`. Either match okapi or accept as canonical-only.

**Other (3 fixtures)**: `893.mif` (P:-prefix context detection),
`895.mif` (`<Char Tab>`-before-Variable elision; same family as Char
clusters), `896-autonumber-building-blocks.mif` (XRefDef inline
PgfNumString building-block extraction), `TestParaLines.mif`
(`<ElementEnd>` skeleton preservation when inside merged ParaLine).

## Files touched in this iteration

- `core/formats/mif/reader.go` — Page/AFrames/TextLine extraction,
  inline PgfNumFormat extraction, Marker/MText extraction,
  applyCodeFinderWithExtras helper, FNote in isMIFContainer.
- `core/formats/mif/config.go` — codeFinder rules use Go's `\x{NNNN}`
  unicode escape syntax instead of the never-compiling `\u…` form.
- `core/formats/mif/PARITY_NOTES.md` — this file.

## Files touched in the multi-Font (Cluster H) iteration

- `core/formats/mif/reader.go` — replaced `extractParaTextImpl` with
  `extractParaRuns`, which walks ParaLine children sequentially and
  returns one `paraTextRun` per text segment between inline-code
  boundaries (`<Font>`, `<Marker>`, `<AFrame>`, `<XRef>...<XRefEnd>`,
  `<TextInset>`, etc.). `processContainer` and the `walkContainer`
  branch of `findStringPositions` both consume the same run sequence,
  so blockIdx ordering stays in lock-step. XRef-internal `<String>`
  values are skipped (treated as part of the XRef inline code, per
  okapi MIFFilter.java:1068).
