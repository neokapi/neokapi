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

// A placeholder's name is program syntax. The compass catalog's own string
// must not fire a vessel rule, and a name in the target must not satisfy one,
// while a real use in the text and the text of a plural branch still count.
func TestTermCheckTool_PlaceholderNamesAreNotTerms(t *testing.T) {
	t.Parallel()
	rules := []coreprofile.TermRule{
		{Term: "vessel", Replacement: "fartøy"},
		{Term: "berth", Replacement: "kaiplass"},
	}
	tests := []struct {
		name       string
		source     string
		target     string
		wantPassed string
	}{
		{
			name:       "argument names do not fire a rule",
			source:     "{vessel} is alongside until {until}.",
			target:     "{vessel} ligger til kai til {until}.",
			wantPassed: "true",
		},
		{
			name:       "a use in the text fires",
			source:     "No vessel is alongside.",
			target:     "Ingen skip ligger til kai.",
			wantPassed: "false",
		},
		{
			name:       "the text of a plural branch fires",
			source:     "{count, plural, one {# berth} other {# berths}} at this terminal.",
			target:     "{count, plural, one {# plass} other {# plasser}} på denne terminalen.",
			wantPassed: "false",
		},
		{
			name:       "an argument name does not satisfy a rule",
			source:     "No vessel is alongside.",
			target:     "Ingen {fartøy} ligger til kai.",
			wantPassed: "false",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tl := tools.NewTermCheckTool(&tools.TermCheckConfig{TermRules: rules, TargetLocale: "nb"})
			block := model.NewBlock("tu1", tt.source)
			block.SetTargetText("nb", tt.target)
			result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

			got := result.Resource.(*model.Block)
			assert.Equal(t, tt.wantPassed, got.Properties[tools.PropTermCheckPassed],
				"errors: %s", got.Properties[tools.PropTermCheckErrors])
		})
	}
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
