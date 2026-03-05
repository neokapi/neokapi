# okf_mif - MIF/FrameMaker Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_mif` |
| Java Class | `net.sf.okapi.filters.mif.MIFFilter` |
| MIME Types | `application/vnd.mif` |
| Extensions | `.mif` |
| Okapi Module | `mif` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/mif/src/test/java/`

#### DocumentTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `iteratesThroughTheStatementsOfASample` | Parses MIF snippet into 11 statements, round-trips to identical string | P2 |
| 2 | `iteratesThroughTheStatementsOfEveryResourceUnderTest` | Iterates all .mif resource files and verifies read output matches original | P1 |

#### ExtractionTest.java (30 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Non-null name, non-empty configurations | P2 |
| 2 | `testStartDocument` | StartDocument event from Test01.mif | P2 |
| 3 | `testSimpleText` | 221 text units from Test01.mif, verifies line breaks and agrave encoding | P1 |
| 4 | `testExtractIndexMarkers` | extractIndexMarkers parameter: enabled extracts "Text of marker" (x-index type), disabled skips to body text | P1 |
| 5 | `testExtractLinks` | extractLinks parameter: enabled extracts hyperlink URLs as separate TUs | P2 |
| 6 | `testBodyOnlyNoVariables` | Body-only extraction without variables from Test01.mif | P1 |
| 7 | `testParagraphLinesProcessing` | Multiple ParaLines merged: "The 1st para line. The 2nd." | P1 |
| 8 | `testSimpleEntry` | Snippet: backslash and ampersand escaping in MIF strings | P1 |
| 9 | `testNoTextEntry` | Snippet: ParaLine with only TextRectID produces no TU | P2 |
| 10 | `testTwoPartsEntry` | Two ParaLines merged into single TU | P1 |
| 11 | `testEmptyString` | Empty string between AFrames handled with codes | P2 |
| 12 | `testEmptyStringInFront` | Empty string before Font+String ignored, just extracts text | P2 |
| 13 | `testTrimFontInFront` | Leading Font tag trimmed from extraction | P2 |
| 14 | `testTabs` | Tab characters with Var produce no extractable text | P2 |
| 15 | `testTabsAndCodes` | Tabs and codes combined produce DocumentPart skeleton | P2 |
| 16 | `testDummyBeforeChar` | Dummy elements before Char (ThinSpace) handled as codes | P2 |
| 17 | `testCodeAtTheFront` | Font change at front creates inline code | P2 |
| 18 | `testCharOnly` | Only Char elements produce no TU | P2 |
| 19 | `testEndsInCharAndCode` | Text ending with Char+Dummy extracts text portion only | P2 |
| 20 | `testDummyCharString` | AFrame+Tab+String produces TU with tab in skeleton | P2 |
| 21 | `testEmptyFTag` | AFrame codes between text with ThinSpace | P2 |
| 22 | `testSoftHyphen` | SoftHyphen between ParaLines produces "However." | P1 |
| 23 | `testNormalFont` | Font with empty FTag treated as normal (no code) | P2 |
| 24 | `testEmptyParaLine` | Empty ParaLine produces no TU | P2 |
| 25 | `testSlashCodes` | Variable definitions with slash codes (\x14, \x05, \x0b) | P2 |
| 26 | `testSlashCodesOutput` | Slash codes output verification | P2 |
| 27 | `testV10IsUsingV9Encoding` | MIF v10 uses same encoding as v9 | P2 |
| 28 | `processesSupportedVersions` | Versions 8.00 and 2015 process successfully | P2 |
| 29 | `doesNotProcessUnsupportedVersions` | Version 7.00 throws OkapiBadFilterInputException | P2 |
| 30 | `extractsBodyPageRelatedInformationOnly` | Only body page content extracted (893.mif) | P1 |

(continued: `extractsMultipleTextFramesPerPage`, `extractsNumberedParagraphFormats`, `extractsNumberedParagraphFormatInTableCells`, `extractsAnchoredFramesContent`, `extractsNestedAnchoredFrames`, `sequentialParagraphFormatsExtracted`, `referenceFormatsConditionallyExtracted`, `textLinesExtracted`, `nestedTextFramesExtracted`, `hardReturnsFormNewTransUnits`, `tabsRepresentedAsCodesAndHardReturnsAsText`, `tabsRepresentedAsCodesAndNewTextualUnitsFormedOnHardReturnsAppearance`)

#### ExtractsTest.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `gathersExtractsFromEveryResourceUnderTest` | Iterates all .mif files and gathers extracts without error | P2 |

#### RoundTripTest.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `hardReturnsAsNonTextualRoundTripped` | Data-driven: 7 MIF files with non-textual hard returns config roundtripped | P1 |
| 2 | `roundTripsWithDifferentParameters` | Data-driven: 28 MIF files roundtripped with null, common, and inline-pgf-num-formats configs | P1 |
| 3 | `consequentialEmptyParaLinesMerged` | Empty ParaLines merged on roundtrip (1187_crlf.mif) | P1 |
| 4 | `tabsEncodedOnExtractionAndHardReturnsEncodedOnMerge` | Tab/hard return encoding roundtrip with both extractHardReturnsAsText modes (1188_crlf.mif) | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripMifIT` | `integration-tests/okapi/src/test/java/.../RoundTripMifIT.java` | 2 |

#### XLIFF Compare IT

None found.

#### Simplifier IT

None found.

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/mif/src/test/resources/`

Key test files:
- `Test01.mif` - Main test document (221 text units)
- `Test01-v8.mif` - Version 8 format
- `Test02-v9.mif` - Version 9 format
- `Test03.mif`, `Test04.mif` - Additional test documents
- `TestEncoding-v9.mif`, `TestEncoding-v10.mif` - Encoding comparison
- `TestFootnote.mif` - Footnote handling
- `TestMarkers.mif` - Index markers and links
- `TestParaLines.mif` - Paragraph line processing
- `ImportedText.mif`, `JATest.mif` - Edge cases
- `893.mif` - Body page only extraction
- `895.mif` - Multiple text frames per page
- `896.mif`, `896-changed.mif`, `896-autonumber-building-blocks.mif` - Numbered paragraph formats
- `902-1.mif` through `902-3.mif` - Anchored frames content
- `904.mif` - Numbered paragraphs in table cells
- `909-1.mif` through `909-3.mif` - Nested anchored frames
- `938-1.mif`, `938-2.mif` - Reference formats
- `940.mif` - Sequential paragraph formats
- `942-1.mif`, `942-2.mif` - Text lines
- `943.mif` - Nested text frames
- `945.mif` - Additional test
- `987.mif` - Hard returns
- `990-marker.mif`, `990-pgf-num-format-1.mif`, `990-pgf-num-format-2.mif` - Hard return edge cases
- `990-ref-format-1.mif`, `990-ref-format-2.mif` - Reference format with hard returns
- `990-text-line.mif` - Text lines with hard returns
- `1052.mif` - Reference formats
- `1187_crlf.mif` - Consequential empty ParaLines
- `1188_crlf.mif` - Tab and hard return encoding

### Synthetic test data to create

None needed - extensive test data available.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/mif/src/test/resources/net/sf/okapi/filters/mif/*.mif okapi-testdata/okf_mif/
cp okapi/filters/mif/src/test/resources/net/sf/okapi/filters/mif/*.fprm okapi-testdata/okf_mif/

# Integration test resources
cp integration-tests/okapi/src/test/resources/mif/*.mif okapi-testdata/okf_mif/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/mif`

Build tag: `//go:build integration`

#### mif_test.go - Extraction Tests

```go
func TestExtract_BasicText(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "simple_text",
            input: "Test01.mif",
            wantBlocks: 221,
            javaRef: "ExtractionTest#testSimpleText",
        },
        {
            name:  "body_page_only",
            input: "893.mif",
            wantBlocks: 1,
            wantTexts: []string{"Goes over the PgfCatalog."},
            javaRef: "ExtractionTest#extractsBodyPageRelatedInformationOnly",
        },
        {
            name:  "multiple_text_frames",
            input: "895.mif",
            wantBlocks: 7,
            javaRef: "ExtractionTest#extractsMultipleTextFramesPerPage",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_IndexMarkers(t *testing.T) {
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "extract_index_markers",
            params: map[string]any{"extractIndexMarkers": true},
            input:  "TestMarkers.mif",
            want:   []string{"Text of marker"},
            javaRef: "ExtractionTest#testExtractIndexMarkers",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "Test01.mif", "Test01-v8.mif", "Test02-v9.mif", "Test03.mif", "Test04.mif",
        "893.mif", "895.mif", "896.mif", "902-1.mif", "904.mif",
        "TestMarkers.mif", "TestParaLines.mif", "ImportedText.mif",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/mif/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/mif/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] All configuration/parameter tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks:
  - MIF (Maker Interchange Format) is Adobe FrameMaker's text-based interchange format
  - Not a binary format - it's a structured text format with statement-based parsing
  - Supports versions 8.00 through 2015; version 7.00 is unsupported
  - Parameters: extractIndexMarkers, extractLinks, extractHardReturnsAsText, extractPgfNumFormatsInline, extractReferenceFormats
  - ParaLines are merged into single text units
  - SoftHyphen between ParaLines produces merged text
  - Font changes create inline codes
  - Anchored frames, text lines, nested text frames all extractable
  - Numbered paragraph format (PgfNumFormat) can be extracted inline or as separate TUs
  - Hard returns can be treated as text (inline) or as TU boundaries

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/mif/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `DocumentTest.java` | `okapi/filters/mif/src/test/java/.../` | 2 |
| `ExtractionTest.java` | `okapi/filters/mif/src/test/java/.../` | 30 |
| `ExtractsTest.java` | `okapi/filters/mif/src/test/java/.../` | 1 |
| `RoundTripTest.java` | `okapi/filters/mif/src/test/java/.../` | 5 |
