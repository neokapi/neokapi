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
// codedText/tagType-mismatched filters, and the `setData` reset of
// referenceFlag — every inline-code-bearing format passes 100%:
//   - html: 69/69, markdown: 46/46, idml: 70/70, openxml: 185/185,
//   - mif: 41/41, icml: 9/9, xml: 199/199
//
// The only remaining bridge-side divergences are in three formats whose
// failure mode is NOT inline-code identity:
//
//   - PO / TS: bridge applies pseudo to source text, okapi reference
//     applies it to the existing target text — different
//     TextModificationStep semantics that the Go-side transform doesn't
//     replicate; also missing TextUnit property round-trip (#, fuzzy
//     flags, type="unfinished" attrs).
//   - CSV: bridge's segmented-cell handling drops cell content into the
//     row id column.

func idmlBridgeSkips() map[string]fileSkip     { return nil }
func openxmlBridgeSkips() map[string]fileSkip  { return nil }
func mifBridgeSkips() map[string]fileSkip      { return nil }
func htmlBridgeSkips() map[string]fileSkip     { return nil }
func markdownBridgeSkips() map[string]fileSkip { return nil }

func poBridgeSkips() map[string]fileSkip {
	const reason = "bridge applies pseudo to source text (okapi pseudos the existing target); also drops fuzzy flag round-trip"
	return map[string]fileSkip{
		"AllCasesTest.po":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test01.po":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test02.po":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test03.po":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test04.po":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test05.po":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestMonoLingual_EN.po":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestMonoLingual_FR.po":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test_DrupalRussianCP1251.po":              {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test_nautilus.af.po":                      {Engines: []string{"bridge", "native"}, Reason: reason},
		"escaping.po":                              {Engines: []string{"bridge", "native"}, Reason: reason},
		"multientry_multilinecomments.po":          {Engines: []string{"bridge", "native"}, Reason: reason},
		"multientry_withtranslation.po":            {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple.po":                                {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_multilinecomments.po":              {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_multilinestringwithtranslation.po": {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_withcontext.po":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"simple_withpluralforms.po":                {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func csvBridgeSkips() map[string]fileSkip {
	const reason = "bridge segmented-cell handling drops cell content into row-id column"
	return map[string]fileSkip{
		"computer_science_article.csv":           {Engines: []string{"bridge", "native"}, Reason: reason},
		"field_delimiter_comma.csv":              {Engines: []string{"bridge", "native"}, Reason: reason},
		"some_blank_rows.csv":                    {Engines: []string{"bridge", "native"}, Reason: reason},
		"test2cols.csv":                          {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_double_quote.csv":        {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_double_quote_inside.csv": {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_single_quote.csv":        {Engines: []string{"bridge", "native"}, Reason: reason},
		"text_qualifier_single_quote_inside.csv": {Engines: []string{"bridge", "native"}, Reason: reason},
	}
}

func tsBridgeSkips() map[string]fileSkip {
	const reason = "bridge emits -ERR:PROP-NOT-FOUND- placeholder where okapi emits type=\"unfinished\" — TextUnit property round-trip gap"
	return map[string]fileSkip{
		"Complete_valid_utf8_bom_crlf.ts": {Engines: []string{"bridge", "native"}, Reason: reason},
		"TSTest01.ts":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestInQT.ts":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"TestInQT_Saved.ts":               {Engines: []string{"bridge", "native"}, Reason: reason},
		"Test_nautilus.af.ts":             {Engines: []string{"bridge", "native"}, Reason: reason},
		"autoSample.ts":                   {Engines: []string{"bridge", "native"}, Reason: reason},
		"issue531.ts":                     {Engines: []string{"bridge", "native"}, Reason: reason},
		"tstest.ts":                       {Engines: []string{"bridge", "native"}, Reason: reason},
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
