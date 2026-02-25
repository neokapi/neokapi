# okf_ttx - TTX Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_ttx` |
| Java Class | `net.sf.okapi.filters.ttx.TTXFilter` |
| MIME Types | `application/x-ttx+xml` |
| Extensions | `.ttx` |
| Okapi Module | `ttx` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/ttx/src/test/java/`

#### TTXFilterTest.java (46 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSegmentedSurroundedByInternalCodes` | Segmented TU between internal codes (ut Type="start"/"end") | P1 |
| 2 | `testNotSegmentedWithDFAndCodes` | Unsegmented content with DF elements and codes | P1 |
| 3 | `testNotSegmentedWithDF` | Unsegmented content with DF elements only | P1 |
| 4 | `testOutputNotSegmentedWithDF_ForcingOutSeg` | Output with forced segmentation on unsegmented DF content | P1 |
| 5 | `testOutputNotSegmentedWithLeadingWS` | Output preserves leading whitespace in unsegmented content | P2 |
| 6 | `testSegmentedSurroundedByDF` | Segmented TU surrounded by DF (displayable format) elements | P1 |
| 7 | `testSegmentedAndNot` | Mixed segmented/unsegmented content with Tu/ut/df elements | P1 |
| 8 | `testOutputSegmentedSurroundedByDF` | Output of segmented content with surrounding DF | P1 |
| 9 | `testBasicWithEscapes` | Basic TTX with XML entity escapes | P1 |
| 10 | `testOutputBasicWithEscapes` | Output preserves XML entity escapes | P1 |
| 11 | `testBasicNoExtractableData` | TTX with no extractable data produces no TUs | P2 |
| 12 | `testOutputNoExtractableData` | Output of TTX with no extractable data | P2 |
| 13 | `testBasicNoTU` | Basic TTX with text but no Tu elements | P2 |
| 14 | `testOutputBasicNoTUWithSegmentation` | Output with segmentation forced on content without Tu elements | P2 |
| 15 | `testOutputEscapesInSkeleton` | Escapes in skeleton preserved correctly | P2 |
| 16 | `testBasicNoTUWithDF` | Content without Tu but with DF elements | P2 |
| 17 | `testOutputBasicNoTUWithDFWithSegementation` | Output with DF and forced segmentation | P2 |
| 18 | `testVariousTags` | Various tag types: ut, df, ph, bpt, ept, it with different attributes | P1 |
| 19 | `testVariousTagsWithSegmentation` | Same tags with segmentation, verifying code types and tag types | P1 |
| 20 | `testOutputVariousTagsWithSegmentation` | Output verification for various tag types | P1 |
| 21 | `testWithMixedSegmentation` | Mixed segmented and unsegmented content in same document | P1 |
| 22 | `testOutputWithMixedSegmentation` | Output for mixed segmentation | P1 |
| 23 | `testTUInfo` | TU attributes: MatchPercent, Origin, CreationDate in AltTranslation annotation | P1 |
| 24 | `testOutputTUInfo` | Output preserves TU info attributes | P1 |
| 25 | `testTUInfoXU` | XU element with MatchPercent and MatchType=ICE | P1 |
| 26 | `testStartingExtraDF` | Extra DF at start of document | P2 |
| 27 | `testOutputStartingExtraDFWithSegmentation` | Output with extra DF and segmentation | P2 |
| 28 | `testWithPINoTU` | Processing instructions without TU | P2 |
| 29 | `testNoTUEndsWithUT` | No TU content ending with ut element | P2 |
| 30 | `testNoTUContentWithSplitStart` | No TU content with split start marker | P2 |
| 31 | `testNoTUContentWithUT` | No TU content with ut elements | P2 |
| 32 | `testOutputNoTUContentWithUTWithSegmentation` | Output for no-TU content with ut and segmentation | P2 |
| 33 | `testPartiallySegmentedEntryNothingTranslatable` | Partially segmented with nothing translatable | P2 |
| 34 | `testPartiallySegmentedEntry` | Partially segmented entry extraction | P1 |
| 35 | `testPartiallySegmentedEntryAfter` | Partially segmented with text after segment | P1 |
| 36 | `testOutputPartiallySegmentedEntryAfter` | Output for partially segmented with trailing text | P1 |
| 37 | `testLargePartiallySegmentedEntry` | Large partially segmented entry | P2 |
| 38 | `testForExternalDF` | External DF elements | P2 |
| 39 | `testOutputForExternalDFwithSegmentation` | Output for external DF with segmentation | P2 |
| 40 | `testForTwoTUs` | Two separate TU elements | P1 |
| 41 | `testForOneTU` | Single TU with source/target | P1 |
| 42 | `testForOneTUWithTextParts` | Single TU with text parts, verifying source/target segments | P1 |
| 43 | `testOutputForTwoTUsWithSegmentation` | Output for two TUs | P1 |
| 44 | `testOutputWithPINoTUWithSegmentation` | Output for PI without TU | P2 |
| 45 | `testBasicNoUT` | Basic content without ut elements, with codes | P1 |
| 46 | `testBasicTwoSegInOneTextUnit` | Two segments in one TU (source segments + target match) | P1 |

(continued: `testOutputBasicTwoSegInOneTextUnit`, `testBasicWithUT`, `testOutputSimple`, `testOutputSimpleGTEscaped`, `testOutputTwoTU`, `testOutputWithOriginalWithoutTraget`, `testDoubleExtraction`, `textDoubleExtractionOriginalAllSegmented`)

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTtxIT` | `integration-tests/okapi/src/test/java/.../RoundTripTtxIT.java` | 2 |

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TtxXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TtxXliffCompareIT.java` | N/A |

#### Simplifier IT

None found.

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/ttx/src/test/resources/`

Key test files used in double extraction:
- `TestFile01.ttx` - General test TTX file
- `TestOriginal01.ttx` - Original document TTX
- Additional TTX snippet tests use inline strings

### Synthetic test data to create

None needed - extensive snippet-based tests cover most scenarios.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/ttx/src/test/resources/net/sf/okapi/filters/ttx/*.ttx okapi-testdata/okf_ttx/

# Integration test resources
cp integration-tests/okapi/src/test/resources/ttx/*.ttx okapi-testdata/okf_ttx/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_ttx`

Build tag: `//go:build integration`

#### ttx_test.go - Extraction Tests

```go
func TestExtract_SegmentedContent(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "basic_with_escapes",
            input: "inline", // TTX snippet
            javaRef: "TTXFilterTest#testBasicWithEscapes",
        },
        {
            name:  "two_segments_one_tu",
            input: "inline",
            javaRef: "TTXFilterTest#testBasicTwoSegInOneTextUnit",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_SegmentMode(t *testing.T) {
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "all_segments",
            params: map[string]any{"segmentMode": "ALL"},
            javaRef: "TTXFilterTest (filterIncUnSeg)",
        },
        {
            name:   "existing_segments_only",
            params: map[string]any{"segmentMode": "EXISTINGSEGMENTS"},
            javaRef: "TTXFilterTest (filterNoUnSeg)",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "TestFile01.ttx",
        "TestOriginal01.ttx",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_ttx/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_ttx/
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
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks:
  - TTX is TRADOS TagEditor eXchange format (XML-based bilingual format)
  - Contains source and target text with match information (MatchPercent, Origin, MatchType)
  - Three segment modes: ALL (include unsegmented), EXISTINGSEGMENTS (only Tu elements), AUTO
  - Elements: Tu (translation unit), ut (unmatched tag), df (displayable format), bpt/ept (paired tags), ph (placeholder), it (isolated tag)
  - XU elements contain ICE (In-Context Exact) matches
  - AltTranslationsAnnotation preserves match metadata
  - Many tests use inline TTX snippets rather than files
  - The test class creates three filter instances with different segment modes

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/ttx/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TTXFilterTest.java` | `okapi/filters/ttx/src/test/java/.../` | 46+ |
