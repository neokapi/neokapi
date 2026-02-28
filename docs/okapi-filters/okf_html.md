# okf_html - HTML/XHTML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_html` |
| Java Class | `net.sf.okapi.filters.html.HtmlFilter` |
| MIME Types | `text/html` |
| Extensions | `.html, .htm, .xhtml` |
| Okapi Module | `html` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/html/src/test/java/`

#### HtmlSnippetsTest.java (96 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testMETA_Issue_1098` | Invalid charset in META tag throws IllegalCharsetNameException | P2 |
| 2 | `testMultipleMETA` | Multiple META tags with different charsets are handled | P1 |
| 3 | `testHref` | Href attribute extraction from anchor tags | P1 |
| 4 | `testButton` | Button element text extraction | P1 |
| 5 | `testCleanupHtmlOption` | Cleanup HTML option normalizes malformed HTML | P2 |
| 6 | `testInlineCodesStorage` | Inline codes (b, img, a) stored correctly in TextFragment | P1 |
| 7 | `testTitleInP` | Title attribute in p tag extracted as translatable | P1 |
| 8 | `testAltInImg` | Alt attribute in img tag extracted as translatable | P1 |
| 9 | `imgStartTagOnlyHandledWithWellFormedConfiguration` | img start-tag-only handled with well-formed config | P2 |
| 10 | `paramStartTagOnlyHandledWithWellFormedConfiguration` | param start-tag-only handled with well-formed config | P2 |
| 11 | `areaStartTagOnlyHandledWithWellFormedConfiguration` | area start-tag-only handled with well-formed config | P2 |
| 12 | `testNoExtractValueInInput` | Input type=hidden value not extracted | P2 |
| 13 | `testExtractValueInInput` | Input type=submit value extracted as translatable | P1 |
| 14 | `testLabelInOption` | Label attribute in option tag extracted | P2 |
| 15 | `testHtmlNonWellFormedEmptyTag` | Non-well-formed empty tags handled without crash | P2 |
| 16 | `testAddingMETAinHTML` | META encoding tag added to HTML output | P1 |
| 17 | `testAddingMETAinXHTML` | META encoding tag added to XHTML output (self-closing) | P1 |
| 18 | `testAddingMETAinXML` | META encoding tag added to XML-flavored output | P2 |
| 19 | `testMETATag1` | META keywords content extracted as translatable | P1 |
| 20 | `testPWithAttributes` | P tag with title and dir attributes extracted | P1 |
| 21 | `testLang` | lang attribute detected on elements | P1 |
| 22 | `testLangUpdate` | lang attribute updated in output for target locale | P1 |
| 23 | `testMultilangUpdate` | Multiple lang attributes updated in output | P2 |
| 24 | `testComplexEmptyElement` | Complex empty element with write/readonly/trans attributes | P2 |
| 25 | `testPWithInlines` | P with inline b and a elements produces correct codes | P1 |
| 26 | `testMETATag2` | META description content extracted | P1 |
| 27 | `testPWithInlines2` | P with inline b and img elements produces correct codes | P1 |
| 28 | `testPWithInlineTextOnly` | P with inline text-only content | P1 |
| 29 | `testTableGroups` | Table/tr/td produce group events | P1 |
| 30 | `testGroupInPara` | Embedded list inside paragraph produces groups | P1 |
| 31 | `testInput` | Input element attributes handled | P1 |
| 32 | `testCollapseWhitespaceWithPre` | Whitespace collapsed except inside pre | P1 |
| 33 | `testCollapseWhitespaceWithoutPre` | Whitespace collapsed globally | P1 |
| 34 | `testEscapedCodesInisdePre` | Escaped HTML codes inside pre element | P2 |
| 35 | `doesNotCrashOnPreservingWhitespaceForClosingPre` | No crash on whitespace-only closing pre | P3 |
| 36 | `testCdataSection` | CDATA section handling | P2 |
| 37 | `testEscapes` | Entity escape handling (&amp; etc.) | P1 |
| 38 | `testEscapedEntities` | Escaped character entities preserved in output | P1 |
| 39 | `testQuoteMode` | Quote mode with quoteModeDefined config | P2 |
| 40 | `testQuoteModeDefault` | Default quote mode behavior | P2 |
| 41 | `testNewlineDetection` | Newline type detection (CR/LF/CRLF) | P2 |
| 42 | `testCodeFinder` | Inline code finder with regex patterns | P2 |
| 43 | `testCodeFinderInAttributes` | Code finder applied to translatable attributes | P2 |
| 44 | `testNormalizeNewlinesInPre` | Newlines normalized inside pre element | P2 |
| 45 | `testSupplementalSupport` | Unicode supplemental characters (emoji etc.) | P2 |
| 46 | `testSimpleSupplementalSupport` | Simple supplemental character handling | P2 |
| 47 | `ITextUnitsInARow` | Multiple text units in sequence | P1 |
| 48 | `ITextUnitsInARowWithTwoHeaders` | Multiple text units with header elements | P1 |
| 49 | `twoITextUnitsInARowNonWellformed` | Two text units in non-well-formed HTML | P2 |
| 50 | `twoITextUnitsInARowNonWellformedWithNonWellFromedConfig` | Two text units with non-well-formed config | P2 |
| 51 | `ITextUnitName` | Text unit name set from element id | P2 |
| 52 | `ITextUnitStartedWithText` | Text unit started with text content | P1 |
| 53 | `textUnbalancedInlineTag` | Unbalanced inline tags handled gracefully | P2 |
| 54 | `textOverlapInlineTags` | Overlapping inline tags handled | P2 |
| 55 | `textWithUnquotedAttribtes` | Unquoted attribute values handled | P2 |
| 56 | `testInlineAnchorAndAmpersand` | Anchor with ampersand in query string | P2 |
| 57 | `testPAndInlineAnchorAndAmpersand` | P element with anchor containing ampersands | P2 |
| 58 | `testCERinOutput` | Character entity references preserved in output | P2 |
| 59 | `minimalCompleteHtml` | Minimal complete HTML doc extraction | P1 |
| 60 | `italicBoldEtc` | Italic, bold, del inline elements | P1 |
| 61 | `simpleTable` | Simple table extraction and roundtrip | P1 |
| 62 | `paraWithBreak` | Paragraph with br break element | P1 |
| 63 | `table` | Complex table with thead/tbody/links | P1 |
| 64 | `testComplexTable` | Complex nested table with menu and list | P1 |
| 65 | `testTextDirectionClarification` | Data-driven: dir attribute set based on target locale (7 cases) | P2 |
| 66 | `testTranslateAttribute` | translate='no' on inline span excludes content | P1 |
| 67 | `testPBlockTranslateAttribute` | translate='no' on block p excludes content | P1 |
| 68 | `testDivBlockTranslateAttribute` | translate='no' on div excludes content | P1 |
| 69 | `testNestedInlineTranslateAttribute1` | Nested span with translate=no produces correct codes | P2 |
| 70 | `testNestedInlineTranslateAttribute2_1` | Nested inline translate=no variant 1 | P2 |
| 71 | `testNestedInlineTranslateAttribute2_2` | Nested inline translate=no variant 2 | P2 |
| 72 | `testNestedInlineTranslateAttribute3` | Deeply nested translate=no with i and span | P2 |
| 73 | `testNestedInlineTranslateAttribute4` | Nested translate=no with translate=yes inside | P2 |
| 74 | `testNestedInlineTranslateAttribute5` | Complex nested translate=no/yes combinations | P2 |
| 75 | `testNestedInlineTranslateAttribute6` | Overlapping translate=no/yes on i and b | P2 |
| 76 | `testFreeMarker` | FreeMarker template syntax in HTML | P3 |
| 77 | `testPlaceholderOnlySegments` | Placeholder-only segments (br, img) produce text units | P2 |
| 78 | `testDivBlockExcludeIncludeTranslateAttribute` | div translate=no with nested translate=yes | P2 |
| 79 | `testDivBlockWithPTranslateAttribute` | div and p both translate=no excludes all | P2 |
| 80 | `testInlineTranslateNo` | Inline strong with translate=no collapses to placeholder | P2 |
| 81 | `testInlineTranslateYes` | Inline strong with translate=yes preserves inline codes | P2 |
| 82 | `testTagLowerCaseFix` | Uppercase tags with unquoted attributes preserved | P2 |
| 83 | `testInlineCdata` | CDATA as inline code when inlineCdata=true | P2 |
| 84 | `testEmptyGroupAtEnd` | Empty group element at end of content | P3 |
| 85 | `testASPXComment` | ASPX comment syntax skipped | P3 |
| 86 | `testASPXEmbeddedTag` | ASPX embedded tag syntax skipped | P3 |
| 87 | `testUlWithScriptTag` | Script tag inside list item excluded | P2 |
| 88 | `testNegativeCondition` | NOT_EQUALS condition in element rules | P2 |
| 89 | `testOkapiMarkerInText` | Okapi internal markers in text content preserved | P3 |
| 90 | `testOkapiMarkerInAttribute` | Okapi internal markers in attribute values | P3 |
| 91 | `testPropertyInTextUnitConvertedToDocumentPart` | Empty p with dir attribute produces document part | P3 |
| 92 | `testPreserveCharacterEntitiesSimple` | preserve_character_entities keeps &amp; entities | P2 |
| 93 | `testPreserveCharacterEntitiesWithInlineElements` | preserve_character_entities with inline elements | P2 |
| 94 | `testPreserveCharacterEntitiesMultipleTypes` | preserve_character_entities with lt, amp, gt, nbsp | P2 |
| 95 | `testNoPreserveCharacterEntitiesMultipleTypes` | Default behavior decodes/re-encodes entities | P2 |
| 96 | `testWithoutPreserveCharacterEntities` | Without preserve, both & and &amp; become &amp; | P2 |

#### HtmlFullFileTest.java (9 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testAllExternalFiles` | Parses all .html/.htm test files without error | P1 |
| 2 | `testNonwellformed` | Non-well-formed file parses without error | P1 |
| 3 | `testEncodingShouldBeFound` | Encoding detected from withEncoding.html (windows-1252) | P1 |
| 4 | `testEncodingShouldBeFound2` | Encoding detected from W3CHTMHLTest1.html (UTF-8) | P1 |
| 5 | `testOkapiIntro` | okapi_intro_test.html extracts "Okapi Framework" as first TU | P1 |
| 6 | `testSkippedScriptandStyleElements` | Script/style elements skipped, first TU is "First Text" | P1 |
| 7 | `testOpenTwiceWithString` | Filter can be opened twice from string | P3 |
| 8 | `testOpenTwiceWithURI` | Filter can be opened twice from URI | P3 |
| 9 | `testOpenTwiceWithStream` | Filter cannot be opened twice from same stream (NPE expected) | P3 |

#### HtmlEventTest.java (17 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testWithDefaultConfig` | Default config produces same events as well-formed config for META/lang tests | P2 |
| 2 | `testHtmlKeywordsNotExtracted` | META keywords not extracted with default non-well-formed config | P2 |
| 3 | `baseTag` | base tag href extracted as writable localizable property | P2 |
| 4 | `testMetaTagContent` | META keywords content extracted as text unit | P1 |
| 5 | `testPWithAttributes` | P with title and dir attributes produces correct event structure | P1 |
| 6 | `testLang` | lang attribute produces writable localizable property | P1 |
| 7 | `testIdOnP` | id attribute on p sets text unit name | P2 |
| 8 | `testXmlLang` | xml:lang attribute produces writable localizable property | P1 |
| 9 | `testComplexEmptyElement` | Complex element with write/readonly/trans attributes | P2 |
| 10 | `testPWithInlines` | P with b and a inline elements produces correct event tree | P1 |
| 11 | `testPWithInlineAnchorAndAmpersand` | Anchor with ampersand query string preserves entity | P2 |
| 12 | `testPWithComment` | Comment inside p becomes inline placeholder code | P2 |
| 13 | `testPWithProcessingInstruction` | PI inside p becomes inline placeholder code | P2 |
| 14 | `testMETATagWithLanguage` | META Content-Language produces language property | P1 |
| 15 | `testMETATagWithEncoding` | META Content-Type charset produces encoding property | P1 |
| 16 | `testMetaWithCharsetAttribute` | HTML5 meta charset attribute produces encoding property | P1 |
| 17 | `testPWithInlines2` | P with b and img (with alt) produces correct events | P1 |
| 18 | `testTableGroups` | Table/tr produces START_GROUP/END_GROUP events | P1 |
| 19 | `testGroupInPara` | Embedded ul inside p produces nested groups | P1 |
| 20 | `testPropertyInEmptyParagraph` | Empty p with dir attribute has non-null property parent | P3 |
| 21 | `testPreserveWhitespace` | Pre element preserves whitespace and tabs | P1 |

#### HtmlConfigurationTest.java (11 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `defaultConfiguration` | Default config has title as TEXT_UNIT_ELEMENT | P2 |
| 2 | `baseTag` | base tag href is writable localizable | P2 |
| 3 | `metaTag` | META tag rules: keywords/description translatable, content-language writable, generator readonly | P1 |
| 4 | `preserveWhiteSpace` | pre has PRESERVE_WHITESPACE rule | P2 |
| 5 | `langAndXmlLang` | lang and xml:lang are writable localizable for any element | P1 |
| 6 | `genericCodeTypes` | Element types: b=bold, i=italic, u=underlined, img=image, a=link | P2 |
| 7 | `textUnitCodeTypes` | p element type is "paragraph" | P2 |
| 8 | `collapseWhitespace` | Default preserveWhitespace=false, configurable to true | P2 |
| 9 | `testCodeFinderRules` | Code finder rules loaded from YAML config | P2 |
| 10 | `inputAttributes` | Input type-dependent translatability (hidden vs submit vs button) | P2 |
| 11 | `attributeID` | id attribute recognized as ID attribute on p | P2 |

#### HtmlConfigurationSupportTest.java (20 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `test_collapse_whitespace` | preserve_whitespace false collapses, true preserves | P1 |
| 2 | `test_PRESERVE_WHITESPACE` | PRESERVE_WHITESPACE rule on pre overrides global collapse | P1 |
| 3 | `test_GLOBAL_PRESERVE_WHITESPACE` | Global preserve_whitespace=true preserves all elements | P2 |
| 4 | `test_EXCLUDE` | EXCLUDE rule on pre skips extraction | P1 |
| 5 | `test_INCLUDE` | INCLUDE rule on b inside excluded pre re-includes | P1 |
| 6 | `test_EXCLUDE_with_positive_condition` | EXCLUDE with condition [x, EQUALS, 'true'] | P2 |
| 7 | `test_INLINE_with_positive_condition` | INLINE with condition [x, EQUALS, 'true'] | P2 |
| 8 | `test_INLINE_without_condition` | INLINE rule without matching condition falls back to block | P2 |
| 9 | `test_INLINE_with_negative_condition` | INLINE rule not applied when condition fails | P2 |
| 10 | `test_EXCLUDE_with_negative_condition` | EXCLUDE not applied when condition doesn't match | P2 |
| 11 | `test_ATTRIBUTE_ID` | ATTRIBUTE_ID rule sets text unit name from id | P2 |
| 12 | `test_idAttributes` | idAttributes config for element-specific ID attributes | P2 |
| 13 | `test_MATCHES` | MATCHES condition for regex-based exclusion | P2 |
| 14 | `test_allElementsExcept` | allElementsExcept excludes specific elements from attribute rule | P2 |
| 15 | `test_onlyTheseElements` | onlyTheseElements limits attribute rule to specific elements | P2 |
| 16 | `test_translatableAttributes_withCondition` | Conditional translatable attribute (alt when attr1=trans) | P2 |
| 17 | `test_translatableAttributes_with2ORConditions` | OR conditions for translatable attributes | P2 |
| 18 | `test_ATTRIBUTE_WRITABLE` | ATTRIBUTE_WRITABLE rule on dir produces writable property | P2 |
| 19 | `test_regex_ATTRIBUTE_WRITABLE` | Regex patterns '.+' for element and attribute rules | P2 |
| 20 | `quoteMode` | quoteModeDefined/quoteMode config for entity handling | P2 |

#### HtmlDetectBomTest.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDetectBom` | UTF-8 BOM detected in ruby.html | P2 |
| 2 | `testDetectUnicodeLittleBom` | UTF-16LE BOM detected in FFFEBOM.html | P2 |
| 3 | `testDetectAndRemoveBom` | BOM detected and removed from stream | P2 |

#### SkipEncodingDeclarationTest.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultBehaviorAddsMetaElement` | Default: META encoding declaration added to head | P1 |
| 2 | `testSkipEncodingDeclarationOmitsMetaElement` | skipEncodingDeclaration=true omits META | P2 |
| 3 | `testXHTMLSelfClosingMetaTag` | XHTML: META uses self-closing syntax | P2 |
| 4 | `testExistingEncodingDeclaration` | Existing META charset preserved in output | P2 |
| 5 | `testExistingEncodingDeclarationWithSkipEnabled` | Existing META preserved even with skip enabled | P2 |

#### ExtractionComparisionTest.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testStartDocument` | StartDocument event correct for 324.html | P2 |
| 2 | `testOpenTwice` | Filter reopen from URI works | P3 |
| 3 | `testDoubleExtractionSingle` | Double extraction roundtrip for test.html | P1 |
| 4 | `testDoubleExtraction` | Double extraction roundtrip for all test files | P1 |
| 5 | `testDoubleExtraction2` | Double extraction roundtrip for test.asp | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripHtmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripHtmlIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/html/`):
All `.html`, `.htm`, `.xhtml` files in the directory (83 files).

**Known failing files**: `98959751.html`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `HtmlXliffCompareIT` | `integration-tests/okapi/src/test/java/.../HtmlXliffCompareIT.java` | 1 |

**Known failing files**: `98959751.html`

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyHtmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyHtmlIT.java` | 2 |

**Known failing files**: `ugly_big.htm`

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `HtmlMemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../HtmlMemoryLeakTestIT.java` | 1 (main method) |

Uses `testBOM.html` with `okf_html` config.

## Test Data Files

### Unit test resources

Source: `okapi/filters/html/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `324.html` | `ExtractionComparisionTest` | StartDocument / double extraction test |
| `bad_textUnit.html` | `HtmlFullFileTest#testAllExternalFiles` | Malformed text unit test |
| `BadTags_HTMLDog.htm` | `HtmlFullFileTest#testAllExternalFiles` | Bad HTML tags |
| `burlington_ufo_center.html` | `HtmlFullFileTest#testAllExternalFiles` | Real-world HTML |
| `Carnation Chiropractic Center Inc.htm` | `HtmlFullFileTest#testAllExternalFiles` | Filename with spaces |
| `collapseWhitespaceOff.yml` | `HtmlConfigurationTest#collapseWhitespace` | Config: preserve whitespace |
| `d_Lux_MediaArts.htm` | `HtmlFullFileTest#testAllExternalFiles` | Real-world HTML |
| `d_Lux_MediaArts2.htm` | `HtmlFullFileTest#testAllExternalFiles` | Real-world HTML |
| `dummyConfiguration.yml` | `HtmlEventTest#testComplexEmptyElement` | Custom config for complex element test |
| `example.phtml` | `HtmlFullFileTest#testAllExternalFiles` | PHP HTML template |
| `ExcludeIncludeTest.html` | `HtmlFullFileTest#testAllExternalFiles` | Exclude/include test |
| `FFFEBOM.html` | `HtmlDetectBomTest#testDetectUnicodeLittleBom` | UTF-16LE BOM file |
| `form.html` | `HtmlFullFileTest#testAllExternalFiles` | HTML form |
| `Gate Openerss.htm` | `HtmlFullFileTest#testAllExternalFiles` | Filename with spaces |
| `home_big.html` | `HtmlFullFileTest#testAllExternalFiles` | Large HTML page |
| `home_crush.html` | `HtmlFullFileTest#testAllExternalFiles` | Real-world HTML |
| `home_links.html` | `HtmlFullFileTest#testAllExternalFiles` | Links-heavy page |
| `home_swing.html` | `HtmlFullFileTest#testAllExternalFiles` | Real-world HTML |
| `home_tagcloud_hell.html` | `HtmlFullFileTest#testAllExternalFiles` | Tag cloud HTML |
| `home.htm` | `HtmlFullFileTest#testAllExternalFiles` | Basic home page |
| `list-block-only.html` | `HtmlFullFileTest#testAllExternalFiles` | List block test |
| `mandrake_fonts.html` | `HtmlFullFileTest#testAllExternalFiles` | Font-heavy page |
| `msg00058.html` | `HtmlFullFileTest#testAllExternalFiles` | Email HTML |
| `nonwellformed.specialtest` | `HtmlFullFileTest#testNonwellformed` | Non-well-formed HTML |
| `okapi_intro_test.html` | `HtmlFullFileTest#testOkapiIntro` | Okapi intro page (windows-1252) |
| `quoteMode.yml` | `HtmlSnippetsTest#testQuoteMode` | Quote mode config |
| `quoteModeDefault.yml` | `HtmlSnippetsTest#testQuoteModeDefault` | Default quote mode config |
| `ruby.html` | `HtmlDetectBomTest#testDetectBom` | UTF-8 BOM file |
| `sanitizer.html` | `HtmlFullFileTest#testAllExternalFiles` | Sanitizer test |
| `simpleSimpleTest.html` | `HtmlFullFileTest#testAllExternalFiles` | Simple HTML |
| `simpleTest.html` | `HtmlFullFileTest#testAllExternalFiles` | Simple HTML |
| `supplementals.html` | `HtmlFullFileTest#testAllExternalFiles` | Supplemental Unicode chars |
| `table-block-only.html` | `HtmlFullFileTest#testAllExternalFiles` | Table block test |
| `TeacherXpress.htm` | `HtmlFullFileTest#testAllExternalFiles` | Real-world HTML |
| `test.asp` | `ExtractionComparisionTest#testDoubleExtraction2` | ASP file roundtrip |
| `test.html` | `ExtractionComparisionTest#testDoubleExtractionSingle` | Basic roundtrip test |
| `testBOM.html` | `HtmlFullFileTest#testAllExternalFiles` | BOM test file |
| `testConfiguration1.yml` | config tests | Custom test config |
| `testStyleScriptStylesheet.html` | `HtmlFullFileTest#testSkippedScriptandStyleElements` | Script/style/stylesheet |
| `ugly_big.htm` | `HtmlFullFileTest#testAllExternalFiles` | Large ugly HTML |
| `ul-snippet.html` | `HtmlFullFileTest#testAllExternalFiles` | UL snippet |
| `UTF8WithBOM.html` | `HtmlFullFileTest#testAllExternalFiles` | UTF-8 BOM |
| `W3CHTMHLTest1.html` | `HtmlFullFileTest#testEncodingShouldBeFound2` | W3C encoding test |
| `We The People Foundation.htm` | `HtmlFullFileTest#testAllExternalFiles` | Filename with spaces |
| `withCodeFinderRules.yml` | `HtmlConfigurationTest#testCodeFinderRules` | Code finder rules config |
| `withEncoding.html` | `HtmlFullFileTest#testEncodingShouldBeFound` | Encoding detection (windows-1252) |
| `World'sWorstWebsite.htm` | `HtmlFullFileTest#testAllExternalFiles` | Filename with apostrophe |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/html/`

| File | Type | Purpose |
|------|------|---------|
| `324.html` | roundtrip | Basic HTML roundtrip |
| `advanced_bold_font_color.html` | roundtrip | Advanced inline formatting |
| `api_simple.html` | roundtrip | API documentation page |
| `bad_textUnit.html` | roundtrip | Malformed text units |
| `burlington_ufo_center.html` | roundtrip | Real-world content |
| `Dachseite-Startseite.html` | roundtrip | German content page |
| `emoji1.html` | roundtrip | Emoji content |
| `example.phtml` | roundtrip | PHP template |
| `ExcludeIncludeTest.html` | roundtrip | Exclude/include logic |
| `form.html` / `form2.html` | roundtrip | HTML forms |
| `France_Culture_fr.html` | roundtrip | French content |
| `Gate Openerss.htm` | roundtrip | Filename with spaces |
| `home_big.html` | roundtrip | Large page |
| `home_crush.html` / `home_links.html` / `home_swing.html` | roundtrip | Various real pages |
| `Issue488/98959751.html` | roundtrip | Known failing file |
| `Issue488/simple_issue488.html` | roundtrip | Issue 488 simple repro |
| `Issue493/Dachseite-Startseite.html` | roundtrip | Issue 493 repro |
| `issue1004/1004.html` | roundtrip | Issue 1004 repro |
| `malformed-table.html` | roundtrip | Malformed table HTML |
| `merged_codes.html` | roundtrip | Merged inline codes |
| `sanitizer.html` | roundtrip | HTML sanitization |
| `segmentation_test.html` | roundtrip | Segmentation rules |
| `simple_bold*.html` (12 files) | roundtrip | Various bold/italic/underline combinations |
| `simple_em*.html` (3 files) | roundtrip | Em/emphasis variants |
| `simple_font*.html` (4 files) | roundtrip | Font style variants |
| `simple_highlight.html` | roundtrip | Highlight styling |
| `simple_italic_bold.html` | roundtrip | Italic+bold |
| `simple_italics.html` | roundtrip | Italics only |
| `simple_link.html` | roundtrip | Link element |
| `simple_lower_case.html` | roundtrip | Lowercase tags |
| `simple_many_bold.html` | roundtrip | Many bold elements |
| `simple_shadow.html` | roundtrip | Shadow style |
| `simple_strike*.html` (2 files) | roundtrip | Strikethrough variants |
| `simple_strong.html` | roundtrip | Strong element |
| `simple_styles*.html` (5 files) | roundtrip | CSS style variants |
| `simple_subscript.html` | roundtrip | Subscript |
| `simple_superscript*.html` (5 files) | roundtrip | Superscript variants |
| `simple_underline.html` | roundtrip | Underline |
| `simple_upper_case*.html` (6 files) | roundtrip | Uppercase tag variants |
| `simple.html` / `simple2.html` | roundtrip | Basic HTML |
| `supplementals.html` | roundtrip | Unicode supplementals |
| `td.html` | roundtrip | Table cell |
| `TeacherXpress.htm` | roundtrip | Real-world page |
| `testBOM.html` | roundtrip / memory leak | BOM handling |
| `testStyleScriptStylesheet.html` | roundtrip | Script/style skip |
| `ugly_big.htm` | roundtrip (simplifier failing) | Large malformed HTML |
| `W3CHTMHLTest1.html` | roundtrip | W3C test |
| `World'sWorstWebsite.htm` | roundtrip | Apostrophe in filename |
| `xhtml/chapter01.xhtml` | roundtrip | XHTML document |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| None needed | Extensive test files already exist | Unit and integration tests provide comprehensive coverage |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/html/src/test/resources/*.html okapi-testdata/okf_html/
cp okapi/filters/html/src/test/resources/*.htm okapi-testdata/okf_html/
cp okapi/filters/html/src/test/resources/*.specialtest okapi-testdata/okf_html/
cp okapi/filters/html/src/test/resources/*.asp okapi-testdata/okf_html/
cp okapi/filters/html/src/test/resources/*.phtml okapi-testdata/okf_html/
cp okapi/filters/html/src/test/resources/*.yml okapi-testdata/okf_html/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/html/* okapi-testdata/okf_html/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_html`

Build tag: `//go:build integration`

#### html_test.go - Extraction Tests

```go
func TestExtract_BasicElements(t *testing.T) {
    // Table-driven: maps 1:1 to Java HtmlSnippetsTest inline tests
    tests := []struct {
        name     string
        input    string // inline HTML snippet
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "minimal complete html",
            input: "<html><body><p>Test1<br/>Test2</p></body></html>",
            wantTexts: []string{"Test1", "Test2"},
            javaRef: "HtmlSnippetsTest#minimalCompleteHtml",
        },
        {
            name:  "simple table",
            input: "<table><tr><td>Test</td></tr></table>",
            wantTexts: []string{"Test"},
            javaRef: "HtmlSnippetsTest#simpleTable",
        },
        {
            name:  "paragraph with break",
            input: "<p>Sentence 1.<br/>Sentence 2.<p>Another para.",
            wantTexts: []string{"Sentence 1.", "Sentence 2.", "Another para."},
            javaRef: "HtmlSnippetsTest#paraWithBreak",
        },
        // ... additional test cases from HtmlSnippetsTest
    }
}

func TestExtract_InlineElements(t *testing.T) {
    // Maps to HtmlSnippetsTest: italic/bold, translate attributes, inline codes
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        javaRef  string
    }{
        // ... test cases for inline handling
    }
}

func TestExtract_MetaTags(t *testing.T) {
    // Maps to HtmlSnippetsTest/HtmlEventTest META tag tests
    tests := []struct {
        name    string
        input   string
        wantTexts []string
        javaRef string
    }{
        // ... META tag extraction cases
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_WhitespaceHandling(t *testing.T) {
    // Maps to HtmlConfigurationSupportTest whitespace tests
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string // expected extracted texts
        javaRef string
    }{
        {
            name:   "collapse whitespace",
            params: map[string]any{"preserve_whitespace": false},
            input:  "<p> t1  \nt2  </p>",
            want:   []string{"t1 t2"},
            javaRef: "HtmlConfigurationSupportTest#test_collapse_whitespace",
        },
        // ... more whitespace cases
    }
}

func TestConfig_ExcludeInclude(t *testing.T) {
    // Maps to HtmlConfigurationSupportTest EXCLUDE/INCLUDE tests
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        // ... exclude/include cases
    }
}

func TestConfig_TranslateAttribute(t *testing.T) {
    // Maps to HtmlSnippetsTest translate='no'/'yes' tests
    tests := []struct {
        name   string
        input  string
        want   []string
        javaRef string
    }{
        // ... translate attribute cases
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripHtmlIT
    testFiles := []string{
        // 83 files from testdata/roundtrip/
    }
    knownFailing := map[string]string{
        "98959751.html": "Known failing in Java (Issue488)",
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java HtmlXliffCompareIT
    // Verifies Part structure matches expected XLIFF output
    knownFailing := map[string]string{
        "98959751.html": "Known failing in Java",
    }
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_html/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_html/
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
  - HTML filter has two configurations: well-formed (XHTML-like) and non-well-formed (default)
  - The `translate` attribute (HTML5) controls extraction at both block and inline level
  - `preserve_character_entities` option affects entity handling in output
  - `skipEncodingDeclaration` controls META charset tag injection
  - Whitespace collapse is default; `preserve_whitespace` and per-element `PRESERVE_WHITESPACE` rule override
  - Script and style elements are skipped by default
  - Known failing: `98959751.html` (roundtrip), `ugly_big.htm` (simplifier)
  - `dir` attribute is updated based on target locale direction (RTL/LTR)
  - ASPX comments and embedded tags are handled as non-translatable

## Current Go Coverage

### Bridge Tests (`core/plugin/bridge/filters/okf_html/`)

Annotated Java methods with `// okapi:` mapping:

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `HtmlSnippetsTest#minimalCompleteHtml` | `TestExtract_MinimalHTML` | Mapped |
| `HtmlSnippetsTest#testInlineCodesStorage` | `TestExtract_InlineCodes` | Mapped |
| `HtmlFullFileTest#testSkippedScriptandStyleElements` | `TestExtract_ScriptStyle` | Mapped |
| `HtmlSnippetsTest#testPWithInlines2` | `TestExtract_PWithInlines` | Mapped |
| `HtmlSnippetsTest#paraWithBreak` | `TestExtract_ParaWithBreak` | Mapped |
| `HtmlSnippetsTest#testEscapes` | `TestExtract_Escapes` | Mapped |
| `HtmlSnippetsTest#testAltInImg` | `TestExtract_Attributes` | Mapped |
| `HtmlSnippetsTest#testTitleInP` | `TestExtract_Attributes` | Mapped |
| `HtmlSnippetsTest#testTableGroups` | `TestExtract_TableGroups` | Mapped |
| `HtmlSnippetsTest#ITextUnitsInARowWithTwoHeaders` | `TestExtract_Headers` | Mapped |
| `HtmlSnippetsTest#testGroupInPara` | `TestExtract_GroupInPara` | Mapped |
| `HtmlSnippetsTest#testMETATag1` | `TestExtract_META` | Mapped |
| `HtmlSnippetsTest#italicBoldEtc` | `TestExtract_ItalicBold` | Mapped |
| `HtmlDetectBomTest#testDetectBom` | `TestExtract_BOM` | Mapped |
| `HtmlSnippetsTest#testSupplementalSupport` | `TestExtract_Supplemental` | Mapped |
| `HtmlSnippetsTest#testCollapseWhitespaceWithPre` | `TestExtract_WhitespacePre` | Mapped |
| `HtmlSnippetsTest#testCollapseWhitespaceWithoutPre` | `TestExtract_WhitespaceNoPre` | Mapped |
| `RoundTripHtmlIT` | `TestRoundTrip` | Mapped |

### Native Tests (`core/formats/html/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `HtmlSnippetsTest#minimalCompleteHtml` | `TestReadBasicElements` | Mapped |
| `HtmlSnippetsTest#ITextUnitsInARowWithTwoHeaders` | `TestReadBasicElements` | Mapped |
| `HtmlSnippetsTest#testPWithInlines` | `TestReadBasicElements` | Mapped |
| `HtmlSnippetsTest#testHref` | `TestReadBasicElements` | Mapped |
| `HtmlSnippetsTest#paraWithBreak` | `TestReadBasicElements` | Mapped |
| `HtmlFullFileTest#testSkippedScriptandStyleElements` | `TestReadScriptStyle` | Mapped |
| `HtmlConfigurationTest#defaultConfiguration` | `TestReadConfig` | Mapped |

**Coverage**: ~18 of 177 Surefire methods have bridge `// okapi:` annotations (~10%).

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/html/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `HtmlSnippetsTest.java` | `okapi/filters/html/src/test/java/.../` | 96 |
| `HtmlFullFileTest.java` | `okapi/filters/html/src/test/java/.../` | 9 |
| `HtmlEventTest.java` | `okapi/filters/html/src/test/java/.../` | 21 |
| `HtmlConfigurationTest.java` | `okapi/filters/html/src/test/java/.../` | 11 |
| `HtmlConfigurationSupportTest.java` | `okapi/filters/html/src/test/java/.../` | 20 |
| `HtmlDetectBomTest.java` | `okapi/filters/html/src/test/java/.../` | 3 |
| `SkipEncodingDeclarationTest.java` | `okapi/filters/html/src/test/java/.../` | 5 |
| `ExtractionComparisionTest.java` | `okapi/filters/html/src/test/java/.../integration/` | 5 |
