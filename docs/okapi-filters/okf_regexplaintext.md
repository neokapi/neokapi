# okf_regexplaintext - Regex Plain Text Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_regexplaintext` |
| Java Class | `net.sf.okapi.filters.plaintext.regex.RegexPlainTextFilter` |
| MIME Types | `text/plain` |
| Extensions | - |
| Okapi Module | `plaintext` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/plaintext/src/test/java/`

#### RegexPlainTextFilterTest.java (6 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testEmptyInput` | Empty input handling | P1 |
| 2 | `testParameters` | Parameter loading and serialization for regex rule, sourceGroup, regexOptions | P2 |
| 3 | `testNameAndMimeType` | Filter name and MIME type correctness | P2 |
| 4 | `testEvents` | Event sequence using regex-based line detection | P1 |
| 5 | `testFiles` | File-based extraction with regex line breaking | P1 |
| 6 | `testDoubleExtraction` | Extract-then-extract-again idempotency | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripRegexIT` | `integration-tests/okapi/src/test/java/.../RoundTripRegexIT.java` | 2 |

Uses `okf_regex` config ID with `.txt` and `.regex` extensions.

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `RegexXliffCompareIT` | `integration-tests/okapi/src/test/java/.../RegexXliffCompareIT.java` | 1 |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyRegexIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyRegexIT.java` | 2 |

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `RegexMemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../RegexMemoryLeakTestIT.java` | inherited |

## Test Data Files

### Unit test resources

Source: `okapi/filters/plaintext/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `csv_test1.txt` | `testEvents`, `testFiles` | Regex extraction test data |
| `csv_test2.txt` | `testFiles` | Second regex test file |
| `test_params1.fprm` | `testParameters` | Regex param configuration |
| `test_params1.txt` | `testParameters` | Parameter test data |
| `test_params2.fprm` | `testParameters` | Second param config |
| `test_params2.txt` | `testParameters` | Second parameter test data |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/regex/`

| File | Type | Purpose |
|------|------|---------|
| `Test01_stringinfo_en.info` | roundtrip | String info format test |
| `dummy.foo` | roundtrip | Dummy test file |
| `meta/test.txt` | roundtrip | Metadata regex test |
| `meta/okf_regex@meta.fprm` | config | Meta regex configuration |
| `meta2/TestRules05.txt` | roundtrip | Rules test file |
| `meta2/okf_regex@TestRules05.fprm` | config | Rules configuration |
| `stringInfo/Test01_stringinfo_en.regex` | roundtrip | StringInfo format |
| `stringInfo/okf_regex@StringInfo.fprm` | config | StringInfo configuration |

### Synthetic test data to create

None needed -- sufficient test files exist.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/plaintext/src/test/resources/csv_test1.txt okapi-testdata/okf_regexplaintext/
cp okapi/filters/plaintext/src/test/resources/csv_test2.txt okapi-testdata/okf_regexplaintext/
cp okapi/filters/plaintext/src/test/resources/test_params*.fprm okapi-testdata/okf_regexplaintext/
cp okapi/filters/plaintext/src/test/resources/test_params*.txt okapi-testdata/okf_regexplaintext/

# Integration test resources
cp integration-tests/okapi/src/test/resources/regex/Test01_stringinfo_en.info okapi-testdata/okf_regexplaintext/roundtrip/
cp integration-tests/okapi/src/test/resources/regex/dummy.foo okapi-testdata/okf_regexplaintext/roundtrip/
cp -r integration-tests/okapi/src/test/resources/regex/meta/ okapi-testdata/okf_regexplaintext/roundtrip/
cp -r integration-tests/okapi/src/test/resources/regex/meta2/ okapi-testdata/okf_regexplaintext/roundtrip/
cp -r integration-tests/okapi/src/test/resources/regex/stringInfo/ okapi-testdata/okf_regexplaintext/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_regexplaintext`

Build tag: `//go:build integration`

#### regexplaintext_test.go - Extraction Tests

```go
func TestExtract_RegexPlainText(t *testing.T) {
    // Table-driven: maps 1:1 to Java RegexPlainTextFilterTest
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
        javaRef    string
    }{
        // testEvents, testFiles
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_RegexRule(t *testing.T) {
    // Maps to Java RegexPlainTextFilterTest#testParameters
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // rule, sourceGroup, regexOptions configurations
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripRegexIT
    testFiles := []string{
        "Test01_stringinfo_en.info",
        "dummy.foo",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java RegexXliffCompareIT
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_regexplaintext/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_regexplaintext/
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
- Key difference from BasePlainTextFilter: uses regex-based line break detection instead of standard line break scanning
- Unique parameters: `rule` (regex pattern), `sourceGroup` (capture group index), `regexOptions` (Java regex flags), `sample` (test sample)
- Configurations: `okf_plaintext_regex` (default), `okf_plaintext_regex_lines` (line mode), `okf_plaintext_regex_paragraphs` (paragraph mode)
- Trades speed and memory for wider line break detection (Unicode separators, custom patterns)

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/plaintext/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `RegexPlainTextFilterTest.java` | `okapi/filters/plaintext/src/test/java/net/sf/okapi/filters/plaintext/` | 6 |
