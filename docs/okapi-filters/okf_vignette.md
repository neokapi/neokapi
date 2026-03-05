# okf_vignette - Vignette Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_vignette` |
| Java Class | `net.sf.okapi.filters.vignette.VignetteFilter` |
| MIME Types | `text/xml` |
| Extensions | - |
| Okapi Module | `vignette` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/vignette/src/test/java/`

#### VignetteFilterTest.java (7 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Filter parameters, name, and configurations are not null | P2 |
| 2 | `testStartDocument` | StartDocument event using Test01.xml | P1 |
| 3 | `testSimpleEntry` | Extracts "ENtext" from simple bilingual Vignette XML doc | P1 |
| 4 | `testSimpleEntryOutput` | Writer output matches expected XML with CDATA and locale attributes | P1 |
| 5 | `testComplexEntry` | Extracts multiple TUs from complex multi-content doc | P1 |
| 6 | `testComplexEntryOutput` | Writer output for complex doc with multiple locales and content IDs | P1 |
| 7 | `testDoubleExtraction` | Extract-then-extract-again idempotency using Test01.xml | P1 |

#### VignetteFilterTest2.java (7 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Filter parameters, name, and configurations are not null | P3 |
| 2 | `testStartDocument` | StartDocument event using Test01.xml | P3 |
| 3 | `testSimpleEntry` | Same as VignetteFilterTest but duplicated test class | P3 |
| 4 | `testSimpleEntryOutput` | Same writer output test (duplicate) | P3 |
| 5 | `testComplexEntry` | Same complex entry test (duplicate) | P3 |
| 6 | `testComplexEntryOutput` | Same complex output test (duplicate) | P3 |
| 7 | `testDoubleExtraction` | Same double extraction (duplicate) | P3 |

### Integration Tests

No dedicated integration tests exist for the Vignette filter.

## Test Data Files

### Unit test resources

Source: `okapi/filters/vignette/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.xml` | `testStartDocument`, `testDoubleExtraction` | Vignette XML test document |

### Integration test resources

No integration test resources exist.

### Synthetic test data to create

None needed -- `Test01.xml` and the inline snippets in the test code provide adequate coverage.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/vignette/src/test/resources/Test01.xml okapi-testdata/okf_vignette/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/vignette`

Build tag: `//go:build integration`

#### vignette_test.go - Extraction Tests

```go
func TestExtract_Vignette(t *testing.T) {
    // Table-driven: maps 1:1 to Java VignetteFilterTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        {
            name:       "simple_entry",
            input:      `<importProject><importContentInstance><contentInstance>...`,
            wantTexts:  []string{"ENtext"},
            javaRef:    "VignetteFilterTest#testSimpleEntry",
        },
        {
            name:       "complex_entry",
            input:      `<importProject>...`, // multi-content instance
            wantTexts:  []string{"EN-id1", "EN-id2"},
            javaRef:    "VignetteFilterTest#testComplexEntry",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java VignetteFilterTest#testDoubleExtraction
    testFiles := []string{
        "Test01.xml",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/vignette/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/vignette/
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
- Vignette is a CMS export/import format using XML with `<importContentInstance>` / `<contentInstance>` / `<attribute>` structure
- Content can be in CDATA sections or escaped HTML within `<valueCLOB>` elements
- Bilingual: source and target are identified by `SOURCE_ID` and `LOCALE_ID` attribute elements
- Parameters: `partsNames` (comma-separated attribute names to extract), `partsConfigurations` (sub-filter configs), `sourceId`, `localeId`, `monolingual`, `useCDATA`, `quoteMode`
- Configurations: `okf_vignette` (default, with CDATA), `okf_vignette-nocdata` (escaped HTML)
- `VignetteFilterTest2.java` appears to be an exact duplicate of `VignetteFilterTest.java` -- prioritize tests from the first class only

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/vignette/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `VignetteFilterTest.java` | `okapi/filters/vignette/src/test/java/net/sf/okapi/filters/vignette/` | 7 |
| `VignetteFilterTest2.java` | `okapi/filters/vignette/src/test/java/net/sf/okapi/filters/vignette/` | 7 (duplicate) |
