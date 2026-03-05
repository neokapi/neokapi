# okf_properties - Properties Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_properties` |
| Java Class | `net.sf.okapi.filters.properties.PropertiesFilter` |
| MIME Types | `text/x-properties` |
| Extensions | `.properties, .lang` |
| Okapi Module | `properties` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/properties/src/test/java/`

#### PropertiesFilterTest.java (30 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Filter has name, display name, configurations | P3 |
| 2 | `testDoubleExtraction` | Double extraction roundtrip for Test01-Test04 with configs | P1 |
| 3 | `testStartDocument` | StartDocument event correct for Test01.properties | P1 |
| 4 | `testLineBreaks_CR` | CR line break detection | P2 |
| 5 | `testMessagePlaceholders` | Java message format placeholders ({0}, {1}) as inline codes | P1 |
| 6 | `testMessagePlaceholdersEscaped` | Escaped message placeholders (''{0}'') | P1 |
| 7 | `testineBreaks_CRLF` | CRLF line break detection | P2 |
| 8 | `testLineBreaks_LF` | LF line break detection | P2 |
| 9 | `testEntry` | Basic key=value entry extraction and TU name from key | P1 |
| 10 | `testSplicedEntry` | Multi-line entry with backslash continuation | P1 |
| 11 | `testEscapes` | Properties escape sequences (\\n, \\t, \\uXXXX) | P1 |
| 12 | `testKeySpecial` | Special characters in key (colons, equals, spaces) | P1 |
| 13 | `testLocDirectives_Skip` | Localization directive: skip entries | P2 |
| 14 | `testLocDirectives_Group` | Localization directive: group entries | P2 |
| 15 | `testSpecialChars` | Special characters in values (unicode, escapes) | P1 |
| 16 | `testSpecialCharsInKey` | Special characters in keys | P1 |
| 17 | `testSpecialCharsOutput` | Special characters roundtrip in output | P1 |
| 18 | `testWithSubfilter` | HTML subfilter on property values | P1 |
| 19 | `testWithSubfilterTwoParas` | HTML subfilter with two paragraphs in value | P2 |
| 20 | `testWithSubfilterWithEmbeddedMessagePH` | Subfilter with embedded {0} placeholders | P2 |
| 21 | `testWithSubfilterWithHTMLEscapes` | Subfilter with HTML entity escapes | P2 |
| 22 | `testWithSubfilterOutput` | Subfilter output roundtrip | P1 |
| 23 | `testWithSubfilterOutputEscapeExtended` | Subfilter output with escaped extended chars | P2 |
| 24 | `testWithSubfilterOutputDoNotEscapeExtended` | Subfilter output without escaping extended chars | P2 |
| 25 | `testHtmlOutput` | HTML entities in output | P2 |
| 26 | `testWithSubfilterWithEmbeddedEscapedMessagePH` | Subfilter with escaped message placeholders | P2 |
| 27 | `testDoubleExtractionSubFilter` | Double extraction with HTML subfilter | P1 |
| 28 | `testIdGeneration_defaultConfig` | TU ID generation from key (default config) | P2 |
| 29 | `testIdGeneration_subfiltersConfig` | TU ID generation with subfilter config | P2 |
| 30 | `testJavaEscapeChars` | Java-specific escape characters | P2 |

### Integration Tests

#### RoundTrip IT

Not present (properties files use unit-test-level double extraction instead).

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `PropertyXliffCompareIT` | `integration-tests/okapi/src/test/java/.../PropertyXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/properties/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.properties` | `testStartDocument`, `testDoubleExtraction` | Basic properties file |
| `Test01_first_trip.properties` | reference | Expected first-trip output |
| `Test02.properties` | `testDoubleExtraction` | Properties with config |
| `Test03.properties` | `testDoubleExtraction` | Properties with config |
| `Test04.properties` | `testDoubleExtraction` | Properties with config |
| `Test05.properties` | various | Additional test properties |
| `issue_216.properties` | `testIdGeneration` | Issue 216 repro |
| `issue_216.fprm` | `testIdGeneration` | Issue 216 config |
| `okf_properties@Test02.fprm` | `testDoubleExtraction` | Test02 config |
| `okf_properties@Test03.fprm` | `testDoubleExtraction` | Test03 config |
| `okf_properties@Test04.fprm` | `testDoubleExtraction` | Test04 config |

### Integration test resources

No dedicated integration test resources directory.

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/properties/src/test/resources/*.properties okapi-testdata/okf_properties/
cp okapi/filters/properties/src/test/resources/*.fprm okapi-testdata/okf_properties/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/properties`

Build tag: `//go:build integration`

#### properties_test.go - Extraction Tests

```go
func TestExtract_BasicEntry(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        wantNames  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "basic key=value",
            input: "key1=Text1",
            wantTexts: []string{"Text1"},
            wantNames: []string{"key1"},
            javaRef: "PropertiesFilterTest#testEntry",
        },
        // ... additional test cases
    }
}

func TestExtract_Escapes(t *testing.T) {
    // Maps to PropertiesFilterTest escape tests
}

func TestExtract_Subfilter(t *testing.T) {
    // Maps to PropertiesFilterTest subfilter tests
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/properties/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/properties/
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
  - Java .properties format: key=value, key:value, key value separators
  - Backslash continuation lines (\ at end of line)
  - Unicode escapes (\\uXXXX) decoded on input
  - Java message format placeholders ({0}, {1}) detected as inline codes
  - Localization directives: #@skip, #@group comments control extraction
  - HTML subfilter can be applied to values containing HTML
  - Extended characters can be escaped or left raw depending on config
  - Keys can contain special characters with backslash escaping

## Current Go Coverage

### Bridge Tests (`core/plugin/bridge/filters/properties/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `PropertiesFilterTest#testEntry` | `TestExtract_BasicEntry` | Mapped |
| `PropertiesFilterTest#testEscapes` | `TestExtract_Escapes` | Mapped |
| `PropertiesFilterTest#testSpecialChars` | `TestExtract_SpecialChars` | Mapped |
| `PropertiesFilterTest#testSplicedEntry` | `TestExtract_SplicedEntry` | Mapped |
| `PropertiesFilterTest#testKeySpecial` | `TestExtract_KeySpecial` | Mapped |

### Native Tests (`core/formats/properties/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `PropertiesFilterTest#testEntry` | `TestReadBasicEntry` | Mapped |
| `PropertiesFilterTest#testKeySpecial` | `TestReadKeySpecial` | Mapped |
| `PropertiesFilterTest#testSplicedEntry` | `TestReadSplicedEntry` | Mapped |
| `PropertiesFilterTest#testEscapes` | `TestReadEscapes` | Mapped |
| `PropertiesFilterTest#testSpecialChars` | `TestReadSpecialChars` | Mapped |
| `PropertiesFilterTest#testSpecialCharsOutput` | `TestWriteSpecialChars` | Mapped |

**Coverage**: ~5 of 30 Surefire methods have bridge `// okapi:` annotations (~17%).

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/properties/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `PropertiesFilterTest.java` | `okapi/filters/properties/src/test/java/.../` | 30 |
