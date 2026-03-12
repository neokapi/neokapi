# okf_xliff - XLIFF Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_xliff` |
| Java Class | `net.sf.okapi.filters.xliff.XLIFFFilter` |
| MIME Types | `application/x-xliff+xml` |
| Extensions | `.xlf, .xliff, .sdlxliff, .mqxliff, .mxliff` |
| Okapi Module | `xliff` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/xliff/src/test/java/`

#### XLIFFFilterTest.java (185 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `corruptCodeIdsAfterJoinAll` | Code IDs remain valid after joining all segments | P2 |
| 2 | `disabled_testMisOrderedCodes` | Mis-ordered inline codes handling | P3 |
| 3 | `testSegmentedTarget` | Segmented target with mrk elements | P1 |
| 4 | `testSegmentedContent` | Segmented source and target content | P1 |
| 5 | `testSegmentIDs` | Segment IDs from mrk mid attributes | P1 |
| 6 | `testWSBetweenSegments` | Whitespace between segments preserved | P1 |
| 7 | `testSegmentedSourceWithOuterCodes` | Segmented source with codes spanning segments | P2 |
| 8 | `testIgnoredSegmentedTarget` | Segmented target ignored when configured | P2 |
| 9 | `testOutputOfResegmentedContent` | Re-segmented content output | P2 |
| 10 | `testGroupIds` | Group id hierarchy in XLIFF | P1 |
| 11 | `testCDATAEntry` | CDATA within trans-unit (data-driven with inlineCdata) | P2 |
| 12 | `testSegmentedEntry` | Segmented entry parsing and output | P1 |
| 13 | `testSegmentedSource1` | Segmented source parsing | P1 |
| 14 | `testSegmentedWithEmptyTarget` | Segmented source with empty target | P1 |
| 15 | `testEmptyTarget` | Empty target element handling | P1 |
| 16 | `testEmptyTargetOutput` | Empty target output behavior | P1 |
| 17 | `testStorageSizeAndAllowedChars` | Storage size and allowed characters properties | P2 |
| 18 | `testMtConfidence` | MT confidence annotation on trans-units | P2 |
| 19 | `testMtConfidenceInline` | MT confidence on inline elements | P2 |
| 20 | `testMtConfidenceAltTrans` | MT confidence in alt-trans elements | P2 |
| 21 | `testLQR` | Language Quality Report annotations | P2 |
| 22 | `testLQRInline` | LQR on inline elements | P2 |
| 23 | `testLangAndSpaceInline` | xml:lang and xml:space on inline elements | P2 |
| 24 | `testNoTarget` | Trans-unit with no target element | P1 |
| 25 | `testNoTargetOutput` | Output when no target exists | P1 |
| 26 | `testNoTargetOutputMonolingual` | Monolingual mode without target | P2 |
| 27 | `testNoTargetOutputMonolingualGenerateTarget` | Generate target in monolingual mode | P2 |
| 28 | `testCREntity` | CR entity (&#13;) handling | P2 |
| 29 | `testUnbalancedIT` | Unbalanced it (isolated) elements | P2 |
| 30 | `testBalancedIT` | Balanced it elements | P2 |
| 31 | `testCREntityOutput` | CR entity in output | P2 |
| 32 | `testMtConfidenceOutput` | MT confidence annotation output | P2 |
| 33 | `testSegmentedEntryWithDifferences` | Segmented entry with source/target differences | P2 |
| 34 | `testSegmentedEntryOutput` | Segmented entry output format | P1 |
| 35 | `testSegSourceWithoutMrkOutput` | Seg-source without mrk elements | P2 |
| 36 | `testRemoveSdlComment` | SDL comment removal from SDLXLIFF | P2 |
| 37 | `testRemoveNestedSdlComment` | Nested SDL comment removal | P2 |
| 38 | `testSpecialAttributeValues` | Special characters in attribute values | P2 |
| 39-185 | Additional XLIFF tests | Inline codes (bx/ex/bpt/ept/ph/it/x/g/mrk), alt-trans, notes, context groups, state handling, SDL/MQ/MemoQ properties, restype, subfilter, bilingual/monolingual modes | P1-P3 |

#### CdataSubfilteringTest.java (test count varies)

Tests CDATA content processed through subfilters in XLIFF.

#### PcdataSubfilteringTest.java (test count varies)

Tests PCDATA subfiltering within XLIFF elements.

#### SdlXliffConfLevelTest.java (test count varies)

Tests SDL XLIFF confidence level annotations.

#### XLIFFFilterBalancingTest.java (test count varies)

Tests code balancing in XLIFF (unmatched bpt/ept pairs).

#### XLIFFFilterCtypeTest.java (test count varies)

Tests ctype attribute handling for inline codes.

#### XLIFFFilterEquivTextTest.java (test count varies)

Tests equiv-text attribute on inline codes.

#### XLIFFFilterLengthConstraintsTest.java (test count varies)

Tests maxbytes/maxwidth length constraints on trans-units.

#### XLIFFFilterSDLPropTest.java (test count varies)

Tests SDL-specific properties in SDLXLIFF.

#### XLIFFFilterXtmPropTest.java (test count varies)

Tests XTM-specific properties in XLIFF.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripXliffIT` | `integration-tests/okapi/src/test/java/.../RoundTripXliffIT.java` | 2 |

**Test files used**: 143 files in `integration-tests/okapi/src/test/resources/xliff/`

**Known failing files**: None known in roundtrip

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `XliffXliffCompareIT` | `integration-tests/okapi/src/test/java/.../XliffXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/xliff/src/test/resources/`

105 files including `.xlf`, `.sdlxliff`, `.mqxliff`, `.fprm` config files.

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/xliff/`

143 files including sdlxliff, mqxliff, and standard XLIFF files.

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/xliff/src/test/resources/*.xlf okapi-testdata/okf_xliff/
cp -r okapi/filters/xliff/src/test/resources/sdlxliff okapi-testdata/okf_xliff/sdlxliff/
cp okapi/filters/xliff/src/test/resources/*.fprm okapi-testdata/okf_xliff/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/xliff/* okapi-testdata/okf_xliff/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/xliff`

Build tag: `//go:build integration`

#### xliff_test.go - Extraction Tests

```go
func TestExtract_BasicTransUnit(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        // ... from XLIFFFilterTest
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/xliff/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/xliff/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] All configuration/parameter tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] XLIFF compare structure matches
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - XLIFF 1.2 filter supports multiple XLIFF dialects: standard, SDL (sdlxliff), MemoQ (mqxliff)
  - Segmented content uses mrk elements with mtype="seg"
  - Inline codes: bx/ex (paired), bpt/ept (paired with rid), ph (placeholder), it (isolated), x (standalone), g (generic group)
  - ctype attribute maps to code types (bold, italic, link, etc.)
  - equiv-text provides display text for inline codes
  - SDL properties include comment, locked, origin-system
  - state attribute (new, translated, reviewed, final) affects processing
  - alt-trans elements provide alternative translations
  - inlineCdata parameter controls CDATA handling
  - Monolingual mode extracts source only; generateTarget creates empty target on output

## Current Go Coverage

### Bridge Tests (`core/plugin/bridge/filters/xliff/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `XLIFFFilterTest#testSegmentedTarget` | `TestExtract_SegmentedTarget` | Mapped |
| `XLIFFFilterTest#testGroupIds` | `TestExtract_GroupIds` | Mapped |
| `XLIFFFilterTest#testWSBetweenSegments` | `TestExtract_WSBetweenSegments` | Mapped |
| `XLIFFFilterTest#testSegmentedContent` | `TestExtract_SegmentedContent` | Mapped |
| `XLIFFFilterTest#testSegmentIDs` | `TestExtract_SegmentedContent` | Mapped |
| `XLIFFFilterTest#testEmptyTarget` | `TestExtract_EmptyTarget` | Mapped |
| `XLIFFFilterTest#testLQR` | `TestExtract_LQR` | Mapped |
| `RoundTripXliffIT` | `TestRoundTrip` | Mapped |

**Coverage**: ~7 of 246 Surefire methods have bridge `// okapi:` annotations (~3%).

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/xliff/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `XLIFFFilterTest.java` | `okapi/filters/xliff/src/test/java/.../` | 185 |
| `CdataSubfilteringTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `PcdataSubfilteringTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `SdlXliffConfLevelTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `XLIFFFilterBalancingTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `XLIFFFilterCtypeTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `XLIFFFilterEquivTextTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `XLIFFFilterLengthConstraintsTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `XLIFFFilterSDLPropTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
| `XLIFFFilterXtmPropTest.java` | `okapi/filters/xliff/src/test/java/.../` | varies |
