package flow

import (
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
)

func toolMap(tools ...registry.ToolInfo) map[registry.ToolID]registry.ToolInfo {
	m := make(map[registry.ToolID]registry.ToolInfo)
	for _, t := range tools {
		m[t.Name] = t
	}
	return m
}

var projectTargets = []string{"de-DE", "fr-FR", "ja-JP"}

func TestResolveFlowLocales_MonolingualOnly(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{{Tool: "word-count"}}}
	infos := toolMap(registry.ToolInfo{Name: "word-count", Cardinality: schema.Monolingual})

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	assert.Nil(t, result, "monolingual-only flow should return nil (run once)")
}

func TestResolveFlowLocales_BilingualNoDefault(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{{Tool: "translate"}}}
	infos := toolMap(registry.ToolInfo{Name: "translate", Cardinality: schema.Bilingual})

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	assert.Len(t, result, 3)
	assert.Equal(t, []string{"en-US", "de-DE"}, result[0])
	assert.Equal(t, []string{"en-US", "fr-FR"}, result[1])
	assert.Equal(t, []string{"en-US", "ja-JP"}, result[2])
}

func TestResolveFlowLocales_BilingualWithDefault(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{{Tool: "pseudo-translate"}}}
	infos := toolMap(registry.ToolInfo{
		Name: "pseudo-translate", Cardinality: schema.Bilingual, DefaultLocale: "qps",
	})

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	assert.Len(t, result, 1)
	assert.Equal(t, []string{"en-US", "qps"}, result[0])
}

func TestResolveFlowLocales_MixedBilingualDefaultAndNoDefault(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{
		{Tool: "translate"},
		{Tool: "pseudo-translate"},
	}}
	infos := toolMap(
		registry.ToolInfo{Name: "translate", Cardinality: schema.Bilingual},
		registry.ToolInfo{Name: "pseudo-translate", Cardinality: schema.Bilingual, DefaultLocale: "qps"},
	)

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	// 3 project targets + 1 default (qps) = 4 passes
	assert.Len(t, result, 4)
	assert.Equal(t, []string{"en-US", "de-DE"}, result[0])
	assert.Equal(t, []string{"en-US", "fr-FR"}, result[1])
	assert.Equal(t, []string{"en-US", "ja-JP"}, result[2])
	assert.Equal(t, []string{"en-US", "qps"}, result[3])
}

func TestResolveFlowLocales_Multilingual(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{{Tool: "consistency-check"}}}
	infos := toolMap(registry.ToolInfo{Name: "consistency-check", Cardinality: schema.Multilingual})

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	assert.Len(t, result, 1)
	assert.Equal(t, []string{"en-US", "de-DE", "fr-FR", "ja-JP"}, result[0])
}

func TestResolveFlowLocales_MonoAndBilingual(t *testing.T) {
	// word-count (mono) + qa (bilingual) → iterate project targets.
	spec := &StepsSpec{Steps: []FlowStep{
		{Tool: "word-count"},
		{Tool: "qa"},
	}}
	infos := toolMap(
		registry.ToolInfo{Name: "word-count", Cardinality: schema.Monolingual},
		registry.ToolInfo{Name: "qa", Cardinality: schema.Bilingual},
	)

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	assert.Len(t, result, 3)
}

func TestResolveFlowLocales_ParallelSteps(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{
		{Parallel: []FlowStep{
			{Tool: "translate"},
			{Tool: "pseudo-translate"},
		}},
	}}
	infos := toolMap(
		registry.ToolInfo{Name: "translate", Cardinality: schema.Bilingual},
		registry.ToolInfo{Name: "pseudo-translate", Cardinality: schema.Bilingual, DefaultLocale: "qps"},
	)

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	assert.Len(t, result, 4) // 3 targets + qps
}

// A project with no target languages resolves to no locale pass whatever the
// flow, so "which locales" cannot answer "does this flow need one at all" —
// FlowNeedsTargetLanguage is the separate question, and it is what decides
// whether a run on a monolingual project is missing something or complete.
func TestFlowNeedsTargetLanguage(t *testing.T) {
	infos := toolMap(
		registry.ToolInfo{Name: "voice-vocab-check", Cardinality: schema.Monolingual},
		registry.ToolInfo{Name: "word-count", Cardinality: schema.Monolingual},
		registry.ToolInfo{Name: "translate", Cardinality: schema.Bilingual},
		registry.ToolInfo{Name: "terminology", Cardinality: schema.Multilingual},
		registry.ToolInfo{Name: "no-cardinality"},
	)

	tests := []struct {
		name  string
		steps []FlowStep
		want  bool
	}{
		{"the scaffolded check flow", []FlowStep{{Tool: "voice-vocab-check"}}, false},
		{"every step monolingual", []FlowStep{{Tool: "voice-vocab-check"}, {Tool: "word-count"}}, false},
		{"one bilingual step is enough", []FlowStep{{Tool: "word-count"}, {Tool: "translate"}}, true},
		{"multilingual", []FlowStep{{Tool: "terminology"}}, true},
		{"nested in a parallel branch", []FlowStep{{Parallel: []FlowStep{{Tool: "translate"}}}}, true},
		{"unset cardinality is assumed to need one", []FlowStep{{Tool: "no-cardinality"}}, true},
		{"a tool the registry does not know", []FlowStep{{Tool: "plugin-tool"}}, true},
		{"no steps", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FlowNeedsTargetLanguage(&StepsSpec{Steps: tt.steps}, infos))
		})
	}

	assert.False(t, FlowNeedsTargetLanguage(nil, infos))
}

func TestResolveFlowLocales_NilSpec(t *testing.T) {
	result := ResolveFlowLocales(nil, nil, "en-US", projectTargets)
	assert.Nil(t, result)
}

func TestResolveFlowLocales_EmptySteps(t *testing.T) {
	spec := &StepsSpec{Steps: []FlowStep{}}
	result := ResolveFlowLocales(spec, nil, "en-US", projectTargets)
	assert.Nil(t, result)
}

func TestResolveFlowLocales_UnknownTool(t *testing.T) {
	// Unknown tool → conservative (assume bilingual, iterate all).
	spec := &StepsSpec{Steps: []FlowStep{{Tool: "unknown-tool"}}}
	result := ResolveFlowLocales(spec, toolMap(), "en-US", projectTargets)
	assert.Len(t, result, 3)
}

func TestResolveFlowLocales_DuplicateDefaultNotRepeated(t *testing.T) {
	// Two tools with the same default → only one pass for that default.
	spec := &StepsSpec{Steps: []FlowStep{
		{Tool: "pseudo1"},
		{Tool: "pseudo2"},
	}}
	infos := toolMap(
		registry.ToolInfo{Name: "pseudo1", Cardinality: schema.Bilingual, DefaultLocale: "qps"},
		registry.ToolInfo{Name: "pseudo2", Cardinality: schema.Bilingual, DefaultLocale: "qps"},
	)

	result := ResolveFlowLocales(spec, infos, "en-US", projectTargets)
	assert.Len(t, result, 1)
	assert.Equal(t, []string{"en-US", "qps"}, result[0])
}
