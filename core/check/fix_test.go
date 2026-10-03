package check

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

func TestFix(t *testing.T) {
	own := change.Ref{Doc: "docs/guide.md", Block: "guide/p"}
	fr := change.Ref{Doc: "docs/guide.md", Block: "guide/p", Edition: model.EditionKey{Locale: "fr"}}
	span := model.Anchor{Kind: model.AnchorRange, Start: model.RunPos{Run: 0, Offset: 3}, End: model.RunPos{Run: 0, Offset: 10}}
	replacing := map[string]string{"replacement": "use"}
	const rev = "r:0123456789abcdef"

	tests := []struct {
		name string
		f    Finding
		at   change.Ref
		rev  string
		want string // the operation's wire form, "" for no fix
	}{
		{
			name: "a run range names the words on the block's own edition",
			f:    Finding{OriginalText: "utilize", Position: span, Metadata: replacing},
			at:   own, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p"},"if_match":"r:0123456789abcdef","edits":[{"range":{"start":{"run":0,"offset":3},"end":{"run":0,"offset":10}},"text":"use"}]}`,
		},
		{
			name: "with no range the quoted text names the words",
			f:    Finding{OriginalText: "utilize", Metadata: replacing},
			at:   own, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p"},"if_match":"r:0123456789abcdef","edits":[{"find":"utilize","text":"use"}]}`,
		},
		{
			name: "a range anchored to the source runs never names words in a translation",
			f:    Finding{OriginalText: "utiliser", Position: span, Metadata: replacing},
			at:   fr, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p","edition":"fr"},"if_match":"r:0123456789abcdef","edits":[{"find":"utiliser","text":"use"}]}`,
		},
		{name: "no replacement, no fix", f: Finding{OriginalText: "utilize", Position: span}, at: own, rev: rev},
		{name: "nothing locates the words, no fix", f: Finding{Metadata: replacing}, at: own, rev: rev},
		{name: "no revision read, no fix", f: Finding{OriginalText: "utilize", Metadata: replacing}, at: own},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op := Fix(tc.f, tc.at, tc.rev)
			if tc.want == "" {
				assert.Nil(t, op)
				return
			}
			require.NotNil(t, op)
			raw, err := json.Marshal(op)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(raw))
			var back change.Op
			require.NoError(t, json.Unmarshal(raw, &back), "the fix decodes as a change set's operation")
		})
	}
}

// A block read for a check carries no language of its own; the fix names the
// revision the change service computes for the same content in the
// document's language.
func TestSourceRevision(t *testing.T) {
	runs := []model.Run{model.TextR("We utilize the widget.")}
	read := model.NewRunsBlock("tu1", runs)
	assert.Equal(t, model.RunsRevision(model.EditionKey{Locale: "en"}, runs), SourceRevision(read, "en"))

	served := model.NewRunsBlock("tu1", runs)
	served.SourceLocale = "en"
	assert.Equal(t, model.EditionRevision(served, model.EditionKey{}), SourceRevision(read, "en"),
		"the check's revision is the one a read of the same content reports")
	assert.Equal(t, model.EditionRevision(served, model.EditionKey{}), SourceRevision(served, "de"),
		"a block that names its language is revised in it")
}
