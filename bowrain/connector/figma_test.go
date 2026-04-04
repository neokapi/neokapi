package connector

import (
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFigmaExtractTextNodes(t *testing.T) {
	c := &FigmaConnector{id: "test", config: map[string]string{}}
	doc := figmaNode{
		ID: "0:0", Name: "Doc", Type: "DOCUMENT",
		Children: []figmaNode{
			{ID: "1:1", Name: "Heading", Type: "TEXT", Characters: "Hello",
				BoundingBox: &figmaBBox{X: 10, Y: 20, Width: 200, Height: 30}},
			{ID: "1:2", Name: "Frame", Type: "FRAME", Children: []figmaNode{
				{ID: "2:1", Name: "Button", Type: "TEXT", Characters: "Click"},
			}},
			{ID: "1:3", Name: "Empty", Type: "TEXT", Characters: ""}, // should be skipped
		},
	}

	var blocks []*model.Block
	c.extractTextNodes(&doc, &blocks)

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "Heading", blocks[0].Name)
	assert.NotNil(t, blocks[0].DisplayHint)
	assert.Equal(t, "Click", blocks[1].SourceText())
}

func TestFigmaCategory(t *testing.T) {
	c := &FigmaConnector{id: "test", config: map[string]string{}}
	assert.Equal(t, platconn.CategoryDesign, c.Category())
}
