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
		got := DisplayRuns(b, b.Source)
		assert.Equal(t, "01-01-21", model.RunsText(got))
		assert.Equal(t, "44197", model.RunsText(b.Source), "the stored value stays in the block")
	})

	t.Run("no display leaves the runs alone", func(t *testing.T) {
		b := &model.Block{ID: "c", Source: serial, Properties: map[string]string{"cell": "A2"}}
		got := DisplayRuns(b, b.Source)
		assert.Equal(t, serial, got)
	})

	t.Run("an empty display renders as empty", func(t *testing.T) {
		b := &model.Block{ID: "c", Source: serial, Properties: map[string]string{model.PropCellDisplay: ""}}
		got := DisplayRuns(b, b.Source)
		require.Len(t, got, 1)
		assert.Empty(t, model.RunsText(got))
	})

	t.Run("nil block passes through", func(t *testing.T) {
		assert.Equal(t, serial, DisplayRuns(nil, serial))
	})
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
	assert.Equal(t, "0.125", model.RunsText(b.Source))
	assert.Equal(t, "0.0%", n.Props[model.PropCellFormat], "the format travels on the node's props")
}
