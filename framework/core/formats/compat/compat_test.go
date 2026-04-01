//go:build integration

package compat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/formats/properties"
	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// report is the global report collector, written to disk at the end of the run.
var report = &reportCollector{}

func TestMain(m *testing.M) {
	code := bridgetest.Run(m)

	// Write HTML report.
	reportDir := os.Getenv("NEOKAPI_COMPAT_REPORT")
	if reportDir == "" {
		reportDir = filepath.Join(os.TempDir(), "neokapi-compat-report")
	}
	if err := report.writeReport(reportDir); err != nil {
		os.Stderr.WriteString("warning: failed to write compat report: " + err.Error() + "\n")
	} else {
		os.Stderr.WriteString("Compat report written to: " + reportDir + "/index.html\n")
	}

	os.Exit(code)
}

// formatSpec describes a format for cross-implementation testing.
type formatSpec struct {
	name          string
	newReader     func() format.DataFormatReader
	newWriter     func() format.DataFormatWriter
	filterClass   string // Java Okapi filter class for bridge
	mimeType      string
	files         []testFile
	compareZIP    bool                // use ZIP entry comparison instead of byte comparison
	normalize     func([]byte) []byte // optional normalization before byte comparison
	normalizeText func(string) string // optional block text normalization (default: whitespace only)
}

// testFile is a test input file path relative to the okapi-testdata root.
type testFile struct {
	name     string // display name for subtest
	relPath  string // path relative to okapi-testdata root
	filename string // filename for tikal (determines extension-based detection)
}

var formats = []formatSpec{
	{
		name:        "json",
		newReader:   func() format.DataFormatReader { return json.NewReader() },
		newWriter:   func() format.DataFormatWriter { return json.NewWriter() },
		filterClass: "net.sf.okapi.filters.json.JSONFilter",
		mimeType:    "application/json",
		files: []testFile{
			{"Josh_Test_News_Email", "integration-tests/okapi/src/test/resources/json/Josh Test News Email.json", "Josh Test News Email.json"},
		},
	},
	{
		name:          "html",
		newReader:     func() format.DataFormatReader { return html.NewReader() },
		newWriter:     func() format.DataFormatWriter { return html.NewWriter() },
		filterClass:   "net.sf.okapi.filters.html.HtmlFilter",
		mimeType:      "text/html",
		normalize:     normalizeHTML,
		normalizeText: normalizeMarkupBlockText,
		files: []testFile{
			{"burlington_ufo_center", "okapi/filters/html/src/test/resources/burlington_ufo_center.html", "burlington_ufo_center.html"},
			{"W3CHTMHLTest1", "okapi/filters/html/src/test/resources/W3CHTMHLTest1.html", "W3CHTMHLTest1.html"},
		},
	},
	{
		name:          "xml",
		newReader:     func() format.DataFormatReader { return xml.NewReader() },
		newWriter:     func() format.DataFormatWriter { return xml.NewWriter() },
		filterClass:   "net.sf.okapi.filters.xmlstream.XmlStreamFilter",
		mimeType:      "text/xml",
		normalizeText: normalizeMarkupBlockText,
		files: []testFile{
			{"openoffice_input", "okapi/filters/its/src/test/resources/openoffice_input.xml", "openoffice_input.xml"},
		},
	},
	{
		name:          "properties",
		newReader:     func() format.DataFormatReader { return properties.NewReader() },
		newWriter:     func() format.DataFormatWriter { return properties.NewWriter() },
		filterClass:   "net.sf.okapi.filters.properties.PropertiesFilter",
		mimeType:      "text/x-java-properties",
		normalizeText: normalizePropertiesBlockText,
		files: []testFile{
			{"Test01", "okapi/filters/properties/src/test/resources/Test01.properties", "Test01.properties"},
		},
	},
	{
		name:        "po",
		newReader:   func() format.DataFormatReader { return po.NewReader() },
		newWriter:   func() format.DataFormatWriter { return po.NewWriter() },
		filterClass: "net.sf.okapi.filters.po.POFilter",
		mimeType:    "application/x-gettext",
		files: []testFile{
			{"AllCasesTest", "okapi/filters/po/src/test/resources/AllCasesTest.po", "AllCasesTest.po"},
			{"Test_nautilus", "okapi/filters/po/src/test/resources/Test_nautilus.af.po", "Test_nautilus.af.po"},
		},
	},
	{
		name:        "yaml",
		newReader:   func() format.DataFormatReader { return yaml.NewReader() },
		newWriter:   func() format.DataFormatWriter { return yaml.NewWriter() },
		filterClass: "net.sf.okapi.filters.yaml.YamlFilter",
		mimeType:    "application/x-yaml",
		files: []testFile{
			{"en_2", "integration-tests/okapi/src/test/resources/yaml/en (2).yml", "en (2).yml"},
			{"en_3", "integration-tests/okapi/src/test/resources/yaml/en (3).yml", "en (3).yml"},
		},
	},
	{
		name:        "xliff",
		newReader:   func() format.DataFormatReader { return xliff.NewReader() },
		newWriter:   func() format.DataFormatWriter { return xliff.NewWriter() },
		filterClass: "net.sf.okapi.filters.xliff.XLIFFFilter",
		mimeType:    "application/xliff+xml",
		files: []testFile{
			{"SF-12-Test01", "okapi/filters/xliff/src/test/resources/SF-12-Test01.xlf", "SF-12-Test01.xlf"},
		},
	},
	{
		name:          "openxml",
		newReader:     func() format.DataFormatReader { return openxml.NewReader() },
		newWriter:     func() format.DataFormatWriter { return openxml.NewWriter() },
		filterClass:   "net.sf.okapi.filters.openxml.OpenXMLFilter",
		mimeType:      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		compareZIP:    true,
		normalizeText: normalizeMarkupBlockText,
		files: []testFile{
			{"big", "integration-tests/okapi/src/test/resources/openxml/docx/big.docx", "big.docx"},
			{"958-4", "okapi/filters/openxml/src/test/resources/958-4.pptx", "958-4.pptx"},
		},
	},
}

// TestFormatCompat runs identity roundtrip through native, bridge, and tikal
// for each format and compares the outputs at two levels:
//
//  1. Event-level (pass/fail): extract blocks from each output with the native
//     reader and compare translatable content. This is Okapi's approach —
//     "do we extract the same translatable content?"
//
//  2. Byte-level (informational): compare raw/normalized output bytes.
//     Records differences in the HTML report for analysis but does not fail.
func TestFormatCompat(t *testing.T) {
	testdataDir := bridgetest.TestdataDir(t)
	registry, cfg := bridgetest.SharedBridge(t)
	tikalPath := os.Getenv("NEOKAPI_TIKAL_PATH")

	for _, fmt := range formats {
		t.Run(fmt.name, func(t *testing.T) {
			for _, file := range fmt.files {
				t.Run(file.name, func(t *testing.T) {
					t.Parallel()

					inputPath := filepath.Join(testdataDir, file.relPath)
					input, err := os.ReadFile(inputPath)
					require.NoError(t, err, "reading test file %s", inputPath)

					// Native roundtrip.
					native := nativeRoundTrip(t, fmt.newReader, fmt.newWriter, input, inputPath)
					nativeTexts := blockTexts(native.parts, fmt.normalizeText)

					// Bridge roundtrip.
					bridgeResult := bridgetest.RoundTrip(t, registry, cfg, fmt.filterClass, input, inputPath, fmt.mimeType, nil)
					bridgeTexts := blockTexts(bridgeResult.Parts, fmt.normalizeText)

					// --- Event-level comparison (pass/fail) ---
					match := report.compareParts(fmt.name, file.name, "native vs bridge (blocks)", nativeTexts, bridgeTexts)
					assert.True(t, match, "native vs bridge: block texts differ (see HTML report)")

					// --- Byte-level comparison (informational) ---
					if !fmt.compareZIP {
						report.compareTextInfo(fmt.name, file.name, "input vs native (bytes)", input, native.output)
						report.compareTextInfo(fmt.name, file.name, "input vs bridge (bytes)", input, bridgeResult.Output)
					}
					if fmt.compareZIP {
						report.compareZIP(fmt.name, file.name, "native vs bridge (bytes)", native.output, bridgeResult.Output)
					} else if fmt.normalize != nil {
						normNative := fmt.normalize(native.output)
						normBridge := fmt.normalize(bridgeResult.Output)
						report.compareTextNormalized(fmt.name, file.name, "native vs bridge (bytes)", native.output, bridgeResult.Output, normNative, normBridge)
					} else {
						report.compareTextInfo(fmt.name, file.name, "native vs bridge (bytes)", native.output, bridgeResult.Output)
					}

					// Tikal roundtrip (optional).
					if tikalPath == "" {
						t.Log("NEOKAPI_TIKAL_PATH not set, skipping tikal comparison")
						return
					}
					tikalOut := tikalRoundTrip(t, tikalPath, input, file.filename)

					// Re-read tikal output with native reader for event comparison.
					tikalParts := extractParts(t, fmt.newReader, tikalOut, inputPath)
					tikalTexts := blockTexts(tikalParts, fmt.normalizeText)

					// --- Event-level comparison (pass/fail) ---
					match = report.compareParts(fmt.name, file.name, "native vs tikal (blocks)", nativeTexts, tikalTexts)
					assert.True(t, match, "native vs tikal: block texts differ (see HTML report)")

					// --- Byte-level comparison (informational) ---
					if !fmt.compareZIP {
						report.compareTextInfo(fmt.name, file.name, "input vs tikal (bytes)", input, tikalOut)
					}
					if fmt.compareZIP {
						report.compareZIP(fmt.name, file.name, "native vs tikal (bytes)", native.output, tikalOut)
					} else if fmt.normalize != nil {
						normNative := fmt.normalize(native.output)
						normTikal := fmt.normalize(tikalOut)
						report.compareTextNormalized(fmt.name, file.name, "native vs tikal (bytes)", native.output, tikalOut, normNative, normTikal)
					} else {
						report.compareTextInfo(fmt.name, file.name, "native vs tikal (bytes)", native.output, tikalOut)
					}
				})
			}
		})
	}
}
