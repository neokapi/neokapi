package formats_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A source edit must reach the file. This is the cross-format matrix for #1473:
// every registered writer is driven through the same round-trip the real
// `kapi ksed -i` takes (host.EditDocument — reader → tool → writer with a wired
// skeleton store), one word of the source is rewritten, and the output is read
// back to prove the edit survived.
//
// The defect this closes is not a missing feature but a *shortcut*: a writer that
// captured the value's verbatim bytes at read time re-emits them whenever the text
// it is about to write equals the block's current source. After a source edit that
// test is always true — the block's source IS the new text — so the original bytes
// win and the edit is silently discarded, exit 0. The shortcut is legitimate (it is
// what keeps an untouched block byte-exact); what was wrong is the discrimination.
//
// The matrix is the deliverable: it is what turns two known-broken formats into a
// closed class, and it fails for any future writer that reintroduces the shortcut
// without a witness of the text as read.

// sourceEditCase is one row of the matrix.
type sourceEditCase struct {
	// format is the registered format id.
	format registry.FormatID
	// input is the document to edit. Exactly one of input / inputFile is set.
	input string
	// inputFile is a testdata path for formats whose documents are binary
	// containers (openxml, odf, epub) and cannot be written inline.
	inputFile string
	// writeLocale overrides the locale the writer emits. Empty means the
	// monolingual source round-trip that `kapi ksed -i` performs.
	writeLocale model.LocaleID
	// from / to override the substitution for a fixture that cannot contain the
	// default word (a committed binary fixture). Empty means editedFrom/editedTo.
	from, to string
	// literal, when false, suppresses the assertion that the OLD word is gone
	// from the output bytes — for formats that legitimately keep a copy of the
	// pre-edit text elsewhere in the file (a bilingual format's <target>, a TM's
	// second language, a PO catalog's previous-msgid comment).
	literal bool
}

const (
	// editedFrom / editedTo are the substitution every case applies to every
	// source text run. They are plain ASCII with no format-significant
	// characters, so no format's escaping can mask the result.
	editedFrom = "utilize"
	editedTo   = "use"
)

// sourceEditMatrix has one row per registered writer. Keep it exhaustive:
// TestSourceEditMatrixCoversEveryWriter fails when a writer is added without a
// row here or an explicit exemption.
func sourceEditMatrix() []sourceEditCase {
	return []sourceEditCase{
		{format: "plaintext", input: "We utilize the finest ingredients.\n", literal: true},
		{format: "markdown", input: "Heading\n=======\n\nWe utilize the finest ingredients.\n", literal: true},
		{format: "mdx", input: "# Heading\n\nWe utilize the finest ingredients.\n", literal: true},
		{format: "asciidoc", input: "= Heading\n\nWe utilize the finest ingredients.\n", literal: true},
		{format: "html", input: "<html><body><p>We utilize the finest ingredients.</p></body></html>\n", literal: true},
		{format: "xml", input: "<?xml version=\"1.0\"?>\n<doc><p>We utilize the finest ingredients.</p></doc>\n", literal: true},
		{format: "yaml", input: "# Application messages\ngreeting:   \"We utilize the finest ingredients\"\nnested:\n  note: 'Please utilize the form'\n", literal: true},
		{format: "json", input: "{\n  \"greeting\": \"We utilize the finest ingredients\"\n}\n", literal: true},
		{format: "i18next", input: "{\n  \"greeting\": \"We utilize the finest ingredients\"\n}\n", literal: true},
		{format: "properties", input: "# Application strings\ngreeting = We utilize the finest ingredients\nnote:Please utilize the form\n", literal: true},
		{format: "po", input: "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"We utilize the finest ingredients\"\nmsgstr \"\"\n", literal: true},
		{
			format:  "csv",
			input:   "id,source\ngreeting,We utilize the finest ingredients\n",
			literal: true,
		},
		{format: "tsv", input: "id\tsource\ngreeting\tWe utilize the finest ingredients\n", literal: true},
		{format: "srt", input: "1\n00:00:01,000 --> 00:00:04,000\nWe utilize the finest ingredients\n", literal: true},
		{format: "vtt", input: "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nWe utilize the finest ingredients\n", literal: true},
		{format: "androidxml", input: "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <string name=\"greeting\">We utilize the finest ingredients</string>\n</resources>\n", literal: true},
		{format: "applestrings", input: "/* Greeting */\n\"greeting\" = \"We utilize the finest ingredients\";\n", literal: true},
		{format: "resx", input: resxFixture, literal: true},
		{format: "arb", input: "{\n  \"@@locale\": \"en\",\n  \"greeting\": \"We utilize the finest ingredients\"\n}\n", literal: true},
		{format: "xcstrings", input: xcstringsFixture, literal: true},
		{format: "designtokens", input: designTokensFixture, literal: true},
		{format: "messageformat", input: "greeting = We utilize the finest ingredients\n", literal: true},
		{format: "doclang", input: doclangFixture, literal: true},
		{format: "kbf", input: kbfFixture, literal: true},

		// Bilingual interchange formats. The write locale is set so the writer
		// emits the target as well as the source; the pre-edit text survives in
		// the target (or the second language), so `literal` stays false.
		{format: "xliff", input: xliffFixture, writeLocale: "fr", literal: false},
		{format: "xliff2", input: xliff2Fixture, writeLocale: "fr", literal: false},
		{format: "tmx", input: tmxFixture, writeLocale: "fr", literal: false},
		{format: "ts", input: tsFixture, writeLocale: "fr", literal: false},

		// Binary containers: real fixtures, read back through the reader.
		{format: "openxml", inputFile: "openxml/testdata/simple.docx", from: "normal", to: "plain", literal: false},
	}
}

// exemptFromSourceEditMatrix names registered writers that cannot participate,
// with the reason. These are not passes — they are formats where "edit the
// source text and write it back" is not a defined operation, so a matrix row
// would assert nothing.
var exemptFromSourceEditMatrix = map[registry.FormatID]string{
	// A compiled gettext catalog is a binary lookup table with no textual
	// document to edit in place: `kapi ksed -i` on a .mo would have to
	// recompile it, and the mo writer is an export sink (grandfathered
	// round-trip debt, see maturity_test.go).
	"mo": "compiled binary catalog — an export sink, not an editable document",
	// Media formats: the writer is the whole-asset replacement sink (AD-030).
	// Blocks come from ASR/OCR via a plugin and carry no bytes the writer can
	// put back into the asset, so there is no source-edit write path at all.
	"image": "whole-asset replacement sink — extraction blocks carry no bytes back into the asset",
	"audio": "whole-asset replacement sink — extraction blocks carry no bytes back into the asset",
	"video": "whole-asset replacement sink — extraction blocks carry no bytes back into the asset",
	// ODF and EPUB write through a package rewrite that needs the original
	// container on disk; they are covered by their own round-trip suites
	// (odf/, epub/) and their writers hold no captured-raw shortcut for block
	// content — see TestNoWriterPrefersRawOverAnEditedSource.
	"odf":  "package rewrite covered by odf's own round-trip suite",
	"epub": "package rewrite covered by epub's own round-trip suite",
}

const resxFixture = `<?xml version="1.0" encoding="utf-8"?>
<root>
  <resheader name="version"><value>2.0</value></resheader>
  <data name="greeting" xml:space="preserve">
    <value>We utilize the finest ingredients</value>
  </data>
</root>
`

const xcstringsFixture = `{
  "sourceLanguage" : "en",
  "strings" : {
    "greeting" : {
      "localizations" : {
        "en" : {
          "stringUnit" : {
            "state" : "translated",
            "value" : "We utilize the finest ingredients"
          }
        }
      }
    }
  },
  "version" : "1.0"
}
`

const designTokensFixture = `{
  "color": {
    "brand": {
      "$value": "#ff0000",
      "$description": "We utilize the finest ingredients"
    }
  }
}
`

const doclangFixture = `<?xml version="1.0" encoding="UTF-8"?>
<doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6">
  <heading level="1">Quarterly Report</heading>
  <text>We utilize the finest ingredients.</text>
</doclang>
`

const kbfFixture = `{
  "schemaVersion": "1.0",
  "kind": "kapi-bundle",
  "project": {
    "id": "app",
    "sourceLocale": "en"
  },
  "documents": [
    {
      "id": "src/App.tsx",
      "documentType": "jsx",
      "path": "src/App.tsx",
      "blocks": [
        {
          "id": "src/App.tsx:1:0",
          "translatable": true,
          "type": "jsx:element",
          "source": [{"text": "We utilize the finest ingredients"}]
        }
      ]
    }
  ]
}
`

const xliffFixture = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="app">
    <body>
      <trans-unit id="greeting">
        <source>We utilize the finest ingredients</source>
        <target>Nous employons les meilleurs ingredients</target>
      </trans-unit>
    </body>
  </file>
</xliff>
`

const xliff2Fixture = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="greeting">
      <segment>
        <source>We utilize the finest ingredients</source>
        <target>Nous employons les meilleurs ingredients</target>
      </segment>
    </unit>
  </file>
</xliff>
`

const tmxFixture = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en" adminlang="en" segtype="sentence" o-tmf="plain" creationtool="test" creationtoolversion="1" datatype="plaintext"/>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>We utilize the finest ingredients</seg></tuv>
      <tuv xml:lang="fr"><seg>Nous employons les meilleurs ingredients</seg></tuv>
    </tu>
  </body>
</tmx>
`

const tsFixture = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="fr">
<context>
    <name>MainWindow</name>
    <message>
        <source>We utilize the finest ingredients</source>
        <translation>Nous employons les meilleurs ingredients</translation>
    </message>
</context>
</TS>
`

// oldWord / newWord give the case's substitution, defaulting to the shared pair.
func (tc sourceEditCase) oldWord() string {
	if tc.from != "" {
		return tc.from
	}
	return editedFrom
}

func (tc sourceEditCase) newWord() string {
	if tc.to != "" {
		return tc.to
	}
	return editedTo
}

// editSourceRuns rewrites every text run of a block's source, which is exactly
// what `kapi ksed` does through tool.BaseTool's Transform handler.
func editSourceRuns(b *model.Block, from, to string) bool {
	edited := false
	runs := make([]model.Run, len(b.Source))
	copy(runs, b.Source)
	for i := range runs {
		if runs[i].Text == nil {
			continue
		}
		replaced := strings.ReplaceAll(runs[i].Text.Text, from, to)
		if replaced == runs[i].Text.Text {
			continue
		}
		txt := *runs[i].Text
		txt.Text = replaced
		runs[i].Text = &txt
		edited = true
	}
	if edited {
		b.SetSourceRuns(runs)
	}
	return edited
}

// runSourceEdit drives one format through the same sequence host.EditDocument
// uses for `kapi ksed -i`: a wired skeleton store between reader and writer, the
// whole part stream buffered, then written. It returns the produced bytes and
// how many blocks were edited.
//
// withSkeleton false is the writer's no-skeleton path — the one a cross-format
// `kapi convert` and any caller that did not wire a store takes. Output there is
// a reconstruction from the content model by design, so byte fidelity is not
// asserted; the edit still has to be in it.
func runSourceEdit(t *testing.T, tc sourceEditCase, input []byte, withSkeleton bool) (out []byte, edits int) {
	t.Helper()
	ctx := context.Background()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	reader, err := reg.NewReader(tc.format)
	require.NoError(t, err, "no reader for %s", tc.format)
	writer, err := reg.NewWriter(tc.format)
	require.NoError(t, err, "no writer for %s", tc.format)

	if withSkeleton {
		store, serr := format.NewWiredSkeleton(reader, writer)
		require.NoError(t, serr, "wire skeleton for %s", tc.format)
		if store != nil {
			defer store.Close()
		}
	}

	doc := &model.RawDocument{
		URI:          "matrix." + string(tc.format),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: tc.writeLocale,
		Reader:       io.NopCloser(bytes.NewReader(input)),
	}
	require.NoError(t, reader.Open(ctx, doc), "open %s", tc.format)

	var parts []*model.Part
	for res := range reader.Read(ctx) {
		require.NoError(t, res.Error, "read %s", tc.format)
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && editSourceRuns(b, tc.oldWord(), tc.newWord()) {
			edits++
		}
		parts = append(parts, res.Part)
	}
	require.NoError(t, reader.Close(), "close reader %s", tc.format)

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf), "set output %s", tc.format)
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(input)
	}
	writer.SetLocale(tc.writeLocale)

	ch := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, writer.Write(ctx, ch), "write %s", tc.format)
	require.NoError(t, writer.Close(), "close writer %s", tc.format)

	return buf.Bytes(), edits
}

// readBackSources reads a document and returns every block's source text.
func readBackSources(t *testing.T, id registry.FormatID, data []byte) []string {
	t.Helper()
	ctx := context.Background()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	reader, err := reg.NewReader(id)
	require.NoError(t, err)

	doc := &model.RawDocument{
		URI:          "readback." + string(id),
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
	require.NoError(t, reader.Open(ctx, doc), "reopen %s", id)

	var texts []string
	for res := range reader.Read(ctx) {
		require.NoError(t, res.Error, "reread %s", id)
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok {
			texts = append(texts, b.SourceText())
		}
	}
	require.NoError(t, reader.Close())
	return texts
}

// TestSourceEditSurvivesTheWrite is the matrix. For every registered writer:
// edit a block's source, write, read back, assert the edit survives.
//
// Before #1473's fix, `yaml`, `properties` and `po` failed here — each writing
// the pre-edit bytes back and reporting success.
func TestSourceEditSurvivesTheWrite(t *testing.T) {
	for _, tc := range sourceEditMatrix() {
		t.Run(string(tc.format), func(t *testing.T) {
			input := []byte(tc.input)
			if tc.inputFile != "" {
				var err error
				input, err = os.ReadFile(filepath.Clean(tc.inputFile))
				require.NoError(t, err, "fixture %s", tc.inputFile)
			}
			require.NotEmpty(t, input, "%s: fixture is empty", tc.format)

			out, edits := runSourceEdit(t, tc, input, true)
			require.Positive(t, edits,
				"%s: the fixture produced no editable source run — the case asserts nothing", tc.format)
			require.NotEmpty(t, out, "%s: the writer produced no output", tc.format)

			// The edit must be readable back out of the written document. This
			// is the assertion the shortcut defeated: the writer reported
			// success while emitting the pre-edit bytes.
			sources := strings.Join(readBackSources(t, tc.format, out), "\n")
			assert.Contains(t, sources, tc.newWord(),
				"%s: the source edit did not survive the write — the writer discarded it and reported success", tc.format)
			assert.NotContains(t, sources, tc.oldWord(),
				"%s: the pre-edit source text is still in the written document", tc.format)

			if tc.literal {
				assert.NotContains(t, string(out), tc.oldWord(),
					"%s: the pre-edit word is still in the written bytes", tc.format)
			}
		})
	}
}

// TestUneditedDocumentStaysByteIdentical is the discriminating control for the
// whole matrix. The shortcut exists to keep an untouched block byte-exact, and
// deleting it would make every test above pass — so the fix must be a sharper
// test, not a removed one. Same round-trip, no edit: the output must be the
// input, byte for byte.
//
// A format that cannot make this claim (a writer that normalises regardless) is
// listed with its reason rather than quietly excluded.
func TestUneditedDocumentStaysByteIdentical(t *testing.T) {
	for _, tc := range sourceEditMatrix() {
		if _, known := notByteExact[tc.format]; known {
			continue
		}
		t.Run(string(tc.format), func(t *testing.T) {
			input := []byte(tc.input)
			if tc.inputFile != "" {
				var err error
				input, err = os.ReadFile(filepath.Clean(tc.inputFile))
				require.NoError(t, err, "fixture %s", tc.inputFile)
			}
			// No edit: from and to are the same word.
			noop := tc
			noop.from, noop.to = tc.oldWord(), tc.oldWord()
			out, _ := runSourceEdit(t, noop, input, true)
			assert.Equal(t, string(input), string(out),
				"%s: an untouched document must round-trip byte-for-byte — "+
					"the verbatim shortcut is what guarantees that, so it must be "+
					"narrowed by the edit check, never removed", tc.format)
		})
	}
}

// notByteExact names formats whose writer normalises an untouched document by
// design, so byte equality is the wrong assertion for them. Each entry is a
// statement about that writer, not an exemption from the matrix above — every one
// of these still has to carry a source edit through.
// The list is short because it was measured, not assumed: 26 of the 29 rows above
// reproduce their input byte for byte on a no-op edit.
var notByteExact = map[registry.FormatID]string{
	// A .kbf is generated, not preserved: the writer stamps its own generator
	// and project block and emits canonical JSON indentation, so the output of
	// a bundle round-trip is a fresh bundle carrying the same content.
	"kbf": "a bundle is generated, not preserved — the writer stamps its own generator/project block",
	// The Qt writer regenerates the <TS> document (attribute order, indentation,
	// and the type="unfinished" flag) rather than replaying source bytes.
	"ts": "the writer regenerates the <TS> document rather than replaying source bytes",
	// A ZIP container: the writer's archive members carry their own extra fields
	// and deflate settings. #1472 recorded the same limit for .docx — 12 of 16
	// members stay identical and word/document.xml plus a few siblings are
	// normalised — so byte equality is claimed only where it is exactly true.
	"openxml": "ZIP container — archive extra fields and deflate settings are the writer's, not the source's",
}

// TestSourceEditSurvivesWithoutASkeleton is the matrix over the writers' OTHER
// path: no skeleton store wired, so the document is reconstructed from the
// content model. A reconstruction is allowed to differ in formatting; it is not
// allowed to differ in *content*. Any writer that reaches for a captured value
// here would drop the edit just as silently.
func TestSourceEditSurvivesWithoutASkeleton(t *testing.T) {
	for _, tc := range sourceEditMatrix() {
		if _, skip := noSkeletonPathExempt[tc.format]; skip {
			continue
		}
		t.Run(string(tc.format), func(t *testing.T) {
			input := []byte(tc.input)
			if tc.inputFile != "" {
				var err error
				input, err = os.ReadFile(filepath.Clean(tc.inputFile))
				require.NoError(t, err, "fixture %s", tc.inputFile)
			}

			out, edits := runSourceEdit(t, tc, input, false)
			require.Positive(t, edits, "%s: nothing editable in the fixture", tc.format)
			require.NotEmpty(t, out, "%s: the writer produced no output", tc.format)

			sources := strings.Join(readBackSources(t, tc.format, out), "\n")
			assert.Contains(t, sources, tc.newWord(),
				"%s: the source edit did not survive a no-skeleton write", tc.format)
			assert.NotContains(t, sources, tc.oldWord(),
				"%s: the pre-edit source text is still in the reconstruction", tc.format)
		})
	}
}

// noSkeletonPathExempt names formats whose no-skeleton path is not a document
// round-trip at all, so the assertion above would be about something else.
var noSkeletonPathExempt = map[registry.FormatID]string{
	// The OpenXML writer's no-skeleton path copies the original package and
	// substitutes only media (writeFromReparse) — it does not re-serialize
	// WordprocessingML from the content model, so it discards text edits by
	// construction. That is a different defect from this class (no captured
	// per-block value is involved) and is filed separately rather than folded
	// in: fixing it means teaching the writer to reconstruct document.xml,
	// which is a far larger change than a gate.
	"openxml": "no-skeleton path copies the package rather than re-serializing (filed separately)",
}

// bilingualLeafCase is one row of the third axis: a format that holds a source
// AND a target for the same key in one file, written with the target locale
// active. The two are separate slots, so a source edit must land in the source
// slot — and must NOT bleed into the target slot, which is the discriminating
// control: a fix that made every leaf re-render from the block would pass the
// first assertion and corrupt the translation.
type bilingualLeafCase struct {
	format registry.FormatID
	input  string
	locale model.LocaleID
	// target is the translation that must still be in the file afterwards.
	target string
}

func bilingualLeafMatrix() []bilingualLeafCase {
	return []bilingualLeafCase{
		{
			format: "xcstrings",
			locale: "fr",
			target: "Nous employons les meilleurs",
			input: `{
  "sourceLanguage" : "en",
  "strings" : {
    "greeting" : {
      "localizations" : {
        "en" : { "stringUnit" : { "state" : "translated", "value" : "We utilize the finest ingredients" } },
        "fr" : { "stringUnit" : { "state" : "translated", "value" : "Nous employons les meilleurs" } }
      }
    }
  },
  "version" : "1.0"
}
`,
		},
		{
			format: "csv",
			locale: "fr",
			target: "Nous employons",
			input:  "id,source,target\ngreeting,We utilize the finest ingredients,Nous employons\n",
		},
		{
			format: "po",
			locale: "fr",
			target: "Nous employons",
			input: "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\n" +
				"msgid \"We utilize the finest ingredients\"\nmsgstr \"Nous employons\"\n",
		},
		{
			format: "xliff",
			locale: "fr",
			target: "Nous employons les meilleurs ingredients",
			input:  xliffFixture,
		},
	}
}

// TestSourceEditSurvivesAlongsideATarget is the third axis: the writer is
// emitting a target locale, so the block it is asked about is not the one whose
// text changed. A captured-value shortcut reached for exactly here — the leaf has
// no translation for the active locale, so "keep the original" looked safe — and
// swallowed the source edit while faithfully writing the translation.
func TestSourceEditSurvivesAlongsideATarget(t *testing.T) {
	for _, tc := range bilingualLeafMatrix() {
		t.Run(string(tc.format), func(t *testing.T) {
			out, edits := runSourceEdit(t, sourceEditCase{
				format:      tc.format,
				input:       tc.input,
				writeLocale: tc.locale,
			}, []byte(tc.input), true)
			require.Positive(t, edits, "%s: nothing editable in the fixture", tc.format)

			sources := strings.Join(readBackSources(t, tc.format, out), "\n")
			assert.Contains(t, sources, editedTo,
				"%s: the source edit was discarded while a target locale was written", tc.format)
			assert.NotContains(t, sources, editedFrom,
				"%s: the pre-edit source text is still in the written document", tc.format)

			// The control: editing the source must leave the translation alone.
			assert.Contains(t, string(out), tc.target,
				"%s: the translation was lost — a source edit must not rewrite the target slot", tc.format)
		})
	}
}

// TestSourceEditMatrixCoversEveryWriter keeps the matrix exhaustive: a new
// registered writer must either take a row or be exempted with a reason, so the
// class stays closed as formats are added.
func TestSourceEditMatrixCoversEveryWriter(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	covered := map[registry.FormatID]bool{}
	for _, tc := range sourceEditMatrix() {
		covered[tc.format] = true
	}

	var missing []string
	for _, name := range reg.WriterNames() {
		if covered[name] {
			continue
		}
		if _, ok := exemptFromSourceEditMatrix[name]; ok {
			continue
		}
		missing = append(missing, string(name))
	}
	assert.Empty(t, missing,
		"registered writers with no source-edit matrix row and no exemption: %v — "+
			"add a row to sourceEditMatrix() or an entry to exemptFromSourceEditMatrix with the reason", missing)

	// The alias must not need its own row.
	assert.True(t, covered["kbf"], "kbf must be in the matrix")
}
