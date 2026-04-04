package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processMultipleParts is defined in helpers_test.go

func TestInconsistencyCheckToolName(t *testing.T) {
	t.Parallel()
	cfg := tools.NewInconsistencyCheckConfig(model.LocaleFrench)
	tl := tools.NewInconsistencyCheckTool(cfg)

	assert.Equal(t, "inconsistency-check", tl.Name())
	assert.Contains(t, tl.Description(), "inconsistenc")
}

func TestInconsistencyCheckConfig(t *testing.T) {
	t.Parallel()
	cfg := &tools.InconsistencyCheckConfig{}
	assert.Equal(t, "inconsistency-check", cfg.ToolName())

	// Validate requires TargetLocale.
	err := cfg.Validate()
	require.Error(t, err)

	cfg.Reset()
	assert.True(t, cfg.CaseSensitive)
	assert.True(t, cfg.CheckTargetInconsistency)
	assert.False(t, cfg.CheckSourceInconsistency)

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestInconsistencyCheckConsistent(t *testing.T) {
	t.Parallel()
	cfg := tools.NewInconsistencyCheckConfig(model.LocaleFrench)
	tl := tools.NewInconsistencyCheckTool(cfg)

	b1 := model.NewBlock("tu1", "Hello")
	b1.SetTargetText(model.LocaleFrench, "Bonjour")
	b2 := model.NewBlock("tu2", "Hello")
	b2.SetTargetText(model.LocaleFrench, "Bonjour")

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: b1},
		{Type: model.PartBlock, Resource: b2},
	}
	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 2)

	for _, r := range results {
		block := r.Resource.(*model.Block)
		assert.Equal(t, "consistent", block.Properties[tools.PropInconsistencyStatus])
		assert.Empty(t, block.Properties[tools.PropInconsistencyType])
	}
}

func TestInconsistencyCheckTargetInconsistency(t *testing.T) {
	t.Parallel()
	cfg := tools.NewInconsistencyCheckConfig(model.LocaleFrench)
	tl := tools.NewInconsistencyCheckTool(cfg)

	b1 := model.NewBlock("tu1", "Hello")
	b1.SetTargetText(model.LocaleFrench, "Bonjour")
	b2 := model.NewBlock("tu2", "Hello")
	b2.SetTargetText(model.LocaleFrench, "Salut")

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: b1},
		{Type: model.PartBlock, Resource: b2},
	}
	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 2)

	// First block is consistent (first occurrence).
	block1 := results[0].Resource.(*model.Block)
	assert.Equal(t, "consistent", block1.Properties[tools.PropInconsistencyStatus])

	// Second block has a different translation for "Hello" → inconsistent.
	block2 := results[1].Resource.(*model.Block)
	assert.Equal(t, "inconsistent", block2.Properties[tools.PropInconsistencyStatus])
	assert.Equal(t, "target-inconsistency", block2.Properties[tools.PropInconsistencyType])

	var alternatives []string
	err := json.Unmarshal([]byte(block2.Properties[tools.PropInconsistencyDetails]), &alternatives)
	require.NoError(t, err)
	require.Len(t, alternatives, 1)
	assert.Equal(t, "Bonjour", alternatives[0])
}

func TestInconsistencyCheckSourceInconsistency(t *testing.T) {
	t.Parallel()
	cfg := tools.NewInconsistencyCheckConfig(model.LocaleFrench)
	cfg.CheckSourceInconsistency = true
	cfg.CheckTargetInconsistency = false
	tl := tools.NewInconsistencyCheckTool(cfg)

	b1 := model.NewBlock("tu1", "Hello")
	b1.SetTargetText(model.LocaleFrench, "Bonjour")
	b2 := model.NewBlock("tu2", "Hi")
	b2.SetTargetText(model.LocaleFrench, "Bonjour")

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: b1},
		{Type: model.PartBlock, Resource: b2},
	}
	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 2)

	// First block is consistent (first occurrence).
	block1 := results[0].Resource.(*model.Block)
	assert.Equal(t, "consistent", block1.Properties[tools.PropInconsistencyStatus])

	// Second block has a different source for "Bonjour" → inconsistent.
	block2 := results[1].Resource.(*model.Block)
	assert.Equal(t, "inconsistent", block2.Properties[tools.PropInconsistencyStatus])
	assert.Equal(t, "source-inconsistency", block2.Properties[tools.PropInconsistencyType])

	var alternatives []string
	err := json.Unmarshal([]byte(block2.Properties[tools.PropInconsistencyDetails]), &alternatives)
	require.NoError(t, err)
	require.Len(t, alternatives, 1)
	assert.Equal(t, "Hello", alternatives[0])
}

func TestInconsistencyCheckCaseInsensitive(t *testing.T) {
	t.Parallel()
	cfg := tools.NewInconsistencyCheckConfig(model.LocaleFrench)
	cfg.CaseSensitive = false
	tl := tools.NewInconsistencyCheckTool(cfg)

	b1 := model.NewBlock("tu1", "Hello")
	b1.SetTargetText(model.LocaleFrench, "Bonjour")
	b2 := model.NewBlock("tu2", "hello")
	b2.SetTargetText(model.LocaleFrench, "BONJOUR")

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: b1},
		{Type: model.PartBlock, Resource: b2},
	}
	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 2)

	// Both map to the same normalized source/target → consistent.
	for _, r := range results {
		block := r.Resource.(*model.Block)
		assert.Equal(t, "consistent", block.Properties[tools.PropInconsistencyStatus])
	}
}

func TestInconsistencyCheckNoTarget(t *testing.T) {
	t.Parallel()
	cfg := tools.NewInconsistencyCheckConfig(model.LocaleFrench)
	tl := tools.NewInconsistencyCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	// No target set.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, resultBlock.Properties[tools.PropInconsistencyStatus])
}

func TestInconsistencyCheckNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := tools.NewInconsistencyCheckConfig(model.LocaleFrench)
	tl := tools.NewInconsistencyCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, resultBlock.Properties[tools.PropInconsistencyStatus])
}
