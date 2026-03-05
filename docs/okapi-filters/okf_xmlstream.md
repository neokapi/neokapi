# okf_xmlstream - XML Stream Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_xmlstream` |
| Java Class | `net.sf.okapi.filters.xmlstream.XmlStreamFilter` |
| MIME Types | `text/xml` |
| Extensions | `.dita, .ditamap` |
| Okapi Module | `xmlstream` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/xmlstream/src/test/java/`

#### XmlSnippetsTest.java (51 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testMultipleMETA` | Multiple META tags in XML | P2 |
| 2 | `testTitleInP` | Title attribute in p element | P1 |
| 3 | `testAltInImg` | Alt attribute in img element | P1 |
| 4 | `testNoExtractValueInInput` | Input hidden value not extracted | P2 |
| 5 | `testExtractValueInInput` | Input submit value extracted | P1 |
| 6 | `testLabelInOption` | Label attribute in option | P2 |
| 7 | `testMETATag1` | META keywords content extraction | P1 |
| 8 | `testPWithAttributes` | P with title and dir attributes | P1 |
| 9 | `testLang` | xml:lang attribute detection | P1 |
| 10 | `testComplexEmptyElement` | Complex empty element with multiple attributes | P2 |
| 11 | `testPWithInlines` | P with inline b and a elements | P1 |
| 12 | `testMETATag2` | META description extraction | P1 |
| 13 | `testPWithInlines2` | P with inline b and img elements | P1 |
| 14 | `testPWithInlineTextOnly` | P with inline text only | P1 |
| 15 | `testTableGroups` | Table/tr/td group events | P1 |
| 16 | `testGroupInPara` | Embedded list in paragraph | P1 |
| 17 | `testInput` | Input element handling | P1 |
| 18 | `testCollapseWhitespaceWithPre` | Whitespace collapse with pre | P1 |
| 19 | `testCollapseWhitespaceWithoutPre` | Global whitespace collapse | P1 |
| 20 | `testEscapedCodesInisdePre` | Escaped codes inside pre | P2 |
| 21 | `testCdataSection` | CDATA section handling | P1 |
| 22 | `testCdataSectionExtraction` | CDATA content extracted as text unit | P1 |
| 23 | `testCdataSectionExtractionAndWS` | CDATA extraction with whitespace | P2 |
| 24 | `testCdataSectionAsHTML` | CDATA processed as HTML subfilter | P2 |
| 25 | `testCdataSectionAsHTMLButEmpty` | Empty CDATA with HTML subfilter | P2 |
| 26 | `testCdataSectionExtractionWithCondition` | Conditional CDATA extraction | P2 |
| 27 | `testEscapes` | XML entity escape handling | P1 |
| 28 | `testEscapes2` | Additional entity escapes | P1 |
| 29 | `testEscapedEntities` | Character entity preservation | P1 |
| 30 | `testNewlineNormalization` | Newline normalization in XML | P2 |
| 31 | `testCodeFinder` | Inline code finder with regex | P2 |
| 32 | `testNormalizeNewlinesInPre` | Newline normalization in pre | P2 |
| 33 | `testSupplementalSupport` | Unicode supplemental characters | P2 |
| 34 | `testSimpleSupplementalSupport` | Simple supplemental chars | P2 |
| 35-51 | Additional snippet tests | Various XML-specific extraction scenarios | P2 |

#### XmlStreamConfigurationSupportTest.java (27 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-27 | Configuration support tests | YAML-based config rules: EXCLUDE, INCLUDE, PRESERVE_WHITESPACE, ATTRIBUTE_ID, ATTRIBUTE_TRANS, ATTRIBUTE_WRITABLE, conditions, regex patterns | P2 |

#### XmlStreamEventTest.java (14 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-14 | Event structure tests | Verify correct Okapi event types and structures for XML elements | P1 |

#### XmlStreamConfigurationTest.java (10 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-10 | Configuration parsing | YAML config parsing for elements, attributes, preserve_whitespace, code types | P2 |

#### XmlStreamSubfilterTest.java (11 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-11 | Subfilter integration | HTML/JSON subfilter applied to XML element content and attributes | P2 |

#### integration/DitaExtractionComparisionTest.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testStartDocument` | DITA StartDocument event | P1 |
| 2 | `testDoubleExtractionSingle` | Double extraction roundtrip for single DITA file | P1 |
| 3 | `testDoubleExtraction` | Double extraction for all DITA test files | P1 |
| 4-5 | Additional DITA tests | DITA-specific extraction scenarios | P1 |

#### integration/CdataSubfilterWithRegexTest.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-3 | CDATA subfilter with regex | CDATA sections processed through HTML subfilter with regex code finder | P2 |

#### integration/PCdataSubfilterTest.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-5 | PCDATA subfilter | PCDATA content processed through subfilter | P2 |

#### integration/DocTypeExtractionTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-2 | DOCTYPE handling | DOCTYPE declarations preserved in extraction | P2 |

#### integration/PIExtractionTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-2 | Processing instruction | PI handling during extraction | P2 |

#### integration/PropertyXmlExtractionComparisionTest.java (7 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-7 | Java properties XML | XML-formatted Java properties extraction and roundtrip | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripXmlStreamIT` | `integration-tests/okapi/src/test/java/.../RoundTripXmlStreamIT.java` | 2 |

**Test files used**: 20 files in `integration-tests/okapi/src/test/resources/xmlstream/`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `XmlStreamXliffCompareIT` | `integration-tests/okapi/src/test/java/.../XmlStreamXliffCompareIT.java` | 1 |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyXmlStreamIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyXmlStreamIT.java` | 2 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/xmlstream/src/test/resources/`

88 files including DITA topics (shovellingsnow.dita, changingtheoil.dita, etc.), XML test files, YAML configs, and subfilter test data.

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/xmlstream/`

20 files including java_properties XML, Issue421 repro files.

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/xmlstream/src/test/resources/*.dita okapi-testdata/okf_xmlstream/
cp okapi/filters/xmlstream/src/test/resources/*.xml okapi-testdata/okf_xmlstream/
cp okapi/filters/xmlstream/src/test/resources/*.yml okapi-testdata/okf_xmlstream/
cp okapi/filters/xmlstream/src/test/resources/*.html okapi-testdata/okf_xmlstream/
cp okapi/filters/xmlstream/src/test/resources/*.fprm okapi-testdata/okf_xmlstream/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/xmlstream/* okapi-testdata/okf_xmlstream/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/xmlstream`

Build tag: `//go:build integration`

#### xmlstream_test.go - Extraction Tests

```go
func TestExtract_BasicElements(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        // ... test cases from XmlSnippetsTest
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_ExcludeInclude(t *testing.T) {
    // Maps to XmlStreamConfigurationSupportTest
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripXmlStreamIT
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/xmlstream/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/xmlstream/
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
  - Uses YAML-based tagged filter configuration (same engine as HTML filter)
  - Supports DITA content natively via DITA-specific configurations
  - CDATA sections can be extracted directly or processed through HTML subfilter
  - Supports attribute-level subfiltering (JSON/HTML in attribute values)
  - `excludeByDefault` mode inverts the extraction logic
  - XML-specific: handles DOCTYPE, PIs, namespaces, CDATA
  - Newlines are normalized per XML spec

## Current Go Coverage

### Bridge Tests (`core/plugin/bridge/filters/xmlstream/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `XmlSnippetsTest#testPWithInlines` | `TestExtract_PWithInlines` | Mapped |
| `XmlSnippetsTest#testEscapes` | `TestExtract_Escapes` | Mapped |
| `XmlSnippetsTest#testCdataSection` | `TestExtract_CDATA` | Mapped |
| `RoundTripXmlStreamIT` | `TestRoundTrip` | Mapped |

**Coverage**: ~4 of 136 Surefire methods have bridge `// okapi:` annotations (~3%).

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/xmlstream/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `XmlSnippetsTest.java` | `okapi/filters/xmlstream/src/test/java/.../` | 51 |
| `XmlStreamConfigurationSupportTest.java` | `okapi/filters/xmlstream/src/test/java/.../` | 27 |
| `XmlStreamEventTest.java` | `okapi/filters/xmlstream/src/test/java/.../` | 14 |
| `XmlStreamConfigurationTest.java` | `okapi/filters/xmlstream/src/test/java/.../` | 10 |
| `XmlStreamSubfilterTest.java` | `okapi/filters/xmlstream/src/test/java/.../` | 11 |
| `DitaExtractionComparisionTest.java` | `okapi/filters/xmlstream/src/test/java/.../integration/` | 5 |
| `CdataSubfilterWithRegexTest.java` | `okapi/filters/xmlstream/src/test/java/.../integration/` | 3 |
| `PCdataSubfilterTest.java` | `okapi/filters/xmlstream/src/test/java/.../integration/` | 5 |
| `DocTypeExtractionTest.java` | `okapi/filters/xmlstream/src/test/java/.../integration/` | 2 |
| `PIExtractionTest.java` | `okapi/filters/xmlstream/src/test/java/.../integration/` | 2 |
| `PropertyXmlExtractionComparisionTest.java` | `okapi/filters/xmlstream/src/test/java/.../integration/` | 7 |
