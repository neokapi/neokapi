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

// ---- XLIFFFilterCtypeTest (7 tests — roundtrip ctype preservation) ----

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeG — ctype roundtrip requires native writer inline code support
func TestCtype_KeepCtypeG(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBx
func TestCtype_KeepCtypeBx(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBxRid
func TestCtype_KeepCtypeBxRid(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBpt
func TestCtype_KeepCtypeBpt(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeBptRid
func TestCtype_KeepCtypeBptRid(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeX
func TestCtype_KeepCtypeX(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

// okapi-unmapped: XLIFFFilterCtypeTest#testKeepCtypeXBoldAsXBold
func TestCtype_KeepCtypeXBoldAsXBold(t *testing.T) {
	t.Skip("pending: requires native writer inline code roundtrip support")
}

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

// ---- XLIFFFilterBalancingTest (9 tests — bridge-only balancing) ----

// okapi-unmapped: XLIFFFilterBalancingTest — bridge-only balancing infrastructure

// ---- XLIFFFilterConfigTest (33 tests — bridge config tests) ----

// okapi-unmapped: XLIFFFilterConfigTest — config tests covered by transform_test.go
