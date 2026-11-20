package connector

import (
	"net/http"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	venueconn "github.com/neokapi/neokapi/core/venue/connector"

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
	assert.Equal(t, venueconn.CategoryDesign, c.Category())
}

func TestFigmaPublishIsReadOnly(t *testing.T) {
	c, err := NewFigmaConnector(map[string]string{"file_key": "design", "token": "fixture"})
	require.NoError(t, err)
	err = c.Publish(t.Context(), nil, platconn.PublishOptions{})
	require.ErrorContains(t, err, "read-only")
}

func TestFigmaFetch(t *testing.T) {
	c, err := NewFigmaConnector(map[string]string{"file_key": "design", "token": "fixture"})
	require.NoError(t, err)
	c.client = &http.Client{Transport: fixtureEndpoint(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "https://api.figma.com/v1/files/design", r.URL.String())
		assert.Equal(t, "fixture", r.Header.Get("X-Figma-Token"))
		_, _ = w.Write([]byte(`{"name":"Page","document":{"type":"DOCUMENT","children":[{"id":"1:1","type":"TEXT","characters":"Heading"}]}}`))
	})}
	items, err := c.Fetch(t.Context(), platconn.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "design", items[0].ID)
	require.Len(t, items[0].Blocks, 1)
	require.Equal(t, "Heading", items[0].Blocks[0].SourceText())
}
