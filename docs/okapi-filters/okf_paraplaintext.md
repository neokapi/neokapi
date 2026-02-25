# okf_paraplaintext - Paragraph Plain Text Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_paraplaintext` |
| Java Class | `net.sf.okapi.filters.plaintext.paragraphs.ParaPlainTextFilter` |
| MIME Types | `text/plain` |
| Extensions | - |
| Okapi Module | `plaintext` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/plaintext/src/test/java/`

#### ParaPlainTextFilterTest.java (16 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testEmptyInput` | Empty input handling | P1 |
| 2 | `testNameAndMimeType` | Filter name and MIME type correctness | P2 |
| 3 | `testFiles` | File-based extraction | P1 |
| 4 | `testFiles2` | Second file-based extraction | P1 |
| 5 | `testSkeleton` | Skeleton writer output | P1 |
| 6 | `testSkeleton2` | Second skeleton test | P1 |
| 7 | `testSkeleton3` | Third skeleton test | P1 |
| 8 | `testSkeleton4` | Fourth skeleton test | P1 |
| 9 | `testSkeleton5` | Fifth skeleton test | P1 |
| 10 | `testEvents` | Event sequence correctness | P1 |
| 11 | `testDoubleExtraction` | Extract-then-extract idempotency | P1 |
| 12 | `testDoubleExtraction2` | Second double extraction variant | P1 |
| 13 | `testDoubleExtraction3` | Third double extraction variant | P1 |
| 14 | `testCancel` | Filter cancellation handling | P3 |
| 15 | `testLineNumbers` | Line number tracking in events | P2 |
| 16 | `testParagraphs` | Paragraph-mode extraction (multiple lines as one TU) | P1 |

### Integration Tests

Integration tests are shared with the plaintext filter family (see `okf_baseplaintext.md`).

## Test Data Files

### Unit test resources

Source: `okapi/filters/plaintext/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test_paragraphs1.txt` | `testParagraphs` | Paragraph extraction test data |
| `cr.txt` | `testFiles`, `testSkeleton*` | CR line endings |
| `crlf.txt` | `testFiles`, `testSkeleton*` | CRLF line endings |
| `lf.txt` | `testFiles`, `testSkeleton*` | LF line endings |
| `mixture.txt` | `testFiles` | Mixed line endings |
| `crlf_end.txt`, `crlf_start.txt` | `testSkeleton*` | Edge cases |
| `crlfcrlf.txt`, `crlfcrlf_end.txt` | `testSkeleton*` | Double blank lines |
| `crlfcrlfcrlf.txt`, `crlfcrlfcrlf_end.txt` | `testSkeleton*` | Triple blank lines |

### Synthetic test data to create

None needed -- sufficient test files exist.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/plaintext/src/test/resources/test_paragraphs1.txt okapi-testdata/okf_paraplaintext/
cp okapi/filters/plaintext/src/test/resources/cr.txt okapi-testdata/okf_paraplaintext/
cp okapi/filters/plaintext/src/test/resources/crlf.txt okapi-testdata/okf_paraplaintext/
cp okapi/filters/plaintext/src/test/resources/lf.txt okapi-testdata/okf_paraplaintext/
cp okapi/filters/plaintext/src/test/resources/mixture.txt okapi-testdata/okf_paraplaintext/
cp okapi/filters/plaintext/src/test/resources/crlf_end.txt okapi-testdata/okf_paraplaintext/
cp okapi/filters/plaintext/src/test/resources/crlf_start.txt okapi-testdata/okf_paraplaintext/
cp okapi/filters/plaintext/src/test/resources/crlfcrlf*.txt okapi-testdata/okf_paraplaintext/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_paraplaintext`

Build tag: `//go:build integration`

#### paraplaintext_test.go - Extraction Tests

```go
func TestExtract_ParagraphPlainText(t *testing.T) {
    // Table-driven: maps 1:1 to Java ParaPlainTextFilterTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // testEvents, testFiles, testParagraphs, etc.
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_ParagraphMode(t *testing.T) {
    // Maps to Java ParaPlainTextFilterTest#testParagraphs
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // extractParagraphs=true vs false
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Shares roundtrip infrastructure with okf_baseplaintext
    testFiles := []string{
        "cr.txt", "crlf.txt", "lf.txt", "mixture.txt",
        "test_paragraphs1.txt",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_paraplaintext/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_paraplaintext/
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
- Key difference from BasePlainTextFilter: supports paragraph mode where text separated by blank lines is extracted as single TUs
- Unique parameter: `extractParagraphs` (boolean, default true)
- Configurations: `okf_plaintext_paragraphs` (default, paragraph mode), `okf_plaintext_paragraphs_lines` (line mode)
- 16 tests provide good coverage of skeleton fidelity and paragraph merging

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/plaintext/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `ParaPlainTextFilterTest.java` | `okapi/filters/plaintext/src/test/java/net/sf/okapi/filters/plaintext/` | 16 |
