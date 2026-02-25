# okf_mosestext - Moses Text Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_mosestext` |
| Java Class | `net.sf.okapi.filters.mosestext.MosesTextFilter` |
| MIME Types | `text/x-mosestext` |
| Extensions | `.txt` |
| Okapi Module | `mosestext` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/mosestext/src/test/java/`

#### MosesTextFilterTest.java (17 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Default filter metadata | P3 |
| 2 | `testStartDocument` | Start document event | P3 |
| 3 | `testLineBreaks_CR` | CR line break handling | P1 |
| 4 | `testineBreaks_CRLF` | CRLF line break handling | P1 |
| 5 | `testLineBreaks_LF` | LF line break handling | P1 |
| 6 | `testEntry` | Basic entry extraction (one line = one segment) | P1 |
| 7 | `testCode1` | Inline code pattern 1 | P1 |
| 8 | `testCode2` | Inline code pattern 2 | P1 |
| 9 | `testCode3` | Inline code pattern 3 | P1 |
| 10 | `testCode4` | Inline code pattern 4 | P1 |
| 11 | `testSpecialChars` | Special character handling | P1 |
| 12 | `testLiterals` | Literal string handling | P1 |
| 13 | `testEscapedG` | Escaped 'G' character (expects EmptyStackException) | P2 |
| 14 | `testWhiteSpaces` | Whitespace preservation | P1 |
| 15 | `testFromFile` | File-based extraction | P1 |
| 16 | `testDoubleExtraction` | Double extraction consistency | P1 |

#### MosesTextFilterWriterTest.java

Writer tests for Moses text output.

### Integration Tests

No dedicated integration tests found for okf_mosestext.

## Test Data Files

### Unit test resources

Source: `okapi/filters/mosestext/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.txt` | `MosesTextFilterTest#testFromFile`, `#testDoubleExtraction` | Basic Moses text |
| `Test02.txt` | `MosesTextFilterTest#testDoubleExtraction` | Additional test |
| `Test-XLIFF01.xlf` | `MosesTextFilterTest` | XLIFF comparison |
| `Test-XLIFF02.xlf` | `MosesTextFilterTest` | XLIFF comparison |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.txt` | Minimal valid Moses text | One sentence per line |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/mosestext/src/test/resources/Test01.txt okapi-testdata/okf_mosestext/
cp okapi/filters/mosestext/src/test/resources/Test02.txt okapi-testdata/okf_mosestext/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_mosestext`

Build tag: `//go:build integration`

#### mosestext_test.go - Extraction Tests

```go
func TestExtract_entries(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "basic entry", javaRef: "MosesTextFilterTest#testEntry"},
        {name: "inline code patterns", javaRef: "MosesTextFilterTest#testCode1"},
        {name: "special chars", javaRef: "MosesTextFilterTest#testSpecialChars"},
        {name: "whitespace preservation", javaRef: "MosesTextFilterTest#testWhiteSpaces"},
    }
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_mosestext/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_mosestext/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - One line = one text unit (Moses SMT plain text format)
  - Handles all three line break styles: CR, CRLF, LF
  - Inline codes for XML-like tags and placeholders
  - Simple format with minimal configuration
  - No dedicated integration roundtrip tests

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/mosestext/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `MosesTextFilterTest.java` | `okapi/filters/mosestext/src/test/java/net/sf/okapi/filters/mosestext/` | 17 |
| `MosesTextFilterWriterTest.java` | `okapi/filters/mosestext/src/test/java/net/sf/okapi/filters/mosestext/` | - |
