# okf_xini - XINI Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_xini` |
| Java Class | `net.sf.okapi.filters.xini.XINIFilter` |
| MIME Types | `text/x-xini` |
| Extensions | `.xini` |
| Okapi Module | `xini` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/xini/src/test/java/`

#### XINIFilterReaderTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `segmentBecomesTU` | XINI segment is converted to a TextUnit event | P1 |
| 2 | `segmentsAreGroupedInTUsByOriginalSegmentId` | Segments with same original segment ID are grouped into one TU | P1 |

#### XINIFilterFormattingTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `tagsBecomeCodes` | XINI formatting tags become inline codes | P1 |
| 2 | `formattingsBecomePreserved` | Formatting roundtrips are preserved | P1 |

#### XINIFilterPlaceholderTest.java (8 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `placeholdersBecomeCodes` | XINI placeholders become inline codes | P1 |
| 2 | `isolatedPlaceholdersBecomeCodes` | Isolated (unpaired) placeholders become codes | P1 |
| 3 | `placeholdersBecomePreserved` | Placeholder roundtrip preservation | P1 |
| 4 | `phTypeMemory100Preserved` | Placeholder type "memory100" is preserved | P2 |
| 5 | `phTypeUpdatedPreserved` | Placeholder type "updated" is preserved | P2 |
| 6 | `phTypeInsertedPreserved` | Placeholder type "inserted" is preserved | P2 |
| 7 | `phTypeDeletedPreserved` | Placeholder type "deleted" is preserved | P2 |

#### XINIFilterMetainformationTest.java (9 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `sourceAndTargetLanguagesPreserved` | Source/target language metadata roundtrip | P1 |
| 2 | `emptyPageDoesntCauseNullPointerException` | Empty page handling without NPE | P2 |
| 3 | `emptyFieldDoesntCauseNullPointerException` | Empty field handling without NPE | P2 |
| 4 | `pageAndElementIsPreserved` | Page and element metadata preservation | P1 |
| 5 | `fieldIsPreserved` | Field metadata preservation | P1 |
| 6 | `tableIsPreserved` | Table metadata preservation | P1 |
| 7 | `iniTableIsPreserved` | INI table metadata preservation | P1 |
| 8 | `segmentIsPreserved` | Segment metadata roundtrip preservation | P1 |

#### SegmentationAndDesegmentationTest.java (27 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `formattingsAreNotBreakingApart` | Formatting does not break during segmentation | P1 |
| 2 | `formattingsAreNotBreakingApart2` | Second formatting preservation variant | P1 |
| 3 | `formattingsAreNotBreakingApart3` | Third formatting preservation variant | P1 |
| 4 | `sentencesAreSegmentedAndWhitespaceIsSavedInAttribute` | Sentence segmentation with whitespace in attributes | P1 |
| 5 | `newSegmentsHaveIncreasingIDs` | New segments get sequential IDs | P2 |
| 6 | `originalSegmentIdIsSavedInAttribute` | Original segment ID stored in attribute | P2 |
| 7 | `placeholderDoesntChange` | Placeholders unchanged during segmentation | P1 |
| 8 | `placeholderDoesntChangeWithDifferentPlacholderType` | Different placeholder types preserved | P1 |
| 9 | `openingTagsPreservedInPlaceholders` | Opening tags in placeholders survive segmentation | P2 |
| 10 | `openingTagsPreservedInSinglePlaceholders` | Single placeholders with opening tags | P2 |
| 11 | `placeholdersAreNotBrokenApart` | Placeholders stay intact during segmentation | P1 |
| 12 | `formattingsAreNotBrokenApart` | Formatting stays intact during segmentation | P1 |
| 13 | `formattingTagsAndPlaceholdersDontChange` | Combined tags/placeholders unchanged | P1 |
| 14 | `lineBreaksArePreserved` | Line breaks survive segmentation | P1 |
| 15 | `surroundingWhitespacesAreMovedIntoAttributes` | Leading/trailing whitespace moved to attributes | P2 |
| 16 | `whitespacesFromInBetweenAreMovedIntoAttributes` | Inter-segment whitespace moved to attributes | P2 |
| 17 | `codesAreNotMovedIntoAttributes` | Inline codes not moved to whitespace attributes | P2 |
| 18 | `isolatedPlaceholdersArePreserved` | Isolated placeholders preserved during segmentation | P1 |
| 19 | `nestedPlaceholdersWithSameIdArePreservedUnchanged` | Nested same-ID placeholders preserved | P2 |
| 20 | `emptyPlaceholdersWithSameIdArePreservedUnchanged` | Empty same-ID placeholders preserved | P2 |
| 21 | `placeholdersWithSameIdArePreservedUnchanged` | Same-ID placeholders preserved | P2 |
| 22 | `desegmentizedXiniContainsTrailingWhitespaces` | Desegmented output has trailing whitespace | P2 |
| 23 | `desegmentizedXiniHasOriginalSegmentIDsRestored` | Original segment IDs restored after desegmentation | P1 |
| 24 | `segmentsMergedIfPreviousSegmentHasSurroundingTag` | Segments merge when previous has surrounding tag | P2 |
| 25 | `segmentsMergedIfNextSegmentHasSurroundingTag` | Segments merge when next has surrounding tag | P2 |
| 26 | `segmentsMergedIfBothSegmentsHaveSurroundingTag` | Segments merge when both have surrounding tags | P2 |

#### Rainbowkit Tests

##### FilterEventsToXiniTransformerTest.java (9 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `exportsPreTranslations` | Pre-translations exported to XINI | P3 |
| 2 | `exportsNonBreakingSpaceAsEmptyTranslation` | NBSP becomes empty translation | P3 |
| 3 | `xiniFieldStoresFieldLabelFromTuProperty` | Field label from TU property | P3 |
| 4 | `xiniFieldIsNullIfTuHasNoProperty` | Null field when TU has no property | P3 |
| 5 | `xiniFieldStoresFieldLabelFromStartGroupProperty` | Field label from StartGroup property | P3 |
| 6 | `labelFromOuterStartGroupIsOveriddenByInnerStartGroup` | Inner group label overrides outer | P3 |
| 7 | `labelFromOuterStartGroupIsUsedAfterEndingInnerGroup` | Outer label restored after inner ends | P3 |
| 8 | `labelFromStartGroupGetsResetByEndGroup` | Label reset on EndGroup | P3 |

##### XINIRainbowKitReaderTest.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `textSplitTagCodeNumbering` | Tag code numbering for split text | P3 |
| 2 | `textSplitTagCodeNumberingDescending` | Descending code numbering | P3 |
| 3 | `textSplitTagCodeNumberingAscending` | Ascending code numbering | P3 |

##### XINIRainbowkitWriterTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `writerUnderTestSavesGroupProperties` | Writer saves group properties | P3 |
| 2 | `writerUnderTestDeletesGroupValueWhenHandlingEndGroupEvent` | Writer deletes group value on EndGroup | P3 |

### Integration Tests

No dedicated integration tests exist for the XINI filter.

## Test Data Files

### Unit test resources

Source: `okapi/filters/xini/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `contents.xini` | `XINIFilterReaderTest`, `XINIFilterMetainformationTest`, multiple tests | Main XINI test document |
| `ascendingPhs.xini` | `XINIRainbowKitReaderTest#textSplitTagCodeNumberingAscending` | Ascending placeholder ordering |
| `descendingPhs.xini` | `XINIRainbowKitReaderTest#textSplitTagCodeNumberingDescending` | Descending placeholder ordering |
| `defaultSegmentation.srx` | `SegmentationAndDesegmentationTest` | SRX segmentation rules |

### Synthetic test data to create

None needed -- the existing `contents.xini` provides comprehensive test data.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/xini/src/test/resources/contents.xini okapi-testdata/okf_xini/
cp okapi/filters/xini/src/test/resources/ascendingPhs.xini okapi-testdata/okf_xini/
cp okapi/filters/xini/src/test/resources/descendingPhs.xini okapi-testdata/okf_xini/
cp okapi/filters/xini/src/test/resources/defaultSegmentation.srx okapi-testdata/okf_xini/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/xini`

Build tag: `//go:build integration`

#### xini_test.go - Extraction Tests

```go
func TestExtract_XINI(t *testing.T) {
    // Table-driven: maps to Java XINIFilterReaderTest + XINIFilterFormattingTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        {
            name:    "segment_becomes_tu",
            input:   "testdata/contents.xini",
            javaRef: "XINIFilterReaderTest#segmentBecomesTU",
        },
        {
            name:    "segments_grouped_by_original_id",
            input:   "testdata/contents.xini",
            javaRef: "XINIFilterReaderTest#segmentsAreGroupedInTUsByOriginalSegmentId",
        },
    }
}
```

#### placeholder_test.go - Placeholder Tests

```go
func TestPlaceholder_XINI(t *testing.T) {
    // Maps to Java XINIFilterPlaceholderTest
    tests := []struct {
        name    string
        input   string
        javaRef string
    }{
        // placeholdersBecomeCodes, isolatedPlaceholdersBecomeCodes,
        // placeholdersBecomePreserved, phType*Preserved
    }
}
```

#### metadata_test.go - Metainformation Tests

```go
func TestMetadata_XINI(t *testing.T) {
    // Maps to Java XINIFilterMetainformationTest
    tests := []struct {
        name    string
        input   string
        javaRef string
    }{
        // sourceAndTargetLanguagesPreserved, pageAndElementIsPreserved,
        // fieldIsPreserved, tableIsPreserved, iniTableIsPreserved, segmentIsPreserved
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "contents.xini",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/xini/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/xini/
```

### Success criteria

- [ ] All extraction tests pass
- [ ] All placeholder tests pass
- [ ] All metadata preservation tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- XINI is the XML-based interchange format used by ONTRAM translation management system
- Complex internal structure: pages, elements, fields, tables, INI tables, segments
- Supports segmentation and desegmentation (27 tests in SegmentationAndDesegmentationTest)
- Key parameter: `useOkapiSegmentation` (boolean, default true) -- controls whether output uses Okapi segmentation or original XINI segmentation
- Configurations: `okf_xini` (default), `okf_xini-noOutputSegmentation`
- Rainbowkit-related tests (14 total) are P3 as they test XINI-specific roundtrip kit functionality not directly used by the bridge
- The `contents.xini` file is the primary test resource used across multiple test classes

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/xini/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `XINIFilterReaderTest.java` | `okapi/filters/xini/src/test/java/net/sf/okapi/filters/xini/` | 2 |
| `XINIFilterFormattingTest.java` | `okapi/filters/xini/src/test/java/net/sf/okapi/filters/xini/` | 2 |
| `XINIFilterPlaceholderTest.java` | `okapi/filters/xini/src/test/java/net/sf/okapi/filters/xini/` | 8 |
| `XINIFilterMetainformationTest.java` | `okapi/filters/xini/src/test/java/net/sf/okapi/filters/xini/` | 9 |
| `SegmentationAndDesegmentationTest.java` | `okapi/filters/xini/src/test/java/net/sf/okapi/filters/xini/` | 27 |
| `FilterEventsToXiniTransformerTest.java` | `okapi/filters/xini/src/test/java/.../rainbowkit/` | 9 |
| `XINIRainbowKitReaderTest.java` | `okapi/filters/xini/src/test/java/.../rainbowkit/` | 3 |
| `XINIRainbowkitWriterTest.java` | `okapi/filters/xini/src/test/java/.../rainbowkit/` | 2 |
