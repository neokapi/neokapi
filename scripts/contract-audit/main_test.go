package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
)

// TestResolveNativeFormat covers the class-level routing for multi-filter
// Okapi packages (table, plaintext) plus the single-package aliases. This
// is the core of the #616 fix: each native format must resolve to exactly
// one canonical id so no phantom 0-okapi duplicate rows survive.
func TestResolveNativeFormat(t *testing.T) {
	cases := []struct {
		name       string
		okapiPkg   string
		shortClass string
		want       string
	}{
		// net.sf.okapi.filters.table splits per class:
		{"csv-comma", "table", "CommaSeparatedValuesFilterTest", "csv"},
		{"csv-tsv", "table", "TabSeparatedValuesFilterTest", "csv"},
		{"csv-generic-table", "table", "TableFilterTest", "csv"},
		{"fixedwidth", "table", "FixedWidthColumnsFilterTest", "fixedwidth"},
		// An unlisted class from the table package falls back to csv.
		{"table-default", "table", "SomeFutureTableFilterTest", "csv"},
		// table IT classes route to the generic csv default.
		{"table-it-roundtrip", "table", "RoundTripTableIT", "csv"},
		{"table-it-xliff", "table", "TableXliffCompareIT", "csv"},
		// net.sf.okapi.filters.plaintext splits per class:
		{"plaintext-base", "plaintext", "PlainTextFilterTest", "plaintext"},
		{"plaintext-regex", "plaintext", "RegexPlainTextFilterTest", "plaintext"},
		{"paraplaintext", "plaintext", "ParaPlainTextFilterTest", "paraplaintext"},
		{"splicedlines", "plaintext", "SplicedLinesFilterTest", "splicedlines"},
		{"plaintext-default", "plaintext", "SomethingElseTest", "plaintext"},
		// Single-package aliases (whole package → one native format):
		{"php", "php", "PHPContentFilterTest", "phpcontent"},
		{"openoffice", "openoffice", "ODFFilterTest", "odf"},
		{"openoffice-legacy", "openoffice", "OpenOfficeFilterTest", "odf"},
		{"xmlstream", "xmlstream", "XmlStreamFilterTest", "xml"},
		// No alias, no split: identity.
		{"html-identity", "html", "HtmlSnippetsTest", "html"},
		{"transtable-identity", "transtable", "TransTableFilterTest", "transtable"},
		{"regex-identity", "regex", "RegexFilterTest", "regex"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveNativeFormat(tt.okapiPkg, tt.shortClass); got != tt.want {
				t.Errorf("resolveNativeFormat(%q, %q) = %q, want %q", tt.okapiPkg, tt.shortClass, got, tt.want)
			}
		})
	}
}

// TestBuildDoc_NoPhantomRows asserts the structural invariant the #616 fix
// guarantees: after class-level re-keying, no row exists that has native
// tests, zero Okapi tests, AND duplicates another row's Okapi data. The
// table package's four classes must land in exactly two native rows (csv,
// fixedwidth), each carrying its own Okapi tests, with the Okapi filter id
// recorded as a sublabel, and no leftover `table` row.
func TestBuildDoc_NoPhantomRows(t *testing.T) {
	// Simulate parsed Okapi surefire results: one suite per Okapi @Test
	// class, all from the net.sf.okapi.filters.table package, routed by
	// resolveNativeFormat into per-native-format filterResults.
	okapiByFilter := map[string]*filterResult{}
	okapiIDs := map[string]map[string]bool{}
	addClass := func(okapiPkg, fqClass string, methods ...string) {
		native := resolveNativeFormat(okapiPkg, shortClass(fqClass))
		fr := okapiByFilter[native]
		if fr == nil {
			fr = &filterResult{}
			okapiByFilter[native] = fr
		}
		suite := testSuite{Name: fqClass}
		for _, m := range methods {
			suite.Tests = append(suite.Tests, testCase{Name: m, ClassName: fqClass, Status: "pass"})
			suite.Total++
			suite.Passed++
		}
		addSuiteToResult(fr, suite)
		recordOkapiID(okapiIDs, native, okapiPkg)
	}
	addClass("table", "net.sf.okapi.filters.table.CommaSeparatedValuesFilterTest", "testA", "testB")
	addClass("table", "net.sf.okapi.filters.table.FixedWidthColumnsFilterTest", "testFw1")
	addClass("table", "net.sf.okapi.filters.table.TableFilterTest", "testTbl")
	addClass("table", "net.sf.okapi.filters.table.TabSeparatedValuesFilterTest", "testTsv")

	// Native go-test results, keyed by native format dir name.
	nativeByFilter := map[string]*filterResult{
		"csv":        {Total: 10, Passed: 10, Suites: []testSuite{{Name: "csv", Total: 10}}},
		"fixedwidth": {Total: 5, Passed: 5, Suites: []testSuite{{Name: "fixedwidth", Total: 5}}},
	}

	unit := map[string]bool{}
	for name := range okapiByFilter {
		unit[name] = true
	}
	cov := newCoverage()
	doc := buildDoc(okapiByFilter, unit, indexClasses(okapiByFilter), &cov, nativeByFilter, nil, nil, nil, nil, nil, okapiIDs, "v1.48.0", "v1.48.0", "test")

	byName := map[string]filterComparison{}
	for _, fc := range doc.Filters {
		byName[fc.FilterName] = fc
	}

	// No leftover `table` row.
	if _, ok := byName["table"]; ok {
		t.Errorf("phantom `table` row survived re-keying")
	}
	// csv aggregates Comma(2) + Table(1) + Tsv(1) = 4 Okapi tests.
	csv := byName["csv"]
	if csv.Okapi == nil || csv.Okapi.Total != 4 {
		t.Errorf("csv okapi total = %v, want 4", csv.Okapi)
	}
	if csv.Native == nil || csv.Native.Total != 10 {
		t.Errorf("csv native total = %v, want 10", csv.Native)
	}
	if len(csv.OkapiFilterIDs) != 1 || csv.OkapiFilterIDs[0] != "table" {
		t.Errorf("csv okapiFilterIds = %v, want [table]", csv.OkapiFilterIDs)
	}
	// fixedwidth carries its own Okapi test, no longer a 0-okapi phantom.
	fw := byName["fixedwidth"]
	if fw.Okapi == nil || fw.Okapi.Total != 1 {
		t.Errorf("fixedwidth okapi total = %v, want 1", fw.Okapi)
	}
	if fw.Native == nil || fw.Native.Total != 5 {
		t.Errorf("fixedwidth native total = %v, want 5", fw.Native)
	}

	// Invariant: no row has native tests + zero Okapi tests while another
	// row's Okapi data would have covered it (the phantom shape).
	for _, fc := range doc.Filters {
		okT := 0
		if fc.Okapi != nil {
			okT = fc.Okapi.Total
		}
		natT := 0
		if fc.Native != nil {
			natT = fc.Native.Total
		}
		if okT == 0 && natT > 0 {
			t.Errorf("phantom row %q: 0 okapi, %d native — re-keying should have merged it", fc.FilterName, natT)
		}
	}
}

// TestClassifyDivergence covers the fault-attribution heuristic over the
// real reason phrasing used across the spec corpus (#616). Only genuine
// "native bug" reasons may classify as native-bug; everything else is a
// neutral correct-by-design / upstream / transport category.
func TestClassifyDivergence(t *testing.T) {
	cases := map[string]string{
		// native-bug: the only alarming category.
		"native bug #504 — the native archive reader extracts ZIP entries line-by-line": "native-bug",
		"native-bug: regex walker misses content":                                       "native-bug",
		// bridge-gap: config/rules can't reach the bridge, or runtime failure.
		"bridge config divergence — neokapi config key `separator` is not recognised": "bridge-gap",
		"Bridge transport gap — bridge emits zero Blocks without a rule list":         "bridge-gap",
		"bridge error: drain stream: daemon: cannot instantiate filter: okf_epub":     "bridge-gap",
		`bridge error: drain stream: daemon: Cannot invoke "javax.xml.stream`:         "bridge-gap",
		// default-diff: same semantic config → same result.
		"bridge default-config divergence (#530) — Okapi extracts the header row": "default-diff",
		// okapi-bug: upstream Okapi filter is wrong; native correct.
		"bridge bug — Okapi VTTFilter joins intra-cue lines with spaces": "okapi-bug",
		"bridge bug #486 — Okapi TTXFilter does not recognise <Seg>":     "okapi-bug",
		"okapi bug: filter mishandles entities":                          "okapi-bug",
		// scope-diff (neutral default): feature scope or representation diff.
		"bridge feature difference — Okapi RTFFilter (Trados-tagged) yields 0 blocks":  "scope-diff",
		"bridge != native (bytewise) — both pass spec assertions independently":        "scope-diff",
		"bridge segments at finer granularity — Okapi DoxygenFilter extracts 3 blocks": "scope-diff",
		// missing-filter.
		"the bridge does not ship the okf_versifiedtxt filter": "missing-filter",
		// fixture.
		"synthetic-fixture artefact: the generated docx omits a required attribute": "fixture",
		// contract: parse-error-by-design.
		"raises a parse error by design — malformed input yields no blocks": "contract",
		// empty → neutral default.
		"": "scope-diff",
	}
	for detail, want := range cases {
		t.Run(want+"/"+truncForName(detail), func(t *testing.T) {
			if got := classifyDivergence(detail); got != want {
				t.Errorf("classifyDivergence(%q) = %q, want %q", detail, got, want)
			}
		})
	}
}

func truncForName(s string) string {
	if len(s) > 24 {
		return s[:24]
	}
	if s == "" {
		return "(empty)"
	}
	return s
}

func TestSplitOkapiPayload(t *testing.T) {
	tests := []struct {
		name                           string
		in                             string
		wantClass, wantMethod, wantRsn string
		wantOK                         bool
	}{
		{"bare", "HtmlSnippetsTest#testEscapes", "HtmlSnippetsTest", "testEscapes", "", true},
		{"emdash reason", "PropertiesFilterTest#testWithSubfilter — subfilter is a recipe concern", "PropertiesFilterTest", "testWithSubfilter", "subfilter is a recipe concern", true},
		{"hyphen reason", "Foo#bar - some note", "Foo", "bar", "some note", true},
		{"parenthetical", "TmxFilterTest#testOutputBasic (bridge-only, tested natively below)", "TmxFilterTest", "testOutputBasic", "bridge-only, tested natively below", true},
		{"fqn class", "net.sf.okapi.filters.html.HtmlSnippetsTest#testFoo", "net.sf.okapi.filters.html.HtmlSnippetsTest", "testFoo", "", true},
		{"no hash", "HtmlSnippetsTest.testEscapes", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, method, reason, ok := splitOkapiPayload(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if class != tt.wantClass || method != tt.wantMethod || reason != tt.wantRsn {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)", class, method, reason, tt.wantClass, tt.wantMethod, tt.wantRsn)
			}
		})
	}
}

func TestStripParamSuffix(t *testing.T) {
	cases := map[string]string{
		"testWitthDefaultConfig[0: BoldWorld.docx]":          "testWitthDefaultConfig",
		"roundTripsWithDifferentParameters[4: 896-auto.mif]": "roundTripsWithDifferentParameters",
		"plainMethod": "plainMethod",
		"":            "",
	}
	for in, want := range cases {
		if got := stripParamSuffix(in); got != want {
			t.Errorf("stripParamSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMarkerKind(t *testing.T) {
	cases := map[string]string{"": "map", "-skip": "skip", "-unmapped": "unmapped", "-deferred": "deferred"}
	for in, want := range cases {
		if got := markerKind(in); got != want {
			t.Errorf("markerKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifySkipKind(t *testing.T) {
	if got := classifySkipKind("deferred", "covered by roundtrip tests"); got != "deferred" {
		t.Errorf("deferred kind should win, got %q", got)
	}
	if got := classifySkipKind("unmapped", "no keyword here"); got != "acknowledged" {
		t.Errorf("unmapped fallback = %q, want acknowledged", got)
	}
	if got := classifySkipKind("skip", "subfilter dispatch is a recipe concern"); got != "subfilter" {
		t.Errorf("reason keyword should classify, got %q", got)
	}
}

// TestScanFile_StandaloneSkips verifies that reviewed not-applicable markers
// (skip/unmapped/deferred) are kept even when the block precedes a non-Test
// helper func, while a live // okapi: map is dropped if orphaned. This is the
// openxml regression: 375 // okapi-unmapped: lines stacked above testdataDir
// were silently discarded (#611).
func TestScanFile_StandaloneSkips(t *testing.T) {
	src := `package x

// okapi-unmapped: ColorValueTest#argbValueAsRgbRepresented — Java-internal color parsing
// okapi-deferred: RoundTripIT#files — covered by double-extraction tests
// okapi: OrphanTest#testOrphan — this map has no Test func below it

func helper() {}

// okapi: HtmlSnippetsTest#testEscapes
func TestEscapes(t *testing.T) {}

// okapi-skip: VendorTest#testSdl — vendor extension, no native port
func TestDocumentingSkip(t *testing.T) {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "core", "formats", "demo", "demo_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	anns, err := scanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]annotation{}
	for _, a := range anns {
		byKey[a.JavaClass+"#"+a.JavaMethod] = a
	}
	// Standalone skip/unmapped/deferred markers above a non-Test func survive.
	if a, ok := byKey["ColorValueTest#argbValueAsRgbRepresented"]; !ok || a.Kind != "unmapped" {
		t.Errorf("standalone okapi-unmapped above helper should survive, got %+v ok=%v", a, ok)
	}
	if a, ok := byKey["RoundTripIT#files"]; !ok || a.Kind != "deferred" {
		t.Errorf("standalone okapi-deferred should survive, got %+v ok=%v", a, ok)
	}
	// An orphaned // okapi: map (not above a Test func) is dropped.
	if _, ok := byKey["OrphanTest#testOrphan"]; ok {
		t.Errorf("orphaned okapi: map should be dropped")
	}
	// A real map above a Test func attaches with the func name.
	if a, ok := byKey["HtmlSnippetsTest#testEscapes"]; !ok || a.Kind != "map" || a.GoFunc != "TestEscapes" {
		t.Errorf("okapi: map above TestEscapes wrong: %+v ok=%v", a, ok)
	}
	// A skip annotation above a Test func attaches to it too.
	if a, ok := byKey["VendorTest#testSdl"]; !ok || a.Kind != "skip" || a.GoFunc != "TestDocumentingSkip" {
		t.Errorf("okapi-skip above TestDocumentingSkip wrong: %+v ok=%v", a, ok)
	}
}

// TestLookupParityMergesSiblingKeys guards the case where one native
// format is exercised under more than one parity key: native `xml` covers
// okf_xml fixtures AND the okf_xmlstream spec. lookupParity must merge
// them, not return the first match, or the spec rows are dropped.
func TestLookupParityMergesSiblingKeys(t *testing.T) {
	bridgeByFilter := map[string]*bridgeRows{
		"xml": {Read: &parityRow{Status: "pass"}}, // okf_xml fixtures, no spec
		"xmlstream": {Spec: []*specRow{
			{FeatureID: "f", Example: "e", Status: "pass"},
		}},
	}
	nativeToParityKey := map[string]string{"xml": "xmlstream"} // from spec format id
	got, ok := lookupParity(bridgeByFilter, nativeToParityKey, "xml")
	if !ok {
		t.Fatal("lookupParity(xml) not found")
	}
	if got.Read == nil {
		t.Error("merged result lost the okf_xml Read row")
	}
	if len(got.Spec) != 1 {
		t.Errorf("merged result should carry the okf_xmlstream spec row, got %d", len(got.Spec))
	}
}

// A filter known only from the Failsafe integration tests has none of its
// Okapi unit tests in the audit: a spec ref to a unit-test class is counted as
// unchecked and reported, never read as drift, while a ref to the integration
// test class that ran is checked. A filter whose unit tests ran is checked: a
// reference to a test that does not exist is drift.
func TestSpecRefs_CheckedOnlyWhereUnitTestsRan(t *testing.T) {
	okapiByFilter := map[string]*filterResult{
		"html": {Suites: []testSuite{{Name: "net.sf.okapi.filters.html.HtmlFilterTest", Tests: []testCase{
			{Name: "testA", ClassName: "net.sf.okapi.filters.html.HtmlFilterTest", Status: "pass"},
		}}}},
		// Only an integration test, as the Failsafe merge leaves it.
		"csv": {Suites: []testSuite{{Name: "RoundTripCsvIT", Tests: []testCase{
			{Name: "roundTrip", ClassName: "net.sf.okapi.roundtrip.integration.RoundTripCsvIT", Status: "pass"},
		}}}},
	}
	specs := map[string]*spec.Spec{
		"html": {Features: []spec.Feature{{ID: "a", OkapiRefs: []string{"HtmlFilterTest#testA", "HtmlFilterTest#gone"}}}},
		"csv":  {Features: []spec.Feature{{ID: "b", OkapiRefs: []string{"CommaSeparatedValuesFilterTest#testFileEvents", "CommaSeparatedValuesFilterTest#testQuotes", "RoundTripCsvIT#roundTrip", "RoundTripCsvIT#gone"}}}},
	}
	cov := newCoverage()
	doc := buildDoc(okapiByFilter, map[string]bool{"html": true}, indexClasses(okapiByFilter), &cov, nil, nil, specs, nil, nil, nil, nil, "v1.48.0", "v1.48.0", "test")
	byName := map[string]filterComparison{}
	for _, fc := range doc.Filters {
		byName[fc.FilterName] = fc
	}
	if got := byName["html"].SpecDrift; len(got) != 1 || got[0].OkapiRef != "HtmlFilterTest#gone" {
		t.Errorf("html drift = %+v, want only HtmlFilterTest#gone", got)
	}
	if byName["html"].SpecRefsUnchecked != 0 {
		t.Errorf("html refs were checked; unchecked = %d", byName["html"].SpecRefsUnchecked)
	}
	if got := byName["csv"].SpecDrift; len(got) != 1 || got[0].OkapiRef != "RoundTripCsvIT#gone" {
		t.Errorf("csv drift = %+v, want only RoundTripCsvIT#gone (its class ran)", got)
	}
	if byName["csv"].SpecRefsChecked != 2 {
		t.Errorf("csv checked = %d, want the 2 integration-test refs", byName["csv"].SpecRefsChecked)
	}
	if byName["csv"].SpecRefsUnchecked != 2 {
		t.Errorf("csv unchecked = %d, want 2", byName["csv"].SpecRefsUnchecked)
	}
	var b strings.Builder
	reportUncheckedSpecRefs(&b, doc.Filters, "v1.48.0")
	if !strings.Contains(b.String(), "NOT CHECKED: 2 okapi_ref(s) in 1 filter(s)") || !strings.Contains(b.String(), "csv (2)") {
		t.Errorf("report does not name the unchecked filter: %q", b.String())
	}
	if cov.SpecRefs["html"] != 2 || cov.SpecRefs["csv"] != 2 || cov.SpecRefsNotChecked["csv"] != 2 {
		t.Errorf("coverage = %+v", cov)
	}
}

// An annotation in a format whose Okapi unit tests did not run, naming a class
// that did not run, is counted as not checked, never as drift. One outside
// core/formats, in a format whose tests ran, or naming a class that ran
// (an integration test here) is checked.
func TestCheckAnnotations(t *testing.T) {
	ix := indexClasses(map[string]*filterResult{
		"html": {Suites: []testSuite{{Tests: []testCase{{Name: "a", ClassName: "net.sf.okapi.filters.html.HtmlFilterTest"}}}}},
		"srt":  {Suites: []testSuite{{Tests: []testCase{{Name: "e", ClassName: "net.sf.okapi.roundtrip.integration.RoundTripSrtIT"}}}}},
	})
	anns := []annotation{
		{JavaClass: "HtmlFilterTest", JavaMethod: "a", File: "core/formats/html/html_test.go"},
		{JavaClass: "HtmlFilterTest", JavaMethod: "gone", File: "core/formats/html/html_test.go"},
		{JavaClass: "JSONFilterTest", JavaMethod: "b", File: "core/formats/json/json_test.go"},
		{JavaClass: "JSONFilterTest", JavaMethod: "c", File: "core/formats/json/more_test.go"},
		{JavaClass: "X", JavaMethod: "d", File: "cli/x_test.go"},
		{JavaClass: "RoundTripSrtIT", JavaMethod: "e", File: "core/formats/srt/srt_test.go"},
	}
	cov := newCoverage()
	drift := checkAnnotations(anns, map[string]bool{"html": true}, ix, &cov)
	if len(drift) != 2 || drift[0].JavaClass != "X" || drift[1].JavaMethod != "gone" {
		t.Errorf("drift = %+v, want cli X#d and html gone", drift)
	}
	if cov.Annotations["html"] != 2 || cov.Annotations["srt"] != 1 || cov.Annotations["(other)"] != 1 || cov.AnnotationsNotChecked["json"] != 2 {
		t.Errorf("coverage = %+v", cov)
	}
	var b strings.Builder
	reportUncheckedAnnotations(&b, cov.AnnotationsNotChecked)
	if !strings.Contains(b.String(), "NOT CHECKED: 2 annotation(s) in 1 format(s)") || !strings.Contains(b.String(), "json (2)") {
		t.Errorf("report = %q", b.String())
	}
}

// A fully qualified reference matches only that class. A simple name that two
// modules share is ambiguous and reported, not guessed at.
func TestResolve_FullNamesAndAmbiguity(t *testing.T) {
	ix := indexClasses(map[string]*filterResult{
		"xml":  {Suites: []testSuite{{Tests: []testCase{{Name: "m", ClassName: "net.sf.okapi.filters.its.XMLFilterTest"}}}}},
		"html": {Suites: []testSuite{{Tests: []testCase{{Name: "n", ClassName: "net.sf.okapi.filters.html.XMLFilterTest"}}}}},
		"tmx":  {Suites: []testSuite{{Tests: []testCase{{Name: "t", ClassName: "net.sf.okapi.filters.tmx.TmxFilterTest"}}}}},
	})
	cases := []struct {
		ref        string
		ran        bool
		wantState  string
		wantExists bool
	}{
		{"net.sf.okapi.filters.its.XMLFilterTest#m", false, refChecked, true},
		{"net.sf.okapi.filters.its.XMLFilterTest#n", false, refChecked, false},
		{"net.sf.okapi.filters.other.XMLFilterTest#m", false, refUnchecked, false},
		{"net.sf.okapi.filters.other.XMLFilterTest#m", true, refChecked, false},
		{"XMLFilterTest#m", true, refAmbiguous, false},
		{"TmxFilterTest#t", false, refChecked, true},
		{"TmxFilterTest#gone", false, refChecked, false},
		{"NoSuchTest#x", false, refUnchecked, false},
		{"NoSuchTest#x", true, refChecked, false},
	}
	for _, c := range cases {
		state, exists := ix.resolve(c.ref, c.ran)
		if state != c.wantState || exists != c.wantExists {
			t.Errorf("resolve(%q, ran=%v) = %s, %v; want %s, %v", c.ref, c.ran, state, exists, c.wantState, c.wantExists)
		}
	}
	cov := newCoverage()
	checkAnnotations([]annotation{{JavaClass: "XMLFilterTest", JavaMethod: "m", File: "core/formats/xml/x_test.go"}},
		map[string]bool{"xml": true}, ix, &cov)
	if len(cov.Ambiguous) != 1 || cov.Annotations["xml"] != 0 {
		t.Errorf("an ambiguous annotation must be reported and not counted as checked: %+v", cov)
	}
}

// The run fails when it checked nothing, when an expected module wrote no
// reports, and when a filter or format checks fewer references than the
// baseline. A rise is reported, not failed.
func TestCoverageFailures(t *testing.T) {
	empty := newCoverage()
	if f := coverageFailures(empty, nil, nil); len(f) != 1 || !strings.Contains(f[0], "nothing was checked") {
		t.Errorf("empty run: %v", f)
	}
	cov := newCoverage()
	cov.SpecRefs["html"] = 10
	cov.Annotations["tmx"] = 3
	if f := coverageFailures(cov, nil, nil); len(f) != 0 {
		t.Errorf("a run that checked something, with nothing missing: %v", f)
	}
	missing := missingModules([]string{"html", "tmx", "xliff"}, []string{"html", "tmx"})
	if f := coverageFailures(cov, missing, nil); len(f) != 1 || !strings.Contains(f[0], "xliff") {
		t.Errorf("missing module: %v", f)
	}
	drops, rises := compareBaseline(baseline{
		SpecRefs:    map[string]int{"html": 12, "po": 4},
		Annotations: map[string]int{"tmx": 2},
	}, cov)
	if len(drops) != 2 || !strings.Contains(drops[0], "html: 10 checked, baseline 12") || !strings.Contains(drops[1], "po: 0 checked, baseline 4") {
		t.Errorf("drops = %v", drops)
	}
	if len(rises) != 1 || !strings.Contains(rises[0], "tmx: 3 checked, baseline 2") {
		t.Errorf("rises = %v", rises)
	}
	if f := coverageFailures(cov, nil, drops); len(f) != 2 {
		t.Errorf("baseline drops must fail: %v", f)
	}
}

// moduleOf reads the module from a report's path; expectedModules takes the
// modules that have tests from the tree, not the ones that reported.
func TestModules(t *testing.T) {
	if got := moduleOf("/w/okapi-java/okapi/filters/html/target/surefire-reports/TEST-x.xml"); got != "html" {
		t.Errorf("moduleOf = %q", got)
	}
	dir := t.TempDir()
	for _, m := range []string{"html/src/test/java", "tmx/src/test/java", "notests/src/main/java"} {
		if err := os.MkdirAll(filepath.Join(dir, m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := expectedModules("all", dir)
	if err != nil || strings.Join(got, ",") != "html,tmx" {
		t.Errorf("expectedModules(all) = %v, %v", got, err)
	}
	if got, _ := expectedModules("tmx, html", ""); strings.Join(got, ",") != "html,tmx" {
		t.Errorf("expectedModules(list) = %v", got)
	}
	reports := filepath.Join(dir, "okapi", "filters", "html", "target", "surefire-reports")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reports, "TEST-a.xml"), []byte("<testsuite/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := modulesWithReports(dir); strings.Join(got, ",") != "html" {
		t.Errorf("modulesWithReports = %v", got)
	}
}

// A path relative to the repository root keeps its directories, so the
// annotation's format can be read from it.
func TestRelPath_RepoRelative(t *testing.T) {
	for in, want := range map[string]string{
		"core/formats/json/json_test.go":              "core/formats/json/json_test.go",
		"/srv/build/neokapi/core/formats/a/a_test.go": "core/formats/a/a_test.go",
		"elsewhere/a_test.go":                         "a_test.go",
	} {
		if got := relPath(in); got != want {
			t.Errorf("relPath(%q) = %q, want %q", in, got, want)
		}
	}
}
