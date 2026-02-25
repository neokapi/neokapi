# okf_tabseparatedvalues - TSV Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_tabseparatedvalues` |
| Java Class | `net.sf.okapi.filters.table.tsv.TabSeparatedValuesFilter` |
| MIME Types | `text/csv` |
| Extensions | - |
| Okapi Module | `table` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/table/src/test/java/`

#### TabSeparatedValuesFilterTest.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testFileEvents` | Event sequence for basic TSV file | P1 |
| 2 | `testFileEvents2` | Event sequence for second TSV variant | P1 |
| 3 | `testSkeleton` | Skeleton writer output for TSV | P1 |
| 4 | `testSkeleton2` | Second skeleton test variant | P1 |
| 5 | `testDoubleExtraction` | Extract-then-extract-again idempotency | P1 |

### Integration Tests

Integration tests are shared with the `okf_table` filter (see `okf_table.md`). The `tab/` subdirectory contains TSV-specific roundtrip test files.

#### RoundTrip IT

Shared via `RoundTripTableIT` with `.tab` extension files.

#### XLIFF Compare IT

Shared via `TableXliffCompareIT` with `.tab` extension files.

#### Simplifier IT

Shared via `RoundTripSimplifyTableIT`.

## Test Data Files

### Unit test resources

Source: `okapi/filters/table/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `TSV_test.txt` | `testFileEvents`, `testFileEvents2` | Basic TSV test data |
| `okf_table@tabtest1.fprm` | `TabSeparatedValuesFilterTest` | TSV filter params |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/table/tab/`

| File | Type | Purpose |
|------|------|---------|
| `field_delimiter_tab.tab` | roundtrip | Tab-delimited roundtrip test |
| `okf_table@tab-default.fprm` | config | Tab-specific configuration |

### Synthetic test data to create

None needed -- sufficient test files exist.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/table/src/test/resources/TSV_test.txt okapi-testdata/okf_tabseparatedvalues/
cp okapi/filters/table/src/test/resources/okf_table@tabtest1.fprm okapi-testdata/okf_tabseparatedvalues/
cp okapi/filters/table/src/test/resources/okf_table@table_src_tab_trg.fprm okapi-testdata/okf_tabseparatedvalues/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/table/tab/ okapi-testdata/okf_tabseparatedvalues/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_tabseparatedvalues`

Build tag: `//go:build integration`

#### tsv_test.go - Extraction Tests

```go
func TestExtract_BasicTSV(t *testing.T) {
    // Table-driven: maps 1:1 to Java TabSeparatedValuesFilterTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // testFileEvents, testFileEvents2
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripTableIT (.tab files)
    testFiles := []string{
        "field_delimiter_tab.tab",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_tabseparatedvalues/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_tabseparatedvalues/
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
- TSV filter shares the table module infrastructure with CSV and FWC filters
- Columns are separated by one or more tab characters
- Parameters: shared base table params (columnNamesLineNum, valuesStartLineNum, sendColumnsMode, etc.) but no fieldDelimiter/textQualifier (tabs are implicit)

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/table/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TabSeparatedValuesFilterTest.java` | `okapi/filters/table/src/test/java/net/sf/okapi/filters/table/` | 5 |
