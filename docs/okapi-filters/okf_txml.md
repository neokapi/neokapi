# okf_txml - TXML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_txml` |
| Java Class | `net.sf.okapi.filters.txml.TXMLFilter` |
| MIME Types | `text/xml` |
| Extensions | `.txml` |
| Okapi Module | `txml` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/txml/src/test/java/`

#### TXMLFilterTest.java (14 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSimpleEntry` | Basic TXML: 2 segments in translatable block, source/target extraction | P1 |
| 2 | `testRevisedEntry` | Segment with revision history preserves current target over revisions | P1 |
| 3 | `testEntryWithCodes` | Segment with ut (unmatched tag) codes: bold tags as inline codes | P1 |
| 4 | `testEntryWithFirstOutOf2SegmentsCommentedOut` | First segment commented out, second segment extracted | P1 |
| 5 | `testEntryWithSecondOutOf2SegmentsCommentedOut` | Second segment commented out, first segment extracted | P1 |
| 6 | `testEntryWithAllSegmentsCommentedOut` | All segments commented out, no TU extracted | P1 |
| 7 | `testEntryWithThirdSegmentsNotCommentedOut` | Two commented, one active segment extracted | P1 |
| 8 | `testEntryWith1SegmentCommentedOut` | Single commented segment, no TU extracted | P2 |
| 9 | `testOutputWithCommentedOutSegments` | Output preserves commented-out segments in place, adds gtmt="false" to active | P1 |
| 10 | `testWS` | Whitespace (ws) elements between segments with inline codes | P1 |
| 11 | `testSegments` | Multiple segments with ws elements including empty source segment | P1 |
| 12 | `testEmptySegments` | Empty source segments preserved, no target created | P2 |
| 13 | `testDoubleExtraction` | Double extraction roundtrip for Test01.docx.txml, Test02.html.txml, Test03.mif.txml | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTxmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripTxmlIT.java` | 2 |

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TxmlXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TxmlXliffCompareIT.java` | N/A |

#### Simplifier IT

None found.

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/txml/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.docx.txml` | `testDoubleExtraction` | TXML from DOCX source |
| `Test02.html.txml` | `testDoubleExtraction` | TXML from HTML source |
| `Test03.mif.txml` | `testDoubleExtraction` | TXML from MIF source |

Most tests use inline TXML snippets.

### Synthetic test data to create

None needed - inline snippet tests provide good coverage.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/txml/src/test/resources/net/sf/okapi/filters/txml/*.txml okapi-testdata/okf_txml/

# Integration test resources
cp integration-tests/okapi/src/test/resources/txml/*.txml okapi-testdata/okf_txml/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/txml`

Build tag: `//go:build integration`

#### txml_test.go - Extraction Tests

```go
func TestExtract_BasicSegments(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "simple_entry",
            input: "inline", // TXML snippet
            wantTexts: []string{"Segment one"},
            javaRef: "TXMLFilterTest#testSimpleEntry",
        },
        {
            name:  "entry_with_codes",
            input: "inline",
            javaRef: "TXMLFilterTest#testEntryWithCodes",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_CommentedSegments(t *testing.T) {
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:  "all_commented_out",
            input: "inline",
            want:  nil, // no TU expected
            javaRef: "TXMLFilterTest#testEntryWithAllSegmentsCommentedOut",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "Test01.docx.txml",
        "Test02.html.txml",
        "Test03.mif.txml",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/txml/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/txml/
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
  - TXML is WorldServer's Translation XML format (bilingual, segmented)
  - Contains translatable blocks with segments that have source, target, and optional revision history
  - Segments can be commented out (XML comments) and should be preserved in output
  - ws (whitespace) elements appear between segments and contain inline codes
  - ut (unmatched tag) elements contain inline codes like bold tags
  - Output adds gtmt="false" attribute to active segments
  - Revision history preserves previous translations
  - Test data includes TXML from different source formats (DOCX, HTML, MIF)
  - Many tests use inline TXML snippets with STARTFILE prefix

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/txml/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TXMLFilterTest.java` | `okapi/filters/txml/src/test/java/.../` | 14 |
