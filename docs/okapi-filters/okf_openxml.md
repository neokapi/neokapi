# okf_openxml - OpenXML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_openxml` |
| Java Class | `net.sf.okapi.filters.openxml.OpenXMLFilter` |
| MIME Types | `text/xml` |
| Extensions | `.docx, .docm, .dotx, .dotm, .xlsx, .xlsm, .xltx, .xltm, .pptx, .pptm, .ppsx, .ppsm, .potx, .potm, .vsdx, .vsdm` |
| Okapi Module | `openxml` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/openxml/src/test/java/`

This is the **largest filter** with 38 test files and 412+ @Test methods. The key test classes are summarized below; helper/utility classes are listed at the end.

#### OpenXMLTest.java (77 @Test methods)

Core extraction and configuration tests covering all three document types (Word, Excel, PowerPoint).

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testReorderedZipPackage` | ZIP package with reordered entries | P1 |
| 2 | `testSlideReordering` | PowerPoint slide reordering | P1 |
| 3 | `testPPTXDocProperties` | PPTX document properties extraction | P1 |
| 4 | `testPPTXIgnoreDocProperties` | PPTX doc properties skipped when disabled | P2 |
| 5 | `testPPTXComments` | PPTX comments extraction | P1 |
| 6 | `testPPTXIgnoreComments` | PPTX comments skipped when disabled | P2 |
| 7 | `testXLSXOnlyExtractStringsNotNumbers` | Excel: strings extracted, numbers skipped | P1 |
| 8 | `testXLSXOrdering` | Excel text unit ordering | P1 |
| 9 | `testXLSXExcludeAllColumns` | Excel: exclude all columns from extraction | P2 |
| 10 | `testXLSXTranslateSheetNames` | Excel sheet names extracted when enabled | P2 |
| 11 | `testPartialExclusionFromColumns` | Partial column exclusion in Excel | P2 |
| 12 | `documentsWithAbsentSharedStringsProcessed` | Excel without shared strings | P2 |
| 13 | `testSmartQuotes` | Smart quote handling | P1 |
| 14 | `testTabAsCharacter` | Tab as character (not tag) | P2 |
| 15 | `testTabAsTag` | Tab as inline tag | P2 |
| 16 | `testLineBreakAsCharacter` | Line break as character | P2 |
| 17 | `testLineBreakAsTag` | Line break as inline tag | P2 |
| 18 | `standardBackgroundForegroundAndFontColorsExcluded` | Color-based exclusion of text | P2 |
| 19 | `cellAndInlineStylesCorrelatedForColorsExclusion` | Cell and inline styles for color exclusion | P2 |
| 20 | `testHiddenTextExtraction` | Data-driven: Word hidden text with/without translation | P1 |
| 21 | `testHiddenTablesByApachePOIWithoutTranslation` | Hidden tables not translated | P2 |
| 22 | `testHiddenTablesByApachePOIWithTranslation` | Hidden tables translated | P2 |
| 23 | `testDocxStylesInclude` | Word style inclusion mode | P1 |
| 24 | `testDocxStylesExclude` | Word style exclusion mode | P1 |
| 25 | `testDocxStylesIncludeWithExcludedColor` | Style include + color exclude combined | P2 |
| 26 | `testDocxHighlightsExclude` | Highlighted text excluded | P2 |
| 27 | `testDocxHighlightsExcludeBlock` | Highlighted block excluded | P2 |
| 28 | `testDocxHighlightsInclude` | Highlighted text included | P2 |
| 29 | `testDocxColorExclude` | Font color exclusion | P2 |
| 30 | `testDocxColorExcludeBlock` | Font color block exclusion | P2 |
| 31 | `explicitStylesInclusionSupported` | Explicit style inclusion | P2 |
| 32 | `explicitHighlightedColorsInclusionSupported` | Explicit highlight color inclusion | P2 |
| 33 | `codeDisplayTextContainsExcludedRunContentForExtractedWordDocuments` | Code display text in Word | P2 |
| 34 | `codeDisplayTextContainsExcludedRunContentForExtractedPowerpointDocuments` | Code display text in PPTX | P2 |
| 35 | `codeDisplayTextContainsExcludedRunContentForExtractedExcelDocuments` | Code display text in XLSX | P2 |
| 36 | `exclusionByDefaultHighlightColorsSupported` | Default highlight color exclusion | P2 |
| 37 | `exclusionByDefaultFontColorsSupported` | Default font color exclusion | P2 |
| 38 | `testOkapiEncryptedDataException` | Encrypted documents throw OkapiEncryptedDataException | P1 |
| 39 | `testLibreOfficeDocWithAbsolutePartPaths` | LibreOffice documents with absolute paths | P2 |
| 40 | `extractsExternalHyperlinks` | External hyperlink extraction | P1 |
| 41 | `extractsNestedContentInTheExpectedOrder` | Nested content ordering | P1 |
| 42 | `extractsComplexFieldsWithRefinedBoundaries` | Complex field boundary detection | P1 |
| 43 | `extractsComplexFieldsWithRefinedBoundariesFromMinifiedDocument` | Minified document complex fields | P2 |
| 44 | `extractsNestedComplexFieldsWithRefinedBoundaries` | Nested complex field boundaries | P1 |
| 45 | `complexFieldsMultipleInstructionsHandled` | Multiple instructions in complex fields | P2 |
| 46 | `extractsStructuralDocumentTagsAsRunContainers` | SDT elements as run containers | P1 |
| 47 | `emptyStructuralDocumentTagContentHandled` | Empty SDT content | P2 |
| 48 | `extractsNoneReorderedNotesAndComments` | Notes/comments without reordering | P1 |
| 49 | `extractsReorderedNotesAndComments` | Notes/comments with reordering | P1 |
| 50 | `extractsReorderedNotesAndCommentsWithNoCommentsPart` | Reordered notes, no comments part | P2 |
| 51 | `extractsMovedInlineContent` | Moved inline content (revisions) | P1 |
| 52 | `extractsMovedParagraphContent` | Moved paragraph content (revisions) | P1 |
| 53 | `extractsMovedContent` | General moved content | P1 |
| 54 | `extractsInStrictMode` | OOXML strict mode support | P1 |
| 55 | `extractsWithOptimisedWordStyles` | Optimised word style processing | P2 |
| 56 | `extractsWithRunFontsHintRespect` | RunFonts hint attribute handling | P2 |
| 57 | `extractsWithImplicitFormatting` | Implicit formatting extraction | P2 |
| 58 | `extractsWithAcceptedDeletedParagraphMarkRevision` | Accepted deleted paragraph mark | P1 |
| 59 | `insertedAndDeletedTableRowRevisionsAccepted` | Table row revision acceptance | P1 |
| 60 | `extractsUnmergedRunsWithDifferentRunFonts` | Unmerged runs with font differences | P2 |
| 61 | `extractsRunsWithMinifiedRunProperties` | Minified run properties | P2 |
| 62 | `extractsRunsFollowedByEmptyParagraph` | Runs before empty paragraphs | P2 |
| 63-65 | `extractsTextEncodingOkapiMarkers{Pptx/Docx/Xlsx}` | Okapi encoding markers per format | P2 |
| 66 | `breakReplacementsInFieldsWithParagraphsExtracted` | Break replacements in fields | P2 |
| 67 | `nonComplexScriptAndComplexScriptPropertiesIdentificationAndMergeImproved` | Complex/non-complex script handling | P2 |
| 68 | `fontsInfoExtracted` | Font information extraction | P2 |
| 69 | `numberingLevelTextExtracted` | Word numbering level text | P2 |
| 70 | `wordFontColorsIgnored` | Word font color ignorance thresholds | P2 |
| 71 | `whitespaceStylesIgnored` | Whitespace style ignorance | P2 |
| 72 | `phoneticGuideAndBaseTextsNested` | Phonetic guide (ruby) text | P2 |
| 73 | `defaultWordRunFormattingConditionallyOptimisedForWordDocuments` | Default run formatting optimization | P2 |
| 74 | `extractionWithCodeFindingSupported` | Code finder in Word documents | P2 |

#### OpenXMLRoundTripTest.java (111 @Test methods)

Comprehensive roundtrip tests using DataProvider pattern for many document variations.

Key method groups:
- `testHiddenTablesWithFormula` - Hidden tables with formulas
- `testHiddenMergeCells` - Hidden merged cells
- `testPhoneticRunPropertyForAsianLanguages` - Asian phonetic properties
- `testExternalHyperlinks` - External hyperlink roundtrip
- `testClarifiablePart` - Clarifiable document parts
- `roundTripsNestedContent` - Nested content roundtrip
- `roundTripsLongRelationshipId` - Long relationship IDs
- `roundTripsWithRefinedComplexFieldsEndBoundaries` - Complex fields
- `roundTripsWithStructuralDocumentTags` - SDT roundtrip
- `roundTripsWithReorderedNotesAndComments` - Notes/comments reordering
- `roundTripsInStrictMode` - OOXML strict mode
- `roundTripsWithOptimisedWordProcessingStyles` - Style optimization
- `testAdditionalDocumentTypes` - POTX, DOTX, XLTX, PPSX types
- `testMultilineFormula` - Multiline formulas
- `roundtripsWithStyleOptimisationApplied` - Style optimization applied
- `roundtripsWithAggressiveCleanup` - Aggressive cleanup mode
- `roundtripsWithRunFontsHintRespect` - Font hints
- `roundtripsWithRunFontsDifferences` - Font differences

Plus ~90 data-driven roundtrip tests covering DOCX, XLSX, PPTX, VSDX files.

#### OpenXmlPptxTest.java (31 @Test methods)

PowerPoint-specific tests:
- `testMaster` - Master slide processing
- `extractsHiddenSlides` / `doesNotExtractHiddenSlides` - Hidden slide handling
- `hiddenSlideRelatedPartsNotExtracted` - Hidden slide parts
- `doesNotExtractEmptyFormatting` - Empty formatting skip
- `extractsWithoutAggressivelyCleanedUpFormatting` / `extractsWithAggressivelyCleanedUpFormatting` - Cleanup modes
- `testIncludeSlidesYes/Charts/SmartArt/No` - Slide inclusion controls
- `testFormattingsPptx` - PPTX formatting extraction
- `fontsInfoExtracted` - Font info in PPTX
- `testFormattedHyperlinkPptx` - Hyperlink formatting
- `testRunMergingWithBaselineAttribute` / `FromMaster` - Run merging
- `testExternalRelationships` - External relationships
- `endParagraphPropertiesDoesNotTriggerAdditionalCodesCreation` - Paragraph properties
- `documentPropertiesTranslatedAndReordered` / `NotTranslatedButReordered` - Doc properties
- `diagramDataTranslatedAndReordered` / `NotTranslatedButReordered` - Diagram data
- `chartsTranslatedAndReordered` / `NotTranslatedButReordered` - Chart handling
- `relationshipsReordered` - Relationship reordering
- `notesTranslatedAndReordered` - Notes handling
- `graphicMetadataExtracted` - Graphic metadata
- `cachedChartStringsExtracted` / `cachedChartNumbersExtracted` - Cached chart data
- `lineBreaksExtractedAsTags` - Line break handling
- `extractionWithCodeFindingSupported` - Code finder in PPTX

#### OpenXmlXlsxTest.java (34 @Test methods)

Excel-specific tests:
- `worksheetRowsAndColumnsIdentificationClarified` - Row/column identification
- `testTextFields` / `testTextFieldsHidden` - Text fields
- `testExcelWorksheetTransUnitProperty` - Worksheet TU properties
- `testSmartArt` / `testSmartArtHidden` - Smart art
- `testSheetNamesHiddenExclude` / `testSheetNamesHiddenInclude` - Sheet name visibility
- `groupsOfWorksheetsAndRowsExtracted` - Worksheet/row groups
- `testFormattings` - Excel formatting
- `fontsInfoExtracted` - Font info in XLSX
- `maxWidthAndSizeUnitPropertiesSpecified` - Column properties
- `sourceColumnsIdentifiedAndExtractedAsTargetColumns` - Source/target columns
- `rowsExcluded` / `columnsExcluded` / `rowsAndColumnsExcluded` - Exclusion
- `metadataMarked` / `mergedCellsAsMetadataMarked` - Metadata marking
- `booleansAndNumbersExtractedAsMetadata` - Boolean/number metadata
- `inlineStringsExtracted` - Inline strings
- `valuesFromCellsOfStringTypeWithEmptyFormulasTreatedAsInlineStrings` - Empty formula cells
- `excelDocumentRevisionsAcceptedWithAllReviewed` / `NotAcceptedWithNotAllReviewed` - Revisions
- `colorExclusionConsideredForThemes` - Theme color exclusion
- `sourceColumnCellStylesTreatedForExclusion` - Cell style exclusion
- `sameCellDataNotCopied` - Cell data deduplication
- `tintedColorsHandlingClarified` - Tinted color handling
- `joinedSourceAndTargetColumnsExtractionHandled` - Joined columns
- `cellsWithOmittedValuesSupported` - Omitted cell values
- `explicitlySpecifiedWorksheetsExtractionAllowed` - Explicit worksheet selection
- `sourceToTargetColumnExtractionWithHiddenContentClarified` - Hidden content
- `explicitlySpecifiedCellsExtractionAllowed` - Explicit cell selection
- `extractionWithCodeFindingSupported` - Code finder in XLSX

#### Other Test Classes (summarized)

| Class | @Test Count | Purpose |
|-------|-------------|---------|
| `TestBlockParser.java` | 37 | Block-level XML parsing |
| `TestStyledTextUnitWriter.java` | 26 | Styled text unit output |
| `TestParagraphSimplifier.java` | 21 | Paragraph simplification logic |
| `ColorValueTest.java` | 10 | Color value parsing (RGB, theme, tint) |
| `ConditionalParametersTest.java` | 6 | Conditional parameters serialization |
| `TestRelationships.java` | 5 | Relationship parsing |
| `SubfilteringTest.java` | 5 | Subfilter integration |
| `WorksheetTest.java` | 4 | Worksheet-level tests |
| `PresentationFragmentsTest.java` | 4 | Presentation fragments |
| `OpenXmlFormattingTest.java` | 4 | Formatting tests |
| `ExcelWorksheetTransUnitPropertyTest.java` | 4 | Excel TU properties |
| `TestXMLSerializer.java` | 3 | XML serialization |
| `StringItemParserTest.java` | 3 | String item parsing |
| `SerialDateTimeTest.java` | 3 | Serial date/time conversion |
| `WorksheetConfigurationsTest.java` | 2 | Worksheet configurations |
| `PartPathTest.java` | 2 | Part path resolution |
| `OpenXMLZipFullFileTest.java` | 2 | Full ZIP file processing |
| `OpenXmlRoundtripPageBreakTest.java` | 2 | Page break roundtrip |
| `OpenXMLFilterLineSeparatorReplacementTest.java` | 2 | Line separator replacement |
| `OpenXMLConfigurationTest.java` | 2 | Configuration tests |
| `ExcelStyleDefinitionsTest.java` | 2 | Excel style definitions |
| `TestContentTypes.java` | 1 | Content type parsing |
| `PresentationNotesStyleDefinitionsTest.java` | 1 | Presentation notes styles |
| `OpenXMLSnippetsTest.java` | 1 | Snippet test |
| `OpenXmlRoundtripSoftLineBreaksDoNotTranslateTest.java` | 1 | Soft line break roundtrip |
| `OpenXmlRoundtripPptxRemoveEmbeddedTest.java` | 1 | Remove embedded Excel |
| `OpenXmlRoundtripPptxMastersTest.java` | 1 | PPTX masters roundtrip |
| `OpenXMLRoundtripLineSeparatorReplacementTest.java` | 1 | Line separator replacement roundtrip |
| `OpenXMLRoundtripAddTabAsCharTest.java` | 1 | Tab as character roundtrip |
| `OpenXMLRepetitionTest.java` | 1 | Repetition handling |
| `OpenXMLDefaultConfigRoundTripTest.java` | 1 | Default config roundtrip |

#### Helper Classes (no @Test)

- `AbstractOpenXMLRoundtripTest.java` - Base class for roundtrip tests
- `OpenXMLTestHelpers.java` - Test utility methods
- `OpenXMLPackageDiffer.java` - ZIP package comparison
- `XMLFactoriesForTest.java` - XML factory setup
- `Translation.java`, `PigLatinTranslation.java`, `TagPeekTranslation.java`, `CodePeekTranslation.java` - Translation implementations for testing
- `ConditionalParametersBuilder.java` - Parameter builder
- `ParagraphSimplifier.java` - Simplifier implementation

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripOpenXmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripOpenXmlIT.java` | 2 |

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `OpenXmXliffCompareIT` | `integration-tests/okapi/src/test/java/.../OpenXmXliffCompareIT.java` | N/A |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyOpenXmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyOpenXmlIT.java` | N/A |

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/openxml/src/test/resources/`

The OpenXML filter has an extensive set of test documents. Key categories:

**Word (DOCX):**
- Various DOCX files for styles, hidden text, hyperlinks, complex fields, SDT, revisions, formatting, etc.

**Excel (XLSX):**
- Worksheet extraction, column/row exclusion, sheet names, formulas, formatting, source/target columns, color exclusion

**PowerPoint (PPTX):**
- Slides, masters, hidden slides, comments, notes, charts, diagrams, SmartArt, formatting

**Other formats:**
- `.dotx` (Word template), `.potx` (PowerPoint template), `.xltx` (Excel template), `.ppsx` (PowerPoint show), `.vsdx`/`.vsdm` (Visio)

### Synthetic test data to create

None needed - most extensive test data set of any filter.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources (DOCX, XLSX, PPTX, and other Office formats)
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.docx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.docm okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.dotx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.xlsx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.xlsm okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.xltx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.pptx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.pptm okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.ppsx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.potx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.vsdx okapi-testdata/okf_openxml/
cp okapi/filters/openxml/src/test/resources/net/sf/okapi/filters/openxml/*.vsdm okapi-testdata/okf_openxml/

# Integration test resources
cp integration-tests/okapi/src/test/resources/openxml/*.docx okapi-testdata/okf_openxml/roundtrip/
cp integration-tests/okapi/src/test/resources/openxml/*.xlsx okapi-testdata/okf_openxml/roundtrip/
cp integration-tests/okapi/src/test/resources/openxml/*.pptx okapi-testdata/okf_openxml/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_openxml`

Build tag: `//go:build integration`

#### openxml_test.go - Core Extraction Tests

```go
func TestExtract_WordDocument(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "smart_quotes",
            input: "SmartQuotes.docx",
            javaRef: "OpenXMLTest#testSmartQuotes",
        },
        {
            name:  "hidden_text_not_extracted",
            input: "HiddenText.docx",
            params: map[string]any{"bPreferenceTranslateWordHidden": false},
            javaRef: "OpenXMLTest#testHiddenTextExtraction",
        },
        {
            name:  "external_hyperlinks",
            input: "ExternalHyperlinks.docx",
            params: map[string]any{"bExtractExternalHyperlinks": true},
            javaRef: "OpenXMLTest#extractsExternalHyperlinks",
        },
    }
}

func TestExtract_ExcelDocument(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "strings_not_numbers",
            input: "TestXLSX.xlsx",
            javaRef: "OpenXMLTest#testXLSXOnlyExtractStringsNotNumbers",
        },
        {
            name:  "sheet_names",
            input: "SheetNames.xlsx",
            params: map[string]any{"bPreferenceTranslateExcelSheetNames": true},
            javaRef: "OpenXMLTest#testXLSXTranslateSheetNames",
        },
    }
}

func TestExtract_PowerPointDocument(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "doc_properties",
            input: "DocProperties.pptx",
            params: map[string]any{"translatePowerpointDocProperties": true},
            javaRef: "OpenXMLTest#testPPTXDocProperties",
        },
        {
            name:  "hidden_slides",
            input: "HiddenSlides.pptx",
            params: map[string]any{"bPreferenceTranslatePowerpointHidden": true},
            javaRef: "OpenXmlPptxTest#extractsHiddenSlides",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_StyleExclusion(t *testing.T) {
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "styles_include",
            params: map[string]any{"bInExcludeMode": false},
            javaRef: "OpenXMLTest#testDocxStylesInclude",
        },
        {
            name:   "styles_exclude",
            params: map[string]any{"bInExcludeMode": true},
            javaRef: "OpenXMLTest#testDocxStylesExclude",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java OpenXMLRoundTripTest (111 data-driven tests)
    testFiles := []string{
        // DOCX files
        "SmartQuotes.docx", "NestedContent.docx", "ComplexFields.docx",
        // XLSX files
        "TestXLSX.xlsx",
        // PPTX files
        "DocProperties.pptx",
        // Additional types
        "Template.dotx", "Template.potx", "Template.xltx",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java OpenXmXliffCompareIT
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_openxml/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_openxml/
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
  - **Largest filter** - 412+ @Test methods, 38 test files
  - Handles three distinct document types: Word (DOCX), Excel (XLSX), PowerPoint (PPTX) plus Visio (VSDX)
  - ZIP-based package format (OOXML)
  - Extensive parameter surface: doc properties, comments, notes, hidden text, sheet names, style/highlight/color inclusion/exclusion modes, slide filtering, aggressive cleanup, code finder, etc.
  - nFileType parameter determines document type: MSWORD, MSEXCEL, MSWORDDOCPROPERTIES, MSPOWERPOINTCOMMENTS
  - Supports OOXML strict mode (ISO 29500) in addition to transitional
  - Complex fields have refined boundary detection for TOC, hyperlinks, etc.
  - Structural Document Tags (SDT/content controls) treated as run containers
  - Revision acceptance: automatically accepts insertions/deletions including table rows
  - Excel source/target column mapping for bilingual spreadsheets
  - Font color and highlight color exclusion with threshold-based ignorance
  - Word style optimization can simplify inline formatting codes
  - Encrypted documents throw OkapiEncryptedDataException
  - Line separator replacement configurable (sPreferenceLineSeparatorReplacement)

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/openxml/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `OpenXMLTest.java` | `okapi/filters/openxml/src/test/java/.../` | 77 |
| `OpenXMLRoundTripTest.java` | `okapi/filters/openxml/src/test/java/.../` | 111 |
| `OpenXmlPptxTest.java` | `okapi/filters/openxml/src/test/java/.../` | 31 |
| `OpenXmlXlsxTest.java` | `okapi/filters/openxml/src/test/java/.../` | 34 |
| `TestBlockParser.java` | `okapi/filters/openxml/src/test/java/.../` | 37 |
| `TestStyledTextUnitWriter.java` | `okapi/filters/openxml/src/test/java/.../` | 26 |
| `TestParagraphSimplifier.java` | `okapi/filters/openxml/src/test/java/.../` | 21 |
| `ColorValueTest.java` | `okapi/filters/openxml/src/test/java/.../` | 10 |
| `ConditionalParametersTest.java` | `okapi/filters/openxml/src/test/java/.../` | 6 |
| `SubfilteringTest.java` | `okapi/filters/openxml/src/test/java/.../` | 5 |
| `TestRelationships.java` | `okapi/filters/openxml/src/test/java/.../` | 5 |
| `WorksheetTest.java` | `okapi/filters/openxml/src/test/java/.../` | 4 |
| `PresentationFragmentsTest.java` | `okapi/filters/openxml/src/test/java/.../` | 4 |
| `OpenXmlFormattingTest.java` | `okapi/filters/openxml/src/test/java/.../` | 4 |
| `ExcelWorksheetTransUnitPropertyTest.java` | `okapi/filters/openxml/src/test/java/.../` | 4 |
| `TestXMLSerializer.java` | `okapi/filters/openxml/src/test/java/.../` | 3 |
| `StringItemParserTest.java` | `okapi/filters/openxml/src/test/java/.../` | 3 |
| `SerialDateTimeTest.java` | `okapi/filters/openxml/src/test/java/.../` | 3 |
| (15 more files with 1-2 tests each) | | ~20 |
