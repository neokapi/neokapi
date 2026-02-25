# okf_xml - XML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_xml` |
| Java Class | `net.sf.okapi.filters.xml.XMLFilter` |
| MIME Types | `text/xml` |
| Extensions | `.xml, .resx, .rdf, .wxl, .stringsdict` |
| Okapi Module | `its` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/its/src/test/java/`

#### XMLFilterTest.java (53 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSpecialEntities` | Entity escaping: lt, gt, quot, apos, nbsp in output | P1 |
| 2 | `testSpecialEntitiesWithOptions` | Custom escape options (escapeQuotes, escapeGT, escapeNbsp) via ITS rules | P1 |
| 3 | `rightAngleBracketEscapedInExcludedContent` | Right angle bracket escaped in non-translatable content | P2 |
| 4 | `rightAngleBracketNotEscapedInExcludedContent` | With escapeGT=no, right angle bracket not escaped | P2 |
| 5 | `testCRLFInAttributes` | CRLF normalization in XML attributes | P1 |
| 6 | `testEOL` | End-of-line handling in XML content | P1 |
| 7 | `testLineBreakAsCode` | Line breaks treated as inline codes | P2 |
| 8 | `testAndroidQuotes` | Android-style quote escaping in strings | P1 |
| 9 | `testDeclaredEntities` | Custom entity declarations preserved in output | P1 |
| 10 | `testLocaleFilter1` | ITS locale filter: include specific locale | P2 |
| 11 | `testLocaleFilter2` | ITS locale filter: exclude specific locale | P2 |
| 12 | `testLocaleFilter3` | ITS locale filter: wildcard matching | P2 |
| 13 | `testLocaleFilter4` | ITS locale filter: negation | P2 |
| 14 | `testLocaleFilter5` | ITS locale filter: multiple locales | P2 |
| 15 | `testLocaleFilter6` | ITS locale filter: case insensitive matching | P2 |
| 16 | `testStack` | Element stack depth handling | P2 |
| 17 | `testComplexIdValue` | Complex XPath expressions for ITS idValue | P2 |
| 18 | `testIdValueV2` | ITS 2.0 idValue with namespace resolution | P2 |
| 19 | `testITSVersionAttribute` | ITS version attribute validation (expects exception) | P2 |
| 20 | `testPreserveSpace1` | xml:space="preserve" handling at multiple levels | P1 |
| 21 | `testITSVersion1` | ITS version 1.0 rules processing | P2 |
| 22 | `testITSVersion2` | ITS version 2.0 rules processing | P2 |
| 23 | `testIdValue` | Basic ITS idValue rule for text unit naming | P1 |
| 24 | `testSubFilterContextPassing` | Sub-filter context propagation | P2 |
| 25 | `testDomain1` | ITS domain annotation extraction | P2 |
| 26 | `testDomain2` | ITS domain with pointer-based extraction | P2 |
| 27 | `testAllowedCharsAndStorageSize` | ITS allowedCharacters and storageSize annotations | P2 |
| 28 | `testStorageSize` | ITS storageSize with encoding and lineBreakType | P2 |
| 29 | `testIdComplexValue` | Complex id value with concat XPath | P2 |
| 30 | `testLocalWithinText` | ITS 2.0 local withinText attribute | P1 |
| 31 | `testLocalWithinTextOnRoot` | ITS 2.0 withinText on root element | P1 |
| 32 | `testEmptyElements` | Empty XML elements extraction | P1 |
| 33 | `testOutputEmptyElements` | Empty XML elements roundtrip output | P1 |
| 34 | `testOutputAttributesAndQuotes` | Attribute quoting in output | P1 |
| 35 | `testDefaultInfo` | Default filter info metadata | P3 |
| 36 | `testSubFilter` | Sub-filter with HTML content inside XML CDATA | P1 |
| 37 | `testSubFilterIds` | Sub-filter text unit ID generation | P2 |
| 38 | `testStartDocument` | Start document event from snippet | P3 |
| 39 | `testStartDocumentFromList` | Start document event from config list | P3 |
| 40 | `testOutputBasic_Comment` | XML comment in output | P1 |
| 41 | `testOutputBasic_PI` | Processing instruction in output | P1 |
| 42 | `testOutputBasic_OneChar` | Single character content | P1 |
| 43 | `testOutputBasic_EmptyRoot` | Empty root element output | P1 |
| 44 | `testOutputSimpleContent` | Simple text content output | P1 |
| 45 | `testOutputSimpleContent_WithEscapes` | Content with XML entities in output | P1 |
| 46 | `testOutputTargetPointer` | ITS targetPointer for bilingual XML | P2 |
| 47 | `testOutputTargetPointerWithExistingTarget` | targetPointer with pre-existing target element | P2 |
| 48 | `testOutputTargetPointerWithInlineCodes` | targetPointer with inline codes in content | P2 |
| 49 | `testOutputSupplementalChars` | Unicode supplemental plane characters | P1 |
| 50 | `testCDATA` | (DataProvider) CDATA section extraction and output | P1 |
| 51 | `testCDATASubfilter` | CDATA with sub-filter content | P1 |
| 52 | `testCREntity` | Carriage return entity handling | P1 |
| 53 | `testCREntityOutput` | Carriage return entity in output | P1 |
| 54 | `testCommentParsing` | XML comment parsing | P1 |
| 55 | `testOutputComment` | XML comment output preservation | P1 |
| 56 | `testPIParsing` | Processing instruction parsing | P1 |
| 57 | `testOutputPI` | Processing instruction output | P1 |
| 58 | `testOutputWhitespacesPreserve` | Whitespace preservation in output | P1 |
| 59 | `testOutputWhitespacesDefault` | Default whitespace normalization in output | P1 |
| 60 | `testOutputWhitespacesITS` | ITS-based whitespace rules in output | P1 |
| 61 | `testOutputStandaloneYes` | standalone="yes" in XML declaration | P2 |
| 62 | `testSeveralUnits` | Multiple text units extraction | P1 |
| 63 | `testTranslatableAttributes` | ITS translatable attributes extraction | P1 |
| 64 | `testLocQualityRatingLocal` | ITS LQR (Localization Quality Rating) annotations | P2 |
| 65 | `testMTConfidence` | ITS MT confidence annotations | P2 |
| 66 | `testTextAnalysis` | ITS text analysis annotations | P2 |
| 67 | `testLocQualityLocalOnUnit` | ITS LQI on text unit level | P2 |
| 68 | `testLocQualityLocalOnCodes` | ITS LQI on inline code level | P2 |
| 69 | `testTerms` | ITS terminology annotations | P2 |
| 70 | `testTranslatableAttributes2` | Additional translatable attributes scenarios | P1 |
| 71 | `testTranslatableAttributesOutput` | Translatable attributes in output | P1 |
| 72 | `testTranslatableAttributesOutputAllowUnescapedQuoteButEscape` | Quote escaping in attribute output | P2 |
| 73 | `testTranslatableAttributesOutputAllowUnescapedQuote` | Unescaped quotes in attribute output | P2 |
| 74 | `testOpenTwice` | Filter reopen without errors | P1 |
| 75 | `testDoubleExtraction` | Double extraction consistency verification | P1 |
| 76 | `testSingleTest` | Single file test execution | P3 |
| 77 | `testCodeFinderOnRESX` | Code finder rules on RESX format | P1 |
| 78 | `testSubfilteringEmptyCDATASection` | Empty CDATA section with sub-filtering | P2 |

#### XMLFilterEncodingTest.java (4 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `utf8ToUtf16le` | Encoding conversion UTF-8 to UTF-16LE | P1 |
| 2 | `utf16WithBom` | UTF-16 with BOM handling | P1 |
| 3 | `utf16WithoutBom` | UTF-16 without BOM handling | P1 |
| 4 | `utf16leWithBomFromFile` | UTF-16LE with BOM from file | P1 |

#### BundledConfigsTest.java (8 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testAndroidUntranslatable` | Android strings: `translatable='false'` excluded | P1 |
| 2 | `testDocBookSimpleInline` | DocBook: emphasis as inline element | P1 |
| 3 | `testDocBookFootnote` | DocBook: footnote handling | P1 |
| 4 | `translatableContentExtracted` | DocBook: translatable paragraphs extracted | P1 |
| 5 | `untranslatableContentExtracted` | DocBook: computeroutput/programlisting excluded | P1 |
| 6 | `withinTextRuleContentHandlingClarified` | DocBook: withinText rule processing | P2 |
| 7 | `inlineNonTranslatableHandlingClarified` | DocBook: inline non-translatable elements | P2 |
| 8 | `codesSimplificationPerformed` | DocBook: inline code simplification | P2 |

#### HtmlSubFilterWrapperTest.java (4 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testHtmlSubFilter` | HTML sub-filter in XML CDATA content | P1 |
| 2 | `testHtmlSubFilter_Blank` | Blank HTML content in sub-filter | P2 |
| 3 | `testHtml5SubFilter` | HTML5 sub-filter in XML CDATA content | P1 |
| 4 | `testHtml5SubFilter_Blank` | Blank HTML5 content in sub-filter | P2 |

#### ParametersTest.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `defaultValuesWrittenAsString` | Default parameters serialization | P2 |
| 2 | `customValuesNotWrittenAsString` | Custom parameters serialization | P2 |
| 3 | `codesSimplificationParametersReadFromString` | Code simplification parameters deserialization | P2 |

#### ITSContentTests.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testAnnotatorsRef` | ITS 2.0 annotatorsRef attribute processing | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripXmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripXmlIT.java` | 4 |

**Test files used** (from `integration-tests/okapi/src/test/resources/xml/`):
- `input.xml`, `test01.xml`, `test02.xml`, `test03.xml`, `test04.xml`
- `test08_utf8nobom.xml`, `TestCDATA1.xml`, `TestMultiLang.xml`
- `Translate1.xml`, `Translate2.xml`, `Translate2_LinkedRules.xml`
- `LocNote-1.xml` through `LocNote-6.xml`
- `XRTT-Source1.xml`, `emoji1.xml`, `openoffice_input.xml`
- `lqi-test1-standoff.xml`, `strings.xml`
- Custom configs: `591/`, `1384/`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `XmlXliffCompareIT` | `integration-tests/okapi/src/test/java/.../XmlXliffCompareIT.java` | 1 |

#### Simplifier IT

Not applicable for okf_xml.

#### Memory Leak IT

Not applicable for okf_xml directly (HtmlMemoryLeakTestIT tests XMLFilter as a secondary filter).

## Test Data Files

### Unit test resources

Source: `okapi/filters/its/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `input.xml` | `XMLFilterTest` | Basic XML extraction input |
| `test01.xml` | `XMLFilterTest#testTranslateOverridenByRule` | XML with ITS translate rules |
| `test02.xml` | `XMLFilterTest` | XML with external rules ref |
| `test02-rules.xml` | (referenced by test02.xml) | External ITS rules |
| `test03.xml` through `test07.xml` | `XMLFilterTest` | Various XML scenarios |
| `test08_utf8nobom.xml` | `XMLFilterEncodingTest` | UTF-8 without BOM |
| `test09.xml` | `XMLFilterTest` | XML test scenario |
| `test10_utf16le-with-bom.xml` | `XMLFilterEncodingTest#utf16leWithBomFromFile` | UTF-16LE with BOM |
| `AndroidTest1.xml` through `AndroidTest3.xml` | `BundledConfigsTest#testAndroidUntranslatable` | Android strings format |
| `docbook-emphasis-example.xml` | `BundledConfigsTest#testDocBookSimpleInline` | DocBook inline elements |
| `docbook-footnote-example.xml` | `BundledConfigsTest#testDocBookFootnote` | DocBook footnotes |
| `inline-non-translatable.xml` | `BundledConfigsTest#inlineNonTranslatableHandlingClarified` | Non-translatable inline |
| `inline-non-translatable-2.xml` | `BundledConfigsTest` | Non-translatable inline variant |
| `Test01.resx` | `XMLFilterTest#testCodeFinderOnRESX` | RESX format with code finder |
| `test_with_placeholders.resx` | `XMLFilterTest` | RESX with placeholder patterns |
| `test_empty_cdata.xml` | `XMLFilterTest#testSubfilteringEmptyCDATASection` | Empty CDATA section |
| `AppleStringsdictTest.stringsdict` | `BundledConfigsTest` | Apple stringsdict format |
| `JavaProperties.xml` | `BundledConfigsTest` | Java properties XML format |
| `MozillaRDFTest01.rdf` | `BundledConfigsTest` | Mozilla RDF format |
| `TestCDATA1.xml` | `XMLFilterTest#testCDATA` | CDATA section handling |
| `Translate1.xml`, `Translate2.xml` | `XMLFilterTest` | Basic translate rules |
| `XRTT-Source1.xml` | `XMLFilterTest` | XRTT source file |
| `LocNote-1.xml` through `LocNote-6.xml` | `XMLFilterTest` | Localization notes |
| `TestMultiLang.xml` | `XMLFilterTest` | Multi-language XML |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/xml/`

| File | Type | Purpose |
|------|------|---------|
| `input.xml` | roundtrip | Generic XML roundtrip |
| `test01.xml` through `test04.xml` | roundtrip | Various XML roundtrip scenarios |
| `test08_utf8nobom.xml` | roundtrip | UTF-8 encoding roundtrip |
| `Translate1.xml`, `Translate2.xml` | roundtrip | Translation rules roundtrip |
| `LocNote-*.xml` | roundtrip | Localization notes roundtrip |
| `emoji1.xml` | roundtrip | Unicode emoji handling |
| `strings.xml` | roundtrip | Android strings roundtrip |
| `openoffice_input.xml` | roundtrip | OpenOffice XML format |
| `custom-configs/591/` | roundtrip | Issue 591 custom config |
| `custom-configs/1384/` | roundtrip | Issue 1384 custom config |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.xml` | Minimal valid XML for smoke test | `<?xml version="1.0"?><doc><p>Hello world</p></doc>` |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/its/src/test/resources/input.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test01.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test02.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test02-rules.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test03.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test04.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test05.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test06.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test07.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test08_utf8nobom.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test09.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test10_utf16le-with-bom.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/AndroidTest1.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/AndroidTest2.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/AndroidTest3.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/Test01.resx okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test_with_placeholders.resx okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/test_empty_cdata.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/TestCDATA1.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/docbook-emphasis-example.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/docbook-footnote-example.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/AppleStringsdictTest.stringsdict okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/JavaProperties.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/MozillaRDFTest01.rdf okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/Translate1.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/Translate2.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/LocNote-1.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/LocNote-2.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/inline-non-translatable.xml okapi-testdata/okf_xml/
cp okapi/filters/its/src/test/resources/inline-non-translatable-2.xml okapi-testdata/okf_xml/

# Integration test resources
cp integration-tests/okapi/src/test/resources/xml/*.xml okapi-testdata/okf_xml/roundtrip/
cp -r integration-tests/okapi/src/test/resources/xml/custom-configs okapi-testdata/okf_xml/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_xml`

Build tag: `//go:build integration`

#### xml_test.go - Extraction Tests

```go
func TestExtract_basicXml(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        params     map[string]any
        javaRef    string
    }{
        {
            name:      "special entities roundtrip",
            input:     `<?xml version="1.0"?><doc><p>&lt;=lt &gt;=gt</p></doc>`,
            javaRef:   "XMLFilterTest#testSpecialEntities",
        },
        {
            name:      "CRLF in attributes",
            javaRef:   "XMLFilterTest#testCRLFInAttributes",
        },
        {
            name:      "declared entities preserved",
            javaRef:   "XMLFilterTest#testDeclaredEntities",
        },
        {
            name:      "empty elements extraction",
            javaRef:   "XMLFilterTest#testEmptyElements",
        },
        {
            name:      "CDATA section extraction",
            javaRef:   "XMLFilterTest#testCDATA",
        },
        {
            name:      "supplemental chars",
            javaRef:   "XMLFilterTest#testOutputSupplementalChars",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_bundledConfigs(t *testing.T) {
    tests := []struct {
        name     string
        configId string
        input    string
        want     []string
        javaRef  string
    }{
        {
            name:     "android untranslatable excluded",
            configId: "okf_xml-AndroidStrings",
            javaRef:  "BundledConfigsTest#testAndroidUntranslatable",
        },
        {
            name:     "resx code finder",
            configId: "okf_xml-resx",
            javaRef:  "XMLFilterTest#testCodeFinderOnRESX",
        },
        {
            name:     "docbook inline emphasis",
            configId: "okf_xml-docbook",
            javaRef:  "BundledConfigsTest#testDocBookSimpleInline",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        // files from testdata/roundtrip/
    }
    knownFailing := map[string]string{
        // none
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java XmlXliffCompareIT
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_xml/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_xml/
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
  - Configuration is entirely via ITS rules XML (not simple key-value params)
  - 7 bundled configurations: generic XML, RESX, Mozilla RDF, Java Properties, Android Strings, WiX Localization, Apple Stringsdict, DocBook
  - Sub-filtering supported: HTML/HTML5 content in CDATA sections
  - ITS 2.0 features: translate, locNote, terminology, LQI, provenance, domain, targetPointer, withinText
  - Custom Okapi extensions in ITS rules namespace `okapi-framework:xmlfilter-options`: escapeQuotes, escapeGT, escapeNbsp, codeFinder
  - Encoding handling: UTF-8, UTF-16LE/BE with/without BOM
  - Schema has only `simplifierRules` and `path` properties; real config is ITS rules XML

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/its/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `XMLFilterTest.java` | `okapi/filters/its/src/test/java/net/sf/okapi/filters/xml/` | 53 |
| `XMLFilterEncodingTest.java` | `okapi/filters/its/src/test/java/net/sf/okapi/filters/xml/` | 4 |
| `BundledConfigsTest.java` | `okapi/filters/its/src/test/java/net/sf/okapi/filters/xml/` | 8 |
| `HtmlSubFilterWrapperTest.java` | `okapi/filters/its/src/test/java/net/sf/okapi/filters/xml/` | 4 |
| `ParametersTest.java` | `okapi/filters/its/src/test/java/net/sf/okapi/filters/its/` | 3 |
| `ITSContentTests.java` | `okapi/filters/its/src/test/java/org/w3c/its/` | 1 |
