package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tool has two routes: walk the runs, or flatten the block to its text.
// Flattening is only safe when nothing in the sequence carries a marking that
// flattening would destroy.
//
// A markdown table cell is the case that got this wrong. Its content arrives as
// text runs alone — no placeholders, no paired codes — so the flatten route
// looked applicable, and `docker compose up` came out as `đöçķéŕ çöḿþöšé üþ`
// while the same span inside a sentence survived, because there the reader
// wraps it in a paired code.
func TestRunsHaveInlineTreatsProtectedTextAsInline(t *testing.T) {
	cases := []struct {
		name string
		runs []model.Run
		want bool
	}{
		{
			name: "plain prose flattens",
			runs: []model.Run{{Text: &model.TextRun{Text: "What runs where"}}},
			want: false,
		},
		{
			name: "a table cell holding commands must not flatten",
			runs: []model.Run{
				{Text: &model.TextRun{Text: "`docker compose up`", NoTranslate: true}},
				{Text: &model.TextRun{Text: " + "}},
				{Text: &model.TextRun{Text: "`make dev-server`", NoTranslate: true}},
			},
			want: true,
		},
		{
			name: "a paired code still counts",
			runs: []model.Run{
				{Text: &model.TextRun{Text: "Run "}},
				{PcOpen: &model.PcOpenRun{ID: "1"}},
				{Text: &model.TextRun{Text: "kapi up", NoTranslate: true}},
				{PcClose: &model.PcCloseRun{ID: "1"}},
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, runsHaveInline(tc.runs))
		})
	}
}

// And the walk itself must leave the protected text exactly as it found it,
// flag included, so the writer puts back what the reader read.
func TestPseudoTranslateRunsKeepsProtectedText(t *testing.T) {
	cfg := &PseudoConfig{}
	cfg.Reset()
	cfg.Prefix, cfg.Suffix = "", ""

	out := pseudoTranslateRuns([]model.Run{
		{Text: &model.TextRun{Text: "`docker compose up`", NoTranslate: true}},
		{Text: &model.TextRun{Text: " and then "}},
		{Text: &model.TextRun{Text: "`make dev-server`", NoTranslate: true}},
	}, cfg)

	require.Len(t, out, 3)
	assert.Equal(t, "`docker compose up`", out[0].Text.Text)
	assert.True(t, out[0].Text.NoTranslate)
	assert.Equal(t, "`make dev-server`", out[2].Text.Text)
	assert.True(t, out[2].Text.NoTranslate)
	// Prose between two commands is still prose. Picked with letters in it:
	// " + " accents to itself, so it would pass this whatever the code did.
	assert.NotEqual(t, " and then ", out[1].Text.Text,
		"the prose between them still translates")
	assert.False(t, out[1].Text.NoTranslate)
}
