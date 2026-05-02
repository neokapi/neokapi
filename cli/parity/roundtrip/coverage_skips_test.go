//go:build parity

package roundtrip_test

// Per-file bridge skip maps for formats where bridge's pseudo-translated
// output diverges from the okapi reference on specific fixtures. Native
// is included on every entry because each format already format-default-
// skips native; the per-file entry extends that to bridge for these
// specific files.
//
// The lists are extracted from a real round-trip suite run against the
// okapi-testdata-1.48.0 release. As bridge bugs get fixed upstream,
// drop entries and the corresponding sub-tests start asserting again.
//
// After the okapi-bridge fixes — per-field source-code hydrate, the
// `Code(TagType, String)` ctor data-loss fix, the id-only fallback for
// codedText/tagType-mismatched filters, the `setData` reset of
// referenceFlag, the Go-side target-base pseudo preference (with
// content-empty fallback to source), and the existing-target property
// carry-over in the streaming applier — every inline-code-bearing
// format plus PO and CSV pass 100%:
//   - html: 69/69, markdown: 46/46, idml: 70/70, openxml: 185/185,
//   - mif: 41/41, icml: 9/9, xml: 199/199, po: 24/24, csv: 32/32
//
// The only remaining divergences live in TS, where the upstream okapi
// pseudo reference path itself emits -ERR:PROP-NOT-FOUND- in place of
// the TS filter's type="unfinished" / variants="no" attributes. The
// bridge produces the correct attribute strings; we just can't compare
// against broken reference output. Marked as engine="okapi" so the
// whole fixture is skipped.

func idmlBridgeSkips() map[string]fileSkip     { return nil }
func openxmlBridgeSkips() map[string]fileSkip  { return nil }
func mifBridgeSkips() map[string]fileSkip      { return nil }
func htmlBridgeSkips() map[string]fileSkip     { return nil }
func markdownBridgeSkips() map[string]fileSkip { return nil }

func poBridgeSkips() map[string]fileSkip { return nil }

func csvBridgeSkips() map[string]fileSkip { return nil }

// tsBridgeSkips marks fixtures where the upstream okapi pseudo reference
// path itself emits -ERR:PROP-NOT-FOUND- in place of the TS filter's
// type="unfinished" / variants="no" attributes. The bridge produces the
// correct attribute strings; we just can't compare against broken
// reference output. The bug is in TextModificationStep stripping the
// target property when applying pseudo to existing targets via the TS
// filter. Marked as engine="okapi" so the skip note reflects that the
// failure is reference-side, not bridge-side.
func tsBridgeSkips() map[string]fileSkip {
	const reason = "okapi pseudo reference emits -ERR:PROP-NOT-FOUND- where the TS filter expects type=\"unfinished\" — upstream TextModificationStep+TS filter property-loss bug"
	return map[string]fileSkip{
		"Complete_valid_utf8_bom_crlf.ts": {Engines: []string{"okapi"}, Reason: reason},
		"TSTest01.ts":                     {Engines: []string{"okapi"}, Reason: reason},
		"TestInQT.ts":                     {Engines: []string{"okapi"}, Reason: reason},
		"TestInQT_Saved.ts":               {Engines: []string{"okapi"}, Reason: reason},
		"Test_nautilus.af.ts":             {Engines: []string{"okapi"}, Reason: reason},
		"alarm_ro.ts":                     {Engines: []string{"okapi"}, Reason: reason},
		"issue531.ts":                     {Engines: []string{"okapi"}, Reason: reason},
		"tstest.ts":                       {Engines: []string{"okapi"}, Reason: reason},
	}
}

// icmlMergedSkips folds the 5 fixtures that crash upstream Okapi's merger
// in with any bridge-side divergences. After the hydrate fix all bridge
// fixtures pass — only the okapi-crashes remain skipped.
func icmlMergedSkips() map[string]fileSkip {
	const okapiReason = "upstream Okapi icml merge crashes on this fixture"
	return map[string]fileSkip{
		"OpenofficeFootnoteTest.icml":                                {Engines: []string{"okapi"}, Reason: okapiReason},
		"TakeItNoItsYoursReallyTheExcellentInevitabilityOfFree.icml": {Engines: []string{"okapi"}, Reason: okapiReason},
		"TestArticle.icml":                                           {Engines: []string{"okapi"}, Reason: okapiReason},
		"ThreeParagraphFootnoteTest.icml":                            {Engines: []string{"okapi"}, Reason: okapiReason},
		"WordFootnoteTest.icml":                                      {Engines: []string{"okapi"}, Reason: okapiReason},
	}
}
