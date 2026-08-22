package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTermCheckToolPass(t *testing.T) {
	t.Parallel()
	cfg := &tools.TermCheckConfig{
		TermRules: []coreprofile.TermRule{
			{Term: "Save", Replacement: "Sauvegarder"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewTermCheckTool(cfg)

	assert.Equal(t, "term-check", tl.Name())

	block := model.NewBlock("tu1", "Save the file")
	block.SetTargetText(model.LocaleFrench, "Sauvegarder le fichier")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropTermCheckPassed])
}

func TestTermCheckToolFail(t *testing.T) {
	t.Parallel()
	cfg := &tools.TermCheckConfig{
		TermRules: []coreprofile.TermRule{
			{Term: "Save", Replacement: "Sauvegarder"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewTermCheckTool(cfg)

	block := model.NewBlock("tu1", "Save the file")
	block.SetTargetText(model.LocaleFrench, "Enregistrer le fichier")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropTermCheckPassed])
	assert.Contains(t, resultBlock.Properties[tools.PropTermCheckErrors], "Sauvegarder")
}

func TestTermCheckToolCaseInsensitive(t *testing.T) {
	t.Parallel()
	cfg := &tools.TermCheckConfig{
		TermRules: []coreprofile.TermRule{
			{Term: "save", Replacement: "sauvegarder"},
		},
		TargetLocale:  model.LocaleFrench,
		CaseSensitive: false,
	}
	tl := tools.NewTermCheckTool(cfg)

	block := model.NewBlock("tu1", "SAVE the file")
	block.SetTargetText(model.LocaleFrench, "SAUVEGARDER le fichier")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropTermCheckPassed])
}

func TestTermCheckToolNoTarget(t *testing.T) {
	t.Parallel()
	cfg := &tools.TermCheckConfig{
		TermRules: []coreprofile.TermRule{
			{Term: "Save", Replacement: "Sauvegarder"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewTermCheckTool(cfg)

	// No target text set.
	block := model.NewBlock("tu1", "Save the file")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropTermCheckPassed]
	assert.False(t, hasPassed) // No target → no check.
}

func TestTermCheckConfigValidation(t *testing.T) {
	t.Parallel()
	cfg := &tools.TermCheckConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	cfg.TermRules = []coreprofile.TermRule{{Term: "", Replacement: "x"}}
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no term")

	cfg.TermRules = []coreprofile.TermRule{{Term: "x", Replacement: ""}}
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no replacement")

	cfg.TermRules = []coreprofile.TermRule{{Term: "Save", Replacement: "Sauvegarder"}}
	err = cfg.Validate()
	require.NoError(t, err)
}
