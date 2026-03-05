# okf_table - Table Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_table` |
| Java Class | `net.sf.okapi.filters.table.TableFilter` |
| MIME Types | `text/csv` |
| Extensions | `.csv` |
| Okapi Module | `table` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/table/src/test/java/`

#### TableFilterTest.java (15 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testEmptyInput` | Handles empty input without errors | P1 |
| 2 | `testNameAndMimeType` | Filter name and MIME type are correct | P2 |
| 3 | `testFileEvents` | Event sequence for basic table file | P1 |
| 4 | `testFileEvents2` | Event sequence for second table variant | P1 |
| 5 | `testColumnDefinedLocales` | Column-defined locale handling | P2 |
| 6 | `testColumnDefinedSource` | Column-defined source column | P2 |
| 7 | `testColumnDefinedTarget` | Column-defined target column | P2 |
| 8 | `testSynchronization` | Event synchronization correctness | P2 |
| 9 | `testTrimMode` | Whitespace trim mode behavior | P2 |
| 10 | `testMultilineColNames` | Multi-line column name handling | P2 |
| 11 | `testSkeleton` | Skeleton writer output correctness | P1 |
| 12 | `testSkeleton3` | Alternative skeleton test case | P1 |
| 13 | `testDoubleExtraction` | Extract-then-extract-again idempotency | P1 |
| 14 | `testIssue124` | Regression fix for issue 124 | P2 |
| 15 | `testIssue1128` | Regression fix for issue 1128 | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTableIT` | `integration-tests/okapi/src/test/java/.../RoundTripTableIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/table/`):
- `simple.csv`
- `computer_science_article.csv`
- `field_delimiter_*.csv` (colon, comma, pipe, semicolon, space)
- `some_blank_cells.csv`, `some_blank_columns.csv`, `some_blank_rows.csv`
- `test2cols.csv`
- `text_qualifier_*.csv` (double/single quote, inside variants)
- `Issue404/Issue_404 .csv`
- `Table_with_locales/*.csv`
- `debug/test.csv`
- `issue1128/strong.csv`
- `tab/field_delimiter_tab.tab`
- `two_coulmns/test2cols.csv`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TableXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TableXliffCompareIT.java` | 1 |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyTableIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyTableIT.java` | 2 |

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `TableMemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../TableMemoryLeakTestIT.java` | inherited |

## Test Data Files

### Unit test resources

Source: `okapi/filters/table/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `csv.txt` | `TableFilterTest#testFileEvents` | Basic CSV data |
| `csv2.txt` | `TableFilterTest#testFileEvents2` | Second CSV variant |
| `Locale_defined_TSV_test.txt` | `TableFilterTest#testColumnDefinedLocales` | Locale column definitions |
| `okf_table@defined_locales.fprm` | `TableFilterTest` | Locale-defined params |
| `okf_table@test124.fprm` | `TableFilterTest#testIssue124` | Issue 124 params |
| `strong.csv` | `TableFilterTest#testIssue1128` | Issue 1128 data |
| `okf_table@strong.fprm` | `TableFilterTest#testIssue1128` | Issue 1128 params |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/table/`

| File | Type | Purpose |
|------|------|---------|
| `simple.csv` | roundtrip | Basic roundtrip test |
| `computer_science_article.csv` | roundtrip | Multi-field article content |
| `field_delimiter_*.csv` | roundtrip | Various delimiter types |
| `some_blank_*.csv` | roundtrip | Blank cell/column/row handling |
| `text_qualifier_*.csv` | roundtrip | Quote handling variants |
| `issue1128/strong.csv` | roundtrip / xliff-compare | Strong tag content in CSV |

### Synthetic test data to create

None needed -- sufficient test files exist in the Okapi test suite.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/table/src/test/resources/csv.txt okapi-testdata/okf_table/
cp okapi/filters/table/src/test/resources/csv2.txt okapi-testdata/okf_table/
cp okapi/filters/table/src/test/resources/csv3.txt okapi-testdata/okf_table/
cp okapi/filters/table/src/test/resources/Locale_defined_TSV_test.txt okapi-testdata/okf_table/
cp okapi/filters/table/src/test/resources/strong.csv okapi-testdata/okf_table/
cp okapi/filters/table/src/test/resources/okf_table@*.fprm okapi-testdata/okf_table/

# Integration test resources
cp integration-tests/okapi/src/test/resources/table/simple.csv okapi-testdata/okf_table/roundtrip/
cp integration-tests/okapi/src/test/resources/table/computer_science_article.csv okapi-testdata/okf_table/roundtrip/
cp integration-tests/okapi/src/test/resources/table/field_delimiter_*.csv okapi-testdata/okf_table/roundtrip/
cp integration-tests/okapi/src/test/resources/table/some_blank_*.csv okapi-testdata/okf_table/roundtrip/
cp integration-tests/okapi/src/test/resources/table/test2cols.csv okapi-testdata/okf_table/roundtrip/
cp integration-tests/okapi/src/test/resources/table/text_qualifier_*.csv okapi-testdata/okf_table/roundtrip/
cp -r integration-tests/okapi/src/test/resources/table/Issue404/ okapi-testdata/okf_table/roundtrip/
cp -r integration-tests/okapi/src/test/resources/table/Table_with_locales/ okapi-testdata/okf_table/roundtrip/
cp -r integration-tests/okapi/src/test/resources/table/issue1128/ okapi-testdata/okf_table/roundtrip/
cp -r integration-tests/okapi/src/test/resources/table/tab/ okapi-testdata/okf_table/roundtrip/
cp -r integration-tests/okapi/src/test/resources/table/two_coulmns/ okapi-testdata/okf_table/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/table`

Build tag: `//go:build integration`

#### table_test.go - Extraction Tests

```go
func TestExtract_BasicTable(t *testing.T) {
    // Table-driven: maps 1:1 to Java TableFilterTest
    tests := []struct {
        name       string
        input      string // testdata path
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // testFileEvents, testFileEvents2, testColumnDefined*, etc.
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_TrimMode(t *testing.T) {
    // Maps to Java TableFilterTest#testTrimMode
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // trim mode configurations
    }
}

func TestConfig_ColumnDefinedLocales(t *testing.T) {
    // Maps to Java TableFilterTest#testColumnDefinedLocales
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripTableIT
    testFiles := []string{
        "simple.csv",
        "computer_science_article.csv",
        "field_delimiter_comma.csv",
        "field_delimiter_colon.csv",
        "field_delimiter_pipe.csv",
        "field_delimiter_semicolon.csv",
        "field_delimiter_space.csv",
        "some_blank_cells.csv",
        "some_blank_columns.csv",
        "some_blank_rows.csv",
        "test2cols.csv",
        "text_qualifier_double_quote.csv",
        "text_qualifier_single_quote.csv",
    }
    knownFailing := map[string]string{
        // none known
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java TableXliffCompareIT
    // Verifies Part structure matches expected XLIFF output
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/table/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/table/
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
- The Table filter is a meta-filter: it dispatches to CSV, TSV, FWC, or BaseTable sub-filters depending on configuration
- The `okf_table` config ID acts as the default entry point; sub-filter IDs (okf_commaseparatedvalues, okf_tabseparatedvalues, okf_fixedwidthcolumns) are used for specific sub-filter configurations
- Several configurations are bundled: `okf_table_csv`, `okf_table_catkeys`, `okf_table_src-tab-trg`, `okf_table_fwc`, `okf_table_tsv`

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/table/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TableFilterTest.java` | `okapi/filters/table/src/test/java/net/sf/okapi/filters/table/` | 15 |
