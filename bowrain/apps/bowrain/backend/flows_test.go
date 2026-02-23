package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowDefToInfoRoundTrip(t *testing.T) {
	app := NewApp()
	defs := app.ListFlowDefinitions()
	require.NotEmpty(t, defs)

	for _, d := range defs {
		assert.NotEmpty(t, d.ID)
		assert.NotEmpty(t, d.Name)
		if d.Source == "built-in" {
			assert.NotEmpty(t, d.Nodes)
			assert.NotEmpty(t, d.Edges)
		}
	}
}

func TestListFlowDefinitionsIncludesBuiltIn(t *testing.T) {
	app := NewApp()
	defs := app.ListFlowDefinitions()

	ids := make(map[string]bool)
	for _, d := range defs {
		ids[d.ID] = true
	}
	assert.True(t, ids["ai-translate"])
	assert.True(t, ids["ai-translate-qa"])
	assert.True(t, ids["pseudo-translate"])
}

func TestGetFlowDefinitionBuiltIn(t *testing.T) {
	app := NewApp()
	info, err := app.GetFlowDefinition("ai-translate")
	require.NoError(t, err)
	assert.Equal(t, "AI Translate", info.Name)
	assert.Equal(t, "built-in", info.Source)
}

func TestGetFlowDefinitionNotFound(t *testing.T) {
	app := NewApp()
	_, err := app.GetFlowDefinition("nonexistent")
	assert.Error(t, err)
}

func TestDeleteBuiltInFlowFails(t *testing.T) {
	app := NewApp()
	err := app.DeleteFlowDefinition("ai-translate")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "built-in")
}

func TestSaveBuiltInFlowFails(t *testing.T) {
	app := NewApp()
	_, err := app.SaveFlowDefinition(FlowDefinitionInfo{
		ID:     "test",
		Name:   "test",
		Source: "built-in",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "built-in")
}
