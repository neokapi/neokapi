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
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// report is the global report collector, written to disk at the end of the run.
var report = &reportCollector{}

func TestMain(m *testing.M) {
	code := bridgetest.Run(m)

	// Write HTML report.
	reportPath := os.Getenv("NEOKAPI_COMPAT_REPORT")
	if reportPath == "" {
		reportPath = filepath.Join(os.TempDir(), "neokapi-compat-report.html")
	}
	if err := report.writeReport(reportPath); err != nil {
		os.Stderr.WriteString("warning: failed to write compat report: " + err.Error() + "\n")
	} else {
		os.Stderr.WriteString("Compat report written to: " + reportPath + "\n")
	}

	os.Exit(code)
}

// formatSpec describes a format for cross-implementation testing.
type formatSpec struct {
	name        string
	newReader   func() format.DataFormatReader
	newWriter   func() format.DataFormatWriter
	filterClass string // Java Okapi filter class for bridge
	mimeType    string
	files       []testFile
	compareZIP  bool // use ZIP entry comparison instead of byte comparison
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
		name:        "html",
		newReader:   func() format.DataFormatReader { return html.NewReader() },
		newWriter:   func() format.DataFormatWriter { return html.NewWriter() },
		filterClass: "net.sf.okapi.filters.html.HtmlFilter",
		mimeType:    "text/html",
		files: []testFile{
			{"burlington_ufo_center", "okapi/filters/html/src/test/resources/burlington_ufo_center.html", "burlington_ufo_center.html"},
			{"W3CHTMHLTest1", "okapi/filters/html/src/test/resources/W3CHTMHLTest1.html", "W3CHTMHLTest1.html"},
		},
	},
	{
		name:        "xml",
		newReader:   func() format.DataFormatReader { return xml.NewReader() },
		newWriter:   func() format.DataFormatWriter { return xml.NewWriter() },
		filterClass: "net.sf.okapi.filters.xmlstream.XmlStreamFilter",
		mimeType:    "text/xml",
		files: []testFile{
			{"openoffice_input", "okapi/filters/its/src/test/resources/openoffice_input.xml", "openoffice_input.xml"},
		},
	},
	{
		name:        "properties",
		newReader:   func() format.DataFormatReader { return properties.NewReader() },
		newWriter:   func() format.DataFormatWriter { return properties.NewWriter() },
		filterClass: "net.sf.okapi.filters.properties.PropertiesFilter",
		mimeType:    "text/x-java-properties",
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
		name:        "openxml",
		newReader:   func() format.DataFormatReader { return openxml.NewReader() },
		newWriter:   func() format.DataFormatWriter { return openxml.NewWriter() },
		filterClass: "net.sf.okapi.filters.openxml.OpenXMLFilter",
		mimeType:    "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		compareZIP:  true,
		files: []testFile{
			{"big", "integration-tests/okapi/src/test/resources/openxml/docx/big.docx", "big.docx"},
			{"958-4", "okapi/filters/openxml/src/test/resources/958-4.pptx", "958-4.pptx"},
		},
	},
}

// TestFormatCompat runs identity roundtrip through native, bridge, and tikal
// for each format and compares the outputs.
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
					nativeOut := nativeRoundTrip(t, fmt.newReader, fmt.newWriter, input, inputPath)

					// Bridge roundtrip.
					bridgeOut := bridgeRoundTrip(t, registry, cfg, fmt.filterClass, input, inputPath, fmt.mimeType)

					// Compare native vs bridge.
					if fmt.compareZIP {
						match := report.compareZIP(fmt.name, file.name, "native vs bridge", nativeOut, bridgeOut)
						assert.True(t, match, "native vs bridge: ZIP contents differ (see HTML report)")
					} else {
						match := report.compareText(fmt.name, file.name, "native vs bridge", nativeOut, bridgeOut)
						assert.True(t, match, "native vs bridge: output differs (see HTML report)")
					}

					// Tikal roundtrip (optional).
					if tikalPath == "" {
						t.Log("NEOKAPI_TIKAL_PATH not set, skipping tikal comparison")
						return
					}
					tikalOut := tikalRoundTrip(t, tikalPath, input, file.filename)

					if fmt.compareZIP {
						match := report.compareZIP(fmt.name, file.name, "native vs tikal", nativeOut, tikalOut)
						assert.True(t, match, "native vs tikal: ZIP contents differ (see HTML report)")
					} else {
						match := report.compareText(fmt.name, file.name, "native vs tikal", nativeOut, tikalOut)
						assert.True(t, match, "native vs tikal: output differs (see HTML report)")
					}
				})
			}
		})
	}
}

// bridgeRoundTrip performs an identity roundtrip through the Java bridge.
func bridgeRoundTrip(t *testing.T, registry *bridge.BridgeRegistry, cfg bridge.BridgeConfig, filterClass string, input []byte, uri, mimeType string) []byte {
	t.Helper()
	result := bridgetest.RoundTrip(t, registry, cfg, filterClass, input, uri, mimeType, nil)
	return result.Output
}
