package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortClassName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"net.sf.okapi.filters.html.HtmlSnippetsTest", "HtmlSnippetsTest"},
		{"HtmlSnippetsTest", "HtmlSnippetsTest"},
		{"com.example.FooTest", "FooTest"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, shortClassName(tt.input))
	}
}

func TestBridgeFilterFromPkg(t *testing.T) {
	tests := []struct {
		pkg  string
		want string
	}{
		{"github.com/gokapi/gokapi/core/plugin/bridge/filters/okf_html", "html"},
		{"github.com/gokapi/gokapi/core/plugin/bridge/filters/okf_json", "json"},
		{"github.com/gokapi/gokapi/core/formats/html", ""},
		{"github.com/gokapi/gokapi/core/tools/pseudo", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, bridgeFilterFromPkg(tt.pkg), "pkg=%s", tt.pkg)
	}
}

func TestNativeFilterFromPkg(t *testing.T) {
	tests := []struct {
		pkg  string
		want string
	}{
		{"github.com/gokapi/gokapi/core/formats/json", "json"},
		{"github.com/gokapi/gokapi/core/formats/html", "html"},
		{"github.com/gokapi/gokapi/core/tools/pseudo", ""},
		{"github.com/gokapi/gokapi/core/plugin/bridge/filters/okf_json", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, nativeFilterFromPkg(tt.pkg), "pkg=%s", tt.pkg)
	}
}

func TestParseFileAnnotations(t *testing.T) {
	dir := t.TempDir()

	// Create a mock bridge test file
	bridgeDir := filepath.Join(dir, "okf_html")
	require.NoError(t, os.MkdirAll(bridgeDir, 0o755))

	content := `package okf_html

import "testing"

// okapi: HtmlSnippetsTest#testHref
func TestExtract_Href(t *testing.T) {
	// test body
}

// okapi: HtmlSnippetsTest#testBoldTag
// okapi: HtmlSnippetsTest#testItalicTag
func TestExtract_InlineCodes(t *testing.T) {
	// test body
}

func TestSomethingWithoutAnnotation(t *testing.T) {
	// no okapi annotation
}
`
	testFile := filepath.Join(bridgeDir, "html_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o644))

	anns := parseFileAnnotations(testFile, "bridge")

	assert.Len(t, anns, 3)

	// First annotation
	assert.Equal(t, "HtmlSnippetsTest", anns[0].JavaClass)
	assert.Equal(t, "testHref", anns[0].JavaMethod)
	assert.Equal(t, "TestExtract_Href", anns[0].GoTest)
	assert.Equal(t, "html", anns[0].Filter)

	// Multi-annotation (two annotations → same Go test)
	assert.Equal(t, "HtmlSnippetsTest", anns[1].JavaClass)
	assert.Equal(t, "testBoldTag", anns[1].JavaMethod)
	assert.Equal(t, "TestExtract_InlineCodes", anns[1].GoTest)

	assert.Equal(t, "HtmlSnippetsTest", anns[2].JavaClass)
	assert.Equal(t, "testItalicTag", anns[2].JavaMethod)
	assert.Equal(t, "TestExtract_InlineCodes", anns[2].GoTest)
}

func TestParseFileAnnotations_Native(t *testing.T) {
	dir := t.TempDir()

	nativeDir := filepath.Join(dir, "json")
	require.NoError(t, os.MkdirAll(nativeDir, 0o755))

	content := `package json

import "testing"

// okapi: JSONFilterTest#testSimpleValue
func TestReadSimpleValue(t *testing.T) {}

func TestNoAnnotation(t *testing.T) {}
`
	testFile := filepath.Join(nativeDir, "reader_test.go")
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o644))

	anns := parseFileAnnotations(testFile, "native")
	assert.Len(t, anns, 1)
	assert.Equal(t, "JSONFilterTest", anns[0].JavaClass)
	assert.Equal(t, "testSimpleValue", anns[0].JavaMethod)
	assert.Equal(t, "TestReadSimpleValue", anns[0].GoTest)
	assert.Equal(t, "json", anns[0].Filter)
}

func TestComputeCoverage(t *testing.T) {
	testCases := []TestCaseMatch{
		{JavaClass: "A", JavaMethod: "m1", OkapiStatus: "pass", BridgeTest: "TestM1", BridgeStatus: "pass"},
		{JavaClass: "A", JavaMethod: "m2", OkapiStatus: "pass", BridgeTest: "TestM2", BridgeStatus: "fail"},
		{JavaClass: "A", JavaMethod: "m3", OkapiStatus: "pass"}, // unmapped
		{JavaClass: "A", JavaMethod: "m4", OkapiStatus: "pass", NativeTest: "TestM4", NativeStatus: "pass"},
	}

	cs := computeCoverage(testCases)
	assert.Equal(t, 4, cs.TotalOkapi)
	assert.Equal(t, 2, cs.BridgeMapped)
	assert.Equal(t, 1, cs.BridgePassing)
	assert.Equal(t, 1, cs.NativeMapped)
	assert.Equal(t, 1, cs.NativePassing)
	assert.InDelta(t, 50.0, cs.CoveragePct, 0.01) // 2/4 = 50%
}

func TestBuildTestStatusMap(t *testing.T) {
	results := map[string]*FilterResult{
		"html": {
			Suites: []Suite{
				{
					Tests: []Test{
						{Name: "TestFoo", Status: "pass"},
						{Name: "TestBar", Status: "fail"},
					},
				},
			},
		},
	}

	m := buildTestStatusMap(results)
	assert.Equal(t, "pass", m["html/TestFoo"])
	assert.Equal(t, "fail", m["html/TestBar"])
	assert.Empty(t, m["html/TestBaz"])
}

func TestBuildTestStatusMap_Nil(t *testing.T) {
	m := buildTestStatusMap(nil)
	assert.Empty(t, m)
}

func TestFilterFromPath(t *testing.T) {
	tests := []struct {
		path string
		kind string
		want string
	}{
		{"core/plugin/bridge/filters/okf_html/html_test.go", "bridge", "html"},
		{"core/plugin/bridge/filters/okf_json/json_test.go", "bridge", "json"},
		{"core/formats/json/reader_test.go", "native", "json"},
		{"core/formats/html/reader_test.go", "native", "html"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, filterFromPath(tt.path, tt.kind), "path=%s kind=%s", tt.path, tt.kind)
	}
}
