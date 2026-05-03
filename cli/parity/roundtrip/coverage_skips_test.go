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
// content-empty fallback to source), the existing-target property
// carry-over in the streaming applier, and the property-preserving
// pseudo step that wraps TextModificationStep — every format passes
// 100%:
//   - html: 69/69, markdown: 46/46, idml: 70/70, openxml: 185/185,
//   - mif: 41/41, icml: 9/9, xml: 199/199, po: 24/24, csv: 32/32,
//   - ts: 9/9
//
// All bridge-skip helpers below currently return nil — they remain as
// hooks so future regressions can be carved out per-fixture without
// growing new helper functions.

func idmlBridgeSkips() map[string]fileSkip     { return nil }
func openxmlBridgeSkips() map[string]fileSkip  { return nil }
func mifBridgeSkips() map[string]fileSkip      { return nil }
func htmlBridgeSkips() map[string]fileSkip     { return nil }
func markdownBridgeSkips() map[string]fileSkip { return nil }

func poBridgeSkips() map[string]fileSkip {
	return map[string]fileSkip{
		"Test01.po": {Engines: []string{"native"}, Reason: "UTF-16 LE encoded fixture; native po reader expects UTF-8 — see task #106 for cross-format UTF-16 support"},
	}
}

func csvBridgeSkips() map[string]fileSkip { return nil }

func tsBridgeSkips() map[string]fileSkip { return nil }

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
