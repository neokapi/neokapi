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
| `HoistAltTransNotes` | yes | safe; matches okapi NOTEMARKER skeleton placement |
| `ReorderHeaderToolToEnd` | yes | safe; matches okapi's first-non-bagged-child rule |
| `SimulateBrokenWindows1252Read` | yes | reader-side; matches okapi's `XLIFFFilter` losing chars > U+007F when the file declares (or falls back to) Windows-1252 |
| `UnwrapSingleSegMrk` | yes | content-aware: drops `<seg-source>` and unwraps `<mrk mtype="seg">` in target only when source text differs from seg-source text (matches `XLIFFFilter.java:2278`) |
| `StripApprovedWhenNoSourceTarget` | yes | drops `approved="…"` from trans-units whose source had no `<target>` element (matches `XLIFFFilter.java:2475` + `XLIFFSkeletonWriter.java:756`) |
| `EscapeBeyondLatin1AsEntities` | yes | encoder-aware: escapes only chars the source-declared encoding cannot represent (matches `XMLEncoder.java:101-110, 191-213`); no-op for UTF-8 sources |
| `StripTransUnitApprovedAttr` | **no** | unconditional `approved` strip — kept as dead code in case a future fixture needs it; the actual okapi rule is `StripApprovedWhenNoSourceTarget` |

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

### UnwrapSingleSegMrk

okapi's `XLIFFFilter` (XLIFFFilter.java:2278) drops the `<seg-source>`
wrapper and unwraps the corresponding `<mrk mtype="seg">…</mrk>` in
`<target>` **only** when the source content differs from the
seg-source content (text-only comparison after entity decoding). When
source equals seg-source, the wrapper is preserved.

Cross-fixture comparison:

| Fixture | tu | source ≡ seg-source? | okapi unwraps? |
|---|---|---|---|
| `about_the.htm.xlf` | tu1 | no (segmenter rewrote source) | yes |
| `segmented.xlf` | tu1 | yes | no |
| `translate_no.xlf` | tu1 | yes | no |

Implemented as a writer post-process pass (`okapi_compat_helpers.go`)
that walks each `<trans-unit>`, compares decoded text, and rewrites
matching segments. Safe to enable everywhere because the
content-equality guard prevents regression.

### StripApprovedWhenNoSourceTarget

okapi sets the APPROVED target-property only inside its
target-processing branch (XLIFFFilter.java:2475). When a `<trans-unit>`
has no `<target>` in the source, that branch never runs and the
property remains unset; the writer (XLIFFSkeletonWriter.java:756) then
emits no `approved="…"` attribute. Trans-units that did have a
`<target>` keep their `approved` attribute.

Implemented as a writer post-process pass that tracks trans-units by
document-order POSITION (not by id, since XLIFF allows duplicate
trans-unit ids) and reads a "had-target" set populated by the reader.

Fixture: `SF-12-Test03.xlf` — 944 trans-units with `approved="no"`;
only the first id="1" has a source target → keeps approved on
round-trip; the other 943 drop it. Matches okapi byte-for-byte.

This flag SUPERSEDES the unconditional `StripTransUnitApprovedAttr`
flag, which is kept in code as dead-but-documented in case a future
fixture surfaces a different rule.

### EscapeBeyondLatin1AsEntities

okapi's `XMLEncoder` (XMLEncoder.java:101-110, 191-213) only escapes
non-ASCII chars when the output charset cannot represent them. The
encoder is constructed only for non-UTF-8/16 outputs, and the per-char
check `!chsEnc.canEncode(value)` decides whether to emit a numeric
reference.

Implementation uses `golang.org/x/text/encoding`'s `Encoder.canEncode`
for an exact charset membership test. For windows-1252 this means
chars in the "Windows extension" range (e.g. U+0152, U+0192 ƒ, U+2026
…, U+20AC €) stay literal while Latin Extended-A/B chars beyond
Latin-1 get escaped.

The reader records the source charset in
`layer.Properties["xliff:source-encoding"]` when the XML declaration
named a non-UTF-8 charset. UTF-8 sources skip the path entirely so the
flag is a no-op for the common case — it only fires on legacy
encodings, exactly when okapi fires it.

Fixture: `SF-12-Test03.xlf` (declared windows-1252, pseudo-output
contains `Ţàĉƒ` → emits `&#x0162;à&#x0109;ƒ`, keeping ƒ literal because
it's representable in windows-1252).

The earlier `EscapeNonASCIIAsEntities` flag — which escaped
indiscriminately above U+007F — was renamed and rewritten to use the
encoder check. The old name no longer exists.

## Investigation tracker

[neokapi#549](https://github.com/neokapi/neokapi/issues/549) is
**resolved** — all 5 originally divergent xliff fixtures
(MQ-12-Test01, SF-12-Test03, Test_Context_and_PH, Typo3Draft,
about_the.htm) reach canonical-equal in the parity test, and each of
the three previously uncharacterized quirks now has a documented,
spec-traceable trigger condition (or has been superseded by a more
precise flag).
