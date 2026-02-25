# okf_basetable - Base Table Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_basetable` |
| Java Class | `net.sf.okapi.filters.table.base.BaseTableFilter` |
| MIME Types | `text/csv` |
| Extensions | `.txt` |
| Okapi Module | `table` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/table/src/test/java/`

The BaseTableFilter does not have its own dedicated test class. It is tested indirectly through `TableFilterTest.java` since `TableFilter` delegates to `BaseTableFilter` for basic table operations.

#### TableFilterTest.java (shared, 15 @Test methods)

See `okf_table.md` for the full test inventory. The BaseTableFilter is exercised by the table filter's default configuration path.

### Integration Tests

Integration tests are shared with the `okf_table` filter (see `okf_table.md`).

## Test Data Files

### Unit test resources

Source: `okapi/filters/table/src/test/resources/`

The BaseTableFilter shares test resources with the table filter family. See `okf_table.md` for the full resource listing.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.txt` | Minimal valid single-column text for smoke test | Simple line-per-row format |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Shared unit test resources from table module
cp okapi/filters/table/src/test/resources/csv.txt okapi-testdata/okf_basetable/
cp okapi/filters/table/src/test/resources/csv2.txt okapi-testdata/okf_basetable/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_basetable`

Build tag: `//go:build integration`

#### basetable_test.go - Extraction Tests

```go
func TestExtract_BaseTable(t *testing.T) {
    // Table-driven: based on shared TableFilterTest patterns
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // basic extraction, skeleton, double extraction
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Shares roundtrip infrastructure with okf_table
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_basetable/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_basetable/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- BaseTableFilter is the parent class for all table sub-filters (CSV, TSV, FWC)
- It provides the core column-mapping and extraction logic
- Configurations: `okf_plaintext` (default), `okf_plaintext_trim_trail`, `okf_plaintext_trim_all`
- Shares parameter set with other table sub-filters: columnNamesLineNum, valuesStartLineNum, sendColumnsMode, sourceColumns, targetColumns, etc.

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/table/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TableFilterTest.java` (shared) | `okapi/filters/table/src/test/java/net/sf/okapi/filters/table/` | 15 |
