package tools_test

import (
	"strconv"
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

	// Whole-block leverage is auditable too: target provenance + an
	// alt-translation annotation carrying the match metadata.
	tgt := resultBlock.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, "tm", tgt.Origin.Kind)
	assert.Equal(t, "tm-leverage", tgt.Origin.Tool)
	assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	assert.InEpsilon(t, 1.0, tgt.Score, 0.001)
	alt, ok := model.AnnoAs[*model.AltTranslation](resultBlock, tools.PropTMAltKey)
	require.True(t, ok, "alt-translation annotation present")
	assert.Equal(t, "Hello world", model.RunsText(alt.Source))
	assert.Equal(t, "Bonjour le monde", model.RunsText(alt.Target))
	assert.Equal(t, model.MatchExact, alt.MatchType)
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

// --- Segment-aware leverage (multi-sentence / prose blocks) ---

// seg1Src is the first sentence's source text. The trailing space is intentional
// (it's the run's text); a const keeps it out of map-key string literals, where
// gocritic flags trailing whitespace as suspicious.
const seg1Src = "Hello world. "

// segBlock builds a two-run block whose runs are the two sentences, with a
// source segmentation overlay splitting on the run boundary. Concatenating the
// segment texts reproduces the source, so the assembled target is faithful.
func segBlock(id, s1, s2 string) *model.Block {
	b := model.NewRunsBlock(id, []model.Run{
		{Text: &model.TextRun{Text: s1}},
		{Text: &model.TextRun{Text: s2}},
	})
	b.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}},
		{ID: "s2", Range: model.RunRange{StartRun: 1, EndRun: 2}},
	})
	return b
}

func TestTMLeverageSegmentedAllExact(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{exact: map[string]string{
		seg1Src:    "Bonjour le monde. ",
		"Goodbye.": "Au revoir.",
	}}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	block := segBlock("tu1", "Hello world. ", "Goodbye.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	assert.Equal(t, "Bonjour le monde. Au revoir.", rb.TargetText(model.LocaleFrench))
	assert.Equal(t, "100", rb.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "segmented-exact", rb.Properties[tools.PropTMMatchType])
	assert.Equal(t, "2/2", rb.Properties[tools.PropTMSegmentMatches])
	// Source runs are never rewritten by leverage.
	assert.Equal(t, "Hello world. Goodbye.", rb.SourceText())

	// The committed target carries provenance + score, not just text.
	tgt := rb.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, "tm", tgt.Origin.Kind)
	assert.Equal(t, "tm-leverage", tgt.Origin.Tool)
	assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	assert.InEpsilon(t, 1.0, tgt.Score, 0.001)

	// Each segment match is recorded as an auditable AltTranslation.
	a0 := altTrans(t, rb, 0)
	assert.Equal(t, "Hello world. ", model.RunsText(a0.Source))
	assert.Equal(t, "Bonjour le monde. ", model.RunsText(a0.Target))
	assert.Equal(t, model.MatchExact, a0.MatchType)
	assert.InEpsilon(t, 1.0, a0.Score, 0.001)
	assert.Equal(t, "tm", a0.Origin)
	a1 := altTrans(t, rb, 1)
	assert.Equal(t, "Goodbye.", model.RunsText(a1.Source))
	assert.Equal(t, "Au revoir.", model.RunsText(a1.Target))
	assert.Equal(t, model.MatchExact, a1.MatchType)
}

// altTrans fetches the per-segment AltTranslation annotation by segment index.
func altTrans(t *testing.T, b *model.Block, idx int) *model.AltTranslation {
	t.Helper()
	a, ok := b.Anno(tools.PropTMSegmentAltPrefix + strconv.Itoa(idx))
	require.True(t, ok, "alt-translation for segment %d present", idx)
	at, ok := a.(*model.AltTranslation)
	require.True(t, ok, "annotation is *AltTranslation")
	return at
}

func TestTMLeverageSegmentedMixedExactFuzzy(t *testing.T) {
	t.Parallel()
	provider := &mockTMProvider{
		exact: map[string]string{seg1Src: "Bonjour le monde. "},
		fuzzy: map[string]fuzzyMatch{"Goodbye.": {translation: "Au revoir.", score: 80}},
	}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	block := segBlock("tu1", "Hello world. ", "Goodbye.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	assert.Equal(t, "Bonjour le monde. Au revoir.", rb.TargetText(model.LocaleFrench))
	// Block score is the weakest leveraged segment.
	assert.Equal(t, "80", rb.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "segmented-fuzzy", rb.Properties[tools.PropTMMatchType])
	assert.Equal(t, "2/2", rb.Properties[tools.PropTMSegmentMatches])
	assert.InEpsilon(t, 0.8, rb.Target(model.LocaleFrench).Score, 0.001)
	// Per-segment annotations carry the individual match type + score.
	assert.Equal(t, model.MatchExact, altTrans(t, rb, 0).MatchType)
	a1 := altTrans(t, rb, 1)
	assert.Equal(t, model.MatchFuzzy, a1.MatchType)
	assert.InEpsilon(t, 0.8, a1.Score, 0.001)
}

func TestTMLeverageSegmentedPartialNoFill(t *testing.T) {
	t.Parallel()
	// Only the first sentence is in the TM; the second misses entirely.
	provider := &mockTMProvider{exact: map[string]string{seg1Src: "Bonjour le monde. "}}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	block := segBlock("tu1", "Hello world. ", "Goodbye.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	// Partial leverage must not write a half-translated target.
	assert.Empty(t, rb.TargetText(model.LocaleFrench))
	assert.Equal(t, "1/2", rb.Properties[tools.PropTMSegmentMatches])
	assert.Empty(t, rb.Properties[tools.PropTMMatchType])
	// ...but the one matched segment is preserved as an AltTranslation for a
	// later stage / the editor, not discarded.
	a0 := altTrans(t, rb, 0)
	assert.Equal(t, "Bonjour le monde. ", model.RunsText(a0.Target))
	assert.Equal(t, model.MatchExact, a0.MatchType)
	_, hasSeg1 := rb.Anno(tools.PropTMSegmentAltPrefix + "1")
	assert.False(t, hasSeg1, "unmatched segment has no alt-translation")
}
