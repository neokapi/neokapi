// okapi-filter: xliff
package xliff_test

// Stub tests for Okapi Java methods that cannot be mapped to native format tests.
// These are either Java-internal (API-level manipulation, clone, skeleton) or
// require testdata files only available through the bridge.

import "testing"

// ---- XLIFFFilterTest (Java-internal / bridge-only) ----

// okapi-unmapped: XLIFFFilterTest#testAddedCloneCode — Java clone API
// (Extraction portion covered natively in TestExtract_AddedCloneCode)

// okapi-unmapped: XLIFFFilterTest#testBlockSkeleton — skeleton is bridge-only
// (Bridge reader generates skeleton parts; native reader does not)

// okapi-unmapped: XLIFFFilterTest#testDataIsReferent — bridge skeleton referent flag

// okapi-unmapped: XLIFFFilterTest#testEmptyTgtLangAttribute — requires testdata file
// (Bridge test uses okf_xliff/empty-tgt-lang.xlf)

// okapi-unmapped: XLIFFFilterTest#testHandleInvalidXmlCharacters — requires testdata file
// (Bridge test uses okf_xliff/invalid_xml_entity.xlf)

// okapi-unmapped: XLIFFFilterTest#testMixedAlTrans — requires testdata file
// (Bridge test uses okf_xliff/Manual-12-AltTrans.xlf)

// okapi-unmapped: XLIFFFilterTest#testAlTransData — requires testdata file (alttrans.xlf)
// (Bridge test uses okf_xliff/alttrans.xlf for alt-trans data assertions)

// ---- XLIFFFilterTest (SDL XLIFF — requires .sdlxliff testdata) ----

// okapi-unmapped: XLIFFFilterTest#testPreserveSpaceByDefaultInSdlXliff — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testSdlTagDefs — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testSdlTagDefsWithSubs — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testSdlXliffApprovedConfStateMapping — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testRemoveSdlComment — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testRemoveNestedSdlComment — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue424 — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testIssue466NoMrk — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue466MixedMrk — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue466PreserveCRLF — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffConfStateMapping — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffInvalidInitialConf — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffInvalidUpdatedState — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffNoConf — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffRemoveStateAndOriginalConf — requires sdlxliff testdata

// ---- XLIFFFilterTest (ITS / LQI — requires testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testLQIAnnotations — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQRInline — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQIRemoval — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQIAndProvModifications1 — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testAddLQIModifications2 — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testITSAnnotations — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testITSAnnotatorsRef — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testITSStandoffManager — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSLQIMapping — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSProvenance — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSProvenanceFile — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSProvenanceGroup — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXTMAnnotations — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQR — requires testdata file

// ---- XLIFFFilterTest (IWS XLIFF — requires testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testIwsTestExtraction — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockLockStatus — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockTmScore — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockTmScoreIncludeMultipleExact — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockMultipleExact — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockFinished — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsNoBlockFinished — requires IWS testdata

// ---- XLIFFFilterTest (IWS skeleton — requires IWS testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffAllPending — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffAllFinished — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffAddsTranslationType — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffNoTranslationStatusInput — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffNoTranslationTypeInput — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffBlockedFinishedNotRemoveTmOrigin — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffKeepTmOrigin — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffTranslatableRemoveTmOrigin — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffTuApproved — requires IWS testdata

// ---- XLIFFFilterTest (target state coordination — requires testdata) ----

// okapi-unmapped: XLIFFFilterTest#testTargetStateCoordOutput — requires testdata file

// ---- XLIFFFilterTest (translate=no file — requires testdata) ----

// okapi-unmapped: XLIFFFilterTest#testTranslateNo — requires testdata file (translate_no.xlf)
// (Covered natively with inline snippets in TestExtract_TranslateNo)

// ---- XLIFFFilterTest (segmented files — require testdata) ----

// okapi-unmapped: XLIFFFilterTest#testSegmentedEntry — requires testdata file (segmented.xlf)
// okapi-unmapped: XLIFFFilterTest#testSegmentedEntryOutput — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testSegmentedEntryWithDifferences — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testSegmentedSource1 — requires testdata file (segsource.xlf)

// ---- XLIFFFilterCtypeTest (9 tests — roundtrip ctype preservation) ----

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeG — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeG(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBx — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeBx(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBxRid — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeBxRid(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBpt — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeBpt(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBptRid — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeBptRid(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeX — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeX(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeXBoldAsXBold — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeXBoldAsXBold(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testTargetIsSegmentedIdsAreNumbers — ctype roundtrip requires native writer inline code support
func TestCtype_TargetIsSegmentedIdsAreNumbers(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testTargetIsSegmentedIdsAreStrings — ctype roundtrip requires native writer inline code support
func TestCtype_TargetIsSegmentedIdsAreStrings(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// ---- XLIFFFilterEquivTextTest (10 tests — equiv-text roundtrip preservation) ----

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextGHello — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepGHello(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextGCustom — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepGCustom(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextX — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepX(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextXWithEscapedContent — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepXWithEscapedContent(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextBx — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepBx(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextEx — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepEx(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextBxEx — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepBxEx(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextBpt — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepBpt(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextEpt — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepEpt(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextPh — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepPh(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterEquivTextTest#testKeepEquivTextIt — equiv-text roundtrip requires native writer inline code support
func TestEquivText_KeepIt(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// ---- XLIFFFilterLengthConstraintsTest (3 tests — length constraint roundtrip) ----

// okapi-unmapped: XLIFFFilterLengthConstraintsTest#testTransUnit — length constraint roundtrip requires native writer support
func TestLengthConstraints_TransUnit(t *testing.T) {
	t.Skip("pending: requires native writer length constraint roundtrip support")
}

// okapi-unmapped: XLIFFFilterLengthConstraintsTest#testSizeUnitDefault — length constraint roundtrip requires native writer support
func TestLengthConstraints_SizeUnitDefault(t *testing.T) {
	t.Skip("pending: requires native writer length constraint roundtrip support")
}

// okapi-unmapped: XLIFFFilterLengthConstraintsTest#testGroup — length constraint roundtrip requires native writer support
func TestLengthConstraints_Group(t *testing.T) {
	t.Skip("pending: requires native writer length constraint roundtrip support")
}

// ---- CdataSubfilteringTest (6 tests — CDATA subfilter infrastructure) ----

// okapi-unmapped: CdataSubfilteringTest#notSubfiltered — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#subfilteredAsHtml — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#inlineNotSubfiltered — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#inlineSubfilteredAsHtml — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#subfilteredWithTargetsCopiedFromSource — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#subfilteredWithTargetsCopiedFromSourceAndTranslated — requires CDATA subfilter infrastructure not available in native reader

// ---- PcdataSubfilteringTest (4 tests — PCDATA subfilter infrastructure) ----

// okapi-unmapped: PcdataSubfilteringTest#subfilteredAsHtml — requires PCDATA subfilter infrastructure not available in native reader
// okapi-unmapped: PcdataSubfilteringTest#subfilteredAsHtmlWithAnnotations — requires PCDATA subfilter infrastructure not available in native reader
// okapi-unmapped: PcdataSubfilteringTest#subfilteredAsHtmlWithAnnotationsSplitIntoMultiple — requires PCDATA subfilter infrastructure not available in native reader
// okapi-unmapped: PcdataSubfilteringTest#subfilteredWithTargetsCopiedFromSourceAndTranslated — requires PCDATA subfilter infrastructure not available in native reader

// ---- XLIFFFilterSDLPropTest (9 tests — SDL XLIFF segment properties, requires sdlxliff testdata) ----

// okapi-unmapped: XLIFFFilterSDLPropTest#testSegmentProperties — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testManipulateSdlSegmentProperties — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testAddingSdlSegmentPropertiesOldTest — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSdlRepetitions — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSdlRepetitionsInPrevOrigin — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testWithPrevOrigin — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testPairingOfMrkAsAnnotationOnly — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSegmentPropertiesOutputUsingSegLevelData — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSegmentPropertiesOutputUsingTCLevelData — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSplitSegmentPropertiesUsingSegLevelData — requires sdlxliff testdata

// ---- XLIFFFilterXtmPropTest (2 tests — XTM XLIFF properties, requires testdata) ----

// okapi-unmapped: XLIFFFilterXtmPropTest#testXtmDetection — requires XTM testdata
// okapi-unmapped: XLIFFFilterXtmPropTest#testSegmentProperties — requires XTM testdata

// ---- SdlXliffConfLevelTest (6 tests — Java-internal SDL confidence level API) ----

// okapi-unmapped: SdlXliffConfLevelTest#testFromConfValue — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testFromConfValueInvalid — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testFromStateValue — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testFromStateValueInvalid — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testIsValidConfValue — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testIsValidStateValue — Java-internal SDL confidence level enum API

// ---- XLIFFFilterBalancingTest (9 tests — inline code balancing across segments) ----

// okapi-unmapped: XLIFFFilterBalancingTest#testValidBalancingWithCTypesAfterJoinAll — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testValidBalancingOverMultipleSegmentsAfterJoinAll — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testValidBalancingBetweenSegmentsAfterJoinAll — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testValidBalancingWithBxAndGTagsAfterJoinAll — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsAfterJoinAll — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAll — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAllWithNamespaces — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testDifferentCTypes — requires balancing testdata files
// okapi-unmapped: XLIFFFilterBalancingTest#testDifferentCTypesWithBreakingMrk — requires balancing testdata files

// ---- Double extraction roundtrip tests (require testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionFR — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionDE — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionES — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionFromDEDEtoENUS — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionSdlXliff — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionSdlXliffAll — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionWithCustomElements — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionWithMultiAltTrans — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAll — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPending — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLocked — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLockedNoTm100 — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLockedNoTm100NotMultipleMatches — requires IWS testdata

// ---- RoundTripXliffIT ----

// okapi-unmapped: RoundTripXliffIT — requires testdata files for file roundtrip
