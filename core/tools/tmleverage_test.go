package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTMProvider implements TMProvider for testing.
type mockTMProvider struct {
	exact map[string]string     // source -> translation
	fuzzy map[string]fuzzyMatch // source -> {translation, score}
}

type fuzzyMatch struct {
	translation string
	score       int
}

func (m *mockTMProvider) LookupExact(source string, _, _ model.LocaleID) (string, bool) {
	if m.exact == nil {
		return "", false
	}
	trans, ok := m.exact[source]
	return trans, ok
}

func (m *mockTMProvider) LookupFuzzy(source string, _, _ model.LocaleID, threshold int) (string, int, bool) {
	if m.fuzzy == nil {
		return "", 0, false
	}
	match, ok := m.fuzzy[source]
	if !ok || match.score < threshold {
		return "", 0, false
	}
	return match.translation, match.score, true
}

func TestTMLeverageTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       &tools.NullTMProvider{},
	}
	tl := tools.NewTMLeverageTool(cfg)

	assert.Equal(t, "tm-leverage", tl.Name())
	assert.Contains(t, tl.Description(), "translation memory")
}

func TestTMLeverageToolExactMatch(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "100", resultBlock.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "exact", resultBlock.Properties[tools.PropTMMatchType])
}

func TestTMLeverageToolFuzzyMatch(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "85", resultBlock.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "fuzzy", resultBlock.Properties[tools.PropTMMatchType])
}

func TestTMLeverageToolNoMatch(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	_, hasScore := resultBlock.Properties[tools.PropTMMatchScore]
	assert.False(t, hasScore)
}

func TestTMLeverageToolExactOverFuzzy(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Exact match should win.
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "100", resultBlock.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "exact", resultBlock.Properties[tools.PropTMMatchType])
}

func TestTMLeverageToolNullProvider(t *testing.T) {
	t.Parallel()
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       &tools.NullTMProvider{},
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestTMLeverageToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestTMLeverageToolEmptySource(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{
		exact: map[string]string{
			"": "something",
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestTMLeverageConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.TMLeverageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.TMLeverageConfig{Provider: &tools.NullTMProvider{}},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name:    "missing provider",
			cfg:     tools.TMLeverageConfig{TargetLocale: model.LocaleFrench},
			wantErr: true,
			errMsg:  "Provider",
		},
		{
			name:    "threshold out of range",
			cfg:     tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, Provider: &tools.NullTMProvider{}, FuzzyThreshold: 101},
			wantErr: true,
			errMsg:  "FuzzyThreshold",
		},
		{
			name: "valid config",
			cfg: tools.TMLeverageConfig{
				TargetLocale:   model.LocaleFrench,
				Provider:       &tools.NullTMProvider{},
				FuzzyThreshold: 80,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
