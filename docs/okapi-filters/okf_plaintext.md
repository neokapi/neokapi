# okf_plaintext - Plain Text Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_plaintext` |
| Java Class | `net.sf.okapi.filters.plaintext.PlainTextFilter` |
| MIME Types | `text/plain` |
| Extensions | `.txt` |
| Okapi Module | `plaintext` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/plaintext/src/test/java/`

#### PlainTextFilterTest.java (15 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testEmptyInput` | Empty input produces no text units | P1 |
| 2 | `testNameAndMimeType` | Filter name and MIME type correct | P3 |
| 3 | `testFiles` | Various text files parsed without error (different line endings) | P1 |
| 4 | `testSkeleton` | Skeleton preserves line breaks between text units | P1 |
| 5 | `testSkeleton2` | Skeleton with multiple consecutive line breaks | P1 |
| 6 | `testSkeleton3` | Skeleton with mixed content | P1 |
| 7 | `testEvents` | Correct event sequence: START_DOCUMENT, TEXT_UNIT(s), END_DOCUMENT | P1 |
| 8 | `testStartDocument` | StartDocument event properties | P1 |
| 9 | `testDoubleExtraction` | Double extraction roundtrip for all test files | P1 |
| 10 | `testCancel` | Filter cancellation support | P3 |
| 11 | `testConfigurations` | Available filter configurations | P3 |
| 12 | `testSynchronization` | Multi-threaded filter access | P3 |
| 13 | `testLineNumbers` | Line numbers tracked in extracted text units | P2 |
| 14 | `testParagraphs` | Paragraph mode: consecutive lines joined, blank lines split | P1 |
| 15 | `testLoadParams` | Parameter loading from file | P3 |

#### ParaPlainTextFilterTest.java (test count varies)

Tests paragraph-mode plain text filter.

#### RegexPlainTextFilterTest.java (test count varies)

Tests regex-based plain text extraction.

#### SplicedLinesFilterTest.java (test count varies)

Tests spliced/continuation line handling.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripPlainTextIT` | `integration-tests/okapi/src/test/java/.../RoundTripPlainTextIT.java` | 2 |

**Test files used**: 26 files in `integration-tests/okapi/src/test/resources/plaintext/`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `PlainTextXliffCompareIT` | `integration-tests/okapi/src/test/java/.../PlainTextXliffCompareIT.java` | 1 |

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `PlainTextMemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../PlainTextMemoryLeakTestIT.java` | 1 (main method) |

## Test Data Files

### Unit test resources

Source: `okapi/filters/plaintext/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `crlf.txt` | `testFiles` | CRLF line endings |
| `crlf_end.txt` | `testFiles` | CRLF with trailing newline |
| `crlf_start.txt` | `testFiles` | CRLF with leading newline |
| `crlfcrlf.txt` | `testFiles` | Double CRLF |
| `crlfcrlf_end.txt` | `testFiles` | Double CRLF at end |
| `crlfcrlfcrlf.txt` | `testFiles` | Triple CRLF |
| `crlfcrlfcrlf_end.txt` | `testFiles` | Triple CRLF at end |
| `cr.txt` | `testFiles` | CR-only line endings |
| `lf.txt` | `testFiles` | LF-only line endings |
| `mixture.txt` | `testFiles` | Mixed line endings |
| `u0085.txt` | `testFiles` | Unicode NEL character |
| `u2028.txt` | `testFiles` | Unicode LINE SEPARATOR |
| `u2029.txt` | `testFiles` | Unicode PARAGRAPH SEPARATOR |
| `al2.txt` | `testFiles` | Additional line ending test |
| `combined_lines.txt` / `combined_lines2.txt` | `testFiles` | Combined line formats |
| `combined_lines_end.txt` | `testFiles` | Combined lines at end |
| `csv_test1.txt` / `csv_test2.txt` | `testFiles` | CSV-like text |
| `test_params1.txt` / `test_params1.fprm` | `testLoadParams` | Parameter test |
| `test_params2.txt` / `test_params2.fprm` | `testLoadParams` | Parameter test 2 |
| `test_paragraphs1.txt` | `testParagraphs` | Paragraph mode test |
| `BOM_MacUTF16withBOM2.txt` | `testFiles` | UTF-16 BOM |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/plaintext/`

26 files with various line ending formats and parameter configurations.

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/plaintext/src/test/resources/*.txt okapi-testdata/okf_plaintext/
cp okapi/filters/plaintext/src/test/resources/*.fprm okapi-testdata/okf_plaintext/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/plaintext/* okapi-testdata/okf_plaintext/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_plaintext`

Build tag: `//go:build integration`

#### plaintext_test.go - Extraction Tests

```go
func TestExtract_BasicLines(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "single line",
            input: "Hello World",
            wantTexts: []string{"Hello World"},
            javaRef: "PlainTextFilterTest#testEvents",
        },
        // ... additional test cases
    }
}

func TestExtract_LineEndings(t *testing.T) {
    // Maps to PlainTextFilterTest line break tests
}

func TestExtract_Paragraphs(t *testing.T) {
    // Maps to PlainTextFilterTest#testParagraphs
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_plaintext/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_plaintext/
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
- Filter-specific quirks:
  - One text unit per line by default
  - Paragraph mode: blank lines split paragraphs, consecutive non-blank lines joined
  - Supports multiple line ending formats: LF, CRLF, CR, NEL (U+0085), LS (U+2028), PS (U+2029)
  - BOM detection for UTF-8 and UTF-16
  - Regex-based extraction mode available (RegexPlainTextFilter variant)
  - Spliced lines mode joins lines ending with backslash
  - Very simple filter: no inline codes, no configuration-based element rules
  - Line numbers can be tracked in extracted text units

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/plaintext/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `PlainTextFilterTest.java` | `okapi/filters/plaintext/src/test/java/.../` | 15 |
| `ParaPlainTextFilterTest.java` | `okapi/filters/plaintext/src/test/java/.../` | varies |
| `RegexPlainTextFilterTest.java` | `okapi/filters/plaintext/src/test/java/.../` | varies |
| `SplicedLinesFilterTest.java` | `okapi/filters/plaintext/src/test/java/.../` | varies |
