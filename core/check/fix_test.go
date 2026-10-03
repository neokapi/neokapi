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
	plain := []model.Run{model.TextR("We utilize the widget.")}
	span := model.Anchor{Kind: model.AnchorRange, Start: model.RunPos{Run: 0, Offset: 3}, End: model.RunPos{Run: 0, Offset: 10}}
	replacing := map[string]string{"replacement": "use"}
	const rev = "r:0123456789abcdef"

	// "First read the <a>setup guide</a> carefully." as the HTML reader
	// builds it, and "We utilize the **widget** daily." as the Markdown one
	// does.
	link := []model.Run{
		model.TextR("First read the "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="https://example.com/setup">`}),
		model.TextR("setup guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}),
		model.TextR(" carefully."),
	}
	bold := []model.Run{
		model.TextR("We utilize the "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR("widget"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "**"}),
		model.TextR(" daily."),
	}
	rangeOf := func(sr, so, er, eo int) model.Anchor {
		return model.Anchor{Kind: model.AnchorRange, Start: model.RunPos{Run: sr, Offset: so}, End: model.RunPos{Run: er, Offset: eo}}
	}

	tests := []struct {
		name string
		f    Finding
		runs []model.Run
		at   change.Ref
		rev  string
		want string // the operation's wire form, "" for no fix
	}{
		{
			name: "a run range names the words on the block's own edition",
			f:    Finding{OriginalText: "utilize", Position: span, Metadata: replacing},
			runs: plain, at: own, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p"},"if_match":"r:0123456789abcdef","edits":[{"range":{"start":{"run":0,"offset":3},"end":{"run":0,"offset":10}},"text":"use"}]}`,
		},
		{
			name: "with no range the quoted text names the words",
			f:    Finding{OriginalText: "utilize", Metadata: replacing},
			runs: plain, at: own, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p"},"if_match":"r:0123456789abcdef","edits":[{"find":"utilize","text":"use"}]}`,
		},
		{
			name: "a range anchored to the source runs never names words in a translation",
			f:    Finding{OriginalText: "utiliser", Position: span, Metadata: replacing},
			runs: []model.Run{model.TextR("Nous utiliser le widget.")}, at: fr, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p","edition":"fr"},"if_match":"r:0123456789abcdef","edits":[{"find":"utiliser","text":"use"}]}`,
		},
		{
			name: "words inside a link are replaced inside it",
			f:    Finding{OriginalText: "setup guide", Position: rangeOf(2, 0, 3, 0), Metadata: map[string]string{"replacement": "handbook"}},
			runs: link, at: own, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p"},"if_match":"r:0123456789abcdef","edits":[{"range":{"start":{"run":2},"end":{"run":3}},"text":"handbook"}]}`,
		},
		{
			name: "words that span a link have no fix, which would delete the link",
			f:    Finding{OriginalText: "read the setup guide", Position: rangeOf(0, 6, 3, 0), Metadata: map[string]string{"replacement": "follow the setup guide"}},
			runs: link, at: own, rev: rev,
		},
		{
			name: "words that span a bold pair have no fix",
			f:    Finding{OriginalText: "utilize the widget", Position: rangeOf(0, 3, 3, 0), Metadata: map[string]string{"replacement": "use the gadget"}},
			runs: bold, at: own, rev: rev,
		},
		{
			name: "quoted words that span a code have no fix",
			f:    Finding{OriginalText: "utilize the widget", Metadata: map[string]string{"replacement": "use the gadget"}},
			runs: bold, at: own, rev: rev,
		},
		{
			name: "words before a code are replaced beside it",
			f:    Finding{OriginalText: "utilize", Position: rangeOf(0, 3, 0, 10), Metadata: replacing},
			runs: bold, at: own, rev: rev,
			want: `{"op":"replace_text","at":{"doc":"docs/guide.md","block":"guide/p"},"if_match":"r:0123456789abcdef","edits":[{"range":{"start":{"run":0,"offset":3},"end":{"run":0,"offset":10}},"text":"use"}]}`,
		},
		{name: "quoted words the edition does not hold have no fix", f: Finding{OriginalText: "leverage", Metadata: replacing}, runs: plain, at: own, rev: rev},
		{name: "a range outside the runs has no fix", f: Finding{OriginalText: "utilize", Position: rangeOf(0, 3, 9, 0), Metadata: replacing}, runs: plain, at: own, rev: rev},
		{name: "no replacement, no fix", f: Finding{OriginalText: "utilize", Position: span}, runs: plain, at: own, rev: rev},
		{name: "nothing locates the words, no fix", f: Finding{Metadata: replacing}, runs: plain, at: own, rev: rev},
		{name: "no revision read, no fix", f: Finding{OriginalText: "utilize", Metadata: replacing}, runs: plain, at: own},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op := Fix(tc.f, tc.runs, tc.at, tc.rev)
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

// A fix the check offers keeps every code of the block it edits: applied as
// it is, the words change and the markup stays.
func TestFix_KeepsTheCodesItIsOffered(t *testing.T) {
	runs := []model.Run{
		model.TextR("First read the "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink"}),
		model.TextR("setup guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink"}),
		model.TextR(" carefully."),
	}
	f := Finding{OriginalText: "read", Position: model.Anchor{Kind: model.AnchorRange, Start: model.RunPos{Run: 0, Offset: 6}, End: model.RunPos{Run: 0, Offset: 10}},
		Metadata: map[string]string{"replacement": "follow"}}
	op := Fix(f, runs, change.Ref{Doc: "a.html", Block: "p"}, "r:0123456789abcdef")
	require.NotNil(t, op)
	edit := op.Body.(*change.ReplaceText).Edits[0]
	start, end := ownOffset(runs, edit.Range.Start), ownOffset(runs, edit.Range.End)
	got := model.ApplyTextEdits(runs, []model.TextEdit{{Start: start, End: end, Replacement: edit.Text}})
	assert.Equal(t, "First follow the setup guide carefully.", model.RunsText(got))
	assert.Len(t, got, len(runs), "the link keeps both of its codes")
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
