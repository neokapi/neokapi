# okf_pensieve - Pensieve TM Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_pensieve` |
| Java Class | `net.sf.okapi.filters.pensieve.PensieveFilter` |
| MIME Types | `application/x-pensieve-tm` |
| Extensions | `.pentm` |
| Okapi Module | `pensieve` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/pensieve/src/test/java/`

No unit tests found in the pensieve filter module.

### Integration Tests

Tests are located in the dedicated `integration-tests/pensieve/` module, not in the filter module itself.

#### TmxHandlerImportIT.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `importTmx_paragraph_tmx_basics` | Imports Paragraph_TM.tmx into Pensieve index (EN-US/DE-DE), searches for exact match "Pumps have been paused for 3 minutes...", verifies German translation | P1 |
| 2 | `importTmx_sample_tmx_basics` | Imports sample_tmx.xml (EN/IT), searches for "hello" and "world" exact matches, verifies Italian translations "ciao" and "mondo" | P1 |
| 3 | `importTmx_sample_metadata` | Imports sample_tmx.xml and verifies metadata: TU ID ("hello123"), FileName, GroupName, Type are preserved in the index | P2 |

#### TmxHandlerExportIT.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `exportTmx_sample_metadata` | Imports sample TMX, then exports it back to TMX format via PensieveSeeker, verifies full TMX output XML including header, TU IDs, props, and translations | P1 |

#### TmStepsIT.java (13 @Test methods)

Note: These tests are pipeline/step integration tests that use the Pensieve TM connector, not the PensieveFilter directly. They test leveraging, word counting, and scoping report features using a pre-built Pensieve TM index.

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testGMXExactMatchedWordCountStep` | GMX exact matched word count (3 words for "Elephants cannot fly.") using leveraging step with Pensieve connector | P3 |
| 2 | `testGMXExactMatchedCharacterCountStep` | GMX exact matched character count (18 chars) using leveraging | P3 |
| 3 | `testGMXLeveragedMatchedWordCountStep` | GMX leveraged matched word count using fuzzy threshold 95 | P3 |
| 4 | `testGMXLeveragedMatchedCharacterCountStep` | GMX leveraged matched character count with fuzzy threshold | P3 |
| 5 | `testGMXFuzzyMatchWordCountStep` | GMX fuzzy match word count step | P3 |
| 6 | `testGMXFuzzyMatchCharacterCountStep` | GMX fuzzy match character count step | P3 |
| 7 | `testLeveraging` | Full leveraging pipeline: segment -> leverage -> verify alt translations and match types | P2 |
| 8 | `test_a_word_is_counted_only_once` | Verifies word counting pipeline counts each word exactly once with Pensieve leveraging | P3 |
| 9 | `test_a_word_is_counted_only_once2` | Extended word counting verification with different pipeline configuration | P3 |
| 10 | `total_counts_should_be_greater_or_equal_to_the_sum_of_categories_in_every_group` | Scoping report: total >= sum of categories per group | P3 |
| 11 | `total_counts_should_be_equal_to_the_sum_of_categories_and_nocategory` | Scoping report: total = categories + no-category | P3 |
| 12 | `total_counts_should_be_equal_to_the_sum_of_translatable_and_nontranslatable` | Scoping report: total = translatable + non-translatable | P3 |
| 13 | `testFields` | Scoping report field validation | P3 |

## Test Data Files

### Unit test resources

No unit test resources (no unit tests exist for this filter).

### Integration test resources

Source: `integration-tests/pensieve/src/test/resources/`

| File | Type | Purpose |
|------|------|---------|
| `Paragraph_TM.tmx` | import test | EN-US/DE-DE paragraph-level TMX for import testing |
| `sample_tmx.xml` | import/export | Simple EN/IT TMX with metadata (tuid, props) |
| `net/sf/okapi/tm/pensieve/tmx/test.txt` | scoping | Plain text input for scoping report tests |
| `net/sf/okapi/tm/pensieve/tmx/default.srx` | config | Segmentation rules for pipeline tests |
| `net/sf/okapi/tm/pensieve/tmx/golden_file_template.txt` | gold | Scoping report expected output template |
| `net/sf/okapi/tm/pensieve/tmx/golden_file_template2.txt` | gold | Second scoping report expected output template |
| `net/sf/okapi/tm/pensieve/tmx/gold/test_scoping_report4.txt` | gold | Expected scoping report 4 |
| `net/sf/okapi/tm/pensieve/tmx/gold/test_scoping_report5.txt` | gold | Expected scoping report 5 |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.pentm` | Minimal valid Pensieve TM directory for smoke test | Lucene index directory with at least 1 translation unit |
| `sample.tmx` | TMX file for import/export roundtrip | Based on sample_tmx.xml |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Integration test resources
cp integration-tests/pensieve/src/test/resources/Paragraph_TM.tmx okapi-testdata/okf_pensieve/
cp integration-tests/pensieve/src/test/resources/sample_tmx.xml okapi-testdata/okf_pensieve/
cp integration-tests/pensieve/src/test/resources/net/sf/okapi/tm/pensieve/tmx/test.txt okapi-testdata/okf_pensieve/
cp integration-tests/pensieve/src/test/resources/net/sf/okapi/tm/pensieve/tmx/default.srx okapi-testdata/okf_pensieve/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/pensieve`

Build tag: `//go:build integration`

#### pensieve_test.go - Extraction Tests

```go
func TestExtract_pensieveTM(t *testing.T) {
    // Synthetic test - Java tests focus on TM import/export, not filter extraction
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:    "minimal pensieve TM",
            input:   "testdata/minimal.pentm",
            javaRef: "synthetic - no direct Java filter test",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Synthetic - Java integration tests focus on TMX import/export
    // through the Pensieve TM engine, not filter roundtrip
    testFiles := []string{
        // Pensieve TM directories need special handling
    }
    knownFailing := map[string]string{
        // TBD - Pensieve is a Lucene-based TM, not a simple file filter
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/pensieve/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/pensieve/
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
- Filter-specific quirks:
  - Pensieve is Okapi's built-in translation memory engine based on Apache Lucene
  - The PensieveFilter reads from a Lucene index directory, not a single file
  - Schema has no configurable parameters (empty properties object)
  - No unit tests exist for the filter itself; all tests are in the integration-tests module
  - The TmStepsIT tests are primarily about pipeline steps (leveraging, word counting, scoping reports) that use Pensieve as a TM backend -- these are P3 and not directly relevant to filter migration
  - The TmxHandlerImportIT and TmxHandlerExportIT tests verify TMX round-tripping through the Pensieve index
  - Creating test data requires building a Lucene index directory
  - Consider whether this filter is even useful via the bridge, since it requires a local Lucene index

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "integration-tests/pensieve/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TmStepsIT.java` | `integration-tests/pensieve/src/test/java/.../tmx/` | 13 |
| `TmxHandlerExportIT.java` | `integration-tests/pensieve/src/test/java/.../tmx/` | 1 |
| `TmxHandlerImportIT.java` | `integration-tests/pensieve/src/test/java/.../tmx/` | 3 |
