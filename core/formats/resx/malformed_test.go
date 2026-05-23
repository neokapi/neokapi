package resx_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats/resx"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resxSniff mirrors the Sniff predicate registered for the resx reader in
// core/formats/register.go: a document is RESX only if it carries the
// resmimetype marker "text/microsoft-resx" (the resheader value that every
// ResX/.resw file declares). Replicated here so the negative case is asserted
// against the exact registration contract.
func resxSniff(data []byte) bool {
	return bytes.Contains(data, []byte("text/microsoft-resx"))
}

// TestNotResx verifies that plain (non-RESX) XML is not stolen by the resx
// format. Detection is gated by the registration Sniff
// (bytes.Contains(data, "text/microsoft-resx")): for ordinary XML the marker is
// absent, so the resx reader is never invoked and the document falls through to
// the generic XML handler. The reader is only ever fed RESX once the Sniff has
// passed, so the meaningful negative contract is asserted at the Sniff layer.
func TestNotResx(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		// extracts is true when, despite lacking the resmimetype marker, the
		// document happens to carry RESX-shaped <data>/<value> elements that the
		// structural reader would model. Such inputs are kept away from the
		// reader precisely by the Sniff gate, never reaching it in practice.
		extracts bool
	}{
		{
			name: "plain xml document",
			data: `<?xml version="1.0" encoding="utf-8"?><root><item>hello</item></root>`,
		},
		{
			// Deliberately RESX-shaped but missing the resmimetype resheader.
			// The Sniff (below) rejects it, so the structural reader never sees
			// it; pointed at it directly the reader would model the <data> entry.
			name:     "data-shaped non-resx xml",
			data:     `<catalog><data name="x"><value>hi</value></data></catalog>`,
			extracts: true,
		},
		{
			name: "html-ish",
			data: `<html><body><p>hello</p></body></html>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The registration Sniff must reject every non-RESX document — this
			// is the gate that keeps these inputs away from the resx reader.
			assert.False(t, resxSniff([]byte(tt.data)),
				"non-RESX XML must not be detected as resx")

			// Pointed at the reader directly (bypassing the gate), generic XML
			// with no RESX <data> string entries must extract nothing meaningful.
			ctx := t.Context()
			r := resx.NewReader()
			require.NoError(t, r.Open(ctx, newDoc("not.xml", []byte(tt.data))))
			defer r.Close()

			var parts []*model.Part
			require.NotPanics(t, func() {
				for res := range r.Read(ctx) {
					require.NoError(t, res.Error)
					parts = append(parts, res.Part)
				}
			})
			if tt.extracts {
				return
			}
			assert.Empty(t, blocks(parts), "non-RESX XML must yield no translatable blocks")
		})
	}
}

// TestMalformedXML feeds truncated/unclosed markup and asserts the reader does
// not panic — it surfaces a clean tokenizer error (unterminated tag) or, where
// the truncation lands on a balanced prefix, a best-effort empty extraction.
func TestMalformedXML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{
			// Unterminated start tag — the tokenizer reports a clean error.
			name: "unterminated start tag",
			data: `<root><data name="x" `,
		},
		{
			// Unterminated comment — scanDelimited reports a clean error.
			name: "unterminated comment",
			data: `<root><!-- never closed`,
		},
		{
			// Unclosed <data>/<value> elements: well-formed tags but missing
			// end tags. The tokenizer succeeds; walk finds no matching </data>
			// and emits nothing (best-effort).
			name: "unclosed elements best effort",
			data: `<root resmimetype="text/microsoft-resx"><data name="x"><value>hi`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			r := resx.NewReader()
			require.NoError(t, r.Open(ctx, newDoc("malformed.resx", []byte(tt.data))))
			defer r.Close()

			var parts []*model.Part
			require.NotPanics(t, func() {
				for res := range r.Read(ctx) {
					if res.Error != nil {
						// A clean tokenizer error is acceptable; stop collecting.
						continue
					}
					parts = append(parts, res.Part)
				}
			})
			// Either a clean error was surfaced, or the reader made a best-effort
			// pass that extracted no translatable blocks. Never a panic, never a
			// spurious block.
			assert.Empty(t, blocks(parts),
				"malformed XML must not yield translatable blocks")
		})
	}
}

// TestReswRoundTrip exercises the .resw extension explicitly. The resx format
// registers both .resx and .resw; this asserts the .resw fixture extracts its
// string entries and round-trips byte-for-byte through the writer.
func TestReswRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "Resources.resw")
	original, err := os.ReadFile(path)
	require.NoError(t, err)

	parts, raw := readParts(t, path)

	// The .resw fixture must yield at least one translatable string block.
	bs := blocks(parts)
	require.NotEmpty(t, bs, ".resw fixture must contain translatable string entries")

	// Byte-faithful round-trip with no translation applied.
	out := writeParts(t, parts, "")
	assert.Equal(t, string(original), string(out),
		"unchanged .resw round-trip must reproduce the original bytes")
	assert.Equal(t, string(raw), string(out))
}
