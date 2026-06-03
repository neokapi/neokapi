package txml_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/txml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAllTXML drains the reader's result channel, returning whether any clean
// PartResult.Error surfaced and the number of translatable blocks emitted. It is
// wrapped in require.NotPanics by callers so a panic fails the test with a clear
// message instead of crashing the run.
//
// A skeleton store is wired in because the reader's skeleton byte-offset
// bookkeeping (start/end offset arithmetic against the raw bytes) is the most
// likely place for an out-of-bounds slice on truncated input — exercising that
// path is the point of the robustness gate.
func readAllTXML(t *testing.T, input string) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()

	reader := txml.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)

	require.NotPanics(t, func() {
		// Open only validates the document/reader; parse errors surface
		// later, during Read.
		err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
			if result.Part != nil && result.Part.Type == model.PartBlock {
				blocks++
			}
		}
	})
	return foundError, blocks
}

// TestReadMalformedDoesNotPanic feeds malformed / garbage / truncated input and
// asserts the reader never panics, hangs, or crashes — it must either surface a
// clean PartResult.Error or return gracefully (empty/partial). This is the
// L1->L2 robustness gate.
//
// The Go XML decoder runs in non-strict mode here (Reader sets
// decoder.Strict = false), which makes it forgiving of mismatched tags and
// undefined entities, so several of these inputs parse leniently rather than
// erroring. The single contract this test asserts is robustness: no panic,
// no hang. Whether a given input surfaces an error or recovers leniently is an
// implementation detail we deliberately do not over-assert.
//
// Run with -race to surface any data race in the reader goroutine that drives
// the result channel.
func TestReadMalformedDoesNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty input",
			input: "",
		},
		{
			// Whitespace only — no <txml>, nothing to extract.
			name:  "whitespace only",
			input: "   \n\t  ",
		},
		{
			// Truncated XML declaration, nothing else.
			name:  "truncated xml declaration",
			input: `<?xml version="1.0"`,
		},
		{
			// Opening <txml> with no close and no body.
			name:  "unterminated txml root",
			input: `<?xml version="1.0" encoding="UTF-8"?><txml locale="en" targetlocale="fr">`,
		},
		{
			// <translatable> opened then EOF — exercises the
			// "unexpected EOF inside <translatable>" path.
			name:  "truncated inside translatable",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1">`,
		},
		{
			// <segment> opened then EOF — "unexpected EOF inside <segment>".
			name:  "truncated inside segment",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1">`,
		},
		{
			// <source> opened then EOF mid text — "unexpected EOF inside <source>".
			name:  "truncated inside source",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1"><source>Hello`,
		},
		{
			// <ut> inline code opened then EOF — "unexpected EOF inside <ut>".
			name:  "truncated inside ut",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1"><source>Hi <ut x="1" type="b">`,
		},
		{
			// <target> opened then EOF — exercises the target byte-offset
			// bookkeeping on a region that never closes.
			name:  "truncated inside target",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1"><source>Hi</source><target>Salut`,
		},
		{
			// Mismatched close tag: </segments> closes nothing opened.
			// Non-strict decoder tolerates it; the point is no panic.
			name:  "mismatched close tag",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1"><source>Hi</segments></translatable></txml>`,
		},
		{
			// Crossed/overlapping tags — invalid nesting.
			name:  "overlapping tags",
			input: `<txml locale="en" targetlocale="fr"><translatable><segment><source>a<target>b</source>c</target></segment></translatable></txml>`,
		},
		{
			// Broken / undefined entity reference inside source text.
			name:  "broken entity in source",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1"><source>A &nonsuch; B</source></segment></translatable></txml>`,
		},
		{
			// Unterminated entity (no closing ';') at EOF inside source.
			name:  "unterminated entity",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1"><source>A &amp`,
		},
		{
			// Bare ampersand in attribute / text — illegal XML.
			name:  "bare ampersand",
			input: `<txml locale="en" targetlocale="fr"><translatable><segment><source>Tom & Jerry</source></segment></translatable></txml>`,
		},
		{
			// Garbage bytes — not XML at all.
			name:  "garbage bytes",
			input: "@@@ %%% ^^^ not xml at all <<<>>>",
		},
		{
			// Invalid UTF-8 bytes sprinkled through otherwise-valid markup.
			name:  "invalid utf8 bytes",
			input: "<txml locale=\"en\" targetlocale=\"fr\"><translatable><segment><source>\xff\xfe\x00bad</source></segment></translatable></txml>",
		},
		{
			// A NUL byte mid-document.
			name:  "nul byte",
			input: "<txml locale=\"en\"><translatable><segment><source>a\x00b</source></segment></translatable></txml>",
		},
		{
			// Looks like XML but has no <txml> structure at all — should
			// extract nothing and not error.
			name:  "missing txml structure",
			input: `<?xml version="1.0"?><root><child>no translatable here</child></root>`,
		},
		{
			// <translatable> with no <segment> children at all — Okapi's
			// all-commented-out behavior: emit no block, no error.
			name:  "empty translatable",
			input: `<txml locale="en" targetlocale="fr"><translatable blockId="b1"></translatable></txml>`,
		},
		{
			// Unclosed comment swallowing the rest of the document.
			name:  "unterminated comment",
			input: `<txml locale="en" targetlocale="fr"><translatable><!-- never closed <segment><source>x</source></segment></translatable></txml>`,
		},
		{
			// Unclosed CDATA section.
			name:  "unterminated cdata",
			input: `<txml locale="en" targetlocale="fr"><translatable><segment><source><![CDATA[unterminated`,
		},
		{
			// Only the closing </txml> — nothing opened.
			name:  "lone closing tag",
			input: `</txml>`,
		},
		{
			// Lone byte-order mark with no following content.
			name:  "lone bom",
			input: "\uFEFF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAllTXML(t, tt.input)
			})
		})
	}
}

// TestReadDeeplyNestedDoesNotPanic feeds a deeply nested chain of unknown
// elements inside a <source> to exercise the recursive skipElement /
// parseInlineContent paths. Go grows goroutine stacks on demand, so this must
// complete without a stack-overflow panic.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadDeeplyNestedDoesNotPanic(t *testing.T) {
	t.Parallel()
	const depth = 5000
	input := `<txml locale="en" targetlocale="fr"><translatable blockId="b1"><segment segmentId="s1"><source>` +
		strings.Repeat("<x>", depth) + "deep" + strings.Repeat("</x>", depth) +
		`</source></segment></translatable></txml>`

	require.NotPanics(t, func() {
		readAllTXML(t, input)
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking, returning a clean error.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := txml.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}

// TestReadEmptyEmitsNoBlocks confirms the empty-input case is handled
// gracefully (no blocks, no panic). Whether the layer markers surface is an
// implementation detail; the contract is "no block, no crash".
func TestReadEmptyEmitsNoBlocks(t *testing.T) {
	t.Parallel()
	_, blocks := readAllTXML(t, "")
	assert.Equal(t, 0, blocks, "empty input must yield no translatable blocks")
}
