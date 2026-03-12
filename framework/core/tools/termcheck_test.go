package tools_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestTermCheckToolPass(t *testing.T) {
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "Save", Target: "Sauvegarder"},
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
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "Save", Target: "Sauvegarder"},
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
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "save", Target: "sauvegarder"},
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
	cfg := &tools.TermCheckConfig{
		Glossary: []tools.GlossaryEntry{
			{Source: "Save", Target: "Sauvegarder"},
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
	cfg := &tools.TermCheckConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	cfg.Glossary = []tools.GlossaryEntry{{Source: "", Target: "x"}}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty source")

	cfg.Glossary = []tools.GlossaryEntry{{Source: "x", Target: ""}}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty target")

	cfg.Glossary = []tools.GlossaryEntry{{Source: "Save", Target: "Sauvegarder"}}
	err = cfg.Validate()
	assert.NoError(t, err)
}
