# okf_multiparsers - Multi Parsers Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_multiparsers` |
| Java Class | `net.sf.okapi.filters.multiparsers.MultiParsersFilter` |
| MIME Types | `application/x-multiparsers` |
| Extensions | `.csv` |
| Okapi Module | `multiparsers` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/multiparsers/src/test/java/`

#### MultiparsersFilterTests.java (6 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSimpleRead` | Reads test01.csv with csvNoExtractCols="0,3", csvFormatCols="2:okf_html,5:okf_markdown", csvStartingRow=2; verifies 8 TUs extracted from records 2-3, columns 1,2,4,5 | P1 |
| 2 | `testSubFilterContent` | Reads test02.csv with sub-filter columns (HTML, Markdown), verifies HTML inline codes are extracted as letter-coded fragments (e.g., `<g1>text</g1><x3/>`) and last entry is "last-Body" | P1 |
| 3 | `testReadWrite` | Reads test02.csv, writes with uppercased targets, verifies output file is created | P1 |
| 4 | `testTwoSubFilterContent` | Reads test03.csv with both columns as sub-filters (0:okf_markdown, 1:okf_html), verifies 3 TUs with correct inline codes from Markdown bold and HTML bold | P2 |
| 5 | `preProcessingForMarkdownTest` | Unit tests the preProcessDataForMarkdown helper: verifies list items followed by indented continuation get `\r\n` + marker injection for proper Markdown parsing | P2 |
| 6 | `autoDetectColumnTypesTest` | Reads test04.csv with csvAutoDetectColumnTypes=true, csvAutoDetectColumnTypesRow=2; verifies 6 TUs with auto-detected plain/html/markdown columns | P1 |

### Integration Tests

No dedicated integration tests found for okf_multiparsers.

## Test Data Files

### Unit test resources

Source: `okapi/filters/multiparsers/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test01.csv` | `testSimpleRead` | CSV with 6 columns, 3 rows; columns 0,3 non-extractable, columns 2,5 have HTML/Markdown sub-filter content |
| `test02.csv` | `testSubFilterContent`, `testReadWrite` | CSV with HTML and Markdown sub-filter columns, inline formatting codes |
| `test03.csv` | `testTwoSubFilterContent` | CSV with two sub-filter columns (Markdown col 0, HTML col 1) |
| `test04.csv` | `autoDetectColumnTypesTest` | CSV with auto-detect row specifying column types |

### Integration test resources

None.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `simple-plaintext.csv` | Minimal CSV with all plain-text columns | Smoke test without sub-filters |
| `mixed-encodings.csv` | CSV with UTF-8 special characters | Verify encoding handling |
| `large-columns.csv` | CSV with many columns | Test scalability of column configuration |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/multiparsers/src/test/resources/test01.csv okapi-testdata/okf_multiparsers/
cp okapi/filters/multiparsers/src/test/resources/test02.csv okapi-testdata/okf_multiparsers/
cp okapi/filters/multiparsers/src/test/resources/test03.csv okapi-testdata/okf_multiparsers/
cp okapi/filters/multiparsers/src/test/resources/test04.csv okapi-testdata/okf_multiparsers/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/multiparsers`

Build tag: `//go:build integration`

#### multiparsers_test.go - Extraction Tests

```go
func TestExtract_csv(t *testing.T) {
    // Table-driven: maps 1:1 to Java MultiparsersFilterTests
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:       "simple read with excluded columns",
            input:      "testdata/test01.csv",
            wantBlocks: 8,
            wantTexts:  []string{"ent1-2", "ent2-2", "ent3-2", "ent4-2", "ent1-3", "ent2-3", "ent3-3", "ent4-3"},
            params:     map[string]any{
                "csvNoExtractCols": "0,3",
                "csvFormatCols":    "2:okf_html,5:okf_markdown",
                "csvStartingRow":   2,
            },
            javaRef:    "MultiparsersFilterTests#testSimpleRead",
        },
        {
            name:       "sub-filter content extraction",
            input:      "testdata/test02.csv",
            params:     map[string]any{
                "csvNoExtractCols": "0,3",
                "csvFormatCols":    "2:okf_html,5:okf_markdown",
            },
            javaRef:    "MultiparsersFilterTests#testSubFilterContent",
        },
        {
            name:       "auto-detect column types",
            input:      "testdata/test04.csv",
            wantBlocks: 6,
            wantTexts:  []string{"some text", "some text2"},
            params:     map[string]any{
                "csvAutoDetectColumnTypes":    true,
                "csvAutoDetectColumnTypesRow": 2,
            },
            javaRef:    "MultiparsersFilterTests#autoDetectColumnTypesTest",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_csvParameters(t *testing.T) {
    // Maps to schema properties: csvNoExtractCols, csvFormatCols, csvStartingRow,
    // csvAutoDetectColumnTypes, csvAutoDetectColumnTypesRow
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "exclude columns",
            params: map[string]any{"csvNoExtractCols": "0,3"},
            input:  "testdata/test01.csv",
            javaRef: "MultiparsersFilterTests#testSimpleRead",
        },
        {
            name:   "sub-filter columns HTML and Markdown",
            params: map[string]any{"csvFormatCols": "2:okf_html,5:okf_markdown"},
            input:  "testdata/test02.csv",
            javaRef: "MultiparsersFilterTests#testSubFilterContent",
        },
        {
            name:   "two sub-filter columns",
            params: map[string]any{"csvFormatCols": "0:okf_markdown,1:okf_html"},
            input:  "testdata/test03.csv",
            want:   []string{"Text bold and more", "HTML bold and more", "Plain text R&D"},
            javaRef: "MultiparsersFilterTests#testTwoSubFilterContent",
        },
        {
            name:   "starting row offset",
            params: map[string]any{
                "csvNoExtractCols": "0,3",
                "csvFormatCols":    "2:okf_html,5:okf_markdown",
                "csvStartingRow":   2,
            },
            input:  "testdata/test01.csv",
            javaRef: "MultiparsersFilterTests#testSimpleRead",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java MultiparsersFilterTests#testReadWrite
    testFiles := []string{
        "test01.csv",
        "test02.csv",
        "test03.csv",
        "test04.csv",
    }
    knownFailing := map[string]string{
        // none known
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/multiparsers/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/multiparsers/
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
- Filter-specific quirks:
  - This filter processes CSV files with per-column sub-filter delegation
  - Requires FilterConfigurationMapper with sub-filter configs (okf_html, okf_markdown) registered
  - `csvNoExtractCols`: comma-separated column indices to skip (0-based)
  - `csvFormatCols`: comma-separated `index:configId` pairs for sub-filter columns
  - `csvStartingRow`: 1-based row to start extraction (skips header rows)
  - `csvAutoDetectColumnTypes`: when true, reads a special row to auto-detect column types
  - `csvAutoDetectColumnTypesRow`: 1-based row number containing type hints
  - The `preProcessDataForMarkdown` method injects markers for proper Markdown list parsing
  - Sub-filter TU IDs follow the pattern `tu{N}_sf{M}_tu{K}` (e.g., "tu4_sf2_tu1")
  - Sub-filtered content produces inline codes (g1, x3, etc.) in extracted text fragments
  - Bridge must ensure sub-filter configurations are available for proper delegation

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/multiparsers/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `MultiparsersFilterTests.java` | `okapi/filters/multiparsers/src/test/java/.../` | 6 |
