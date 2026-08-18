package flow_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2077. A container reader delegates part of its content to a sub-reader, and
// each sub-reader is a fresh instance whose block counter starts at 1 — so every
// delegated member's first block is `tu1`, and a container's members collide with
// one another. The harm is #609's and #2067's: the block store keys on (source
// document, block id) and the translate tools consult a stored target before
// running a translator, so the first member's translation is served for every
// later one — and a persistent store writes it back as the answer for next time.
//
// The discriminating shape is one container, three members, three distinct
// sources, read → translate → write, twice: the second run is the one that reads
// what the first run stored, which is where a collision becomes visible even in
// containers whose members are written back independently.
func TestContainers_MembersSharingABlockIDKeepTheirOwnTargets(t *testing.T) {
	for _, tc := range []struct {
		name      string
		file      string
		format    registry.FormatID
		doc       []byte
		configure func(format.DataFormatReader)
		// overlays is how many blocks the container holds in total. It is three
		// — one per member — everywhere but EPUB, whose chapters carry a
		// <title> apiece; those three are themselves members repeating an id,
		// and repeating the source text with it, so they count too.
		overlays int
		// writesMembersBack is false for the one container that has no path
		// from a translated member back into the file it came from. The XML
		// subfilter's skeleton leaves the matched element's bytes verbatim and
		// references no child layer, so a run over it produces the translation
		// in the store and an unchanged file — the #2078 shape in a different
		// format, tracked as #2085.
		//
		// Where it is true it means what it says and no more: that each member's
		// own translation reaches the file. It is not a fidelity assertion, and
		// deliberately not — ODF's members come back as running text with their
		// XML structure gone (#2086), which this cannot and should not hide.
		writesMembersBack bool
	}{
		{
			name:              "epub delegates every spine item to a fresh html reader",
			file:              "book.epub",
			doc:               menuEPUB(),
			overlays:          6,
			writesMembersBack: true,
		},
		{
			name:   "json delegates every subfiltered value to a fresh html reader",
			file:   "bundle.json",
			format: "json",
			doc:    []byte(`{"a_html":"<p>File</p>","b_html":"<p>Edit</p>","c_html":"<p>Help</p>"}`),
			configure: func(r format.DataFormatReader) {
				c := r.Config().(*json.Config)
				c.SubfilterFormat = "html"
				c.SubfilterRules = "_html$"
			},
			overlays:          3,
			writesMembersBack: true,
		},
		{
			name:   "xml delegates every subfiltered element to a fresh html reader",
			file:   "menu.xml",
			format: "xml",
			doc: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<menu><a>&lt;p&gt;File&lt;/p&gt;</a><b>&lt;p&gt;Edit&lt;/p&gt;</b><c>&lt;p&gt;Help&lt;/p&gt;</c></menu>`),
			configure: func(r format.DataFormatReader) {
				c := r.Config().(*xml.Config)
				c.Subfilters = []format.SubfilterMapping{
					{Pattern: "menu.a", Format: "html"},
					{Pattern: "menu.b", Format: "html"},
					{Pattern: "menu.c", Format: "html"},
				}
			},
			overlays: 3,
		},
		{
			name:              "odf delegates content.xml and styles.xml to fresh xml readers",
			file:              "menu.odt",
			doc:               menuODT(),
			overlays:          3,
			writesMembersBack: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _, store, reg := overlayKeyFixture(t)
			src := filepath.Join(root, "src", tc.file)
			require.NoError(t, os.WriteFile(src, tc.doc, 0o644))

			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg:    reg,
				SourceLocale: "en",
				Store:        store,
				ProjectRoot:  root,
				DetectFormat: func(string) registry.FormatID { return tc.format },
				ConfigureReader: func(r format.DataFormatReader, _ registry.FormatID) error {
					if tc.configure != nil {
						tc.configure(r)
					}
					return nil
				},
			})

			var text string
			for _, pass := range []string{"first", "second"} {
				out := filepath.Join(root, "out", pass, tc.file)
				require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
					pseudoTools(t, "qps"), src, out, "qps"))
				written, err := os.ReadFile(out)
				require.NoError(t, err)
				text = string(written)
				if isZip(written) {
					text = zipText(t, written)
				}
			}

			if tc.writesMembersBack {
				for _, want := range []string{"Ƒîļé", "Éđîţ", "Ĥéļþ"} {
					assert.Contains(t, text, want,
						"each member must carry the translation of its own source, "+
							"including on the pass that reads what the first one stored")
				}
			}
			assert.Len(t, overlayKeys(t, store, "qps"), tc.overlays,
				"a container's members are separate blocks, whatever their own readers call them")
		})
	}
}

func isZip(data []byte) bool { return bytes.HasPrefix(data, []byte("PK\x03\x04")) }

// zipText concatenates every entry of a zip container, so an assertion over a
// container's content does not have to know which member holds it.
func zipText(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	var sb bytes.Buffer
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		sb.Write(body)
	}
	return sb.String()
}

// menuEPUB is a three-chapter book whose chapters hold one paragraph each.
func menuEPUB() []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	_, _ = io.WriteString(w, "application/epub+zip")

	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
    <item id="ch3" href="chapter3.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/><itemref idref="ch2"/><itemref idref="ch3"/></spine>
</package>`,
		"OEBPS/chapter1.xhtml": chapterXHTML("File"),
		"OEBPS/chapter2.xhtml": chapterXHTML("Edit"),
		"OEBPS/chapter3.xhtml": chapterXHTML("Help"),
	}
	for name, content := range entries {
		fw, _ := zw.Create(name)
		_, _ = io.WriteString(fw, content)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func chapterXHTML(text string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter</title></head>
<body><p>` + text + `</p></body>
</html>`
}

// menuODT is a text document whose translatable content is split across the two
// parts the ODF reader delegates: content.xml and styles.xml.
func menuODT() []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	_, _ = io.WriteString(w, "application/vnd.oasis.opendocument.text")

	const ns = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
		`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"`
	entries := map[string]string{
		"META-INF/manifest.xml": `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">
  <manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.text"/>
  <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
  <manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>
</manifest:manifest>`,
		"content.xml": `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content ` + ns + ` office:version="1.2">
 <office:body><office:text><text:p>File</text:p><text:p>Edit</text:p></office:text></office:body>
</office:document-content>`,
		"styles.xml": `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles ` + ns + ` office:version="1.2">
 <office:master-styles><style:master-page style:name="Standard"><text:p>Help</text:p></style:master-page></office:master-styles>
</office:document-styles>`,
	}
	for name, content := range entries {
		fw, _ := zw.Create(name)
		_, _ = io.WriteString(fw, content)
	}
	_ = zw.Close()
	return buf.Bytes()
}
