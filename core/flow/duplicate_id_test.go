package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2067. A reader that takes a block's id from the document is trusting a
// uniqueness constraint the format states and real documents break, and every
// instance loses a translation the same silent way: the block store keys on
// (source document, block id), the translate tools consult a stored target
// before running a translator, so two units the key cannot tell apart are two
// units with one target — and a persistent store writes that back as the answer
// for next time.
//
// This is the discriminating test, run against every reader the sweep found
// whose ids the document supplies and every reader it cleared: one document,
// three units calling themselves the same thing, three distinct source texts,
// read → translate → write. Each unit must come back carrying the translation of
// its own source, and the store must hold three overlays rather than one.
//
// A cleared reader stays in the table. Its ids are a counter, so nothing here
// can collide — and this is what says so, and what would notice if a later
// change made a document-supplied key the id after all.
//
// XLIFF 1.2 is covered by TestOverlayKey_UnitsSharingAnIDInOneFileDoNotCollide,
// which also states the ad-hoc, no-project-root scope.
func TestReaders_UnitsSharingAnIDKeepTheirOwnTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		file string
		// format forces detection for a reader that claims no extension of its
		// own (Qt TS and Android resources are both sniffed, and `strings.xml`
		// is an ordinary .xml name).
		format registry.FormatID
		// configure applies the reader config the shape needs — CSV only derives
		// a block id from a cell when key columns are declared.
		configure func(format.DataFormatReader)
		doc       string
		// want is the translated text of each unit, in the pseudo-translator's
		// output alphabet. The .properties writer escapes non-ASCII, so its
		// expectations are the escaped spelling of the same three strings.
		want []string
	}{
		// ── The readers whose ids the document supplies ──
		{
			// XLIFF 2 requires `unit/@id` and requires it to be unique within
			// the enclosing <file> — the same clause as XLIFF 1.2, broken by
			// the same documents, and a <file>-scoped guarantee is not
			// uniqueness across a multi-file document anyway.
			name: "xliff2 repeats unit/@id",
			file: "menu.xlf",
			doc: `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="qps">
 <file id="f1">
  <unit id="1"><segment><source>File</source><target>File</target></segment></unit>
  <unit id="1"><segment><source>Edit</source><target>Edit</target></segment></unit>
  <unit id="1"><segment><source>Help</source><target>Help</target></segment></unit>
 </file>
</xliff>`,
		},
		{
			// TMX declares `tuid` as CDATA #IMPLIED rather than an XML ID and
			// leaves its value to the producer, so a memory whose units all
			// carry one id is a conforming file, not a broken one.
			name: "tmx repeats tuid",
			file: "memory.tmx",
			doc: `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
 <header creationtool="probe" creationtoolversion="1" segtype="sentence" o-tmf="tmx" adminlang="en" srclang="en" datatype="plaintext"/>
 <body>
  <tu tuid="1"><tuv xml:lang="en"><seg>File</seg></tuv></tu>
  <tu tuid="1"><tuv xml:lang="en"><seg>Edit</seg></tuv></tu>
  <tu tuid="1"><tuv xml:lang="en"><seg>Help</seg></tuv></tu>
 </body>
</tmx>`,
		},
		{
			// Qt's id-based workflow expects one id per string; a catalog
			// merged from several sources repeats them.
			name:   "qt ts repeats message/@id",
			file:   "app.ts",
			format: "ts",
			doc: `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="qps">
<context>
    <name>MainWindow</name>
    <message id="m1">
        <source>File</source>
        <translation type="unfinished"></translation>
    </message>
    <message id="m1">
        <source>Edit</source>
        <translation type="unfinished"></translation>
    </message>
    <message id="m1">
        <source>Help</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>`,
		},
		{
			// A declared key column holds ordinary data cells. Nothing in CSV
			// forbids two rows sharing one, and the reader does not infer a
			// column whose values repeat (csv.TestCSVIdentity_RepeatedKeyIsNotInferred)
			// precisely because a repeated key is not an address.
			name: "csv repeats a key column's value",
			file: "strings.csv",
			configure: func(r format.DataFormatReader) {
				c := r.Config().(*csv.Config)
				c.KeyColumns = []int{0}
				c.TranslatableColumns = []int{1}
			},
			doc: "id,text\nk,File\nk,Edit\nk,Help\n",
		},
		{
			// A .kbf.json bundle supplies every block's id and bundles many
			// documents into one file, so uniqueness rests entirely on whoever
			// generated it.
			name: "kbf repeats a block id",
			file: "bundle.kbf.json",
			doc: `{"schemaVersion":"1.0","kind":"kapi-bundle",
 "generator":{"id":"probe","version":"1"},
 "project":{"id":"app","sourceLocale":"en"},
 "documents":[{"id":"a.tsx","documentType":"jsx","path":"a.tsx","blocks":[
  {"id":"b1","hash":"h1","translatable":true,"type":"jsx:element","source":[{"text":"File"}]},
  {"id":"b1","hash":"h2","translatable":true,"type":"jsx:element","source":[{"text":"Edit"}]},
  {"id":"b1","hash":"h3","translatable":true,"type":"jsx:element","source":[{"text":"Help"}]}]}]}`,
		},

		// ── The readers the sweep cleared: their ids are a counter ──
		{
			// gettext identifies a message by (msgctxt, msgid), and msgctxt is
			// a shared bucket rather than a per-message key — so a catalog
			// repeats one constantly. It is the block's NAME here; the id is
			// positional.
			name: "po repeats msgctxt",
			file: "app.po",
			doc: `msgid ""
msgstr "Content-Type: text/plain; charset=UTF-8\n"

msgctxt "menu"
msgid "File"
msgstr ""

msgctxt "menu"
msgid "Edit"
msgstr ""

msgctxt "menu"
msgid "Help"
msgstr ""
`,
		},
		{
			name: "properties repeats a key",
			file: "app.properties",
			doc:  "menu=File\nmenu=Edit\nmenu=Help\n",
			// The .properties writer escapes non-ASCII, which is the format's
			// own rule and not this test's concern.
			want: []string{
				`\u0191\u00ee\u013c\u00e9`, // Ƒîļé
				`\u00c9\u0111\u00ee\u0163`, // Éđîţ
				`\u0124\u00e9\u013c\u00fe`, // Ĥéļþ
			},
		},
		{
			name: "json repeats an object key",
			file: "app.json",
			doc:  `{"menu":"File","menu":"Edit","menu":"Help"}`,
		},
		{
			name: "yaml repeats a mapping key",
			file: "app.yml",
			doc:  "menu: File\nmenu: Edit\nmenu: Help\n",
		},
		{
			name: "resx repeats data/@name",
			file: "app.resx",
			doc: `<?xml version="1.0" encoding="utf-8"?>
<root>
  <data name="menu" xml:space="preserve"><value>File</value></data>
  <data name="menu" xml:space="preserve"><value>Edit</value></data>
  <data name="menu" xml:space="preserve"><value>Help</value></data>
</root>`,
		},
		{
			name:   "android resources repeat string/@name",
			file:   "strings.xml",
			format: "androidxml",
			doc: `<?xml version="1.0" encoding="utf-8"?>
<resources>
  <string name="menu">File</string>
  <string name="menu">Edit</string>
  <string name="menu">Help</string>
</resources>`,
		},
		{
			name: "apple strings repeat a key",
			file: "Localizable.strings",
			doc:  "\"menu\" = \"File\";\n\"menu\" = \"Edit\";\n\"menu\" = \"Help\";\n",
		},
		{
			name: "arb repeats a key",
			file: "app.arb",
			doc:  `{"menu":"File","menu":"Edit","menu":"Help"}`,
		},
		{
			name: "xcstrings repeats a key",
			file: "Localizable.xcstrings",
			doc: `{"sourceLanguage":"en","version":"1.0","strings":{
 "menu":{"localizations":{"en":{"stringUnit":{"state":"translated","value":"File"}}}},
 "menu":{"localizations":{"en":{"stringUnit":{"state":"translated","value":"Edit"}}}},
 "menu":{"localizations":{"en":{"stringUnit":{"state":"translated","value":"Help"}}}}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if want == nil {
				want = []string{"Ƒîļé", "Éđîţ", "Ĥéļþ"}
			}

			root, _, store, reg := overlayKeyFixture(t)
			src := filepath.Join(root, "src", tc.file)
			require.NoError(t, os.WriteFile(src, []byte(tc.doc), 0o644))

			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg:    reg,
				SourceLocale: "en",
				Store:        store,
				ProjectRoot:  root,
				// An empty answer defers to the runner's own content-aware
				// detection, which is what every case but the two sniffed
				// formats wants — .xlf is claimed by both XLIFF versions and
				// `strings.xml` is an ordinary .xml name.
				DetectFormat: func(string) registry.FormatID { return tc.format },
				ConfigureReader: func(r format.DataFormatReader, _ registry.FormatID) error {
					if tc.configure != nil {
						tc.configure(r)
					}
					return nil
				},
			})
			out := filepath.Join(root, "out", tc.file)
			require.NoError(t, runner.RunFile(context.Background(), "pseudo-translate",
				pseudoTools(t, "qps"), src, out, "qps"))

			written, err := os.ReadFile(out)
			require.NoError(t, err)
			for _, w := range want {
				assert.Contains(t, string(written), w,
					"each unit must carry the translation of its own source")
			}
			assert.Len(t, overlayKeys(t, store, "qps"), 3,
				"three units are three overlays, whatever they call themselves")
		})
	}
}
