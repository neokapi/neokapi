# okf_splicedlines - Spliced Lines Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_splicedlines` |
| Java Class | `net.sf.okapi.filters.plaintext.spliced.SplicedLinesFilter` |
| MIME Types | `text/plain` |
| Extensions | - |
| Okapi Module | `plaintext` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/plaintext/src/test/java/`

#### SplicedLinesFilterTest.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testCombinedLines` | Lines joined by splicer character are extracted as one TU | P1 |
| 2 | `testDoubleExtraction` | Extract-then-extract-again idempotency | P1 |
| 3 | `testSkeleton` | Skeleton writer output preserves splicer characters | P1 |
| 4 | `testSkeleton2` | Second skeleton test variant | P1 |
| 5 | `testSkeleton3` | Third skeleton test variant | P1 |

### Integration Tests

No dedicated integration tests for the Spliced Lines filter. It shares the plaintext integration test infrastructure.

## Test Data Files

### Unit test resources

Source: `okapi/filters/plaintext/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `combined_lines.txt` | `testCombinedLines`, `testSkeleton*` | Lines with backslash splicer |
| `combined_lines2.txt` | `testSkeleton*` | Second combined lines variant |
| `combined_lines_end.txt` | `testSkeleton*` | Combined lines with trailing splicer |

### Synthetic test data to create

None needed -- sufficient test files exist.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/plaintext/src/test/resources/combined_lines.txt okapi-testdata/okf_splicedlines/
cp okapi/filters/plaintext/src/test/resources/combined_lines2.txt okapi-testdata/okf_splicedlines/
cp okapi/filters/plaintext/src/test/resources/combined_lines_end.txt okapi-testdata/okf_splicedlines/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/splicedlines`

Build tag: `//go:build integration`

#### splicedlines_test.go - Extraction Tests

```go
func TestExtract_SplicedLines(t *testing.T) {
    // Table-driven: maps 1:1 to Java SplicedLinesFilterTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // testCombinedLines
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_SplicerCharacter(t *testing.T) {
    // Tests different splicer characters
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // backslash, underscore, custom splicer
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "combined_lines.txt",
        "combined_lines2.txt",
        "combined_lines_end.txt",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/splicedlines/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/splicedlines/
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
- Key concept: lines ending with a splicer character (e.g., backslash) are joined with the next line and extracted as a single TU
- Unique parameters: `splicer` (character used to join lines), `createPlaceholders` (whether to create inline codes for the splicer+linebreak)
- Configurations: `okf_plaintext_spliced` (default), `okf_plaintext_spliced_backslash`, `okf_plaintext_spliced_underscore`, `okf_plaintext_spliced_custom`
- Only 5 tests, but they cover the core functionality well (combined lines + skeleton fidelity)

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/plaintext/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `SplicedLinesFilterTest.java` | `okapi/filters/plaintext/src/test/java/net/sf/okapi/filters/plaintext/` | 5 |
