# XLIFF okapi-compat quirks

Reference notes for the opt-in `xliff.OkapiCompatConfig` flags. Each
flag exists to reproduce a behavior of okapi's `XLIFFFilter` /
`XLIFFWriter` so the parity round-trip suite can compare native output
byte-for-byte against the okapi reference engine.

These are **not** production defaults. neokapi's xliff writer follows
the XLIFF 1.2 spec and intuitive output choices. The compat layer is
wired only in `cli/parity/roundtrip/coverage_test.go` (writerOverlay /
nativeConfig).

The okapi sources cited here are from the upstream `okapi-framework/okapi`
repository, snapshot 1.48.0 in our test data tarball
(`.parity/okapi-testdata/1.48.0/okapi/filters/xliff/src/main/java/net/sf/okapi/filters/xliff/`).
See the corresponding upstream test fixtures under
`integration-tests/okapi/src/test/resources/xliff/` and
`okapi/filters/xliff/src/test/resources/`.

## Flag inventory

| Flag | Default-on in parity? | Notes |
|---|---|---|
| `LowercaseLangSubtag` | yes | safe; aligns with BCP-47 §2.1.1 |
| `StripPhaseDateAttr` | yes | safe; okapi's `Phase` model drops `date` |
| `StripCDataCREntities` | yes | safe; okapi normalizes CR→LF in TextFragment |
| `HoistAltTransNotes` | yes | safe; matches okapi note-bag flattening |
| `ReorderHeaderToolToEnd` | yes | safe; matches okapi typed header bags |
| `SimulateBrokenWindows1252Read` | yes | reader-side; matches okapi's `XLIFFFilter` losing chars > U+007F when the file declares (or falls back to) Windows-1252 |
| `StripTransUnitApprovedAttr` | **no** | regresses Manual-12-AltTrans, RB-11, SF-12-Test02 — okapi PRESERVES `approved="yes"` in those fixtures |
| `UnwrapSingleSegMrk` | **no** | regresses translate_no.xlf — okapi keeps the wrapper for translate=no trans-units; condition not yet characterized |
| `EscapeNonASCIIAsEntities` | **no** | regresses Manual-12-AltTrans, RB-11 — okapi outputs literal UTF-8 in normal cases. SF-12-Test03 IS a fixture where okapi escapes; the trigger is unclear (Swordfish PIs? declared encoding? broken-1252 read?) |

## Quirk details

### LowercaseLangSubtag

okapi's `LocaleId.fromBCP47` lowercases the language subtag (RFC 5646
§2.1.1) before re-emitting it on the writer side. Source files with
`source-language="EN"` come out as `source-language="en"`. Region
subtags retain their original casing. Spec-aligned, safe in production
but neokapi otherwise echoes the source's casing verbatim.

### StripPhaseDateAttr

XLIFF 1.2 §2.3.1 defines `<phase date="…">` as optional. okapi's
`Phase` Java model field for `date` is read but never re-emitted on
write — the writer omits the attribute entirely. Native preserves it.

Fixtures: any file with `<phase date="…">` in `<header>` (e.g.
`about_the.htm.xlf`).

### StripCDataCREntities

okapi's `TextFragment` normalizes `\r\n` → `\n` and `\r` → `\n` on
read and never re-emits the CR character or its `&#xD;` numeric
reference on write. Native preserves the source's CR escapes verbatim.

Fixtures: `translate_no.xlf` has `&#xD;` entities embedded in source
text that okapi drops; native (without the flag) keeps them.

### HoistAltTransNotes

okapi's `XLIFFFilter` collects every `<note>` inside an `<alt-trans>`
and adds it to the parent trans-unit's note bag. The writer then emits
all trans-unit notes BEFORE the `<alt-trans>` element. The original
"this note belongs to alt-trans alternate X" relationship is lost.

XLIFF 1.2 §2.5 places `<note>` at trans-unit and alt-trans level
both — okapi's flattening is lossy but spec-compatible because the
output is still a valid xliff document with the note attached to the
trans-unit instead of the alternate.

Fixtures: `Test_Context_and_PH.xlf`, `altTrans-100.xlf`.

### ReorderHeaderToolToEnd

XLIFF 1.2 §2.3 lists `<header>` children but doesn't mandate a strict
order. okapi's reader collects header children into typed bags
(`tool`, `note`, `phase-group`, `count-group`, `prop-group`,
`reference`, `skl`) and the writer emits them in a fixed order
that places `<tool>` after `<note>` siblings.

Fixtures: `RB-11-Test01.xlf`, `SF-12-Test02.xlf` — both have a
`<tool>` before any `<note>` in the source; okapi reorders.

### SimulateBrokenWindows1252Read (reader-side)

okapi's `XLIFFFilter` opens the file with the declared XML encoding,
but the internal `TextFragment` data structure normalizes some
codepoints during parse — specifically, characters from Windows-1252
positions 0x80-0x9F that aren't valid ISO-8859-1 (e.g. `€` at 0x80,
typographic quotes 0x91-0x94, em/en-dashes 0x96-0x97) end up as
U+FFFD REPLACEMENT CHARACTER in the okapi output. Our flag
reproduces the same loss by replacing every non-ASCII rune with
U+FFFD when the source file declared (or fell back to) a non-UTF-8
charset.

Fixtures: `SF-12-Test03.xlf` — declares `encoding="UTF-8"` but
arrives via Swordfish workflow that touched windows-1252 encoded
intermediate. The `?` chars in the okapi output (`accents: �, �, �`)
match the U+FFFD pattern.

This flag is the only OkapiCompat behavior that runs **at read
time**, so it sits in `nativeConfig` instead of `writerOverlay` in
the parity test config.

### StripTransUnitApprovedAttr (off)

XLIFF 1.2 §2.4 defines `approved` as an optional yes/no attribute on
`<trans-unit>`. We initially conjectured okapi strips `approved="no"`
or `approved="yes"`, but on closer inspection okapi's writer PRESERVES
the attribute exactly as authored in every fixture we've inspected
(Manual-12-AltTrans `approved="yes"`, RB-11 `approved="yes"`,
SF-12-Test02 `approved="yes"`, SF-12-Test03 `approved="no"`).

The flag's helper (`stripAttrInTag`) and registration remain in place
so a future fixture revealing the actual strip rule can re-enable it
narrowly. Don't enable it across the board.

### UnwrapSingleSegMrk (off)

okapi's `XLIFFFilter` sometimes drops a single-segment `<mrk
mtype="seg">` wrapper from `<seg-source>` / `<target>` on round-trip,
producing flat text. We initially conjectured this fired for
`translate="no"` trans-units, but `translate_no.xlf` shows okapi
preserving the wrapper exactly there.

Cross-fixture comparison:

| Fixture | trans-unit | translate attr | xml:space | okapi unwraps? |
|---|---|---|---|---|
| `about_the.htm.xlf` | tu1 | `no` | `preserve` | yes |
| `segmented.xlf` | tu1 | (default yes) | (default) | no |
| `translate_no.xlf` | tu1 | `no` | `preserve` | no |

The condition is not characterized by any single attribute. Possible
factors: the presence of `<alt-trans>` siblings, MadCap-specific
extension attributes, or whether the seg-source content equals the
source content verbatim. Until characterized, leave the flag off.

### EscapeNonASCIIAsEntities (off)

In normal output, okapi emits literal UTF-8 for non-ASCII characters
(matches the file's declared encoding). But in some Swordfish-flavour
fixtures (`SF-12-Test03.xlf`) it emits `&#xNNNN;` numeric character
references for every char above U+007F, including in source text that
the file's encoding can perfectly represent.

Possible triggers:

- Presence of `<?encoding UTF-8?>` processing instruction (Swordfish
  marker)
- Presence of `xliff-core-1.2-transitional.xsd` schema reference
- Some interaction with the broken-1252 read path

Until the trigger is understood, leave the flag off — blanket-enabling
regresses Manual-12-AltTrans, RB-11, and SF-12-Test02.

## Investigation tracker

[neokapi#549](https://github.com/neokapi/neokapi/issues/549) tracks
characterizing the actual triggers for `UnwrapSingleSegMrk`,
`EscapeNonASCIIAsEntities`, and `StripTransUnitApprovedAttr` so the
remaining 5 divergent xliff fixtures (MQ-12-Test01, SF-12-Test03,
Test_Context_and_PH, Typo3Draft, about_the.htm) can reach canonical-equal.
