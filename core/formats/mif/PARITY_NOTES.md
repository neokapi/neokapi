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
| native (this package) | 41 | **41** | 0 | 0 | 0 |

Cleared 41 of 41 fixtures so far through twelve commits that added
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
| FNote bare-`>` ParaLine close rewrite + empty-glyph Char elision (Cluster K) | 07d084d5 |
| codeFinder `^[A-Z]:` (gated) + autonumber building blocks + Char-only run owner fix | 5a4b66ea |
| Empty multi-ParaLine collapse (1187_crlf, Cluster Q) | 7cdcae16 |
| Content-bearing multi-ParaLine collapse after Char rewrite (1188_crlf, Cluster S) | 7cdcae16 |
| `\xNN ` hex escape decoding + rawValue tracking for skeleton refs (Cluster T, 987.mif) | (this iteration) |

Remaining clusters break down as follows.

## Divergence clusters (still open)

### Cluster C — `<Char HardReturn>` writer elision (RESOLVED)

**Was Affecting**: `1187_crlf.mif`, `1188_crlf.mif`, `987.mif`, `Test01.mif`.
**Now**: all four byte-equal (987.mif resolved via Cluster T).

**Resolution**: `findStringPositions` now also returns an `elisions` slice
that drops `<Char Foo>` lines from the skeleton when the glyph was inlined
into the surrounding String run. Mirrors okapi `MIFFilter.processPara`
(MIFFilter.java:1116-1126 + 740-741) which appends Char glyph values to
`paraTextBuf` and re-emits the merged buffer as a single `<String>`,
never re-emitting the original `<Char>` statement.

### Cluster R — multi-ParaLine merge with `<ElementEnd>` preservation (RESOLVED — 1 fixture)

**Was Affecting**: `TestParaLines.mif`. **Now**: byte-equal.

**Resolution**: When a Para has multiple non-empty ParaLines and the
second ParaLine carries `<ElementEnd ...>` (or other structure-tag
markers) between its `<String>` and `> # end of ParaLine`, the existing
multi-ParaLine merge elision dropped those markers along with the
wrapper bytes. `findStringPositions` now detects `<ElementEnd>` lines
inside the proposed elision range (`hasElementEndLine`) and shifts the
boundary to drop the FIRST close instead of the second — making the
second close the surviving close — then carves the `<ElementEnd>` line
out of the elision (`splitElisionPreservingElementEnd`). This mirrors
okapi `MIFFilter.processPara`'s "default: skip over" branch
(MIFFilter.java:1044-1066, 1145-1153) which appends non-extracted
statements to `paraCodeBuf`, preserving them in source order. Per the
MIF Reference §"Element Statements", `<ElementEnd>` is a structure-tag
boundary that must survive ParaLine collapse.

### Cluster S — content-bearing multi-ParaLine collapse after Char rewrite (RESOLVED — 1 fixture)

**Was Affecting**: `1188_crlf.mif`. **Now**: byte-equal.

**Resolution**: New `collapseContentMultiParaLineAfterCharRewrite` pass
in `findStringPositions`. When the FIRST ParaLine of a Para ends with
`<Char HardReturn>` (rewritten to a synthesized `<String '\n'>` by
paraCharRewrite) AND has NO preceding `<String>` in the same ParaLine,
AND is followed by a SECOND ParaLine with content, the pass elides the
inter-ParaLine wrapper boundary (from the `\r` after `<Char HardReturn>`'s
closing `>` through the `\r` of `<ParaLine\r\n`), leaving only the
trailing `\n` as the single LF separator between the synthesized String
and the second ParaLine's first child. Mirrors okapi
`MIFFilter.processPara` (MIFFilter.java:739-766): the HardReturn at the
end of a ParaLine flushes the current text unit via `addTextUnit` and
resets `skel = new GenericSkeleton()` (line 764); the subsequent
`readUntilText` for the second ParaLine starts with a fresh
`paraCodeBuf` (cleared at line 772), so the close line of the first
ParaLine and the opener of the second never accumulate into the
output skeleton. The String-presence guard ensures the existing
multi-String multi-ParaLine merge (line 953 in findStringPositions)
remains the sole handler for Paras whose first ParaLine contains an
explicit `<String>` -- both passes firing would over-elide. Per the
MIF Reference §"ParaLine Statement", inter-ParaLine wrapper bytes are
cosmetic.

### Cluster Q — empty multi-ParaLine collapse (RESOLVED — 1 fixture)

**Was Affecting**: `1187_crlf.mif`. **Now**: byte-equal.

**Resolution**: New `collapseEmptyMultiParaLines` pass in
`findStringPositions` scans rawText for runs of two or more adjacent
empty `<ParaLine>...> # end of ParaLine` wrappers (no String/Char/Marker
between opener and close) and emits elisions that mirror okapi's
`MIFFilter.processPara` normalization: every non-first ParaLine drops its
`<ParaLine` opener + the trailing first whitespace char (matching
readTag's `Char`/`ParaLine` !storeCharStatement deletion at
MIFFilter.java:1527-1532); every non-last ParaLine close drops its bare
`>` byte (matching the `paraLevel==1 && !inPgf` fall-through at
MIFFilter.java:1171-1187 which leaves the `>` unappended; the `>` is
re-inserted only at the LAST ` # end of ParaLine` via lastIndexOf at
MIFFilter.java:1191-1199). Per the MIF Reference §"ParaLine Statement",
the `# end of <Tag>` comment is purely cosmetic — but okapi normalizes
the surrounding bytes anyway, so native must mirror byte-for-byte.

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

### Cluster O — Global codeFinder default rules (RESOLVED — 2 fixtures)

**Was Affecting**: `893.mif`, `896-autonumber-building-blocks.mif`.
**Now**: both byte-equal.

**Resolution**: `config.go` now ships the full upstream Parameters.java
default rule list (MIFFilter Parameters.java:196-207): `^[A-Z]:`,
`<Default ¶ Font>`, the existing tag/dollar/glyph rules, plus the
Asian autonumber building-block rules (`zenkaku [naA]`, `kanji kazu`,
`daiji`, `hira iroha`, …). The `^[A-Z]:` rule is gated per-block via
`paraTextRun.precededByInlineCode`: when a ParaLine has an inline-code
statement (Font/Marker/AFrame/XRef/Variable/…) before the run's text
accumulates, the rule is suppressed. This mirrors Java's
TextFragment.coded-text representation where a leading inline-code
inserts a marker character (U+E101..U+E103) at offset 0, preventing
`^[A-Z]:` from matching (InlineCodeFinder.java:161-176 +
MIFFilter.java:693-734). Native runs split at code boundaries instead
of carrying an in-text marker, so the equivalent gating must be made
explicit via a per-run flag.

A second fix in the same iteration repairs Char-only run owner
assignment in `findStringPositions`: the previous walk left `ri`
stale across inline-code boundaries, so a `<Char Tab>` between two
`<Variable>` blocks was attributed to the preceding String run
(suppressing rewrite) instead of the new Char-only run. The walk now
mirrors `extractParaRuns` precisely, accumulating pending Char indexes
and assigning them on flush, so Char-only runs are correctly emitted as
synthesized `<String>` entries (mirrors okapi MIFFilter.java:739-741 +
761 paraTextBuf flush).

### Cluster P — multi-ParaLine elision drops `<ElementEnd>` (1 fixture)

**Affects**: `TestParaLines.mif`.

**Symptom**: `<ElementEnd 'Para'>` line appearing between `<String>`
and `> # end of ParaLine` in a multi-ParaLine merge case is dropped by
the merge elision. The merge-elision step needs to skip over
`<ElementEnd>` (and similar structural-only tags) without removing
them.

### Cluster T — `\xNN ` hex escape decoding for translatable Strings (RESOLVED -- 1 fixture)

**Was Affecting**: `987.mif`. **Now**: byte-equal (41/41 in native).

**Symptom**: Source has `<String `Para 1.\x09 '>` (literal MIF hex
escape for value 0x09 with mandatory trailing space). Native left the
String un-extracted because the byte form `\x09 ` round-tripped through
`unescapeMIFString` -> `escapeMIFForSearch` as `\\x09 ` (with the
backslash doubled), and the pattern never matched the source bytes in
`findStringPositions`. The skeleton kept the source verbatim while
okapi pseudo-translated to `<String `Ƥàŕà 1.\n'>` (per okapi's
`Hexadecimal.toString` which maps 0x09 -> `\n`, then MIFEncoder which
re-encodes `\n` -> `\\n`). Result: -1 byte off and missing pseudo.

**Resolution**: Three coordinated changes in `reader.go`:

  - `unescapeMIFString` now recognises the `\xNN ` form (2 hex digits +
    mandatory trailing space, mirroring `MIFFilter.readHexa` with
    `readExtraSpace=true` at MIFFilter.java:1813-1819) and substitutes
    the integer value's Unicode literal via the new
    `hexadecimalLiteral` helper, which mirrors okapi's
    `Hexadecimal.toString` table (Hexadecimal.java:43-84): 0x04 -> SHY,
    0x05 -> ZWJ, 0x06 -> removed, 0x08 -> tab, 0x09 -> LF, 0x10 -> figure
    space, 0x11 -> NBSP, 0x12 -> thin space, 0x13 -> en space,
    0x14 -> em space, 0x15 -> non-breaking hyphen. Unknown values
    fall through so source bytes round-trip when no translation
    transforms them (okapi's "unknown" branch wraps `\xNN ` in
    inline-code markers; native preserves the raw form, which matches
    byte-equal when those Strings stay un-extracted -- e.g. PgfCatalog
    bodies that aren't pseudo'd anyway).
  - `mifStatement` gained a `rawValue` field holding the literal source
    bytes BEFORE in-string escape decoding. `pushSingleLine` now calls
    `unquoteMIFRaw` alongside `unquoteMIF` so both forms are
    available. Required because `escapeMIFForSearch` is the inverse of
    `unescapeMIFString` but ONLY for the simple-escape set
    (`\t`/`\n`/`\>`/`\\`/`\q`/`\Q`) -- it cannot reconstruct the
    source's choice of `\xNN ` form versus the canonical short form.
  - `findStringPositions` now carries a `stringsAreRaw` flag on
    `itemInfo`. String/MText/TextLine items set it to true and pass
    `rawValue`-derived strings into the search; the matcher then uses
    those bytes verbatim instead of re-encoding via
    `escapeMIFForSearch`. PgfNumFormat / VariableDef keep
    `stringsAreRaw=false` since their items use decoded `value`
    today; bridging them to raw is unnecessary as long as the
    corresponding fixtures stay byte-equal (they do -- those values
    don't carry `\xNN` in any extractable position).

Per the Adobe FrameMaker MIF Reference §"String Tokens" the `\xNN `
form encodes one of FrameMaker's reserved internal character values
(numeric / non-breaking spaces, hard hyphen, hard return, etc.) -- the
trailing space is part of the lexeme, not document content. Per
MIFFilter.readHexa the same byte sequence is normalized to its Unicode
equivalent on extract; the writer's encoder then commits to the canonical
short form (`\t`, `\n`, …). Native now matches that round-trip exactly.

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
