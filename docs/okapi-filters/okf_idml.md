# okf_idml - IDML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_idml` |
| Java Class | `net.sf.okapi.filters.idml.IDMLFilter` |
| MIME Types | `application/vnd.adobe.indesign-idml-package` |
| Extensions | `.idml` |
| Okapi Module | `idml` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/idml/src/test/java/`

#### ExtractionTest.java (38 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Filter has non-null parameters, name, and configurations | P2 |
| 2 | `testSimpleEntry` | Basic extraction of "Hello World!" from helloworld-1.idml | P1 |
| 3 | `testSimpleEntry2` | Extraction from Test00.idml with inline codes and text content | P1 |
| 4 | `testWhitespaces` | 14 text units with tabs, whitespace, m-space, n-space preservation | P1 |
| 5 | `testNewline` | Newline handling produces separate text units | P1 |
| 6 | `testStartDocument` | StartDocument event from Test01.idml | P2 |
| 7 | `testObjectsWithoutPathPointsAndText` | Objects without path points produce 0 text units | P2 |
| 8 | `testAnchoredFrameWithoutPathPoints` | Anchored frame text extracted at index 4 | P2 |
| 9 | `testDocumentWithoutPathPoints` | Document without path points extracts correctly | P2 |
| 10 | `testSkipDiscretionaryHyphens` | skipDiscretionaryHyphens parameter removes soft hyphens | P2 |
| 11 | `testChangeTracking` | Change tracking with conditional text (8-conditional-text), change-tracking-3.idml with 14 text units | P1 |
| 12 | `extractsBreaksInline` | extractBreaksInline merges paragraph breaks into single text unit | P2 |
| 13 | `customTextVariablesExtracted` | extractCustomTextVariables extracts custom variable content | P2 |
| 14 | `indexTopicsExtracted` | extractIndexTopics extracts index topic entries | P2 |
| 15 | `endNotesExtracted` | Endnotes extracted from 856-1.idml | P2 |
| 16 | `doesNotMergeTagsThatDifferByKerning` | Tags with different kerning values remain separate | P2 |
| 17 | `mergesTagsThatDifferByKerningWithEmptyIgnoranceThresholds` | ignoreCharacterKerning merges all kerning differences | P2 |
| 18 | `mergesTagsThatDifferByKerningWithMinIgnoranceThreshold` | Kerning min threshold preserves tags below threshold | P2 |
| 19 | `mergesTagsThatDifferByKerningWithMaxIgnoranceThreshold` | Kerning max threshold preserves tags above threshold | P2 |
| 20 | `mergesTagsThatDifferByKerningWithMinAndMaxIgnoranceThresholds` | Both min and max kerning thresholds applied | P2 |
| 21 | `doesNotMergeTagsThatDifferByTracking` | Tags with different tracking values remain separate | P2 |
| 22 | `mergesTagsThatDifferByTrackingWithEmptyIgnoranceThresholds` | ignoreCharacterTracking merges all tracking differences | P2 |
| 23 | `mergesTagsThatDifferByTrackingWithMinIgnoranceThreshold` | Tracking min threshold applied | P2 |
| 24 | `mergesTagsThatDifferByTrackingWithMaxIgnoranceThreshold` | Tracking max threshold applied | P2 |
| 25 | `mergesTagsThatDifferByTrackingWithMinAndMaxIgnoranceThresholds` | Both min and max tracking thresholds | P2 |
| 26 | `doesNotMergeTagsThatDifferByLeading` | Tags with different leading values remain separate | P2 |
| 27 | `mergesTagsThatDifferByLeadingWithoutIgnoranceThresholds` | ignoreCharacterLeading merges all leading differences | P2 |
| 28 | `mergesTagsThatDifferByLeadingWithMinIgnoranceThreshold` | Leading min threshold applied | P2 |
| 29 | `mergesTagsThatDifferByLeadingWithMaxIgnoranceThreshold` | Leading max threshold applied | P2 |
| 30 | `mergesTagsThatDifferByLeadingWithMinAndMaxIgnoranceThresholds` | Both min and max leading thresholds | P2 |
| 31 | `doesNotMergeTagsThatDifferByBaselineShift` | Tags with different baseline shift remain separate | P2 |
| 32 | `mergesTagsThatDifferByBaselineShiftWithoutIgnoranceThresholds` | ignoreCharacterBaselineShift merges all | P2 |
| 33 | `mergesTagsThatDifferByBaselineShiftWithMinIgnoranceThreshold` | Baseline shift min threshold | P2 |
| 34 | `mergesTagsThatDifferByBaselineShiftWithMaxIgnoranceThreshold` | Baseline shift max threshold | P2 |
| 35 | `mergesTagsThatDifferByBaselineShiftWithMinAndMaxIgnoranceThresholds` | Both min and max baseline shift thresholds | P2 |
| 36 | `doesNotMergeTagsThatDifferByKerningMethod` | Kerning method differences preserve tags | P2 |
| 37 | `mergesTagsThatDifferByKerningMethod` | ignoreCharacterKerning also merges kerning method diffs | P2 |
| 38 | `doesNotMergeTagsThatDifferByKerningInReferencesAndXmlStructures` | Kerning in hyperlinks/footnotes/tables/tags not merged | P2 |

(continued - additional tests in ExtractionTest.java not listed above for brevity, but include: `mergesTagsThatDifferByKerningInReferencesAndXmlStructures`, `extractsWithLeastAvailableStyleFormattingBaselined`, `pasteboardItemsWithoutAnchorPointsPositionedCorrectly`, `hiddenPasteboardItemsExtracted`, `hyperlinkTextSourcesExtractedAsReferenceGroups`, `hyperlinkTextSourcesExtractedInline`, `externalHyperlinksExtracted`, `specialCharacterPatternApplied`, `codeFinderApplied`, `mathZonesConditionallyExtracted`, `stylesExcluded`, `adjacentCodesMerged`)

#### IDMLFilterInParallelTest.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testInMultipleThreads` | Thread safety: 10 threads x 2 rounds processing TextPathTest04.idml | P3 |

#### ParametersTest.java (18 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `initialisesDefaultParameters` | Default parameter string serialization matches expected format | P2 |
| 2 | `initialisesStyleIgnorances` | Style ignorances initialized from parameter string with thresholds | P2 |
| 3 | `setsCharacterKerningMinIgnoranceThreshold` | Setting kerning min threshold normalizes to double | P3 |
| 4 | `failsToSetCharacterKerningMinIgnoranceThreshold` | Invalid kerning min threshold throws IllegalArgumentException | P3 |
| 5 | `setsCharacterKerningMaxIgnoranceThreshold` | Setting kerning max threshold normalizes to double | P3 |
| 6 | `failsToSetCharacterKerningMaxIgnoranceThreshold` | Invalid kerning max threshold throws IllegalArgumentException | P3 |
| 7 | `setsCharacterTrackingMinIgnoranceThreshold` | Tracking min threshold normalization | P3 |
| 8 | `failsToSetCharacterTrackingMinIgnoranceThreshold` | Invalid tracking min threshold error | P3 |
| 9 | `setsCharacterTrackingMaxIgnoranceThreshold` | Tracking max threshold normalization | P3 |
| 10 | `failsToSetCharacterTrackingMaxIgnoranceThreshold` | Invalid tracking max threshold error | P3 |
| 11 | `setsCharacterLeadingMinIgnoranceThreshold` | Leading min threshold normalization | P3 |
| 12 | `failsToSetCharacterLeadingMinIgnoranceThreshold` | Invalid leading min threshold error | P3 |
| 13 | `setsCharacterLeadingMaxIgnoranceThreshold` | Leading max threshold normalization | P3 |
| 14 | `failsToSetCharacterLeadingMaxIgnoranceThreshold` | Invalid leading max threshold error | P3 |
| 15 | `setsCharacterBaselineShiftMinIgnoranceThreshold` | Baseline shift min threshold normalization | P3 |
| 16 | `failsToSetCharacterBaselineShiftMinIgnoranceThreshold` | Invalid baseline shift min threshold error | P3 |
| 17 | `setsCharacterBaselineShiftMaxIgnoranceThreshold` | Baseline shift max threshold normalization | P3 |
| 18 | `failsToSetCharacterBaselineShiftMaxIgnoranceThreshold` | Invalid baseline shift max threshold error | P3 |

(Also includes: `excludedStyleConfigurationsInitialised`, `fontMappingsAreInitialised`)

#### RoundTripTest.java (17 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDoubleExtraction` | Double extraction roundtrip for ~50 IDML files with various configs | P1 |
| 2 | `documentWithChainedFontMappings` | Chained font mappings (Times->Arial Unicode MS->Meiryo) | P1 |
| 3 | `documentsWithDefaultParameters` | Default parameter roundtrip for 926.idml | P1 |
| 4 | `fontMappingForNamesWithProcessingInstructionsSupported` | Font names with PIs like `<?ACE b?>` | P2 |
| 5 | `emptyTargetsMerged` | Empty targets merged back correctly for 629.idml | P1 |
| 6 | `specialCharactersExtractedAndMerged` | Special characters roundtrip for 175-special-characters.idml | P1 |
| 7 | `customTextVariablesExtractedAndMerged` | Custom text variables roundtrip | P2 |
| 8 | `indexTopicsExtractedAndMerged` | Index topics roundtrip | P2 |
| 9 | `endNotesExtractedAndMerged` | Endnotes roundtrip for 856-1.idml and 856-2.idml | P1 |
| 10 | `hyperlinkTextSourcesExtractedAndMerged` | Hyperlink text sources roundtrip, both inline and as reference groups | P1 |
| 11 | `externalHyperlinksExtractedAndMerged` | External hyperlinks extraction and merge | P2 |
| 12 | `emptyContentStylesPreserved` | Empty paragraph styles preserved (1369-*.idml) | P2 |
| 13 | `mathZonesConditionalExtractionSupported` | Math zones conditional extraction roundtrip | P2 |
| 14 | `stylesExclusionSupported` | Style exclusion roundtrip | P2 |
| 15 | `adjacentCodesMergeSupported` | Adjacent codes merge roundtrip with code finder | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripIdmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripIdmlIT.java` | 2 |

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `IdmlXliffCompareIT` | `integration-tests/okapi/src/test/java/.../IdmlXliffCompareIT.java` | N/A |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyIdmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyIdmlIT.java` | N/A |

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/idml/src/test/resources/`

Key test files:
- `helloworld-1.idml` - Minimal hello world document
- `Test00.idml` through `Test03.idml` - Basic test documents
- `tabsAndWhitespaces.idml` - Whitespace handling
- `newline.idml` - Newline handling
- `idmltest.idml` - General test
- `01-pages-with-text-frames*.idml` (6 files) - Page layout variations
- `02-island-spread-and-threaded-text-frames.idml` - Threaded frames
- `03-hyperlink-and-table-content.idml` - Hyperlinks and tables
- `04-complex-formatting.idml` - Complex formatting
- `05-complex-ordering.idml` - Complex text ordering
- `06-hello-world-*.idml` (3 files) - Hello world variants
- `07-paragraph-breaks.idml` - Paragraph break handling
- `08-conditional-text-and-tracked-changes.idml` - Conditional text/tracked changes
- `08-direct-story-content.idml` - Direct story content
- `09-footnotes.idml` - Footnotes
- `10-tables.idml` - Table handling
- `11-xml-structures.idml` - XML structures
- `618-*.idml` (3 files) - Path point edge cases
- `Bindestrich.idml` - Discretionary hyphens
- `756-character-kerning.idml` - Character kerning
- `756-character-tracking.idml` - Character tracking
- `756-character-leading.idml` - Character leading
- `756-character-baseline-shift.idml` - Baseline shift
- `777-character-kerning-method.idml` - Kerning method
- `779-reference-and-tag-styles.idml` - Reference/tag styles
- `856-1.idml`, `856-2.idml` - Endnotes
- `923-baselined-formatting.idml` - Baselined formatting
- `926.idml` - Font mappings
- `935-complex-ordering-without-anchor-points.idml` - Ordering without anchors
- `1016.idml` - Hidden pasteboard items (533+ text units)
- `1138.idml` - Custom text variables
- `1139.idml` - Index topics
- `1179-0.idml` through `1179-4.idml` - Hyperlink text sources
- `1369-empty-paragraph-styles.idml` - Empty paragraph styles
- `1369-empty-paragraph-in-table-cell-styles.idml` - Empty table cell styles
- `1412-math-zones.idml` - Math zones
- `1415-adjacent-codes.idml` - Adjacent codes
- `1418-styles-exclusion.idml` - Style exclusion
- `1432-font-name.idml` - Font names with PIs
- `629.idml` - Empty target merge
- `change-tracking-3.idml` - Change tracking variant
- `ConditionalText.idml` - Conditional text
- `testWithSpecialChars.idml` - Special characters
- `TextPathTest01.idml` through `TextPathTest04.idml` - Text path tests
- `codefinder.idml` - Code finder
- `175-special-characters.idml` - Special characters

### Synthetic test data to create

None needed - extensive test data already exists.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/idml/src/test/resources/net/sf/okapi/filters/idml/*.idml okapi-testdata/okf_idml/
cp okapi/filters/idml/src/test/resources/net/sf/okapi/filters/idml/*.fprm okapi-testdata/okf_idml/

# Integration test resources
cp integration-tests/okapi/src/test/resources/idml/*.idml okapi-testdata/okf_idml/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_idml`

Build tag: `//go:build integration`

#### idml_test.go - Extraction Tests

```go
func TestExtract_SimpleEntry(t *testing.T) {
    // Table-driven: maps 1:1 to Java ExtractionTest
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "hello_world",
            input: "helloworld-1.idml",
            wantTexts: []string{"Hello World!"},
            javaRef: "ExtractionTest#testSimpleEntry",
        },
        {
            name:  "whitespaces",
            input: "tabsAndWhitespaces.idml",
            wantBlocks: 14,
            javaRef: "ExtractionTest#testWhitespaces",
        },
        {
            name:  "change_tracking",
            input: "08-conditional-text-and-tracked-changes.idml",
            wantTexts: []string{"Conditional Text Sample", "New text."},
            javaRef: "ExtractionTest#testChangeTracking",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_KerningIgnorance(t *testing.T) {
    // Maps to Java ExtractionTest kerning/tracking/leading tests
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "kerning_ignored",
            params: map[string]any{"ignoreCharacterKerning": true},
            input:  "756-character-kerning.idml",
            want:   []string{"Kerning-25-10-5-2+0+2+5+10+25"},
            javaRef: "ExtractionTest#mergesTagsThatDifferByKerningWithEmptyIgnoranceThresholds",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripTest#testDoubleExtraction
    testFiles := []string{
        "helloworld-1.idml",
        "Test00.idml", "Test01.idml", "Test02.idml", "Test03.idml",
        "01-pages-with-text-frames.idml",
        "03-hyperlink-and-table-content.idml",
        "07-paragraph-breaks.idml",
        "08-conditional-text-and-tracked-changes.idml",
        "09-footnotes.idml", "10-tables.idml",
    }
    knownFailing := map[string]string{
        // None expected
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java IdmlXliffCompareIT
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_idml/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_idml/
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
  - IDML is a ZIP package containing InDesign XML stories
  - Heavy use of character style ignorance thresholds (kerning, tracking, leading, baseline shift)
  - Font mappings support chained remapping with locale patterns
  - Supports extracting hidden pasteboard items, custom text variables, index topics
  - Thread-safe (verified by IDMLFilterInParallelTest)
  - Style exclusion via pattern matching on style names

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/idml/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `ExtractionTest.java` | `okapi/filters/idml/src/test/java/.../` | 38 |
| `IDMLFilterInParallelTest.java` | `okapi/filters/idml/src/test/java/.../` | 1 |
| `ParametersTest.java` | `okapi/filters/idml/src/test/java/.../` | 20 |
| `RoundTripTest.java` | `okapi/filters/idml/src/test/java/.../` | 17 |
