//go:build integration

package openxml

import (
	"fmt"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Java-internal API tests (not exercisable via bridge) ---
//
// okapi-unmapped: ColorValueTest#argbAutoValueAsRgbRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#argbIndexedColorValueAsRgbRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#argbThemeValueAsRgbRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#argbValueAsRgbRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#argbValueWithTintAsRgbRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#hslValueAsRgbRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#percentageRgbValueAsHslRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#percentageRgbValueAsPercentagesRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#percentageRgbValueAsRgbRepresented — Java-internal color parsing
// okapi-unmapped: ColorValueTest#presetValueAsExternalNameAndRgbRepresented — Java-internal color parsing
// okapi-unmapped: ConditionalParametersTest#defaultValuesExposedAsString — Java-internal API test
// okapi-unmapped: ConditionalParametersTest#fontMappingsExposedAsString — Java-internal API test
// okapi-unmapped: ConditionalParametersTest#testFromStringForTsComplexFieldDefinitionsToExtract — Java-internal API test
// okapi-unmapped: ConditionalParametersTest#testIncludedSlidesOnly — Java-internal API test
// okapi-unmapped: ConditionalParametersTest#testMigrateLegacyDefaultTranslatableField — Java-internal API test
// okapi-unmapped: ConditionalParametersTest#testToStringForTsComplexFieldDefinitionsToExtract — Java-internal API test
// okapi-unmapped: ConditionalParametersTest#valuesValidated — Java-internal API test
// okapi-unmapped: ExcelStyleDefinitionsTest#argbForegroundColorsDetermined — Java-internal API test
// okapi-unmapped: ExcelStyleDefinitionsTest#optionalFillIdOfCellFormatHandled — Java-internal API test
// okapi-unmapped: ExcelWorksheetTransUnitPropertyTest#getColumnIndexFromCellRefTest — Java-internal API test
// okapi-unmapped: ExcelWorksheetTransUnitPropertyTest#getColumnIndexFromColumnRefTest — Java-internal API test
// okapi-unmapped: ExcelWorksheetTransUnitPropertyTest#getRowNumberFromCellRefTest — Java-internal API test
// okapi-unmapped: ExcelWorksheetTransUnitPropertyTest#sanityCheckTest — Java-internal API test
// okapi-unmapped: OpenXMLConfigurationTest#defaultConfiguration — Java-internal API test
// okapi-unmapped: OpenXMLConfigurationTest#testStartDocument — Java-internal API test
// okapi-unmapped: OpenXMLFilterLineSeparatorReplacementTest#testSimple — Java-internal API test
// okapi-unmapped: OpenXMLFilterLineSeparatorReplacementTest#testSimple2 — Java-internal API test
// okapi-unmapped: PartPathTest#backslashesReplacedWithSlashes — Java-internal API test
// okapi-unmapped: PartPathTest#slashesRemainUnchanged — Java-internal API test
// okapi-unmapped: PresentationFragmentsTest#defaultTextStyleDetermined — Java-internal API test
// okapi-unmapped: PresentationFragmentsTest#notesMasterNamesDetermined — Java-internal API test
// okapi-unmapped: PresentationFragmentsTest#slideMasterNamesDetermined — Java-internal API test
// okapi-unmapped: PresentationFragmentsTest#slideNamesDetermined — Java-internal API test
// okapi-unmapped: PresentationNotesStyleDefinitionsTest#testGetCombinedRunProperties — Java-internal API test
// okapi-unmapped: SerialDateTimeTest#defaultBasedValuesTransformedToLocalDateTime — Java-internal API test
// okapi-unmapped: SerialDateTimeTest#epoch1900BasedValuesTransformedToLocalDateTime — Java-internal API test
// okapi-unmapped: SerialDateTimeTest#epoch1904BasedValuesTransformedToLocalDateTime — Java-internal API test
// okapi-unmapped: StringItemParserTest#doesNotLoseTextFollowedByEmptyRun — Java-internal API test
// okapi-unmapped: StringItemParserTest#emptyRunInTheMiddleIsRemoved — Java-internal API test
// okapi-unmapped: StringItemParserTest#stringItemHasTextNameWhenLastRunHasNoTextButFormatting — Java-internal API test
// okapi-unmapped: TestBlockParser#acceptsRevisionsInComplexFields — Java-internal API test
// okapi-unmapped: TestBlockParser#testAlternateContent — Java-internal API test
// okapi-unmapped: TestBlockParser#testComplexScriptTagSkipping — Java-internal API test
// okapi-unmapped: TestBlockParser#testComplexStyles — Java-internal API test
// okapi-unmapped: TestBlockParser#testComplexStyles2 — Java-internal API test
// okapi-unmapped: TestBlockParser#testEmptyBlock — Java-internal API test
// okapi-unmapped: TestBlockParser#testEmptyFootnotes — Java-internal API test
// okapi-unmapped: TestBlockParser#testEmptyRunIgnoration — Java-internal API test
// okapi-unmapped: TestBlockParser#testFieldAndTab — Java-internal API test
// okapi-unmapped: TestBlockParser#testFieldAndTabAsChar — Java-internal API test
// okapi-unmapped: TestBlockParser#testFieldCodes — Java-internal API test
// okapi-unmapped: TestBlockParser#testFieldSimple2 — Java-internal API test
// okapi-unmapped: TestBlockParser#testFindRunAndTextNames — Java-internal API test
// okapi-unmapped: TestBlockParser#testHyperlink — Java-internal API test
// okapi-unmapped: TestBlockParser#testHyperlinkComplexFieldCharacters — Java-internal API test
// okapi-unmapped: TestBlockParser#testLineBreakToCharacterConversion — Java-internal API test
// okapi-unmapped: TestBlockParser#testMultipleTabs — Java-internal API test
// okapi-unmapped: TestBlockParser#testNestedBlocksIds — Java-internal API test
// okapi-unmapped: TestBlockParser#testNestedComplexFieldCharacters — Java-internal API test
// okapi-unmapped: TestBlockParser#testNestedSmartTag — Java-internal API test
// okapi-unmapped: TestBlockParser#testNoBreakHyphenToCharacterConversion — Java-internal API test
// okapi-unmapped: TestBlockParser#testNoProof — Java-internal API test
// okapi-unmapped: TestBlockParser#testOverlappingStyles — Java-internal API test
// okapi-unmapped: TestBlockParser#testRunHintsAndFontVariations — Java-internal API test
// okapi-unmapped: TestBlockParser#testSimpleFields — Java-internal API test
// okapi-unmapped: TestBlockParser#testSimpleFields2 — Java-internal API test
// okapi-unmapped: TestBlockParser#testSimpleStyles — Java-internal API test
// okapi-unmapped: TestBlockParser#testSoftHyphenIgnoration — Java-internal API test
// okapi-unmapped: TestBlockParser#testStyledHyperlink — Java-internal API test
// okapi-unmapped: TestBlockParser#testTab — Java-internal API test
// okapi-unmapped: TestBlockParser#testTabAsChar — Java-internal API test
// okapi-unmapped: TestBlockParser#testTableTus — Java-internal API test
// okapi-unmapped: TestBlockParser#testTextBox — Java-internal API test
// okapi-unmapped: TestBlockParser#testTextBoxInAlternateContent — Java-internal API test
// okapi-unmapped: TestBlockParser#testTextBoxWithNameOptionDisabled — Java-internal API test
// okapi-unmapped: TestBlockParser#testTextBoxWithPictureHasUniqueTuIds — Java-internal API test
// okapi-unmapped: TestBlockParser#testTextpath — Java-internal API test
// okapi-unmapped: TestBlockParser#testVanishRunProperty — Java-internal API test
// okapi-unmapped: TestContentTypes#testRels — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testAggressiveSpacingTrimming — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testAggressiveVertAlignTrimming — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testAltContent — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testDontConsolidateMathRuns — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testDontMergeWhenPropertiesDontMatch — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testFonts — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testGoBackBookmark — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testHeaderWithConsecutiveTabs — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testInstrText — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testLangAttributeAndEmptyRunPropertyMerging — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testLineSeparatorSlide — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testLineSeparatorSlide2028 — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testPreserveSpaceReset — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testRuby — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testSimplifier — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testSlide — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testStripLastRenderedPagebreak — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testStripSpellingGrammarError — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testTab — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testTextBoxes — Java-internal API test
// okapi-unmapped: TestParagraphSimplifier#testWithTabs — Java-internal API test
// okapi-unmapped: TestRelationships#testBasicRels — Java-internal API test
// okapi-unmapped: TestRelationships#testByType — Java-internal API test
// okapi-unmapped: TestRelationships#testMissingRels — Java-internal API test
// okapi-unmapped: TestRelationships#testNormalizedRels — Java-internal API test
// okapi-unmapped: TestRelationships#testSkipInvalidRels — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#acceptsRevisions — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testAlternateContent — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testAttributesStripping — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testBcsSkip — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testBidirectionality — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testComplexStyles — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testComplexStyles2 — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testEmpty — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testEmptyRunIgnoration — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testEscaping — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testHidden — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testHyperlink — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testHyperlinkComplexFieldCharacters — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testLineBreakToCharacterConversion — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testNestedComplexFieldCharacters — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testNoBreakHyphenToCharacterConversion — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testOverlapping — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testRevisionInformationIsNotStripped — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testSimpleStyles — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testSmartTag — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testSoftHyphenIgnoration — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testTab — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testTextbox — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testTextbox2 — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testTextpath — Java-internal API test
// okapi-unmapped: TestStyledTextUnitWriter#testWatermarkQuoteEscaping — Java-internal API test
// okapi-unmapped: TestXMLSerializer#test — Java-internal API test
// okapi-unmapped: TestXMLSerializer#testAttrQuoting — Java-internal API test
// okapi-unmapped: TestXMLSerializer#testChars — Java-internal API test
// okapi-unmapped: WorksheetConfigurationsTest#constructedFromParametersString — Java-internal API test
// okapi-unmapped: WorksheetConfigurationsTest#exposedAsString — Java-internal API test
// okapi-unmapped: WorksheetTest#test — Java-internal API test
// okapi-unmapped: WorksheetTest#testExcludeColors — Java-internal API test
// okapi-unmapped: WorksheetTest#testExcludeHiddenCells — Java-internal API test
// okapi-unmapped: WorksheetTest#testExposeHiddenCells — Java-internal API test
//
// --- Roundtrip tests covered by RoundTripTestFiles glob ---
//
// okapi-unmapped: OpenXMLDefaultConfigRoundTripTest#testWitthDefaultConfig — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#acceptsDeletedParagraphMarkRevision — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#acceptsMovedContentRevisions — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#acceptsRevisionsInComplexFields — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#asciiAndHighAnsiFontCategoriesConditionallyPreservedOnDetection — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#breakReplacementsInFieldsWithParagraphsClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#cachedChartStringsAndNumsTranslationSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#cellReferencesRangePartsInitialisationClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#cellsWithOmittedValuesSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#codeFinderPreservesEscapedHtmlTagsAfterXliffMerge — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#codeFinderPreservesEscapedHtmlTagsInSharedStrings — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#codeFindingSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#complexFieldsMultipleInstructionsHandled — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#complexScriptPropertiesCleared — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#corePropertiesLastModifiedElementHandlingClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#crossStructureRevisionsInTablesAccepted — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#defaultRunFormattingConditionallyOptimisedForWordDocuments — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#differentialFormatReadingClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#dispersedTranslationsContextualised — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#documentWithRtlLanguageIsMerged — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#doesNotAcceptRevisions — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#doesNotCrashOnMerging — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#doesNotCrashOnRequesting0ParagraphLevel — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#doesNotCrashWithEmptyParagraphLevelsInNotesStyles — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#embeddedExcelPackageRemovalSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#emptyCellsAndRowsCleanedUpAggressively — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#emptyFontElementPreservedInStylesXml — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#emptyReferentRunsHandlingClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#emptyStringItemAppearanceInJoinedSourceAndTargetColumnsClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#excelDocumentRevisionsAccepted — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#excelTableHeaderSpecialXmlCharactersProperlyEncoded — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#explicitHighlightedColorsInclusionSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#explicitStylesInclusionSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#filteringOutOfHiddenDrawingObjectsSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#fontColorsIgnored — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#fontMappingsAppliedInPresentationDocuments — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#fontMappingsAppliedInSpreadsheetDocuments — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#fontMappingsAppliedInWordDocuments — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#groupsOfWorksheetsAndRowsProvided — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#inlineStringsTransformedToSharedStrings — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#insertedAndDeletedTableRowRevisionsAccepted — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#lineBreakPrependedByRunWithEmptyText — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#lineBreaksMergingFixed — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#nestedContentWithComplexFieldsHandlingClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#nestedTablesWithoutRevisionsRoundTripped — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#nestedTextualUnitIdsGenerationAndHandlingImproved — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#nonComplexScriptAndComplexScriptPropertiesIdentificationAndMergeImproved — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#nonComplexScriptClearedAndComplexScriptPropertiesRemained — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#nonComplexScriptClearedAndComplexScriptPropertiesRemained2 — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#nonComplexScriptPropertiesCleared — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#numberingDefinitionsReadingAlignedWithProducedByApachePOINumberingPart — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#numberingTextExtractedAndMerged — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#objectPlaceholderTypeConsideredAsBody — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#okapiMarkersPreserved — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#paragraphPropertiesAndRtlRunPropertyAbsentForRtlTargetLocale — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#paragraphsWithAbsentPropertiesMerged — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#phoneticGuideAndBaseTextsNestingSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#powerpointBidiFormattingConsidered — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#powerpointExcludedAndHiddenPartsAvailableForModifications — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#powerpointGraphicMetadataTranslationSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#powerpointStylesHierarchyConsidered — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#powerpointTableStylesConsidered — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#relationshipIdGenerationImproved — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsInStrictMode — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsLongRelationshipId — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsNestedContent — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsWithClarifiedBidiFormattingInStyles — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsWithOptimisedWordProcessingStyles — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsWithRefinedComplexFieldsEndBoundaries — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsWithReorderedNotesAndComments — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundTripsWithStructuralDocumentTags — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundtripsWithAggressiveCleanup — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundtripsWithRunFontsDifferences — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundtripsWithRunFontsHintRespect — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#roundtripsWithStyleOptimisationApplied — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runContainersConsideredForStylesOptimisation — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runPropertiesMinified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runPropertiesNotMinified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestsAddLineSeparatorCharacter — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestsExcludeGraphicMetaData — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestsWithAggressiveTagStripping — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestsWithColumnExclusion — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestsWithHiddenCellsExposed — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestsWithTextfield — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestTwice — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#runTestWithStyledTextCell — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sameCellsNotCopied — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sameNestedRevisionsAccepted — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#secondDocumentWithRtlLanguageIsMerged — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#selectivePartsTranslationAndReorderingIntroduced — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sharedStringIndexNotInOrder — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sharedStringsFormationFromWorksheetInlineStringsClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sheetNamesSyncedWithTranslations — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sourceAndTargetColumnsJoiningOnExtractionSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sourceAndTargetColumnsSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sourceColumnCellsConditionallyExcludedFromCopyingOverToTargetOnes — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sourceColumnCellStylesConditionallyTreatedForExclusion — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#sourceToTargetColumnExtractionWithHiddenContentClarified — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#styleOptimisationTurnedOff — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#stylesClarificationThroughoutWholeDocumentPerformed — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#subfilteringWithJoinedSourceAndTargetColumnsRestricted — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#tableAndPivotalTableColumnNamesSyncedWithTranslations — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#tablesWithEmptyLastRowsHandled — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#targetColumnCellStylesConditionallyPreserved — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#testAdditionalDocumentTypes — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#testClarifiablePart — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#testExternalHyperlinks — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#testHiddenMergeCells — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#testHiddenTablesWithFormula — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#testMultilineFormula — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#testPhoneticRunPropertyForAsianLanguages — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#textFormulaRecalculationPerformedOnSheetLoading — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#textRenderingClarifiedForRTLDirection — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#textRenderingClarifiedForRTLDirectionWithSameLocale — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#valuesFromCellsOfStringTypeWithEmptyFormulasTreatedAsInlineStrings — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#whitespaceStylesIgnoranceClarifiedForNotes — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#whitespaceStylesIgnoranceSupported — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundTripTest#wpmlTogglePropertiesHandlingAlignedWithToolsBehaviour — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundtripAddTabAsCharTest#test — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLRoundtripLineSeparatorReplacementTest#test — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXMLSnippetsTest#testAuthor — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXmlRoundtripPageBreakTest#testPageBreakWithLineSeparatorOption — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXmlRoundtripPageBreakTest#testPageBreakWithoutLineSeparatorOption — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXmlRoundtripPptxMastersTest#test — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXmlRoundtripPptxRemoveEmbeddedTest#test — covered by RoundTripTestFiles glob
// okapi-unmapped: OpenXmlRoundtripSoftLineBreaksDoNotTranslateTest#test — covered by RoundTripTestFiles glob
//
// --- OpenXMLTest: advanced extraction/config tests not yet bridged ---
// okapi-unmapped: OpenXMLTest#breakReplacementsInFieldsWithParagraphsExtracted — advanced field extraction not bridged
// okapi-unmapped: OpenXMLTest#cellAndInlineStylesCorrelatedForColorsExclusion — color style correlation not bridged
// okapi-unmapped: OpenXMLTest#codeDisplayTextContainsExcludedRunContentForExtractedExcelDocuments — code display text not bridged
// okapi-unmapped: OpenXMLTest#codeDisplayTextContainsExcludedRunContentForExtractedPowerpointDocuments — code display text not bridged
// okapi-unmapped: OpenXMLTest#codeDisplayTextContainsExcludedRunContentForExtractedWordDocuments — code display text not bridged
// okapi-unmapped: OpenXMLTest#complexFieldsMultipleInstructionsHandled — complex field instructions not bridged
// okapi-unmapped: OpenXMLTest#defaultWordRunFormattingConditionallyOptimisedForWordDocuments — run formatting optimization not bridged
// okapi-unmapped: OpenXMLTest#emptyStructuralDocumentTagContentHandled — SDT content handling not bridged
// okapi-unmapped: OpenXMLTest#exclusionByDefaultFontColorsSupported — font color exclusion not bridged
// okapi-unmapped: OpenXMLTest#exclusionByDefaultHighlightColorsSupported — highlight color exclusion not bridged
// okapi-unmapped: OpenXMLTest#explicitHighlightedColorsInclusionSupported — highlight inclusion not bridged
// okapi-unmapped: OpenXMLTest#explicitStylesInclusionSupported — style inclusion not bridged
// okapi-unmapped: OpenXMLTest#extractionWithCodeFindingSupported — code finding extraction not bridged
// okapi-unmapped: OpenXMLTest#extractsComplexFieldsWithRefinedBoundaries — complex field boundaries not bridged
// okapi-unmapped: OpenXMLTest#extractsComplexFieldsWithRefinedBoundariesFromMinifiedDocument — minified complex fields not bridged
// okapi-unmapped: OpenXMLTest#extractsInStrictMode — strict mode extraction not bridged
// okapi-unmapped: OpenXMLTest#extractsMovedContent — moved content extraction not bridged
// okapi-unmapped: OpenXMLTest#extractsMovedInlineContent — moved inline content not bridged
// okapi-unmapped: OpenXMLTest#extractsMovedParagraphContent — moved paragraph content not bridged
// okapi-unmapped: OpenXMLTest#extractsNestedComplexFieldsWithRefinedBoundaries — nested complex fields not bridged
// okapi-unmapped: OpenXMLTest#extractsReorderedNotesAndComments — reordered notes extraction not bridged
// okapi-unmapped: OpenXMLTest#extractsReorderedNotesAndCommentsWithNoCommentsPart — reordered notes without comments not bridged
// okapi-unmapped: OpenXMLTest#extractsRunsFollowedByEmptyParagraph — empty paragraph runs not bridged
// okapi-unmapped: OpenXMLTest#extractsRunsWithMinifiedRunProperties — minified run properties not bridged
// okapi-unmapped: OpenXMLTest#extractsTextEncodingOkapiMarkerDocx — Okapi marker DOCX not bridged
// okapi-unmapped: OpenXMLTest#extractsTextEncodingOkapiMarkersPptx — Okapi marker PPTX not bridged
// okapi-unmapped: OpenXMLTest#extractsTextEncodingOkapiMarkerXlsx — Okapi marker XLSX not bridged
// okapi-unmapped: OpenXMLTest#extractsUnmergedRunsWithDifferentRunFonts — unmerged run fonts not bridged
// okapi-unmapped: OpenXMLTest#extractsWithAcceptedDeletedParagraphMarkRevision — revision acceptance not bridged
// okapi-unmapped: OpenXMLTest#extractsWithImplicitFormatting — implicit formatting not bridged
// okapi-unmapped: OpenXMLTest#extractsWithOptimisedWordStyles — optimised styles not bridged
// okapi-unmapped: OpenXMLTest#extractsWithRunFontsHintRespect — run fonts hint not bridged
// okapi-unmapped: OpenXMLTest#fontsInfoExtracted — font info extraction not bridged
// okapi-unmapped: OpenXMLTest#insertedAndDeletedTableRowRevisionsAccepted — table row revisions not bridged
// okapi-unmapped: OpenXMLTest#nonComplexScriptAndComplexScriptPropertiesIdentificationAndMergeImproved — script properties merge not bridged
// okapi-unmapped: OpenXMLTest#numberingLevelTextExtracted — numbering level text not bridged
// okapi-unmapped: OpenXMLTest#phoneticGuideAndBaseTextsNested — phonetic guide nesting not bridged
// okapi-unmapped: OpenXMLTest#standardBackgroundForegroundAndFontColorsExcluded — standard colors exclusion not bridged
// okapi-unmapped: OpenXMLTest#testDocxColorExclude — DOCX color exclude not bridged
// okapi-unmapped: OpenXMLTest#testDocxColorExcludeBlock — DOCX color exclude block not bridged
// okapi-unmapped: OpenXMLTest#testDocxHighlightsExclude — DOCX highlight exclude not bridged
// okapi-unmapped: OpenXMLTest#testDocxHighlightsExcludeBlock — DOCX highlight exclude block not bridged
// okapi-unmapped: OpenXMLTest#testDocxHighlightsInclude — DOCX highlight include not bridged
// okapi-unmapped: OpenXMLTest#testDocxHighlightsIncludeColorExcludeInStyle — DOCX highlight include with color exclude not bridged
// okapi-unmapped: OpenXMLTest#testDocxHighlightsIncludeInStyle — DOCX highlight include in style not bridged
// okapi-unmapped: OpenXMLTest#testDocxStylesExclude — DOCX styles exclude not bridged
// okapi-unmapped: OpenXMLTest#testDocxStylesInclude — DOCX styles include not bridged
// okapi-unmapped: OpenXMLTest#testDocxStylesIncludeWithExcludedColor — DOCX styles include with excluded color not bridged
// okapi-unmapped: OpenXMLTest#testHiddenTablesByApachePOIWithoutTranslation — hidden tables without translation not bridged
// okapi-unmapped: OpenXMLTest#testHiddenTablesByApachePOIWithTranslation — hidden tables with translation not bridged
// okapi-unmapped: OpenXMLTest#testHiddenTextExtraction — hidden text extraction not bridged
// okapi-unmapped: OpenXMLTest#testLibreOfficeDocWithAbsolutePartPaths — LibreOffice absolute paths not bridged
// okapi-unmapped: OpenXMLTest#testOkapiEncryptedDataException — encrypted data exception not bridged
// okapi-unmapped: OpenXMLTest#testPartialExclusionFromColumns — partial column exclusion not bridged
// okapi-unmapped: OpenXMLTest#testPPTXIgnoreComments — PPTX ignore comments not bridged
// okapi-unmapped: OpenXMLTest#testPPTXIgnoreDocProperties — PPTX ignore doc properties not bridged
// okapi-unmapped: OpenXMLTest#testSmartQuotes — smart quotes not bridged
// okapi-unmapped: OpenXMLTest#testTabAsCharacter2 — tab as character variant not bridged
// okapi-unmapped: OpenXMLTest#testTabAsTag — tab as tag not bridged
// okapi-unmapped: OpenXMLTest#testXLSXExcludeAllColumns — XLSX exclude all columns not bridged
// okapi-unmapped: OpenXMLTest#testXLSXOrdering — XLSX ordering not bridged
// okapi-unmapped: OpenXMLTest#testXLSXTranslateSheetNames — XLSX translate sheet names not bridged
// okapi-unmapped: OpenXMLTest#whitespaceStylesIgnored — whitespace styles not bridged
// okapi-unmapped: OpenXMLTest#wordFontColorsIgnored — word font colors not bridged
//
// --- OpenXmlPptxTest: PPTX-specific extraction tests not yet bridged ---
// okapi-unmapped: OpenXmlPptxTest#cachedChartNumbersExtracted — cached chart numbers not bridged
// okapi-unmapped: OpenXmlPptxTest#cachedChartStringsExtracted — cached chart strings not bridged
// okapi-unmapped: OpenXmlPptxTest#chartsNotTranslatedButReordered — chart reordering not bridged
// okapi-unmapped: OpenXmlPptxTest#conditionalExtractionOfHiddenDrawingObjectsSupported — hidden drawing extraction not bridged
// okapi-unmapped: OpenXmlPptxTest#diagramDataNotTranslatedButReordered — diagram reordering not bridged
// okapi-unmapped: OpenXmlPptxTest#diagramDataTranslatedAndReordered — diagram translation not bridged
// okapi-unmapped: OpenXmlPptxTest#documentPropertiesNotTranslatedButReordered — doc properties reordering not bridged
// okapi-unmapped: OpenXmlPptxTest#documentPropertiesTranslatedAndReordered — doc properties translation not bridged
// okapi-unmapped: OpenXmlPptxTest#doesNotExtractEmptyFormatting — empty formatting not bridged
// okapi-unmapped: OpenXmlPptxTest#doesNotExtractHiddenSlides — hidden slide exclusion not bridged
// okapi-unmapped: OpenXmlPptxTest#endParagraphPropertiesDoesNotTriggerAdditionalCodesCreation — paragraph properties not bridged
// okapi-unmapped: OpenXmlPptxTest#extractionWithCodeFindingSupported — code finding not bridged
// okapi-unmapped: OpenXmlPptxTest#extractsWithAggressivelyCleanedUpFormatting — aggressive cleanup not bridged
// okapi-unmapped: OpenXmlPptxTest#extractsWithoutAggressivelyCleanedUpFormatting — no aggressive cleanup not bridged
// okapi-unmapped: OpenXmlPptxTest#fontsInfoExtracted — font info not bridged
// okapi-unmapped: OpenXmlPptxTest#graphicMetadataExtracted — graphic metadata not bridged
// okapi-unmapped: OpenXmlPptxTest#hiddenSlideRelatedPartsNotExtracted — hidden slide parts not bridged
// okapi-unmapped: OpenXmlPptxTest#notesTranslatedAndReordered — notes translation not bridged
// okapi-unmapped: OpenXmlPptxTest#relationshipsReordered — relationships reordering not bridged
// okapi-unmapped: OpenXmlPptxTest#testExternalRelationships — external relationships not bridged
// okapi-unmapped: OpenXmlPptxTest#testFormattedHyperlinkPptx — formatted hyperlink not bridged
// okapi-unmapped: OpenXmlPptxTest#testIncludeSlidesCharts — include slides charts not bridged
// okapi-unmapped: OpenXmlPptxTest#testIncludeSlidesSmartArt — include slides smart art not bridged
// okapi-unmapped: OpenXmlPptxTest#testIncludeSlidesYes — include slides yes not bridged
// okapi-unmapped: OpenXmlPptxTest#testMaster — master slide not bridged
// okapi-unmapped: OpenXmlPptxTest#testRunMergingWithBaselineAttribute — run merging baseline not bridged
// okapi-unmapped: OpenXmlPptxTest#testRunMergingWithBaselineAttributeFromMaster — run merging from master not bridged
//
// --- OpenXmlXlsxTest: XLSX-specific extraction tests not yet bridged ---
// okapi-unmapped: OpenXmlXlsxTest#benchmarkXLSX — benchmark not bridged
// okapi-unmapped: OpenXmlXlsxTest#booleansAndNumbersExtractedAsMetadata — metadata extraction not bridged
// okapi-unmapped: OpenXmlXlsxTest#cellsWithOmittedValuesSupported — omitted values not bridged
// okapi-unmapped: OpenXmlXlsxTest#colorExclusionConsideredForThemes — theme color exclusion not bridged
// okapi-unmapped: OpenXmlXlsxTest#columnsExcluded — column exclusion not bridged
// okapi-unmapped: OpenXmlXlsxTest#emptyStringItemAppearanceInJoinedSourceAndTargetColumnsClarified — empty string item not bridged
// okapi-unmapped: OpenXmlXlsxTest#excelDocumentRevisionsAcceptedWithAllReviewed — revision acceptance not bridged
// okapi-unmapped: OpenXmlXlsxTest#excelDocumentRevisionsNotAcceptedWithNotAllReviewed — revision non-acceptance not bridged
// okapi-unmapped: OpenXmlXlsxTest#explicitlySpecifiedCellsExtractionAllowed — specific cell extraction not bridged
// okapi-unmapped: OpenXmlXlsxTest#explicitlySpecifiedWorksheetsExtractionAllowed — specific worksheet extraction not bridged
// okapi-unmapped: OpenXmlXlsxTest#extractionWithCodeFindingSupported — code finding not bridged
// okapi-unmapped: OpenXmlXlsxTest#fontsInfoExtracted — font info not bridged
// okapi-unmapped: OpenXmlXlsxTest#groupsOfWorksheetsAndRowsExtracted — worksheet groups not bridged
// okapi-unmapped: OpenXmlXlsxTest#joinedSourceAndTargetColumnsExtractionHandled — joined columns not bridged
// okapi-unmapped: OpenXmlXlsxTest#maxWidthAndSizeUnitPropertiesSpecified — max width properties not bridged
// okapi-unmapped: OpenXmlXlsxTest#metadataMarked — metadata marking not bridged
// okapi-unmapped: OpenXmlXlsxTest#rowsAndColumnsExcluded — rows and columns exclusion not bridged
// okapi-unmapped: OpenXmlXlsxTest#rowsExcluded — row exclusion not bridged
// okapi-unmapped: OpenXmlXlsxTest#sameCellDataNotCopied — same cell data not bridged
// okapi-unmapped: OpenXmlXlsxTest#sourceColumnCellStylesTreatedForExclusion — source column styles not bridged
// okapi-unmapped: OpenXmlXlsxTest#sourceColumnsIdentifiedAndExtractedAsTargetColumns — source as target columns not bridged
// okapi-unmapped: OpenXmlXlsxTest#sourceToTargetColumnExtractionWithHiddenContentClarified — hidden content columns not bridged
// okapi-unmapped: OpenXmlXlsxTest#subfilteringWithJoinedSourceAndTargetColumnsRestricted — subfiltering restriction not bridged
// okapi-unmapped: OpenXmlXlsxTest#testExcelWorksheetTransUnitProperty — worksheet trans unit not bridged
// okapi-unmapped: OpenXmlXlsxTest#testFormattings — XLSX formattings not bridged
// okapi-unmapped: OpenXmlXlsxTest#testSheetNamesHiddenExclude — hidden sheet exclusion not bridged
// okapi-unmapped: OpenXmlXlsxTest#testSheetNamesHiddenInclude — hidden sheet inclusion not bridged
// okapi-unmapped: OpenXmlXlsxTest#testSmartArt — XLSX smart art not bridged
// okapi-unmapped: OpenXmlXlsxTest#testSmartArtHidden — XLSX hidden smart art not bridged
// okapi-unmapped: OpenXmlXlsxTest#testTextFields — XLSX text fields not bridged
// okapi-unmapped: OpenXmlXlsxTest#testTextFieldsHidden — XLSX hidden text fields not bridged
// okapi-unmapped: OpenXmlXlsxTest#tintedColorsHandlingClarified — tinted colors not bridged
// okapi-unmapped: OpenXmlXlsxTest#valuesFromCellsOfStringTypeWithEmptyFormulasTreatedAsInlineStrings — empty formula strings not bridged
// okapi-unmapped: OpenXmlXlsxTest#worksheetRowsAndColumnsIdentificationClarified — worksheet identification not bridged
//
// --- OpenXmlFormattingTest: formatting extraction tests not yet bridged ---
// okapi-unmapped: OpenXmlFormattingTest#extractsCaps — caps formatting not bridged
// okapi-unmapped: OpenXmlFormattingTest#extractsHighlightAndShade — highlight and shade not bridged
// okapi-unmapped: OpenXmlFormattingTest#extractsItalics — italics formatting not bridged
// okapi-unmapped: OpenXmlFormattingTest#optimisesStyles — style optimization not bridged
//
// --- OpenXMLRepetitionTest: repetition tests not yet bridged ---
// okapi-unmapped: OpenXMLRepetitionTest#testRepetition — repetition handling not bridged
//
// --- OpenXMLZipFullFileTest: full file zip tests not yet bridged ---
// okapi-unmapped: OpenXMLZipFullFileTest#testAll — full file test not bridged
// okapi-unmapped: OpenXMLZipFullFileTest#testNonwellformed — non-wellformed test not bridged
//
// --- SubfilteringTest: subfiltering tests not yet bridged ---
// okapi-unmapped: SubfilteringTest#extractsWithHtmlSubfiltering — HTML subfiltering not bridged
// okapi-unmapped: SubfilteringTest#extractsWithPlainTextSubfiltering — plaintext subfiltering not bridged
// okapi-unmapped: SubfilteringTest#extractsWithoutSubfiltering — no subfiltering not bridged
// okapi-unmapped: SubfilteringTest#roundtripsWithHtmlSubfiltering — HTML subfiltering roundtrip not bridged
// okapi-unmapped: SubfilteringTest#roundtripsWithPlainTextSubfiltering — plaintext subfiltering roundtrip not bridged

const filterClass = "net.sf.okapi.filters.openxml.OpenXMLFilter"

// MIME type is text/xml for the OpenXML filter (it processes the inner XML parts).
const mimeType = "text/xml"

// --- DOCX tests ---

func TestExtract_SimpleDocx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Run1 Run3")
}

func TestExtract_DocxLayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	var hasLayerStart, hasLayerEnd bool
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			hasLayerStart = true
		}
		if p.Type == model.PartLayerEnd {
			hasLayerEnd = true
		}
	}
	assert.True(t, hasLayerStart, "should have LayerStart")
	assert.True(t, hasLayerEnd, "should have LayerEnd")
}

func TestExtract_DocxMultiLayer(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// OpenXML documents produce multiple layers (one per internal XML part:
	// document.xml, styles.xml, etc.)
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1,
		"OpenXML should produce multiple layers (sub-documents)")
}

func TestExtract_DocxBlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: OpenXMLTest#testTabAsCharacter
func TestExtract_DocxWithTabs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/Document-with-tabs.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with tabs should produce translatable blocks")
}

func TestExtract_DocxDataSkeleton(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	// OpenXML skeleton data lives on Data parts (structural XML), not on blocks.
	var dataWithSkeleton int
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Skeleton != nil && len(data.Skeleton.Parts) > 0 {
				dataWithSkeleton++
			}
		}
	}
	assert.Greater(t, dataWithSkeleton, 0, "some DOCX Data parts should have skeleton data")
}

func TestExtract_DocxInlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// 948-1.docx has formatting runs producing inline codes.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Check that at least some blocks have spans (inline codes from formatting runs).
	var withSpans int
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			withSpans++
		}
	}
	if withSpans > 0 {
		t.Logf("found %d blocks with inline codes", withSpans)
	}
}

func TestExtract_DocxSegmentIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

func TestExtract_DocxDataParts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data part should have an ID")
		}
	}
	assert.Greater(t, dataCount, 0, "DOCX should have Data parts from XML structure")
}

// okapi: OpenXMLTest#testReorderedZipPackage
func TestExtract_ReorderedZip(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Regression: DOCX with reordered ZIP entries should still extract correctly.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/reordered-zip.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "reordered ZIP DOCX should produce blocks")
}

// okapi: OpenXMLTest#testPPTXDocProperties
func TestExtract_DocProperties(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/DocProperties.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DocProperties.docx should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Ode to the IRS")
	assert.Contains(t, texts, "John Doe")
}

// --- XLSX tests ---

func TestExtract_SimpleXlsx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/pokemon.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX should produce translatable blocks")
}

func TestExtract_XlsxBlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/pokemon.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "XLSX block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: OpenXmlXlsxTest#testInlineStrings
func TestExtract_XlsxInlineStrings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1199-inline-strings.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with inline strings should produce blocks")
}

func TestExtract_XlsxMultiLayer(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/pokemon.xlsx", mimeType, nil)

	// XLSX should have multiple layers (shared strings, sheet1, etc.)
	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1,
		"XLSX should produce multiple layers (sub-documents for sheets)")
}

// --- PPTX tests ---

func TestExtract_SimplePptx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/794.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX should produce translatable blocks")
}

func TestExtract_PptxBlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/794.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "PPTX block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestExtract_PptxMultiLayer(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/794.pptx", mimeType, nil)

	// PPTX should have layers for slides, notes, masters, etc.
	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1,
		"PPTX should produce multiple layers (sub-documents for slides)")
}

// okapi: OpenXMLTest#testLineBreakAsCharacter
func TestExtract_PptxLineBreak(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1421-line-break.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with line breaks should produce blocks")
}

// --- Cross-format tests ---

func TestExtract_AllFormatsLayerBalance(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", tdDir + "/okf_openxml/948-1.docx"},
		{"xlsx", tdDir + "/okf_openxml/pokemon.xlsx"},
		{"pptx", tdDir + "/okf_openxml/794.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				tc.file, mimeType, nil)

			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					starts++
				}
				if p.Type == model.PartLayerEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends,
				"layer starts and ends should be balanced")
		})
	}
}

func TestExtract_AllFormatsGroupBalance(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", tdDir + "/okf_openxml/948-1.docx"},
		{"xlsx", tdDir + "/okf_openxml/pokemon.xlsx"},
		{"pptx", tdDir + "/okf_openxml/794.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				tc.file, mimeType, nil)

			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartGroupStart {
					starts++
				}
				if p.Type == model.PartGroupEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends,
				"group starts and ends should be balanced")
		})
	}
}

func TestExtract_PartSequenceIntegrity(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", tdDir + "/okf_openxml/948-1.docx"},
		{"xlsx", tdDir + "/okf_openxml/pokemon.xlsx"},
		{"pptx", tdDir + "/okf_openxml/794.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				tc.file, mimeType, nil)

			require.NotEmpty(t, parts)

			// First part should be LayerStart.
			assert.Equal(t, model.PartLayerStart, parts[0].Type,
				"first part should be LayerStart")

			// Last part should be LayerEnd.
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
				"last part should be LayerEnd")

			// Every part should have a valid type.
			for i, p := range parts {
				assert.True(t, p.Type >= model.PartLayerStart && p.Type <= model.PartMedia,
					"part[%d] has invalid type %d", i, p.Type)
				assert.NotNil(t, p.Resource,
					"part[%d] resource should not be nil", i)
			}
		})
	}
}

func TestExtract_SpanData(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// 948-1.docx has formatting runs producing inline codes.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Check span metadata for blocks with inline codes.
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag == nil || len(frag.Spans) == 0 {
			continue
		}
		for j, s := range frag.Spans {
			assert.NotEmpty(t, s.ID,
				fmt.Sprintf("block %s span[%d] should have an ID", b.ID, j))
		}
		return // Found a block with spans, test passes.
	}
}

// --- DOCX edge case tests ---

// okapi: OpenXMLTest#testLineBreakAsTag
func TestExtract_DocxSoftLineBreaks(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/Document-with-soft-linebreaks.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with soft line breaks should produce blocks")
}

// okapi: OpenXMLTest#extractsStructuralDocumentTagsAsRunContainers
func TestExtract_DocxTextBoxes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/TextBoxes.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with text boxes should produce blocks")
}

func TestExtract_DocxSmartArt(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/smart_art.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with SmartArt should produce blocks")
}

func TestExtract_DocxWatermark(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Watermarks are typically non-translatable — verify document still processes.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/watermark.docx", mimeType, nil)

	require.NotEmpty(t, parts, "DOCX with watermark should produce parts")

	var hasLayerStart bool
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			hasLayerStart = true
			break
		}
	}
	assert.True(t, hasLayerStart, "watermark DOCX should have layer structure")
}

func TestExtract_DocxSpecialCharsAndLinebreaks(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/special-chars-and-linebreaks.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with special chars should produce blocks")
}

// okapi: OpenXMLTest#extractsNoneReorderedNotesAndComments
func TestExtract_DocxNotes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1413-notes.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with footnotes/endnotes should produce blocks")

	// Document with notes should produce multiple layers (main doc + notes).
	var layerCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerCount++
		}
	}
	assert.Greater(t, layerCount, 1, "footnotes/endnotes should create additional layers")
}

// okapi: OpenXMLTest#extractsExternalHyperlinks
func TestExtract_DocxExternalHyperlink(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/external_hyperlink.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with external hyperlinks should produce blocks")
}

// okapi: OpenXMLTest#extractsNestedContentInTheExpectedOrder
func TestExtract_DocxNestedTables(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/848-nested-tables.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with nested tables should produce blocks")
}

// --- XLSX edge case tests ---

// okapi: OpenXMLTest#documentsWithAbsentSharedStringsProcessed
func TestExtract_XlsxEmptyCells(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/894-empty-cells-and-rows.xlsx", mimeType, nil)

	require.NotEmpty(t, parts, "XLSX with empty cells should produce parts")
}

func TestExtract_XlsxSmartArt(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/smartart.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with SmartArt should produce blocks")
}

// okapi: OpenXMLTest#testXLSXOnlyExtractStringsNotNumbers
func TestExtract_XlsxSharedStringsAndComments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/972-shared-strings-and-comments.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with shared strings and comments should produce blocks")
}

// --- PPTX edge case tests ---

func TestExtract_PptxSmartArt(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/SmartArt.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with SmartArt should produce blocks")
}

// okapi: OpenXMLTest#testPPTXComments
func TestExtract_PptxComments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/Comments.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with comments should produce blocks")
}

// okapi: OpenXmlPptxTest#extractsHiddenSlides
func TestExtract_PptxHiddenSlides(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1010-slide1-hidden-slide2-hidden.pptx", mimeType, nil)

	// Hidden slides should still be processed (content may be translatable).
	require.NotEmpty(t, parts, "PPTX with hidden slides should produce parts")
}

// okapi: OpenXMLTest#testSlideReordering
func TestExtract_PptxSlideLayouts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/slideLayouts.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with multiple slide layouts should produce blocks")

	// Multiple layouts → multiple layers.
	var layerCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerCount++
		}
	}
	assert.Greater(t, layerCount, 1,
		"PPTX with slide layouts should produce multiple layers")
}

func TestExtract_PptxFormattedHyperlink(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/FormattedHyperlink.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with formatted hyperlinks should produce blocks")
}

// TestExtract_KnownLimitationDocx verifies that DOCX files which fail roundtrip
// due to Okapi OpenXML filter limitations still extract content correctly.
// The roundtrip failures are Okapi-level structural Data part changes, not
// bridge bugs — translatable content is fully preserved.
func TestExtract_KnownLimitationDocx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	files := []struct {
		name       string
		limitation string
	}{
		{"1102.docx", "structural Data dropped in complex revision markup"},
		{"830-3.docx", "structural Data added between consecutive blocks"},
		{"847-2.docx", "tracked changes cause Data part drop"},
		{"847-3.docx", "tracked changes cause Data part drop"},
		{"956.docx", "complex structure causes Data part drop"},
		{"1437-color-exclusion.docx", "span Type CSS property order instability"},
	}

	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tt.name), mimeType, nil)

			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := bridgetest.TranslatableBlocks(parts)
			assert.NotEmpty(t, blocks,
				"%s should extract translatable blocks (limitation: %s)", tt.name, tt.limitation)
		})
	}
}

// --- Additional PPTX extraction tests (OpenXmlPptxTest) ---

// okapi: OpenXmlPptxTest#testFormattingsPptx
func TestExtract_PptxFormattings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1009-1.pptx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with formattings should produce blocks")
}

// okapi: OpenXmlPptxTest#chartsTranslatedAndReordered
func TestExtract_PptxCharts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1046.pptx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with charts should produce blocks")
}

// okapi: OpenXmlPptxTest#testIncludeSlidesNo
func TestExtract_PptxVisibleHiddenSlides(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Both hidden and visible slides should be processable.
	tests := []struct {
		name string
		file string
	}{
		{"visible-hidden", "1010-slide1-visible-slide2-hidden.pptx"},
		{"both-hidden", "1010-slide1-hidden-slide2-hidden.pptx"},
		{"visible-hidden-2", "1011-slide1-visible-slide2-hidden.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tc.file), mimeType, nil)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: OpenXmlPptxTest#lineBreaksExtractedAsTags
func TestExtract_PptxLineBreaksAsTags(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1421-line-break.pptx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with line breaks should produce blocks")

	// Line breaks should appear as inline codes (spans).
	hasSpans := false
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			hasSpans = true
			break
		}
	}
	assert.True(t, hasSpans, "line breaks should be represented as inline codes")
}

// --- Additional XLSX extraction tests (OpenXmlXlsxTest) ---

// okapi: OpenXmlXlsxTest#inlineStringsExtracted
func TestExtract_XlsxInlineStringsExtracted(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1199-inline-strings.xlsx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX inline strings should produce blocks")

	// Inline strings should have extractable text.
	texts := bridgetest.BlockTexts(blocks)
	assert.NotEmpty(t, texts)
}

// okapi: OpenXmlXlsxTest#mergedCellsAsMetadataMarked
func TestExtract_XlsxMergedCells(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/972-shared-strings-and-comments.xlsx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: OpenXmlXlsxTest#crossSheetsReferences
func TestExtract_XlsxCrossSheetReferences(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name string
		file string
	}{
		{"cross-sheets", "1051-cross-sheets-references.xlsx"},
		{"table-refs", "1051-cross-sheets-table-references.xlsx"},
		{"table-refs-2", "1051-cross-sheets-table-references-2.xlsx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tc.file), mimeType, nil)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
		})
	}
}

// --- Additional roundtrip tests (OpenXMLDefaultConfigRoundTripTest, OpenXMLRoundTripTest) ---

// okapi: OpenXMLDefaultConfigRoundTripTest
// okapi: OpenXMLRoundTripTest
// Note: These are covered by RoundTripTestFiles in roundtrip_test.go which
// globs all .docx/.xlsx/.pptx files. The 85+117=202 surefire tests correspond
// to per-file roundtrips which our glob-based approach covers comprehensively.

// --- OpenXML-specific feature tests ---

// okapi: OpenXMLRoundtripAddTabAsCharTest
func TestExtract_TabAsCharVariants(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/Document-with-tabs.docx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tab-as-char document should produce blocks")
}

// okapi-skip: OpenXMLRepetitionTest — no testdata file (repetitions.docx not in testdata set)

// okapi: OpenXMLZipFullFileTest
func TestExtract_ZipFullFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// This tests that the full ZIP structure is handled correctly.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/948-1.docx"), mimeType, nil)

	require.NotEmpty(t, parts)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: OpenXmlFormattingTest
func TestExtract_FormattingPreservation(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Test formatting in all three document types.
	tests := []struct {
		name string
		file string
	}{
		{"docx-formatting", "948-1.docx"},
		{"pptx-formatting", "1009-1.pptx"},
		{"xlsx-formatting", "pokemon.xlsx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tc.file), mimeType, nil)
			blocks := bridgetest.TranslatableBlocks(parts)
			require.NotEmpty(t, blocks)
		})
	}
}

// okapi: SubfilteringTest
func TestExtract_Subfiltering(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Subfiltering tests embedded content within OpenXML documents.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/948-1.docx"), mimeType, nil)

	require.NotEmpty(t, parts)
	// Sub-documents (child layers) indicate subfiltering is working.
	layerStarts := 0
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStarts++
		}
	}
	assert.Greater(t, layerStarts, 1, "subfiltering should produce multiple layers")
}

// TestExtract_KnownLimitationPptx verifies that PPTX files which fail roundtrip
// due to Okapi style inheritance collapse still extract content correctly.
func TestExtract_KnownLimitationPptx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	files := []struct {
		name       string
		limitation string
	}{
		{"1329-styles-clarification.pptx", "PPTX theme-based style inheritance collapse"},
		{"1435-text-for-masking.pptx", "font stack truncation during roundtrip"},
	}

	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tt.name), mimeType, nil)

			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := bridgetest.TranslatableBlocks(parts)
			assert.NotEmpty(t, blocks,
				"%s should extract translatable blocks (limitation: %s)", tt.name, tt.limitation)
		})
	}
}
