package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A markdown code span is a pcOpen/pcClose pair around plain text runs, so its
// contents used to reach the hygiene flattening as prose and every shape rule
// judged them. The bytes inside a code span are the point — a documented double
// space, a pseudo-translation wrapper, a repeated token in a shell line — so the
// span stands as one sentinel and nothing inside it is prose. These tests pin
// the flattening and the projection back to RunsText coordinates, which the
// swallowed text makes diverge by more than the sentinel's own width.

func csOpen(id string) model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: model.TypeInlineCode, SubType: "md:code", Data: "`"}}
}

func csClose(id string) model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: id, Type: model.TypeInlineCode, SubType: "md:code", Data: "`"}}
}

func boldOpen(id string) model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: "fmt:bold", Data: "**"}}
}

func boldClose(id string) model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: id, Type: "fmt:bold", Data: "**"}}
}

func TestHygieneTextCollapsesACodeSpan(t *testing.T) {
	sentinel := string(model.ObjectReplacement)

	tests := []struct {
		name string
		runs []model.Run
		want string
	}{
		{
			name: "a code span is one sentinel, whatever it holds",
			runs: []model.Run{csOpen("1"), hvTx("x  y"), csClose("1")},
			want: sentinel,
		},
		{
			name: "the prose around it is untouched",
			runs: []model.Run{
				hvTx("Shows "), csOpen("1"), hvTx("the the"), csClose("1"), hvTx(" in prose."),
			},
			want: "Shows " + sentinel + " in prose.",
		},
		{
			name: "two code spans are two sentinels",
			runs: []model.Run{
				csOpen("1"), hvTx("a  b"), csClose("1"), hvTx(" and "), csOpen("2"), hvTx("the the"), csClose("2"),
			},
			want: sentinel + " and " + sentinel,
		},
		{
			name: "a non-code pair keeps its text: bold prose is prose",
			runs: []model.Run{boldOpen("1"), hvTx("the the"), boldClose("1")},
			want: sentinel + "the the" + sentinel,
		},
		{
			name: "an unpaired opening half collapses on its own, losing no text",
			runs: []model.Run{csOpen("1"), hvTx("dangling")},
			want: sentinel + "dangling",
		},
		{
			name: "a placeholder inside a code span goes with it",
			runs: []model.Run{csOpen("1"), hvTx("a"), hvPh("2"), hvTx("b"), csClose("1")},
			want: sentinel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := model.NewHygieneView(tt.runs)
			assert.Equal(t, tt.want, v.Text())
			assert.Equal(t, model.RunsHygieneText(tt.runs), v.Text(),
				"the view is the single walk behind RunsHygieneText; they cannot differ")
		})
	}
}

// A collapsed span swallows text that RunsText still carries, so the two
// coordinate spaces diverge by the span's contents rather than by the sentinel's
// width. Any range a rule reports after the span must still cover the text it
// found.
func TestHygieneViewRangeSurvivesACollapsedCodeSpan(t *testing.T) {
	tests := []struct {
		name string
		runs []model.Run
		find string
	}{
		{
			name: "double space after a code span",
			runs: []model.Run{csOpen("1"), hvTx("kapi up"), csClose("1"), hvTx(" runs  the loop")},
			find: "  ",
		},
		{
			name: "doubled word after two code spans",
			runs: []model.Run{
				csOpen("1"), hvTx("--memory"), csClose("1"), hvTx(" and "),
				csOpen("2"), hvTx("--termstore"), csClose("2"), hvTx(" are the the flags"),
			},
			find: "the the",
		},
		{
			name: "non-ASCII inside the span does not shift the anchor",
			runs: []model.Run{csOpen("1"), hvTx("café ▒ ▒"), csClose("1"), hvTx(" then  here")},
			find: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := model.NewHygieneView(tt.runs)
			start := indexOf(t, v.Text(), tt.find)
			r := v.Range(start, start+len(tt.find))

			bs, be := r.ByteSpan(tt.runs)
			plain := model.RunsText(tt.runs)
			require.LessOrEqual(t, be, len(plain))
			assert.Equal(t, tt.find, plain[bs:be])
		})
	}
}

// A block whose whole content is a code span still holds content: the sentinel
// occupies a position the way a placeholder does.
func TestRunsHaveContentForACodeSpan(t *testing.T) {
	runs := []model.Run{csOpen("1"), hvTx("  "), csClose("1")}
	assert.True(t, model.RunsHaveContent(runs))
}
