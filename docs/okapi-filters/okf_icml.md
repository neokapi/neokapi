# okf_icml - ICML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_icml` |
| Java Class | `net.sf.okapi.filters.icml.ICMLFilter` |
| MIME Types | `application/x-icml+xml` |
| Extensions | `.icml, .wcml` |
| Okapi Module | `icml` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/icml/src/test/java/`

#### ICMLFilterTest.java (13 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `createSkeletonWriter_ThenReturnNull` | Skeleton writer is null (ICML uses FilterWriter instead) | P3 |
| 2 | `createFilterWriter_ThenReturnICMLFilterWriter` | Filter writer is ICMLFilterWriter class | P3 |
| 3 | `getEncoderManager_ThenReturnEncoderManager` | Encoder manager is not null | P3 |
| 4 | `getName_ThenReturnName` | Filter name is "okf_icml" | P3 |
| 5 | `getDisplayName_ThenReturnDisplayName` | Display name is "ICML Filter" | P3 |
| 6 | `getMimeType_ThenReturnMimeType` | MIME type matches ICML_MIME_TYPE | P3 |
| 7 | `getConfigurations_ThenReturnDefaultSettings` | Default config has correct ID, extensions, MIME, name, description | P2 |
| 8 | `getParameters_WhenParametersSet_ThenReturnParametersWithSettings` | Custom parameters (extractMasterSpreads=false, extractNotes=false, newTuOnBr=false, simplifyCodes=false, skipThreshold=1) | P2 |
| 9 | `getParameters_WhenNoParametersSet_ThenReturnParametersWithDefaultSettings` | Default parameters (extractMasterSpreads=true, extractNotes=false, newTuOnBr=false, simplifyCodes=true, skipThreshold=1000) | P2 |
| 10 | `toString_WhenMultipleContent_ThenExtractInTranslationUnit` | TU from Test01.wcml has correct content/structure with multiple CharacterStyleRanges | P1 |
| 11 | `toString_WhenBreak_ThenTranslationUnitIsEmpty` | Break TU has empty string representation | P1 |
| 12 | `toString_WhenContentInTableCell_ThenSeparateTranslationUnit` | Table cell content in separate TU with cell structure | P1 |
| 13 | `open_WhenSuccessfull_ThenReturnTrue` | StartDocument test with Test01.wcml | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripIcmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripIcmlIT.java` | 2 |

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `IcmlXliffCompareIT` | `integration-tests/okapi/src/test/java/.../IcmlXliffCompareIT.java` | N/A |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyIcmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyIcmlIT.java` | N/A |

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/icml/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.wcml` | `ICMLFilterTest#multiple tests` | Main test document with multiple content, breaks, table cells |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.icml` | Minimal valid ICML document for smoke test | Simple one-paragraph document |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/icml/src/test/resources/net/sf/okapi/filters/icml/tests/*.wcml okapi-testdata/okf_icml/
cp okapi/filters/icml/src/test/resources/net/sf/okapi/filters/icml/tests/*.icml okapi-testdata/okf_icml/

# Integration test resources
cp integration-tests/okapi/src/test/resources/icml/*.icml okapi-testdata/okf_icml/roundtrip/
cp integration-tests/okapi/src/test/resources/icml/*.wcml okapi-testdata/okf_icml/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/icml`

Build tag: `//go:build integration`

#### icml_test.go - Extraction Tests

```go
func TestExtract_BasicContent(t *testing.T) {
    // Table-driven: maps 1:1 to Java ICMLFilterTest
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "multiple_content_ranges",
            input: "Test01.wcml",
            javaRef: "ICMLFilterTest#toString_WhenMultipleContent_ThenExtractInTranslationUnit",
        },
        {
            name:  "table_cell_content",
            input: "Test01.wcml",
            javaRef: "ICMLFilterTest#toString_WhenContentInTableCell_ThenSeparateTranslationUnit",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_Parameters(t *testing.T) {
    // Maps to Java ICMLFilterTest parameter tests
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "default_parameters",
            params: nil,
            javaRef: "ICMLFilterTest#getParameters_WhenNoParametersSet_ThenReturnParametersWithDefaultSettings",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripIcmlIT
    testFiles := []string{
        "Test01.wcml",
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
    // Maps to Java IcmlXliffCompareIT
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/icml/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/icml/
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
  - ICML is Adobe InDesign's InCopy Markup Language (XML-based, not a ZIP package like IDML)
  - Supports both `.icml` and `.wcml` extensions
  - Parameters: extractMasterSpreads, extractNotes, newTuOnBr, simplifyCodes, skipThreshold
  - SkeletonWriter returns null; uses ICMLFilterWriter for output
  - Limited unit test coverage (13 tests) - may need synthetic test data

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/icml/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `ICMLFilterTest.java` | `okapi/filters/icml/src/test/java/.../tests/` | 13 |
