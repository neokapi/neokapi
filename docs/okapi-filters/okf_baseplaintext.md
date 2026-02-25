# okf_baseplaintext - Base Plain Text Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_baseplaintext` |
| Java Class | `net.sf.okapi.filters.plaintext.base.BasePlainTextFilter` |
| MIME Types | `text/plain` |
| Extensions | `.txt` |
| Okapi Module | `plaintext` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/plaintext/src/test/java/`

The BasePlainTextFilter is tested via `PlainTextFilterTest.java` which is the primary test class for the plain text filter family. The `okf_baseplaintext` filter ID maps to the same underlying `BasePlainTextFilter` class that `okf_plaintext` uses (already documented in Phase 1).

#### PlainTextFilterTest.java (shared, 15 @Test methods)

See the `okf_plaintext` filter documentation for the complete test inventory. The BasePlainTextFilter is the implementation class behind both `okf_plaintext` and `okf_baseplaintext` filter IDs.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripPlainTextIT` | `integration-tests/okapi/src/test/java/.../RoundTripPlainTextIT.java` | 2 |

**Known failing files**: `BOM_MacUTF16withBOM2.txt`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `PlainTextXliffCompareIT` | `integration-tests/okapi/src/test/java/.../PlainTextXliffCompareIT.java` | 1 |

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `PlainTextMemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../PlainTextMemoryLeakTestIT.java` | inherited |

## Test Data Files

### Unit test resources

Source: `okapi/filters/plaintext/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `al2.txt` | `PlainTextFilterTest` | Text extraction test |
| `cr.txt` | `PlainTextFilterTest` | CR line ending |
| `crlf.txt` | `PlainTextFilterTest` | CRLF line ending |
| `lf.txt` | `PlainTextFilterTest` | LF line ending |
| `mixture.txt` | `PlainTextFilterTest` | Mixed line endings |
| `BOM_MacUTF16withBOM2.txt` | `PlainTextFilterTest` | BOM handling |
| `u0085.txt`, `u2028.txt`, `u2029.txt` | `PlainTextFilterTest` | Unicode line separator handling |
| `test_params1.fprm`, `test_params2.fprm` | `PlainTextFilterTest` | Parameter files |
| `test_params1.txt`, `test_params2.txt` | `PlainTextFilterTest` | Parameter test data |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/plaintext/`

| File | Type | Purpose |
|------|------|---------|
| `BOM_MacUTF16withBOM2.txt` | roundtrip (known-failing) | UTF-16 BOM handling |
| `TMS-633.txt` | roundtrip | Regression test |
| `combined_lines.txt`, `combined_lines2.txt`, `combined_lines_end.txt` | roundtrip | Line combination handling |
| `cr.txt`, `crlf.txt`, `lf.txt` | roundtrip | Line ending variants |
| `crlf_end.txt`, `crlf_start.txt` | roundtrip | Edge cases |
| `crlfcrlf.txt`, `crlfcrlfcrlf.txt` | roundtrip | Multiple blank lines |
| `lgpl.txt` | roundtrip | Large real-world text |
| `mixture.txt` | roundtrip | Mixed line endings |
| `test_paragraphs1.txt` | roundtrip | Paragraph extraction |
| `u0085.txt`, `u2028.txt`, `u2029.txt` | roundtrip | Unicode line separators |
| `params1/`, `params2/` | roundtrip | Parameterized configs |

### Synthetic test data to create

None needed -- shared with okf_plaintext (Phase 1).

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/plaintext/src/test/resources/*.txt okapi-testdata/okf_baseplaintext/
cp okapi/filters/plaintext/src/test/resources/*.fprm okapi-testdata/okf_baseplaintext/

# Integration test resources
cp integration-tests/okapi/src/test/resources/plaintext/*.txt okapi-testdata/okf_baseplaintext/roundtrip/
cp -r integration-tests/okapi/src/test/resources/plaintext/params1/ okapi-testdata/okf_baseplaintext/roundtrip/
cp -r integration-tests/okapi/src/test/resources/plaintext/params2/ okapi-testdata/okf_baseplaintext/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_baseplaintext`

Build tag: `//go:build integration`

#### baseplaintext_test.go - Extraction Tests

```go
func TestExtract_BasePlainText(t *testing.T) {
    // Table-driven: shared with okf_plaintext tests
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // line-by-line extraction, trim modes, whitespace handling
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripPlainTextIT
    testFiles := []string{
        "cr.txt", "crlf.txt", "lf.txt", "mixture.txt",
        "combined_lines.txt", "combined_lines2.txt",
        "TMS-633.txt", "lgpl.txt",
    }
    knownFailing := map[string]string{
        "BOM_MacUTF16withBOM2.txt": "UTF-16 BOM handling issue",
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_baseplaintext/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_baseplaintext/
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
- `okf_baseplaintext` and `okf_plaintext` map to the same Java class (`BasePlainTextFilter`)
- This is the base class for all plaintext filter variants (paragraphs, regex, spliced)
- Configurations: `okf_plaintext` (default), `okf_plaintext_trim_trail`, `okf_plaintext_trim_all`
- Parameters: `unescapeSource`, `trimLeading`, `trimTrailing`, `preserveWS`, `useCodeFinder`, `codeFinderRules`, `wrapMode`

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/plaintext/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `PlainTextFilterTest.java` (shared) | `okapi/filters/plaintext/src/test/java/net/sf/okapi/filters/plaintext/` | 15 |
