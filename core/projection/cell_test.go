package projection

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayRuns(t *testing.T) {
	serial := []model.Run{{Text: &model.TextRun{Text: "44197"}}}

	t.Run("a stamped display replaces the runs", func(t *testing.T) {
		b := &model.Block{ID: "c", Source: serial, Properties: map[string]string{
			model.PropCellDisplay: "01-01-21",
			model.PropCellFormat:  "mm-dd-yy",
		}}
		got := DisplayRuns(b, b.SourceRuns())
		assert.Equal(t, "01-01-21", model.RunsText(got))
		assert.Equal(t, "44197", model.RunsText(b.SourceRuns()), "the stored value stays in the block")
	})

	t.Run("no display leaves the runs alone", func(t *testing.T) {
		b := &model.Block{ID: "c", Source: serial, Properties: map[string]string{"cell": "A2"}}
		got := DisplayRuns(b, b.SourceRuns())
		assert.Equal(t, serial, got)
	})

	t.Run("an empty display renders as empty", func(t *testing.T) {
		b := &model.Block{ID: "c", Source: serial, Properties: map[string]string{model.PropCellDisplay: ""}}
		got := DisplayRuns(b, b.SourceRuns())
		require.Len(t, got, 1)
		assert.Empty(t, model.RunsText(got))
	})

	t.Run("nil block passes through", func(t *testing.T) {
		assert.Equal(t, serial, DisplayRuns(nil, serial))
	})
}

// A character reference a reader keeps as an inline code is shown as its
// character, so every serializer writes it in its own spelling rather than
// dropping it.
func TestDisplayRuns_CharacterReferencesAreText(t *testing.T) {
	ref := func(id, data string) model.Run {
		return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "code:entity", Data: data}}
	}
	br := model.Run{Ph: &model.PlaceholderRun{ID: "3", Type: "struct:break", Data: "<br/>", Equiv: "\n"}}
	src := []model.Run{
		{Text: &model.TextRun{Text: "Fish "}}, ref("1", "&amp;"), {Text: &model.TextRun{Text: " chips "}},
		ref("2", "&lt;"), {Text: &model.TextRun{Text: "3"}}, br,
		{Plural: &model.PluralRun{Forms: map[model.PluralForm][]model.Run{model.PluralOther: {ref("4", "&rsquo;")}}}},
	}
	b := &model.Block{ID: "p", Source: src}

	got := DisplayRuns(b, b.SourceRuns())

	assert.Equal(t, "Fish & chips <3’", model.RunsText(got))
	assert.Equal(t, br, got[5], "other codes stay codes")
	assert.Equal(t, "&amp;", src[1].Ph.Data, "the block keeps its runs")
}

func TestProjectBlockRendersTheDisplay(t *testing.T) {
	b := &model.Block{
		ID:     "cell-sheet1-B2",
		Type:   "cell",
		Source: []model.Run{{Text: &model.TextRun{Text: "0.125"}}},
		Properties: map[string]string{
			"cell":                "B2",
			model.PropCellDisplay: "12.5%",
			model.PropCellFormat:  "0.0%",
		},
	}
	b.SetSemanticRole(model.RoleTableCell, 0)
	n := ProjectBlock(b)
	assert.Equal(t, "12.5%", n.Text())
	assert.Equal(t, "0.125", model.RunsText(b.SourceRuns()))
	assert.Equal(t, "0.0%", n.Props[model.PropCellFormat], "the format travels on the node's props")
}
