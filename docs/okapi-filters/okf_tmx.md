# okf_tmx - TMX Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_tmx` |
| Java Class | `net.sf.okapi.filters.tmx.TmxFilter` |
| MIME Types | `application/x-tmx+xml` |
| Extensions | `.tmx` |
| Okapi Module | `tmx` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/tmx/src/test/java/`

#### TmxFilterTest.java (49 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testTUProperties` | Translation unit properties extraction | P1 |
| 2 | `testTUDuplicateProperties` | Duplicate property handling | P2 |
| 3 | `testDefaultInfo` | Default filter info metadata | P3 |
| 4 | `testGetName` | Filter name accessor | P3 |
| 5 | `testGetMimeType` | MIME type accessor | P3 |
| 6 | `testLang11` | TMX 1.1 language handling | P1 |
| 7 | `testSpecialChars` | Special character escaping in TMX | P1 |
| 8 | `testLineBreaks` | Line break handling in TU content | P1 |
| 9 | `testXmlLangOverLang` | xml:lang takes precedence over lang attribute | P1 |
| 10 | `testEscapes` | Entity and escape handling | P1 |
| 11 | `testTargetAttributes` | Target element attribute extraction | P1 |
| 12 | `testCancel` | Filter cancellation handling | P3 |
| 13 | `testSourceLangNotSpecified` | Exception when source lang missing | P2 |
| 14 | `testTargetLangNotSpecified` | Exception when target lang missing | P2 |
| 15 | `testTargetLangNotSpecified2` | Variant of missing target lang | P2 |
| 16 | `testSourceLangNull` | Exception when source lang null | P2 |
| 17 | `testTargetLangNull` | Exception when target lang null | P2 |
| 18 | `testTuXmlLangMissing` | Exception when TU xml:lang missing | P2 |
| 19 | `testInvalidXml` | Exception on invalid XML | P2 |
| 20 | `testEmptyTu` | Exception on empty TU | P2 |
| 21 | `testInvalidElementInTu` | Exception on invalid element in TU | P2 |
| 22 | `testInvalidElementInSub` | Exception on invalid element in sub | P2 |
| 23 | `testInvalidElementInPlaceholder` | Exception on invalid element in placeholder | P2 |
| 24 | `testOpenInvalidInputStream` | Exception on invalid input stream | P2 |
| 25 | `testOpenInvalidUri` | Exception on invalid URI | P2 |
| 26 | `testInputStream` | Opening from InputStream | P1 |
| 27 | `testConsolidatedStream` | Consolidated bilingual stream | P1 |
| 28 | `testOutputWithLT` | Output with less-than in content | P1 |
| 29 | `testUnConsolidatedStream` | Non-consolidated stream | P1 |
| 30 | `testOutputBasic_Comment` | Comment preservation in output | P1 |
| 31 | `testStartDocument` | Start document event | P3 |
| 32 | `testPropAndNoteInStartDocument` | Properties and notes in header | P2 |
| 33 | `testStartDocumentFromList` | Start document from config list | P3 |
| 34 | `testDTDHandling` | TMX DTD reference handling | P1 |
| 35 | `testSegTypeSentence` | Segment type=sentence handling | P1 |
| 36 | `testSegTypePara` | Segment type=paragraph handling | P1 |
| 37 | `testSegTypeOrSentence` | Segment type or sentence | P2 |
| 38 | `testSegTypeOrParagraph` | Segment type or paragraph | P2 |
| 39 | `testSegTypeOrSentenceDefault` | Default sentence segment type | P2 |
| 40 | `testSegTypeOrParagraphDefault` | Default paragraph segment type | P2 |
| 41 | `testSegTypeOrSentenceUnknown` | Unknown sentence segment type | P2 |
| 42 | `testSegTypeOrParagraphUnknown` | Unknown paragraph segment type | P2 |
| 43 | `testSegTypeHeaderSentence` | Header-level sentence segtype | P2 |
| 44 | `testSegTypeHeaderParagraph` | Header-level paragraph segtype | P2 |
| 45 | `testSegTypeHeaderSentenceOverwrite` | Header segtype overwrite | P2 |
| 46 | `testSegTypeHeaderParagraphOverwrite` | Header segtype overwrite | P2 |
| 47 | `testSimpleTransUnit` | Simple TU extraction | P1 |
| 48 | `testMultiTransUnitWithEmptyLocales` | Multiple TUs with empty locales | P1 |
| 49 | `testMulipleTargets` | Multiple target languages | P1 |
| 50 | `testUtInSeg` | Inline codes in segment | P1 |
| 51 | `testUtInSub` | Inline codes in sub element | P1 |
| 52 | `testUtInHi` | Inline codes in hi element | P1 |
| 53 | `testIsolatedCodes` | Isolated inline codes | P1 |
| 54 | `testDoubleExtraction` | Double extraction consistency | P1 |
| 55 | `testDoubleExtractionCompKit` | Double extraction with comparison kit | P1 |
| 56 | `testTUTUVAttrEscaping` | TU/TUV attribute escaping | P1 |

#### ParametersTest.java (TMX-specific)

Shared parameter tests for TMX filter.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTmxIT` | `integration-tests/okapi/src/test/java/.../RoundTripTmxIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/tmx/`):
- `a_small_test.tmx`, `a_small_test2.tmx`, `code_fail.tmx`, `code_id_difference.tmx`
- `ImportTest2A.tmx`, `ImportTest2B.tmx`, `ImportTest2C.tmx`
- `okapi-confusion.tmx`, `Paragraph_TM.tmx`, `sampleTMX2.tmx`
- `simple.tmx`, `small_complete.tmx`

**Known failing files**: `code_id_difference.tmx`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TmxXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TmxXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/tmx/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `a_small_test2.tmx` | `TmxFilterTest` | Small TMX test |
| `header_with_prop_and_note.tmx` | `TmxFilterTest#testPropAndNoteInStartDocument` | Header properties/notes |
| `html_test.tmx` | `TmxFilterTest` | HTML content in TMX |
| `ImportTest2A.tmx` | `TmxFilterTest#testDoubleExtraction` | Import test A |
| `ImportTest2B.tmx` | `TmxFilterTest#testDoubleExtraction` | Import test B |
| `ImportTest2C.tmx` | `TmxFilterTest#testDoubleExtraction` | Import test C |
| `Paragraph_TM.tmx` | `TmxFilterTest` | Paragraph-level TM |
| `sampleTMX2.tmx` | `TmxFilterTest` | Sample TMX v2 |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/tmx/`

| File | Type | Purpose |
|------|------|---------|
| `a_small_test.tmx` | roundtrip | Small TMX roundtrip |
| `simple.tmx` | roundtrip | Simple TMX roundtrip |
| `small_complete.tmx` | roundtrip | Complete TMX roundtrip |
| `code_id_difference.tmx` | roundtrip | Code ID difference (known failing) |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/tmx/src/test/resources/*.tmx okapi-testdata/okf_tmx/

# Integration test resources
cp integration-tests/okapi/src/test/resources/tmx/*.tmx okapi-testdata/okf_tmx/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/tmx`

Build tag: `//go:build integration`

#### tmx_test.go - Extraction Tests

```go
func TestExtract_simpleTransUnit(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "simple TU extraction", javaRef: "TmxFilterTest#testSimpleTransUnit"},
        {name: "special chars", javaRef: "TmxFilterTest#testSpecialChars"},
        {name: "line breaks", javaRef: "TmxFilterTest#testLineBreaks"},
        {name: "inline codes in segment", javaRef: "TmxFilterTest#testUtInSeg"},
        {name: "isolated codes", javaRef: "TmxFilterTest#testIsolatedCodes"},
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    knownFailing := map[string]string{
        "code_id_difference.tmx": "Code ID difference causes event mismatch",
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/tmx/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/tmx/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - Bilingual filter: requires both source and target locale
  - TMX is a translation memory exchange format, not a source document format
  - Supports TMX 1.1 and 1.4 formats
  - Segment types: sentence, paragraph, or header-level
  - Inline codes: `<bpt>`, `<ept>`, `<it>`, `<ph>`, `<ut>`, `<hi>`, `<sub>`
  - DTD reference handling for TMX validation
  - Multiple target languages supported

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/tmx/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TmxFilterTest.java` | `okapi/filters/tmx/src/test/java/net/sf/okapi/filters/tmx/` | 49 |
| `ParametersTest.java` | `okapi/filters/tmx/src/test/java/net/sf/okapi/filters/tmx/` | - |
