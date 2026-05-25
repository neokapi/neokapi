package format_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipFamilyDetector mirrors the production registration of the ZIP-based
// formats that all share the PK magic prefix (OOXML / EPUB / ODF / IDML).
func zipFamilyDetector() *format.Detector {
	d := format.NewDetector()
	pk := [][]byte{{0x50, 0x4B, 0x03, 0x04}}
	d.Register("openxml", format.FormatSignature{
		MIMETypes: []string{
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		},
		MagicBytes: pk,
	})
	d.Register("epub", format.FormatSignature{
		MIMETypes:  []string{"application/epub+zip"},
		MagicBytes: pk,
		Sniff:      func(b []byte) bool { return bytes.Contains(b, []byte("application/epub+zip")) },
	})
	d.Register("odf", format.FormatSignature{
		MIMETypes:  []string{"application/vnd.oasis.opendocument.text"},
		MagicBytes: pk,
	})
	d.Register("idml", format.FormatSignature{MagicBytes: pk})
	return d
}

// TestDetectByContent_EpubSniffBeatsSharedMagic verifies that a precise Sniff
// runs ahead of the coarse shared ZIP magic — an EPUB (uncompressed "mimetype"
// member) is identified by its sniffer, not by a magic-prefix name tiebreak.
func TestDetectByContent_EpubSniffBeatsSharedMagic(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	require.NoError(t, err)
	_, _ = w.Write([]byte("application/epub+zip"))
	w, err = zw.Create("META-INF/container.xml")
	require.NoError(t, err)
	_, _ = w.Write([]byte(`<?xml version="1.0"?><container/>`))
	require.NoError(t, zw.Close())

	name, err := zipFamilyDetector().DetectByContent(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	assert.Equal(t, "epub", name)
}

// TestDetectByContent_DocxResolvesToOpenXMLNotEpub is the regression for the
// reported bug: a .docx is "just a ZIP" by magic, so without container-aware
// detection it resolved to epub (alphabetical tiebreak) and the epub reader
// then failed. mimetype now disambiguates it to openxml.
func TestDetectByContent_DocxResolvesToOpenXMLNotEpub(t *testing.T) {
	fixtures, _ := filepath.Glob("../formats/openxml/testdata/*.docx")
	if len(fixtures) == 0 {
		t.Skip("no .docx fixtures available")
	}
	content, err := os.ReadFile(fixtures[0])
	require.NoError(t, err)

	name, err := zipFamilyDetector().DetectByContent(bytes.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, "openxml", name)
}
