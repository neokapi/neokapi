# okf_commaseparatedvalues - CSV Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_commaseparatedvalues` |
| Java Class | `net.sf.okapi.filters.table.csv.CommaSeparatedValuesFilter` |
| MIME Types | `text/csv` |
| Extensions | `.csv` |
| Okapi Module | `table` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/table/src/test/java/`

#### CommaSeparatedValuesFilterTest.java (56 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testCatkeys` | Haiku CatKeys resource file extraction | P1 |
| 2 | `testThreeColumnsSrcTrgData` | 3-column source/target/data extraction | P1 |
| 3 | `testThreeColumnsSrcDataWithSubfilter` | 3-column extraction with sub-filter processing | P1 |
| 4 | `testThreeColumnsExtractAllWithSubfilter` | Extract all columns with sub-filter | P2 |
| 5 | `testSubfilterTuIds` | TU IDs when using sub-filters | P2 |
| 6 | `testThreeColumnsSrcTrgData_2` | Second 3-column variant | P1 |
| 7 | `testThreeColumnsSrcTrgData_3` | Third 3-column variant | P1 |
| 8 | `testFileEvents96_2` | Event sequence for issue 96 variant 2 | P2 |
| 9 | `testFileEvents96_3` | Event sequence for issue 96 variant 3 | P2 |
| 10 | `testSkeletonWriter` | Skeleton writer output | P1 |
| 11 | `testEscapeQualifiersDoubleQuotes` | Double-quote text qualifier escaping | P1 |
| 12 | `testEscapeQualifiersBackslash` | Backslash text qualifier escaping | P1 |
| 13 | `testEscapeQualifiersInUnqualifiedFields` | Qualifier escaping in unqualified fields | P2 |
| 14 | `testEmptyInput` | Empty input handling | P1 |
| 15 | `testNameAndMimeType` | Filter name and MIME type correctness | P2 |
| 16 | `testParameters` | Parameter loading and serialization | P2 |
| 17 | `testSkeleton` | Skeleton output for basic CSV | P1 |
| 18 | `testSkeleton2` | Second skeleton test variant | P1 |
| 19 | `testSkeleton3` | Third skeleton test variant | P1 |
| 20 | `testFileEvents` | Basic event sequence | P1 |
| 21 | `testFileEvents106` | Event sequence for issue 106 | P2 |
| 22 | `testFileEvents106_2` | Issue 106 variant 2 | P2 |
| 23 | `testFileEvents2` | Second event sequence test | P1 |
| 24 | `testFileEvents2a` | Event sequence variant 2a | P2 |
| 25 | `testFileEvents3` | Third event sequence test | P1 |
| 26 | `testFileEvents6` | Sixth event sequence test | P2 |
| 27 | `testFileEvents7` | Seventh event sequence test | P2 |
| 28 | `testFileEvents8` | Eighth event sequence test | P2 |
| 29 | `testFileEvents4` | Fourth event sequence test | P2 |
| 30 | `testFileEvents5` | Fifth event sequence test | P2 |
| 31 | `testFileEvents106_3` | Issue 106 variant 3 | P2 |
| 32 | `testFileEvents106_4` | Issue 106 variant 4 | P2 |
| 33 | `testFileEvents118` | Event sequence for issue 118 | P2 |
| 34 | `testFileEvents118_2` | Issue 118 variant 2 | P2 |
| 35 | `testFileEvents96` | Event sequence for issue 96 | P2 |
| 36 | `testFileEvents97` | Event sequence for issue 97 | P2 |
| 37 | `testQualifiedValues` | Text-qualified value handling | P1 |
| 38 | `testQualifiedValues2` | Second qualified values test | P1 |
| 39 | `testDoubleExtraction` | Extract-then-extract idempotency (Ignored) | P3 |
| 40 | `testRecordId` | Record ID column extraction | P2 |
| 41 | `testSourceId` | Source ID column extraction | P2 |
| 42 | `testEmptySourceId` | Empty source ID handling | P2 |
| 43 | `testTabDelimited2Column` | Tab-delimited 2-column extraction | P1 |
| 44 | `testTabDelimited2ColumnRoundTrip` | Tab-delimited 2-column roundtrip | P1 |
| 45 | `testEmptyLinesInCell` | Empty lines within CSV cells | P2 |
| 46 | `testAddTextQualifiers` | Adding text qualifiers to output | P2 |
| 47 | `testAddTextQualifiersForQualifiers` | Adding qualifiers when source has qualifiers | P2 |
| 48 | `testEmptyQualifiersWithoutSourceQualifiers` | Empty qualifier handling without source qualifiers | P2 |
| 49 | `testEmptyQualifiersWithSourceQualifiers` | Empty qualifier handling with source qualifiers | P2 |
| 50 | `testEmptyQualifiersWithSourceQualifiersAddQualifiers` | Empty qualifiers with add-qualifiers flag | P2 |
| 51 | `testUnqualifiedTargetWithSourceQualifiers` | Unqualified target when source has qualifiers | P2 |
| 52 | `testDontEscapeUnremovedQualifiers` | No escaping when qualifiers not removed | P2 |
| 53 | `testDoEscapeRemovedQualifiers` | Escaping when qualifiers are removed | P2 |
| 54 | `testTrgAtCol4_Issue511` | Target at column 4 (issue 511) | P2 |
| 55 | `testIssue_1153` | Regression fix for issue 1153 | P2 |
| 56 | `testCommentColumnsAsMetadata` | Comment columns exposed as metadata | P2 |

### Integration Tests

Integration tests are shared with the `okf_table` filter (see `okf_table.md`).

## Test Data Files

### Unit test resources

Source: `okapi/filters/table/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `csv_test1.txt` - `csv_testf.txt` | Various `testFileEvents*` methods | CSV data with different layouts |
| `csv_testg.txt` - `csv_testg5.txt` | `testFileEvents118*` | Issue 118 test files |
| `csv_testh.txt` | `testCommentColumnsAsMetadata` | Comment column test data |
| `CSVTest_96.txt`, `CSVTest_96_2.txt` | `testFileEvents96*` | Issue 96 test data |
| `CSVTest_97.txt` | `testFileEvents97` | Issue 97 test data |
| `CSVTesting01.csv` | `testParameters` | Parameter loading test |
| `test01.catkeys` | `testCatkeys` | Haiku CatKeys resource |
| `test2cols.csv`, `test2cols2.csv` | `testTabDelimited2Column*` | 2-column tab-delimited |
| `testContent_escaped.csv` - `testContent_escaped4.csv` | `testEscapeQualifiers*` | Qualifier escaping tests |
| `test_tsv_simple.txt` | `testTrgAtCol4_Issue511` | TSV simple test |
| `test_params1.txt` - `test_params4.txt` | `testParameters` | Parameter files |
| `okf_table@*.fprm` | Various tests | Filter parameter configurations |

### Integration test resources

See `okf_table.md` for shared table integration test resources.

### Synthetic test data to create

None needed -- ample test files exist in the Okapi test suite.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/table/src/test/resources/csv_test*.txt okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/CSVTest_*.txt okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/CSVTesting01.csv okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/test01.catkeys okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/test2cols.csv okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/test2cols2.csv okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/testContent_escaped*.csv okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/test_tsv_simple.txt okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/test_params*.txt okapi-testdata/okf_commaseparatedvalues/
cp okapi/filters/table/src/test/resources/okf_table@*.fprm okapi-testdata/okf_commaseparatedvalues/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/commaseparatedvalues`

Build tag: `//go:build integration`

#### csv_test.go - Extraction Tests

```go
func TestExtract_BasicCSV(t *testing.T) {
    // Table-driven: maps 1:1 to Java CommaSeparatedValuesFilterTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // testCatkeys, testThreeColumns*, testFileEvents, etc.
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_TextQualifiers(t *testing.T) {
    // Maps to Java CommaSeparatedValuesFilterTest#testEscapeQualifiers*
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // qualifier escaping configurations
    }
}

func TestConfig_ColumnMapping(t *testing.T) {
    // Maps to Java testRecordId, testSourceId, testTrgAtCol4_Issue511
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Shares roundtrip test files with okf_table
    // Maps to Java RoundTripTableIT (CSV files)
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/commaseparatedvalues/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/commaseparatedvalues/
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
- This is the most extensively tested sub-filter in the table family (56 tests)
- The `testDoubleExtraction` test is `@Ignored` in Java due to property type differences
- Parameters include: `fieldDelimiter`, `textQualifier`, `removeQualifiers`, `escapingMode`, `addQualifiers`, column mapping fields (`sourceColumns`, `targetColumns`, `commentColumns`, etc.)
- CatKeys format uses tab delimiter with a special non-matching text qualifier string

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/table/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `CommaSeparatedValuesFilterTest.java` | `okapi/filters/table/src/test/java/net/sf/okapi/filters/table/` | 56 |
