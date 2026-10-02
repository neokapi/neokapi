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
