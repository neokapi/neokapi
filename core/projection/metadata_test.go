package projection

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A "property" block that carries a drawing's non-visual property or a document
// core property is metadata, not a line of the document, and stays out of the
// render tree; a "property" block holding visible text (a VML textpath string)
// and a caption the document itself holds both render.
func TestProjectStream_LeavesMetadataBlocksOut(t *testing.T) {
	name := model.NewBlock("n", "Picture 1")
	name.Type = "property"
	name.Properties["element"] = "drawing-name"
	alt := model.NewBlock("a", "A bar chart of payouts")
	alt.Type = "property"
	alt.Translatable = false
	alt.Properties["element"] = "drawing-descr"
	alt.SetSemanticRole(model.RoleCaption, 0)
	creator := model.NewBlock("d", "KapiMart Partner Team")
	creator.Type = "property"
	creator.Properties["partPath"] = "docProps/core.xml"
	creator.Properties["element"] = "creator"
	textpath := model.NewBlock("v", "WordArt banner")
	textpath.Type = "property"
	caption := model.NewBlock("c", "Figure 1. Payouts by quarter.")
	caption.SetSemanticRole(model.RoleCaption, 0)

	root := ProjectStream([]*model.Part{
		blockPart(name), blockPart(alt), blockPart(creator), blockPart(textpath), blockPart(caption),
	})
	require.Len(t, root.Children, 2)
	assert.Equal(t, "WordArt banner", root.Children[0].Text())
	assert.Equal(t, model.RoleCaption, root.Children[1].Role)
	assert.Equal(t, "Figure 1. Payouts by quarter.", root.Children[1].Text())

	assert.True(t, IsMetadata(name))
	assert.True(t, IsMetadata(alt))
	assert.True(t, IsMetadata(creator))
	assert.False(t, IsMetadata(textpath))
	assert.False(t, IsMetadata(caption))
}
