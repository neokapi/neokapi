package flow_test

import (
	"archive/zip"
	"bytes"
	"context"
	encxml "encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const odtMimeType = "application/vnd.oasis.opendocument.text"

// writeODT builds a minimal but conforming OpenDocument text package.
func writeODT(t *testing.T, path, contentXML, stylesXML string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// OpenDocument 1.3 part 3 §3.3: `mimetype` is the first entry and is
	// stored uncompressed, so the package type is readable from the first
	// bytes of the file.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	require.NoError(t, err)
	_, err = w.Write([]byte(odtMimeType))
	require.NoError(t, err)

	for name, body := range map[string]string{
		"content.xml": contentXML,
		"styles.xml":  stylesXML,
		"META-INF/manifest.xml": `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0">
  <manifest:file-entry manifest:full-path="/" manifest:media-type="` + odtMimeType + `"/>
  <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
  <manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>
</manifest:manifest>`,
	} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
}

// A run over an ODF document writes an ODF document. The venue is the one a
// `kapi run` uses — FileRunner over the format registry, which hands every
// reader and writer that asks for one the registry as its subfilter resolver —
// and the assertion is about the package, not about whether the translated
// strings appear somewhere in the zip.
func TestFileRunner_ODFPackageSurvivesARun(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.odt")
	writeODT(t, inputPath,
		`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:body><office:text><text:p>File</text:p><text:p>Edit</text:p></office:text></office:body></office:document-content>`,
		`<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:master-styles><text:p>Help</text:p></office:master-styles></office:document-styles>`)

	outputPath := filepath.Join(dir, "out", "input.odt")

	pseudoTool, err := tools.NewPseudoTranslateFromConfig(map[string]any{
		"target_locale": "qps",
	}, "qps")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    reg,
		SourceLocale: "en-US",
	})
	require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudoTool}, inputPath, outputPath, "qps"))

	out, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err, "the output is not a readable ZIP archive")

	require.NotEmpty(t, zr.File)
	assert.Equal(t, "mimetype", zr.File[0].Name, "mimetype must be the first entry")
	assert.Equal(t, zip.Store, zr.File[0].Method, "mimetype must be stored uncompressed")

	bodies := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		bodies[f.Name] = string(b)
	}
	assert.Equal(t, odtMimeType, bodies["mimetype"])
	assert.Contains(t, bodies, "META-INF/manifest.xml")

	for name, wantRoot := range map[string]string{
		"content.xml": "document-content",
		"styles.xml":  "document-styles",
	} {
		body, ok := bodies[name]
		require.Truef(t, ok, "%s is missing from the output package", name)
		d := encxml.NewDecoder(bytes.NewReader([]byte(body)))
		var root encxml.Name
		for {
			tok, err := d.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoErrorf(t, err, "%s does not parse as XML: %q", name, body)
			if se, ok := tok.(encxml.StartElement); ok && root.Local == "" {
				root = se.Name
			}
		}
		assert.Equalf(t, wantRoot, root.Local, "%s root element", name)
		assert.Equalf(t, "urn:oasis:names:tc:opendocument:xmlns:office:1.0", root.Space,
			"%s root namespace", name)
		assert.Containsf(t, body, "<text:p>", "%s lost its paragraph elements", name)
	}

	// Both streams were translated, and each kept its own text — styles.xml's
	// paragraph is not content.xml's.
	assert.NotContains(t, bodies["content.xml"], ">File<")
	assert.NotContains(t, bodies["styles.xml"], ">Help<")
	assert.NotEqual(t,
		betweenParagraphTags(t, bodies["content.xml"]),
		betweenParagraphTags(t, bodies["styles.xml"]),
		"styles.xml came back holding content.xml's translation")
}

// betweenParagraphTags returns the text of the first <text:p> element.
func betweenParagraphTags(t *testing.T, xmlBody string) string {
	t.Helper()
	d := encxml.NewDecoder(bytes.NewReader([]byte(xmlBody)))
	for {
		tok, err := d.Token()
		if err != nil {
			return ""
		}
		se, ok := tok.(encxml.StartElement)
		if !ok || se.Name.Local != "p" {
			continue
		}
		var text string
		require.NoError(t, d.DecodeElement(&text, &se))
		return text
	}
}
