package androidxml_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/androidxml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readResult drains the reader for a document and reports the extracted block
// count plus the first error result (if any), never panicking. It is the shared
// harness for the malformed-input tests below.
func readResult(t *testing.T, doc string) (blockCount int, firstErr error) {
	t.Helper()
	r := androidxml.NewReader()
	require.NoError(t, r.Open(t.Context(), newDoc("strings.xml", []byte(doc))))
	defer r.Close()
	for res := range r.Read(t.Context()) {
		if res.Error != nil && firstErr == nil {
			firstErr = res.Error
		}
		if res.Part != nil && res.Part.Type == model.PartBlock {
			blockCount++
		}
	}
	return blockCount, firstErr
}

// TestReadMalformedXML feeds a range of broken Android resource documents and
// asserts the hand-rolled tokenizer/reader never panics: each input yields
// either a clean error result on the channel or a best-effort partial
// extraction. The tokenizer surfaces unterminated lexical constructs as errors;
// a merely unbalanced element span is skipped best-effort without an error.
func TestReadMalformedXML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		doc  string
		// wantErr is true when the tokenizer must report an error (an
		// unterminated lexical construct); false when the reader recovers
		// best-effort (a structurally unbalanced but fully-tokenized document).
		wantErr bool
	}{
		{
			// Truncated <string>: the opening tag and text tokenize, but there
			// is no </string>. matchEnd fails, the entry is skipped — no panic,
			// no error, zero blocks.
			name:    "truncated unclosed string tag",
			doc:     `<?xml version="1.0" encoding="utf-8"?><resources><string name="a">Hello`,
			wantErr: false,
		},
		{
			// A <resources> with no closing tag. The container span is
			// unbalanced; the reader still walks the inner <string> best-effort.
			name:    "broken unclosed resources doc",
			doc:     `<?xml version="1.0" encoding="utf-8"?><resources><string name="a">Hi</string>`,
			wantErr: false,
		},
		{
			// A start tag with no closing '>' — the tokenizer cannot finish the
			// tag and reports a clean error.
			name:    "unterminated start tag",
			doc:     `<?xml version="1.0"?><resources><string name="a`,
			wantErr: true,
		},
		{
			// CDATA opened but never closed — tokenizer error, no panic.
			name:    "unterminated cdata",
			doc:     `<resources><string name="a"><![CDATA[oops</string></resources>`,
			wantErr: true,
		},
		{
			// Comment opened but never closed — tokenizer error, no panic.
			name:    "unterminated comment",
			doc:     `<resources><!-- never ends <string name="a">x</string></resources>`,
			wantErr: true,
		},
		{
			// End tag with no closing '>' — tokenizer error.
			name:    "unterminated end tag",
			doc:     `<resources><string name="a">x</string`,
			wantErr: true,
		},
		{
			// Pure garbage that opens a tag and never closes it.
			name:    "garbage with open tag",
			doc:     `<<<>>> not xml at all <resources`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The whole point is that none of this panics.
			assert.NotPanics(t, func() {
				_, err := readResult(t, tt.doc)
				if tt.wantErr {
					assert.Error(t, err, "malformed input should surface a clean error result")
				} else {
					assert.NoError(t, err, "best-effort recovery should not error")
				}
			})
		})
	}
}

// TestReadNilDocument verifies Open rejects a nil document and a document with a
// nil reader rather than dereferencing them.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()

	r := androidxml.NewReader()
	require.Error(t, r.Open(t.Context(), nil), "nil document must error")

	r2 := androidxml.NewReader()
	require.Error(t, r2.Open(t.Context(), &model.RawDocument{URI: "strings.xml"}),
		"document with nil reader must error")
}

// TestRichFixtureExtraction verifies the richer fixture (CDATA HTML, xliff:g
// do-not-translate spans, inline styling, <br/>, plurals with embedded codes,
// and comment-as-note) is modeled as expected.
func TestRichFixtureExtraction(t *testing.T) {
	t.Parallel()

	parts, _ := readParts(t, filepath.Join("testdata", "strings_rich.xml"))
	by := blockByName(parts)

	// Every translatable entry surfaces, including both plural items.
	wantNames := []string{
		"privacy_policy", "downloading_file", "formatted_body", "multiline_note",
		"new_messages[one]", "new_messages[other]",
	}
	for _, n := range wantNames {
		assert.Contains(t, by, n, "expected block %q", n)
	}
	require.Len(t, blocks(parts), len(wantNames))

	// CDATA (embedded HTML) is preserved verbatim as one opaque code — its
	// inner &nbsp; / &amp; entities are NOT decoded (CDATA is byte-opaque), and
	// the translatable text outside it is empty.
	privacy := by["privacy_policy"]
	require.NotNil(t, privacy)
	assert.True(t, runsHaveInlineCodes(privacy.SourceRuns()), "CDATA should be a code")
	assert.Equal(t, "", privacy.SourceText(), "CDATA contributes no translatable text")
	assert.Equal(t,
		`<![CDATA[Read our <a href="https://example.com/privacy">Privacy&nbsp;Policy</a> &amp; <b>Terms</b> before continuing.]]>`,
		model.RenderRunsWithData(privacy.SourceRuns()))
	// The preceding comment becomes a developer note.
	note, ok := privacy.Annotations["note"].(*model.NoteAnnotation)
	require.True(t, ok, "privacy_policy should carry a note")
	assert.Equal(t, "The privacy policy link is rendered as styled HTML.", note.Text)
	assert.Equal(t, "developer", note.From)

	// xliff:g span protects the filename; the connective text stays translatable.
	dl := by["downloading_file"]
	require.NotNil(t, dl)
	assert.True(t, runsHaveInlineCodes(dl.SourceRuns()))
	assert.Equal(t, "Downloading  now…", dl.SourceText())
	assert.Equal(t,
		`Downloading <xliff:g id="filename" example="report.pdf">%1$s</xliff:g> now…`,
		model.RenderRunsWithData(dl.SourceRuns()))

	// Nested inline styling (<b>, <i>) around an xliff:g span re-renders verbatim.
	body := by["formatted_body"]
	require.NotNil(t, body)
	assert.Equal(t,
		`You sent <b>%1$d</b> messages to <i><xliff:g id="recipient">%2$s</xliff:g></i>`,
		model.RenderRunsWithData(body.SourceRuns()))

	// A self-closing inline element (<br/>) is a standalone code.
	ml := by["multiline_note"]
	require.NotNil(t, ml)
	assert.Equal(t,
		`First line<br/>Second line is <u>underlined</u>`,
		model.RenderRunsWithData(ml.SourceRuns()))

	// Plural items each protect their embedded count code.
	one := by["new_messages[one]"]
	require.NotNil(t, one)
	assert.Equal(t, "plurals", one.Properties["androidxml.kind"])
	assert.Equal(t, "one", one.Properties["androidxml.quantity"])
	assert.Equal(t,
		`You have <xliff:g id="count" example="1">%1$d</xliff:g> new message`,
		model.RenderRunsWithData(one.SourceRuns()))
}

// TestRichFixtureByteFaithfulRoundTrip verifies the richer fixture round-trips
// byte-for-byte through read→write with no translation applied — exercising the
// CDATA, xliff:g, inline styling, self-close, and plural rewrite paths of the
// lossless tokenizer.
func TestRichFixtureByteFaithfulRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "strings_rich.xml")
	parts, original := readParts(t, path)
	out := writeParts(t, parts, "")
	assert.Equal(t, string(original), string(out),
		"rich-fixture round-trip must reproduce the original bytes exactly")
}

// TestRichFixtureTranslationSplice verifies that translating one plural item in
// the richer fixture splices in place while leaving the CDATA entry, the
// xliff:g markup, and the sibling plural item untouched. The translated value
// reuses the protected count code run verbatim, and reverting the single changed
// value reproduces the original byte-for-byte.
func TestRichFixtureTranslationSplice(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "strings_rich.xml")
	parts, original := readParts(t, path)

	by := blockByName(parts)
	other := by["new_messages[other]"]
	require.NotNil(t, other)

	// Build a German target reusing every non-text run (the paired xliff:g open
	// span, its inner placeholder, and its close) verbatim so the markup is
	// re-emitted unchanged; only the surrounding literal text is German.
	var runs []model.Run
	runs = append(runs, model.Run{Text: &model.TextRun{Text: "Sie haben "}})
	for _, r := range other.SourceRuns() {
		if r.Text == nil {
			runs = append(runs, r)
		}
	}
	runs = append(runs, model.Run{Text: &model.TextRun{Text: " neue Nachrichten"}})
	other.SetTargetRuns("de", runs)

	out := writeParts(t, parts, "de")
	outStr := string(out)

	// The xliff:g code survives verbatim in the translated value.
	assert.Contains(t, outStr,
		`<item quantity="other">Sie haben <xliff:g id="count" example="3">%1$d</xliff:g> neue Nachrichten</item>`)
	// The sibling "one" item and the CDATA entry are untouched.
	assert.Contains(t, outStr,
		`<item quantity="one">You have <xliff:g id="count" example="1">%1$d</xliff:g> new message</item>`)
	assert.Contains(t, outStr,
		`<![CDATA[Read our <a href="https://example.com/privacy">Privacy&nbsp;Policy</a>`)

	// Reverting the one changed value reproduces the original byte-for-byte.
	rev := strings.Replace(outStr,
		`Sie haben <xliff:g id="count" example="3">%1$d</xliff:g> neue Nachrichten`,
		`You have <xliff:g id="count" example="3">%1$d</xliff:g> new messages`, 1)
	assert.Equal(t, string(original), rev,
		"only the one translated value should differ from the original")
}
