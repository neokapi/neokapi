package editor

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BuildContentTree ships a generative-projection render AST alongside the raw
// anatomy Root, so the preview renders inline runs + reconstructed tables from
// one Go-built tree (strategy/preview-fidelity #5).
func TestBuildContentTree_ShipsRenderProjection(t *testing.T) {
	hcell := func(id, text string) *model.Block {
		b := model.NewBlock(id, text)
		b.SetSemanticRole(model.RoleTableHeader, 0)
		return b
	}
	dcell := func(id, text string) *model.Block {
		b := model.NewBlock(id, text)
		b.SetSemanticRole(model.RoleTableCell, 0)
		return b
	}
	parts := []*model.Part{
		groupStart(&model.GroupStart{ID: "t", Name: "table", Type: "table"}),
		groupStart(&model.GroupStart{ID: "r0", Name: "table-row", Type: "table-row"}),
		blockPart(hcell("a", "A")),
		blockPart(hcell("b", "B")),
		groupEnd("r0"),
		groupStart(&model.GroupStart{ID: "r1", Name: "table-row", Type: "table-row"}),
		blockPart(dcell("c", "1")),
		blockPart(dcell("d", "2")),
		groupEnd("r1"),
		groupEnd("t"),
	}

	tree := BuildContentTree(parts, "markdown")
	require.NotNil(t, tree.Render, "render projection must be shipped")
	require.Len(t, tree.Render.Children, 1)
	table := tree.Render.Children[0]
	assert.Equal(t, model.RoleTable, table.Role)
	require.Len(t, table.Children, 2, "two rows")
	assert.True(t, table.Children[0].Children[0].Header, "header cell flagged")

	// The render tree must survive the inspect JSON boundary intact.
	raw, err := json.Marshal(tree)
	require.NoError(t, err)
	var rt struct {
		Render *projection.RenderNode `json:"render"`
	}
	require.NoError(t, json.Unmarshal(raw, &rt))
	require.NotNil(t, rt.Render)
	assert.Equal(t, projection.RoleDocument, rt.Render.Role)
	assert.Equal(t, model.RoleTable, rt.Render.Children[0].Role)
}
