# okf_json - JSON Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_json` |
| Java Class | `net.sf.okapi.filters.json.JSONFilter` |
| MIME Types | `application/json` |
| Extensions | `.json` |
| Okapi Module | `json` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/json/src/test/java/`

#### JSONFilterTest.java (38 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testList` | Standalone string in JSON array extracted | P1 |
| 2 | `testEscapedForwardSlashDecoding` | Escaped forward slashes decoded correctly regardless of config | P1 |
| 3 | `testObject` | Nested object value extracted | P1 |
| 4 | `testValue` | Simple key-value pair extracted | P1 |
| 5 | `testAllWithKeyNoException` | Default: all key-value pairs extracted, key used as TU name | P1 |
| 6 | `testAllWithKeyWithException` | Exceptions parameter excludes matching keys from extraction | P1 |
| 7 | `testNoneWithKeywithException` | extractAllPairs=false with exceptions re-includes matching keys | P2 |
| 8 | `testPath` | useFullKeyPath produces /parent/child key paths as TU names | P1 |
| 9 | `testLeadingSlash` | useLeadingSlashOnKeyPath controls leading / in key paths | P2 |
| 10 | `testStandaloneYes` | extractStandalone=true extracts strings in arrays | P1 |
| 11 | `testStandaloneDefaultWhichIsNo` | Default: standalone strings in arrays not extracted | P1 |
| 12 | `testSmartQuotes` | Smart quotes (curly) handled with HTML subfilter | P2 |
| 13 | `testEscape` | Unicode escape \\u00E0 decoded to raw character | P1 |
| 14 | `testEscapes` | All JSON escape sequences (\\n, \\", \\\\, \\b, \\f, \\t, \\r, \\/) | P1 |
| 15 | `testWhiteSpaceAndComments` | Comments (/* */, //, #, <!-- -->) preserved in skeleton | P2 |
| 16 | `testMultilineComment` | Multi-line /* */ comments handled | P2 |
| 17 | `testNestedComments` | Nested /* */ comments handled | P2 |
| 18 | `testEmptyValue` | Empty string value preserved in output | P1 |
| 19 | `testDecimalNumber` | Decimal numbers not extracted as text units | P1 |
| 20 | `testDoubleExtraction` | Double extraction roundtrip for 14 test files | P1 |
| 21 | `testDoubleExtractionOnPreviousFailure` | Double extraction for customer_form.json | P1 |
| 22 | `testDoubleExtractionOnInvalid` | Double extraction for invalid JSON file | P2 |
| 23 | `testDefaultInfo` | Filter has name, display name, and configurations | P3 |
| 24 | `testSimpleEntrySkeleton` | Skeleton preserves whitespace and line breaks | P1 |
| 25 | `testLineBreaks` | Line break type detected from document (\\r) | P2 |
| 26 | `testSubfilter` | HTML subfilter: JSON unescaping, HTML parsing, roundtrip | P1 |
| 27 | `testSubfilterEasyToDebug` | Simplified subfilter test for debugging | P2 |
| 28 | `testSubFilterDoubleExtraction` | Double extraction with HTML subfilter | P1 |
| 29 | `testSubfiltersProduceDistinctTextUnitIds` | Subfilter TU IDs are unique | P2 |
| 30 | `testEscapeForwardSlashes` | Default: forward slashes escaped in output | P1 |
| 31 | `testNoEscapeForwardSlashes` | escapeForwardSlashes=false preserves slashes | P1 |
| 32 | `testEscapeForwardSlashesSubfilter` | Forward slash escaping with HTML subfilter | P2 |
| 33 | `testInlineCodeFinderEscaping` | Code finder with JSON-escaped HTML tags | P2 |
| 34 | `testInlineCodeFinderNewLineCharacter` | Code finder detecting \\n as inline code | P2 |
| 35 | `testNoteRules` | noteRules marks keys as notes attached to next TU | P2 |
| 36 | `testIdRules` | idRules uses key value as TU name/ID | P2 |
| 37 | `testNestedIdRules` | Nested id rules with useIdStack produce compound IDs | P2 |
| 38 | `testGenericMetaRules` | genericMetaRules attaches metadata annotations to TUs | P2 |
| 39 | `testGenericMetaRulesWithId` | Generic meta rules combined with id rules | P2 |
| 40 | `testExtractionRules` | extractionRules limits which keys are extracted | P1 |
| 41 | `metaDataAndExtractionRulesWithSubfilter` | Metadata + extraction rules + HTML subfilter combined | P2 |
| 42 | `metaDataAndExtractionRulesNestedNotes` | Nested note rules with metadata from file | P2 |
| 43 | `testSubfilterRules` | subfilterRules applies subfilter only to specific keys | P2 |
| 44 | `testArrayWithinArray` | Nested arrays produce array:N paths | P2 |
| 45 | `testArrayWithinArrayWithinArray` | Triple-nested arrays produce correct paths | P2 |
| 46 | `testArrayWithObject` | Array containing objects produces correct paths | P2 |
| 47 | `testNamedArray` | Named arrays produce key-based paths | P2 |
| 48 | `testMaxwidthRules` | maxwidthRules sets MAX_WIDTH property on TUs | P2 |
| 49 | `testMaxwidthRulesWithSizeChar` | maxwidthSizeUnit=char sets SIZE_UNIT property | P2 |
| 50 | `testVariableMaxWidthInNestedObjects` | Variable max width in nested objects from file | P2 |

#### JsonSnippetParserTest.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSingleObject` | Parser tokenizes JSON with comments and unicode escapes | P3 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripJsonIT` | `integration-tests/okapi/src/test/java/.../RoundTripJsonIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/json/`):
All `.json` files in the directory (75+ files).

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `JsonXliffCompareIT` | `integration-tests/okapi/src/test/java/.../JsonXliffCompareIT.java` | 1 |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyJsonIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyJsonIT.java` | 2 |

#### Memory Leak IT

Not present for JSON filter.

## Test Data Files

### Unit test resources

Source: `okapi/filters/json/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `1EdwardParallax.json` | `testDoubleExtraction` | Real-world JSON content |
| `array-test.json` | `testDoubleExtraction` | Array structure tests |
| `books.json` | `testDoubleExtraction` | Book catalog JSON |
| `customer_form.json` | `testDoubleExtractionOnPreviousFailure` | Customer form (previous failure repro) |
| `geo.json` | `testDoubleExtraction` | Geo/location JSON |
| `invalid_by_most_processors.json` | `testDoubleExtractionOnInvalid` | Relaxed JSON parsing test |
| `metadata-nested.json` | `metaDataAndExtractionRulesNestedNotes` | Nested metadata notes |
| `metadata.fprm` | `metaDataAndExtractionRulesWithSubfilter` | Metadata extraction config |
| `metadata.json` | `metaDataAndExtractionRulesWithSubfilter` | Metadata with notes/IDs |
| `nested_charsize.fprm` | `testVariableMaxWidthInNestedObjects` | Max width config |
| `nested_charsize.json` | `testVariableMaxWidthInNestedObjects` | Nested max width values |
| `test01.json` - `test06.json` | `testDoubleExtraction` | Various JSON structures |
| `test07-subfilter.json` | `testSubFilterDoubleExtraction` | HTML-in-JSON subfilter test |
| `test08.json` - `test09.json` | `testDoubleExtraction` | Additional JSON structures |
| `twitter.json` | `testDoubleExtraction` | Twitter API response JSON |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/json/`

| File | Type | Purpose |
|------|------|---------|
| 75+ JSON files | roundtrip | Various real-world JSON: Drupal, WordPress, Magento, Twitter, Facebook, Flickr, YouTube, marketing content, CMS exports |
| `debug/null_path_example.json` | debug | Null path edge case |
| `debug/okf_json@exceptions.fprm` | debug | Exception config |
| `maxwidth/maxwidth.json` | roundtrip | Max width test |
| `maxwidth/okf_json@maxwidth.fprm` | roundtrip | Max width config |
| `metarules/1 Edward Parallax.json` | roundtrip | Metadata rules test |
| `metarules/okf_json@metadata.fprm` | roundtrip | Metadata config |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| None needed | Comprehensive test files exist | Both inline snippets and file-based tests |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/json/src/test/resources/*.json okapi-testdata/okf_json/
cp okapi/filters/json/src/test/resources/*.fprm okapi-testdata/okf_json/

# Integration test resources
cp integration-tests/okapi/src/test/resources/json/*.json okapi-testdata/okf_json/roundtrip/
cp -r integration-tests/okapi/src/test/resources/json/debug okapi-testdata/okf_json/roundtrip/debug/
cp -r integration-tests/okapi/src/test/resources/json/maxwidth okapi-testdata/okf_json/roundtrip/maxwidth/
cp -r integration-tests/okapi/src/test/resources/json/metarules okapi-testdata/okf_json/roundtrip/metarules/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_json`

Build tag: `//go:build integration`

#### json_test.go - Extraction Tests

```go
func TestExtract_BasicKeyValue(t *testing.T) {
    // Table-driven: maps 1:1 to Java JSONFilterTest
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        wantNames  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "simple value",
            input: `{"key" : "Text1"}`,
            wantTexts: []string{"Text1"},
            wantNames: []string{"key"},
            javaRef: "JSONFilterTest#testValue",
        },
        {
            name:  "nested object",
            input: `{"key" : { "key2" : "Text1" } }`,
            wantTexts: []string{"Text1"},
            javaRef: "JSONFilterTest#testObject",
        },
        // ... additional test cases
    }
}

func TestExtract_KeyPaths(t *testing.T) {
    // Maps to JSONFilterTest: testPath, testLeadingSlash, testArrayWithinArray, etc.
    tests := []struct {
        name      string
        input     string
        params    map[string]any
        wantNames []string
        javaRef   string
    }{
        // ... key path test cases
    }
}

func TestExtract_Escaping(t *testing.T) {
    // Maps to JSONFilterTest: testEscape, testEscapes, testEscapedForwardSlashDecoding
    tests := []struct {
        name     string
        input    string
        wantText string
        javaRef  string
    }{
        // ... escape handling test cases
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_ExtractionRules(t *testing.T) {
    // Maps to JSONFilterTest: testExtractionRules, testAllWithKeyWithException, etc.
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // ... extraction rule cases
    }
}

func TestConfig_Subfilter(t *testing.T) {
    // Maps to JSONFilterTest subfilter tests
    // Note: subfilter tests require HTML filter to be available
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // ... subfilter cases
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripJsonIT
    testFiles := []string{
        // 75+ files from testdata/roundtrip/
    }
    knownFailing := map[string]string{
        // None known
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java JsonXliffCompareIT
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_json/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_json/
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
- Filter-specific quirks:
  - JSON filter supports relaxed JSON (comments: //, #, /* */, <!-- -->; unquoted keys)
  - `extractAllPairs` (default true) + `exceptions` pattern controls what keys are extracted
  - `extractStandalone` (default false) controls whether array strings are extracted
  - `useFullKeyPath` produces hierarchical paths like `/parent/child` as TU names
  - `escapeForwardSlashes` (default true) escapes / as \\/ in output
  - `subfilter` and `subfilterRules` enable HTML subfiltering on specific keys
  - `noteRules`, `idRules`, `genericMetaRules`, `extractionRules` provide fine-grained control
  - `maxwidthRules` and `maxwidthSizeUnit` attach length constraints to TUs
  - Unicode escapes (\\uXXXX) are decoded on input, output as raw characters in UTF-8

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/json/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `JSONFilterTest.java` | `okapi/filters/json/src/test/java/.../` | 50 |
| `JsonSnippetParserTest.java` | `okapi/filters/json/src/test/java/.../parser/` | 1 |
