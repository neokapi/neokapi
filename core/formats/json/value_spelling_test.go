package json

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

const spelledValues = `{
  "link": "See https://example.com/guide",
  "escaped": "<b>bold<\/b> and caf\u00e9\ttab",
  "edited": "We utilize the form"
}
`

// An untouched value is written with the bytes it was read with. The writer
// encoded every value from its text, so each write respelled values nothing
// had changed: a URL's slashes became `\/` under the default
// escapeForwardSlashes, and `\u00e9` became `é`.
func TestUntouchedValueKeepsItsSpelling(t *testing.T) {
	edit := func(b *model.Block) {
		if text := b.SourceText(); strings.Contains(text, "utilize") {
			b.EditSourceRuns([]model.Run{model.TextR(strings.ReplaceAll(text, "utilize", "use"))})
		}
	}
	tests := []struct {
		name   string
		mutate func(*model.Block)
		want   string
	}{
		{name: "untouched", want: spelledValues},
		{
			name:   "a sibling edited",
			mutate: edit,
			want:   strings.Replace(spelledValues, "We utilize the form", "We use the form", 1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := writerCase{mutate: tc.mutate}
			assert.Equal(t, tc.want, bufferedWriterRoundtrip(t, spelledValues, c), "buffered")
			assert.Equal(t, tc.want, streamingWriterRoundtrip(t, spelledValues, c), "streaming")
		})
	}
}

// replaceIn edits every block whose source holds from, replacing it with to.
func replaceIn(from, to string) func(*model.Block) {
	return func(b *model.Block) {
		if text := b.SourceText(); strings.Contains(text, from) {
			b.EditSourceRuns([]model.Run{model.TextR(strings.ReplaceAll(text, from, to))})
		}
	}
}

// An edited value keeps the spelling of every character the edit left: an
// escape stays an escape, a literal slash stays literal, and only the words
// the edit changed differ. The edit of "b" is the one a fix of its two terms
// makes: the \u escape and the literal slash of "</b>" stay as written.
func TestEditedValueKeepsTheSpellingOfWhatItKept(t *testing.T) {
	const doc = "{\n    \"a\": \"Hello {name}, utilize it\",\n  \"b\" :\"caf\\u00e9 utilize <b>e-mail</b>\",\n  \"c\": \"nothing\"\n}"
	tests := []struct {
		name   string
		doc    string
		mutate func(*model.Block)
		want   string
	}{
		{
			name: "two words of an escaped value",
			doc:  doc,
			mutate: func(b *model.Block) {
				replaceIn("utilize", "use")(b)
				replaceIn("e-mail", "email")(b)
			},
			want: "{\n    \"a\": \"Hello {name}, use it\",\n  \"b\" :\"caf\\u00e9 use <b>email</b>\",\n  \"c\": \"nothing\"\n}",
		},
		{
			name:   "an escaped slash stays escaped",
			doc:    `{"a": "<b>utilize<\/b> it"}`,
			mutate: replaceIn("utilize", "use"),
			want:   `{"a": "<b>use<\/b> it"}`,
		},
		{
			name:   "an added non-ASCII character is escaped as the file escapes",
			doc:    "{\"a\": \"caf\\u00e9\", \"b\": \"utilize\"}",
			mutate: replaceIn("utilize", "naïve"),
			want:   "{\"a\": \"caf\\u00e9\", \"b\": \"na\\u00efve\"}",
		},
		{
			name:   "an added character beyond the BMP is a surrogate pair",
			doc:    "{\"a\": \"caf\\u00E9\", \"b\": \"utilize\"}",
			mutate: replaceIn("utilize", "\U0001F600"),
			want:   "{\"a\": \"caf\\u00E9\", \"b\": \"\\uD83D\\uDE00\"}",
		},
		{
			name:   "an added non-ASCII character is UTF-8 in a UTF-8 file",
			doc:    `{"a": "café", "b": "utilize"}`,
			mutate: replaceIn("utilize", "naïve"),
			want:   `{"a": "café", "b": "naïve"}`,
		},
		{
			name:   "an added slash follows the file's slashes",
			doc:    `{"a": "see a/b", "b": "utilize"}`,
			mutate: replaceIn("utilize", "and/or"),
			want:   `{"a": "see a/b", "b": "and/or"}`,
		},
		{
			name:   "an added quote is escaped for the value's delimiter",
			doc:    `{"a": "utilize \"it\""}`,
			mutate: replaceIn("utilize", `say "use"`),
			want:   `{"a": "say \"use\" \"it\""}`,
		},
		{
			name:   "an astral escape survives beside the edit",
			doc:    "{\"a\": \"\\ud83d\\ude00 utilize\"}",
			mutate: replaceIn("utilize", "use"),
			want:   "{\"a\": \"\\ud83d\\ude00 use\"}",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := writerCase{mutate: tc.mutate}
			assert.Equal(t, tc.want, bufferedWriterRoundtrip(t, tc.doc, c), "buffered")
			assert.Equal(t, tc.want, streamingWriterRoundtrip(t, tc.doc, c), "streaming")
		})
	}
}
