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
| native (this package) | 41 | **35** | 0 | 0 | 6 |

Cleared 35 of 41 fixtures so far through eight commits that added
extraction for FrameMaker translatable surfaces the original reader
walked past:

| Fix | Commits |
|---|---|
| `<Page>` + `<AFrames>` → `<Frame>` → `<TextLine><String>` extraction | f0d6f22c |
| `^[A-Z]:` codeFinder rule for PgfNumFormat (catalog) | 764adb70 |
| Inline `<Para><Pgf><PgfNumFormat>` overrides | c87f4218 |
| `<Marker><MText>` for Index + Hypertext markers | 404c1ba9 |
| codeFinder `\x{NNNN}` escapes + walk into `<FNote>` | c68c016b |
| Multi-Font run splitting (Cluster H + L/M side-effects) | 16e172dd |
| `<Char>` glyph elision/rewrite (Clusters C + F + partial G) | 7ce509c7 |
| FNote bare-`>` ParaLine close rewrite + empty-glyph Char elision (Cluster K) | (this iteration) |

Remaining clusters break down as follows.

## Divergence clusters (still open)

### Cluster C — `<Char HardReturn>` writer elision (RESOLVED for `Test01.mif`; partial elsewhere)

**Was Affecting**: `1187_crlf.mif`, `1188_crlf.mif`, `987.mif`, `Test01.mif`.
**Now**: only `1187_crlf.mif`, `1188_crlf.mif`, `987.mif` remain divergent --
for reasons OTHER than the Char HardReturn elision. `Test01.mif` is byte-equal.

**Resolution**: `findStringPositions` now also returns an `elisions` slice
that drops `<Char Foo>` lines from the skeleton when the glyph was inlined
into the surrounding String run. Mirrors okapi `MIFFilter.processPara`
(MIFFilter.java:1116-1126 + 740-741) which appends Char glyph values to
`paraTextBuf` and re-emits the merged buffer as a single `<String>`,
never re-emitting the original `<Char>` statement.

**Remaining divergences** in 1187_crlf, 1188_crlf, 987 are unrelated to
the Char elision itself:

  - 1187_crlf.mif: empty-ParaLine collapse (okapi rewrites a pair of
    empty `<ParaLine>...</ParaLine>` siblings into a malformed
    `   # end of ParaLine\n  \n  > # end of ParaLine` sequence -- looks
    like an okapi-side quirk).
  - 1188_crlf.mif: cross-ParaLine merge with `<Char HardReturn>` between
    `</Font>` and the next ParaLine. Native correctly elides + rewrites
    the HardReturn but doesn't yet handle the cross-ParaLine merge that
    okapi performs on the surrounding `<Char Tab>` and following Strings.
  - 987.mif: -1 byte off; the diverging byte is in a String value where
    the source has `\x09` (literal MIF hex escape for tab) and okapi's
    bridge produces `\n` in the output -- bridge-side quirk unrelated to
    Char clusters.

### Cluster F — `<Char Cent>` / `<Char Pound>` glyph-to-String rewrite (RESOLVED -- 1 fixture)

**Was Affecting**: `Test01-v8.mif`. **Now**: byte-equal.

**Resolution**: A new `paraCharRewrite` mechanism in `findStringPositions`
rewrites `<Char NAME>` lines as `<indent><String 'X'>` (with X the glyph
value, MIF-escaped) when the Char appears in a "Char-only run" (the merged
text of the run is non-empty but no surrounding `<String>` exists in the
source). The `charRewrite` op is processed alongside refs and elisions by
the merged sort in the `readContent` skeleton emission loop. Mirrors okapi
`MIFFilter.processPara` flush at MIFFilter.java:739-741 + addTextUnit at
761 (paraTextBuf becomes a synthesized `<String>` on flush).

### Cluster G — Marker structural rewrite (RESOLVED -- 4 fixtures)

**Was Affecting**: `938-1.mif`, `990-marker.mif`, `990-ref-format-1.mif`,
`TestMarkers.mif`. **Now**: all four byte-equal.

**Resolution**: Two complementary mechanisms in `findStringPositions`:

  - `<Char HardSpace>` immediately before `<Marker>` is rewritten as
    `<String ' '>` AND its trailing newline+indent is dropped via the
    `joinNext` flag, so output joins `<String ' '><Marker ` on the same
    line (mirrors okapi's writeParagraph which emits the synthesized
    String + the Marker code without inter-tag whitespace).
  - `<String '...'>` immediately before `<Marker>` triggers a
    String-Marker join elision: the `'>` close of the String stays, but
    the trailing whitespace + newline (up to the `<Marker ` keyword) is
    added to the elision set so output joins them on the same line.

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

### Cluster K — FNote/Para `>` close-line rewrite (RESOLVED -- 4 fixtures)

**Was Affecting**: `TestFootnote.mif`, `Test02-v9.mif`,
`TestEncoding-v9.mif`, `TestEncoding-v10.mif`. **Now**: all four
byte-equal.

**Resolution**: Two complementary mechanisms in `findStringPositions`:

  - `rewriteFNoteParaCloses` scans rawText for the bare-`>` ParaLine
    close pattern (a `\n[ \t]*>\n[ \t]*> # end of Para\n` sequence
    typical of FNote bodies where the source omits the
    `# end of ParaLine` comment) and emits an elision dropping the
    bare `>` byte plus a rewrite inserting ` # end of ParaLine\n>`
    immediately after the source's `>` of the `> # end of Para` line.
    Mirrors okapi MIFFilter.processPara at MIFFilter.java:1191-1200
    which unconditionally appends ` # end of ParaLine\n>` on every
    ParaLine close (paraLevel→0); when the source already has the
    comment the insert-`>`-before-comment branch yields a byte-equal
    output, so only the bare-`>` case needs an explicit rewrite.
  - The per-Para Char elision now also drops `<Char>` lines whose
    glyph value is empty (e.g. `<Char SoftHyphen>`). Mirrors okapi's
    readTag at MIFFilter.java:1527-1532 which deletes the
    just-appended `<Char` from sb regardless of glyph value -- the
    source line is removed before the literal is even read.

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

**Affects**: `893.mif`, `896-autonumber-building-blocks.mif`,
`TestParaLines.mif`.

  - `893.mif`: leading-letter `^[A-Z]:` rule applied to plain
    `<String>` text in cells (`P:Body` → `P:ßōďŷ` in ref vs
    `Ƥ:ßōďŷ` in native). Okapi's bridge applies a context-sensitive
    leading-prefix rule that native doesn't yet mirror.
  - `896-autonumber-building-blocks.mif`: native pseudo-translates
    okapi-recognised auto-numbering building-block names (`zenkaku a`,
    `kanji kazu`, etc.) that the bridge keeps as code. Need a
    PgfNumFormat-context codeFinder pattern for the building-block
    vocabulary.
  - `TestParaLines.mif`: `<ElementEnd 'Para'>` line appearing between
    `<String>` and `> # end of ParaLine` in a multi-ParaLine merge
    case is dropped by the merge elision. Need to teach the
    multi-ParaLine elision to skip over `<ElementEnd>` (and similar
    structural-only tags) without removing them.

## Triage suggestion for the next iteration

The Char-handling sub-project (Cluster C + F + G) and the FNote
ParaLine close rewrite (Cluster K) are now resolved; the remaining 6
divergent fixtures break down into smaller threads:

**Cross-ParaLine merge after Char rewrite (1 fixture)**: `1188_crlf.mif`.
The Char HardReturn rewrite is now correct, but the SECOND ParaLine
(introduced after `</Font>`) needs to be merged into the first ParaLine
along with its surrounding `<Char Tab>` (which becomes inline content
in okapi's writeParagraph).

**Empty-ParaLine collapse okapi quirk (1 fixture)**: `1187_crlf.mif`.
Reference emits a malformed `   # end of ParaLine\n  \n  > # end of
ParaLine` sequence for two consecutive empty ParaLines. Looks like an
okapi quirk; consider canonical-only if reproducing it exactly proves
infeasible.

**Other (4 fixtures)**: `893.mif` (leading-letter context detection),
`896-autonumber-building-blocks.mif` (auto-numbering building-block
codeFinder), `987.mif` (`\x09` escape -> `\n` bridge quirk),
`TestParaLines.mif` (`<ElementEnd>` preservation in multi-ParaLine
merge).

## Files touched in this iteration (Char clusters C+F+G)

- `core/formats/mif/reader.go`:
  - Added `charGlyphMap` + `charLiteral` to centralize the okapi
    `CharLiteralToken` mapping (CharLiteralToken.java:40-86).
  - Refactored `findStringPositions` to return `(refs, elisions,
    rewrites)` and produce a unified op stream consumed by the new
    sort-merge in `readContent`.
  - Added per-Para `paraInlineChar` tracking so each Char glyph that
    contributed inlined text gets a corresponding elision range in
    the skeleton.
  - Added per-Para `paraCharRewrite` tracking for Char-only runs
    (Cluster F) and Char-followed-by-Marker runs (Cluster G), with
    `joinNext` controlling whether the Char line's trailing newline
    is dropped so the next sibling joins on the same output line.
  - Added a String-Marker join elision: when a String item's last
    `'>` is immediately followed by a `<Marker `, drop the trailing
    whitespace+newline so the two tags appear on the same output
    line (mirrors okapi's writeParagraph layout).
  - Same-ParaLine multi-String elision now drops the previous
    String's `'>` (so the second String's `'>` closes the merged
    output) instead of overshooting via the old
    `expandToEnclosingParaLine` helper, which was removed.
  - Multi-ParaLine merge elision now keeps the FIRST ParaLine's
    `> # end of ParaLine` line and elides only the second ParaLine
    wrapper + its close.
  - Replaced `strings.TrimSpace(run.text) == ""` with
    `run.text == ""` (matching okapi's `Util.isEmpty(text)` which
    only checks for empty, not whitespace-only); whitespace-only
    runs ARE extracted (e.g. `<String ' '>`).

## Files touched in earlier iterations

(Cluster H multi-Font split — commit 16e172dd)

- `core/formats/mif/reader.go` — replaced `extractParaTextImpl` with
  `extractParaRuns`, which walks ParaLine children sequentially and
  returns one `paraTextRun` per text segment between inline-code
  boundaries (`<Font>`, `<Marker>`, `<AFrame>`, `<XRef>...<XRefEnd>`,
  `<TextInset>`, etc.). `processContainer` and the `walkContainer`
  branch of `findStringPositions` both consume the same run sequence,
  so blockIdx ordering stays in lock-step. XRef-internal `<String>`
  values are skipped (treated as part of the XRef inline code, per
  okapi MIFFilter.java:1068).

(Earlier iterations)

- `core/formats/mif/reader.go` — Page/AFrames/TextLine extraction,
  inline PgfNumFormat extraction, Marker/MText extraction,
  applyCodeFinderWithExtras helper, FNote in isMIFContainer.
- `core/formats/mif/config.go` — codeFinder rules use Go's `\x{NNNN}`
  unicode escape syntax instead of the never-compiling `\u…` form.
