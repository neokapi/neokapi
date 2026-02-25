# okf_fixedwidthcolumns - Fixed Width Columns Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_fixedwidthcolumns` |
| Java Class | `net.sf.okapi.filters.table.fwc.FixedWidthColumnsFilter` |
| MIME Types | `text/csv` |
| Extensions | - |
| Okapi Module | `table` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/table/src/test/java/`

#### FixedWidthColumnsFilterTest.java (15 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testEmptyInput` | Empty input handling | P1 |
| 2 | `testNameAndMimeType` | Filter name and MIME type correctness | P2 |
| 3 | `testParameters` | Parameter loading and serialization | P2 |
| 4 | `testListedColumns` | Column position-based extraction | P1 |
| 5 | `testListedColumns2` | Second listed columns variant | P1 |
| 6 | `testListedColumns3` | Third listed columns variant | P1 |
| 7 | `testListedColumns4` | Fourth listed columns variant | P1 |
| 8 | `testListedColumns5` | Fifth listed columns variant | P1 |
| 9 | `testFileEvents` | Event sequence for fixed-width file | P1 |
| 10 | `testHeader` | Header line handling | P2 |
| 11 | `testSkeleton` | Skeleton writer output | P1 |
| 12 | `testSkeleton2` | Second skeleton test | P1 |
| 13 | `testSkeleton3` | Third skeleton test | P1 |
| 14 | `testDoubleExtraction` | Extract-then-extract-again idempotency | P1 |
| 15 | `testSkelRefs` | Skeleton references correctness | P2 |

### Integration Tests

Integration tests are shared with the `okf_table` filter (see `okf_table.md`).

## Test Data Files

### Unit test resources

Source: `okapi/filters/table/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `fwc_test4.txt` | `testListedColumns*`, `testFileEvents` | Fixed-width column data |
| `fwc_test5.txt` | `testListedColumns3`-`testListedColumns5` | Alternative FWC data |
| `test_params1.txt` - `test_params4.txt` | `testParameters` | Parameter test files |

### Synthetic test data to create

None needed -- sufficient test files exist.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/table/src/test/resources/fwc_test4.txt okapi-testdata/okf_fixedwidthcolumns/
cp okapi/filters/table/src/test/resources/fwc_test5.txt okapi-testdata/okf_fixedwidthcolumns/
cp okapi/filters/table/src/test/resources/test_params*.txt okapi-testdata/okf_fixedwidthcolumns/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_fixedwidthcolumns`

Build tag: `//go:build integration`

#### fwc_test.go - Extraction Tests

```go
func TestExtract_FixedWidthColumns(t *testing.T) {
    // Table-driven: maps 1:1 to Java FixedWidthColumnsFilterTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // testListedColumns, testListedColumns2-5, testFileEvents
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_ColumnPositions(t *testing.T) {
    // Maps to Java FixedWidthColumnsFilterTest#testListedColumns*
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // columnStartPositions, columnEndPositions configurations
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Shares test infrastructure with okf_table
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_fixedwidthcolumns/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_fixedwidthcolumns/
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
- Unique parameters: `columnStartPositions` and `columnEndPositions` (comma-separated position lists)
- Default trimMode is 2 (trim to column boundaries) unlike CSV which uses trimMode 1
- Columns are defined by character position ranges rather than delimiters

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/table/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `FixedWidthColumnsFilterTest.java` | `okapi/filters/table/src/test/java/net/sf/okapi/filters/table/` | 15 |
