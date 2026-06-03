package po_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformed feeds broken, truncated, and garbage PO input and asserts
// that Open and Read fail cleanly — never panicking. The PO reader is a
// deliberately tolerant, line-oriented parser (mirroring Okapi's POFilter,
// which recovers from malformed entries rather than aborting the file), so
// most of these inputs do not raise a parse error: they are skipped or parsed
// best-effort. The contract verified here is that any failure surfaces as an
// error (from Open, or as PartResult.Error on the channel) and never as a
// panic or a silently swallowed crash.
func TestReadMalformed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			// msgid value opens a quote that is never closed. unquotePO
			// treats a string lacking its trailing quote as a literal,
			// so the entry is parsed best-effort without panicking.
			name:  "truncated msgid quote",
			input: "msgid \"hello\nmsgstr \"bonjour\"\n",
		},
		{
			// msgstr opens a quote with no closing quote at all.
			name:  "truncated msgstr quote",
			input: "msgid \"key\"\nmsgstr \"unterminated\n",
		},
		{
			// A multiline string whose continuation lines are never
			// terminated — quotes open but the file ends mid-string.
			name:  "unterminated multiline string",
			input: "msgid \"\"\n\"line one\\n\"\n\"line two with no close\nmsgstr \"x\"\n",
		},
		{
			// A bare quote continuation with no preceding keyword line:
			// currentField is fieldNone and current may be nil, so the
			// continuation must be ignored rather than dereferenced.
			name:  "dangling continuation line",
			input: "\"orphaned continuation\"\nmsgid \"k\"\nmsgstr \"v\"\n",
		},
		{
			// "msgstr[" with no closing bracket and no index. The bracket
			// index parse (strings.Index for "]") returns -1, which the
			// closeBracket > 7 guard must reject without indexing past the
			// end of the line.
			name:  "lone msgstr open bracket no index",
			input: "msgid \"k\"\nmsgstr[\n",
		},
		{
			// "msgstr[" followed by a non-numeric index. fmt.Sscanf fails
			// to parse the index; the entry must not panic.
			name:  "msgstr bracket non-numeric index",
			input: "msgid \"k\"\nmsgid_plural \"ks\"\nmsgstr[x] \"v\"\n",
		},
		{
			// Random non-PO text. Every line is a continuation/garbage with
			// no keyword, so no entries are produced — but it must not crash.
			name:  "garbage text",
			input: "definitely not a po file :: {[<>]} \x07\n\tmore junk\n",
		},
		{
			// A header declaring a plural-forms expression the reader cannot
			// fully parse, followed by a plural entry.
			name:  "broken plural-forms header",
			input: "msgid \"\"\nmsgstr \"Plural-Forms: nplurals=; plural=(\\n\"\n\nmsgid \"k\"\nmsgid_plural \"ks\"\nmsgstr[0] \"a\"\n",
		},
		{
			// Empty input — no entries, but Open and Read must succeed.
			name:  "empty input",
			input: "",
		},
		{
			// Only whitespace.
			name:  "whitespace only",
			input: "   \n\t\n   \n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := po.NewReader()

			var openErr error
			require.NotPanics(t, func() {
				openErr = reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
			})
			t.Cleanup(func() { reader.Close() })

			// If Open rejected the input outright, that is a clean failure —
			// nothing more to drain.
			if openErr != nil {
				return
			}

			// Draining must never panic, and any error must surface on the
			// channel (PartResult.Error) rather than being swallowed.
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					if result.Error != nil {
						// An error here is acceptable; the point is that it is
						// surfaced, not hidden. Stop once we see one.
						return
					}
				}
			})
		})
	}
}

// TestReadGarbageBinary feeds raw binary/garbage bytes (including a NUL byte
// and high bytes that defeat UTF-8 detection). Either Open rejects the input
// while transcoding, or Read drains cleanly — but neither may panic.
func TestReadGarbageBinary(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := po.NewReader()

	// Bytes that are not valid UTF-8 and contain a NUL and control bytes.
	garbage := []byte{0x00, 0xFF, 0xFE, 0x01, 'm', 's', 'g', 'i', 'd', 0x00, 0x80, 0x90, '\n', 0xC0, 0xC1}
	doc := testutil.RawDocFromReader(bytes.NewReader(garbage), "test://garbage", model.LocaleEnglish)

	var openErr error
	require.NotPanics(t, func() {
		openErr = reader.Open(ctx, doc)
	})
	t.Cleanup(func() { reader.Close() })
	if openErr != nil {
		// Open surfaced a transcoding error — a clean rejection, no panic.
		return
	}

	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				return
			}
		}
	})
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// (a RawDocument missing its underlying stream) without panicking.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := po.NewReader()
	doc := &model.RawDocument{
		URI:          "test://no-reader",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       nil,
	}
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, doc)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "po:")
}
