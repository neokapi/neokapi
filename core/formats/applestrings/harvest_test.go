package applestrings_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/applestrings"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readResult drains the reader for raw bytes under the given URI and reports the
// extracted block count plus the first error result (if any), never panicking.
// It is the shared harness for the malformed-input tests below.
func readResult(t *testing.T, uri string, data []byte) (blockCount int, firstErr error) {
	t.Helper()
	r := applestrings.NewReader()
	doc := &model.RawDocument{URI: uri, Encoding: "UTF-8", Reader: nopReader(data)}
	require.NoError(t, r.Open(t.Context(), doc))
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

// TestReadMalformedStrings feeds broken legacy .strings inputs through the
// hand-rolled strings lexer and asserts it never panics: a structurally invalid
// table surfaces a clean error result, while an empty or comment-only file is a
// valid (zero-entry) document. Covers unterminated quotes, a missing ';', a junk
// line, a missing '=', and an unterminated block comment.
func TestReadMalformedStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{
			name:    "unterminated quote",
			doc:     `"key" = "value`,
			wantErr: true,
		},
		{
			name:    "missing semicolon between entries",
			doc:     "\"key\" = \"value\"\n\"k2\" = \"v2\";",
			wantErr: true,
		},
		{
			name:    "junk non-entry line",
			doc:     "this is not a valid entry\n",
			wantErr: true,
		},
		{
			name:    "missing equals",
			doc:     `"key" "value";`,
			wantErr: true,
		},
		{
			name:    "unterminated block comment",
			doc:     "/* never ends\n\"k\" = \"v\";",
			wantErr: true,
		},
		{
			name:    "unterminated escape at end of value",
			doc:     `"key" = "value\`,
			wantErr: true,
		},
		{
			// A well-formed but empty table is valid: zero entries, no error.
			name:    "empty file",
			doc:     ``,
			wantErr: false,
		},
		{
			// Comment-only file: valid, the dangling comment is harmless.
			name:    "comment only",
			doc:     `/* just a comment, no entries */`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.NotPanics(t, func() {
				n, err := readResult(t, "x.strings", []byte(tt.doc))
				if tt.wantErr {
					assert.Error(t, err, "malformed .strings should surface a clean error result")
				} else {
					assert.NoError(t, err)
					assert.Zero(t, n)
				}
			})
		})
	}
}

// TestReadMalformedStringsdict feeds broken/truncated .stringsdict plist-XML
// inputs through the hand-rolled plist lexer/parser and asserts it never panics:
// a broken or truncated plist surfaces a clean error result, while a structurally
// valid plist that simply lacks a translatable <dict> is a valid (zero-leaf)
// document.
func TestReadMalformedStringsdict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{
			// Root <dict> opened but the document is truncated before it closes.
			name:    "truncated plist before dict close",
			doc:     `<?xml version="1.0"?><!DOCTYPE plist><plist version="1.0"><dict><key>a</key>`,
			wantErr: true,
		},
		{
			// A <string> opened but never closed; the enclosing dict is therefore
			// also unterminated.
			name:    "unterminated string element",
			doc:     `<plist><dict><key>a</key><dict><key>NSStringLocalizedFormatKey</key><string>oops`,
			wantErr: true,
		},
		{
			// Start tag with no closing '>' — plist tokenizer error.
			name:    "broken start tag",
			doc:     `<plist><dict><key`,
			wantErr: true,
		},
		{
			// Comment opened but never closed — plist tokenizer error.
			name:    "unterminated comment",
			doc:     `<plist><!-- never ends <dict></dict></plist>`,
			wantErr: true,
		},
		{
			// DOCTYPE internal subset opened but never closed — declaration error.
			name:    "unterminated doctype subset",
			doc:     `<?xml version="1.0"?><!DOCTYPE plist [ <!ELEMENT`,
			wantErr: true,
		},
		{
			// CDATA opened but never closed — plist tokenizer error.
			name:    "unterminated cdata",
			doc:     `<plist><dict><key>a</key><string><![CDATA[oops</string></dict></plist>`,
			wantErr: true,
		},
		{
			// A valid plist that has no <dict>: nothing translatable, but the
			// bytes are well-formed, so it round-trips with zero leaves and no
			// error.
			name:    "valid plist no dict",
			doc:     `<?xml version="1.0"?><plist version="1.0"></plist>`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.NotPanics(t, func() {
				n, err := readResult(t, "x.stringsdict", []byte(tt.doc))
				if tt.wantErr {
					assert.Error(t, err, "malformed .stringsdict should surface a clean error result")
				} else {
					assert.NoError(t, err)
					assert.Zero(t, n)
				}
			})
		})
	}
}

// TestReadNilDocument verifies Open rejects a nil document and a document with a
// nil reader rather than dereferencing them.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()

	r := applestrings.NewReader()
	require.Error(t, r.Open(t.Context(), nil), "nil document must error")

	r2 := applestrings.NewReader()
	require.Error(t, r2.Open(t.Context(), &model.RawDocument{URI: "x.strings"}),
		"document with nil reader must error")
}
