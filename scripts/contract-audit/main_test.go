package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
