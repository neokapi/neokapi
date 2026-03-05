# okf_regex - Regex Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_regex` |
| Java Class | `net.sf.okapi.filters.regex.RegexFilter` |
| MIME Types | `text/x-regex` |
| Extensions | `.srt, .strings` |
| Okapi Module | `regex` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/regex/src/test/java/`

#### RegexFilterTest.java (16 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testStartDocument` | Start document event properties | P3 |
| 2 | `testDoubleExtraction` | Double extraction with 6 different rule sets (TestRules01-06) | P1 |
| 3 | `testConfigurations` | Loading bundled configurations (SRT, macStrings, INI, etc.) | P1 |
| 4 | `testNameExtraction` | ID/name extraction from regex groups | P1 |
| 5 | `testNoteExtraction` | Note/comment extraction from regex groups | P1 |
| 6 | `testMeta` | Metadata extraction from regex groups | P2 |
| 7 | `testSimpleRule` | Simple regex rule extraction | P1 |
| 8 | `testIDAndText` | ID and text extraction from named groups | P1 |
| 9 | `testEscapeDoubleChar` | Double character escape handling | P1 |
| 10 | `testEscapeDoubleCharNoEscape` | Without double char escape | P1 |
| 11 | `testCollapseNewline` | Newline collapsing in extracted text | P2 |
| 12 | `testEmptyLines` | Empty line handling between entries | P2 |
| 13 | `testSemicolonInData` | Semicolons in data content | P2 |
| 14 | `testBackslashEscapeHandling` | Backslash escape sequences | P2 |
| 15 | `testSubFiltering` | Sub-filtering within regex-matched content | P2 |
| 16 | `testNoteWithSubfilter` | Notes with sub-filter content | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripRegexIT` | `integration-tests/okapi/src/test/java/.../RoundTripRegexIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/regex/`):
- `dummy.foo`, `meta/`, `meta2/`, `stringInfo/`, `Test01_stringinfo_en.info`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `RegexXliffCompareIT` | `integration-tests/okapi/src/test/java/.../RegexXliffCompareIT.java` | 1 |

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `RegexMemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../RegexMemoryLeakTestIT.java` | 1 (main) |

## Test Data Files

### Unit test resources

Source: `okapi/filters/regex/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `TestRules01.txt` through `TestRules06.txt` | `RegexFilterTest#testDoubleExtraction` | Rule-specific test files |
| `okf_regex@TestRules01.fprm` through `06.fprm` | Various | Custom rule configurations |
| `okf_regex@SRT.fprm` | `RegexFilterTest#testConfigurations` | SRT subtitle rules |
| `okf_regex@macStrings.fprm` | `RegexFilterTest#testConfigurations` | Mac .strings rules |
| `okf_regex@macStrings_HTML.fprm` | `RegexFilterTest#testConfigurations` | Mac .strings with HTML |
| `okf_regex@INI.fprm` | `RegexFilterTest#testConfigurations` | INI file rules |
| `okf_regex@StringInfo.fprm` | `RegexFilterTest#testConfigurations` | StringInfo rules |
| `okf_regex@SymbianRLS.fprm` | `RegexFilterTest#testConfigurations` | Symbian RLS rules |
| `test.strings` | `RegexFilterTest` | Mac strings test file |
| `Test01_srt_en.srt` | `RegexFilterTest` | SRT subtitle test |
| `Test01_stringinfo_en.info` | `RegexFilterTest` | StringInfo test |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/regex/`

| File | Type | Purpose |
|------|------|---------|
| `dummy.foo` | roundtrip | Generic regex roundtrip |
| `Test01_stringinfo_en.info` | roundtrip | StringInfo format |
| `meta/`, `meta2/` | roundtrip | Meta extraction tests |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/regex/src/test/resources/TestRules*.txt okapi-testdata/okf_regex/
cp okapi/filters/regex/src/test/resources/*.fprm okapi-testdata/okf_regex/
cp okapi/filters/regex/src/test/resources/test.strings okapi-testdata/okf_regex/
cp okapi/filters/regex/src/test/resources/Test01_srt_en.srt okapi-testdata/okf_regex/
cp okapi/filters/regex/src/test/resources/Test01_stringinfo_en.info okapi-testdata/okf_regex/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/regex/ okapi-testdata/okf_regex/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/regex`

Build tag: `//go:build integration`

#### regex_test.go - Extraction Tests

```go
func TestExtract_simpleRule(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantTexts  []string
        javaRef    string
    }{
        {name: "simple rule extraction", javaRef: "RegexFilterTest#testSimpleRule"},
        {name: "ID and text extraction", javaRef: "RegexFilterTest#testIDAndText"},
        {name: "name extraction", javaRef: "RegexFilterTest#testNameExtraction"},
        {name: "escape double char", javaRef: "RegexFilterTest#testEscapeDoubleChar"},
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/regex/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/regex/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All configuration tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - Highly configurable: rules defined via .fprm parameter files
  - Regex groups map to source text, ID, note, and metadata
  - Bundled configs: SRT subtitles, Mac .strings, INI, StringInfo, Symbian RLS
  - Supports sub-filtering for embedded formats within regex-matched content
  - Escape handling: double-char escapes, backslash escapes
  - Empty line and newline collapsing options

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/regex/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `RegexFilterTest.java` | `okapi/filters/regex/src/test/java/net/sf/okapi/filters/regex/` | 16 |
