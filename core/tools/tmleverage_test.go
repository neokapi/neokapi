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

	assert.Equal(t, "recycle", tl.Name())
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
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](resultBlock, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "exact", tm.Type)

	// Whole-block leverage is auditable too: target provenance + an
	// alt-translation annotation carrying the match metadata.
	tgt := resultBlock.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, "tm", tgt.Origin.Kind)
	assert.Equal(t, "recycle", tgt.Origin.Tool)
	assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	assert.InEpsilon(t, 1.0, tgt.Score, 0.001)
	alts := resultBlock.AltTranslations()
	require.Len(t, alts, 1, "one alt-translation candidate present")
	alt := alts[0]
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
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](resultBlock, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 85, tm.Score)
	assert.Equal(t, "fuzzy", tm.Type)
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
	_, ok := model.AnnoAs[*tools.TMMatchAnnotation](resultBlock, string(model.AnnoTMMatch))
	assert.False(t, ok)
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
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](resultBlock, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "exact", tm.Type)
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
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](rb, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "segmented-exact", tm.Type)
	assert.Equal(t, "2/2", tm.SegmentMatches)
	// Source runs are never rewritten by leverage.
	assert.Equal(t, "Hello world. Goodbye.", rb.SourceText())

	// The committed target carries provenance + score, not just text.
	tgt := rb.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, "tm", tgt.Origin.Kind)
	assert.Equal(t, "recycle", tgt.Origin.Tool)
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

// altTrans fetches the per-segment AltTranslation from the tm-segment-alts
// collection by its SegmentIndex.
func altTrans(t *testing.T, b *model.Block, idx int) *model.AltTranslation {
	t.Helper()
	at := segAlt(b, idx)
	require.NotNil(t, at, "alt-translation for segment %d present", idx)
	return at
}

// segAlt returns the per-segment alt-translation with the given SegmentIndex, or nil.
func segAlt(b *model.Block, idx int) *model.AltTranslation {
	v, ok := model.AnnoAs[*model.AltTranslations](b, tools.PropTMSegmentAlts)
	if !ok {
		return nil
	}
	for _, a := range v.Items {
		if a.SegmentIndex == idx {
			return a
		}
	}
	return nil
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
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](rb, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 80, tm.Score)
	assert.Equal(t, "segmented-fuzzy", tm.Type)
	assert.Equal(t, "2/2", tm.SegmentMatches)
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
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](rb, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Empty(t, tm.Type)
	assert.Equal(t, "1/2", tm.SegmentMatches)
	// ...but the one matched segment is preserved as an AltTranslation for a
	// later stage / the editor, not discarded.
	a0 := altTrans(t, rb, 0)
	assert.Equal(t, "Bonjour le monde. ", model.RunsText(a0.Target))
	assert.Equal(t, model.MatchExact, a0.MatchType)
	assert.Nil(t, segAlt(rb, 1), "unmatched segment has no alt-translation")
}

// --- structure-aware (BlockTMProvider) leverage ---

// mockBlockTMProvider implements both TMProvider and BlockTMProvider: the
// text maps drive the fallback path, the block match drives the
// structure-aware path.
type mockBlockTMProvider struct {
	mockTMProvider
	match tools.TMBlockMatch
	found bool
	calls int
}

func (m *mockBlockTMProvider) LookupBlock(_ *model.Block, _, _ model.LocaleID, _ int) (tools.TMBlockMatch, bool) {
	m.calls++
	return m.match, m.found
}

// iconBlock builds a block shaped like a KLF icon button: a standalone
// placeholder run followed by text.
func iconBlock(id string) *model.Block {
	b := model.NewBlock(id, "")
	b.Source = []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:element", Data: "{=m0}", Equiv: "=m0"}},
		{Text: &model.TextRun{Text: " Install"}},
	}
	return b
}

func iconTargetRuns() []model.Run {
	return []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:element", Data: "{=m0}", Equiv: "=m0"}},
		{Text: &model.TextRun{Text: " Installer"}},
	}
}

// TestTMLeverageBlockAwareRunsFill: a structural exact match fills the
// target with the entry's RUNS — the placeholder survives as a model
// object, not flattened text.
func TestTMLeverageBlockAwareRunsFill(t *testing.T) {
	t.Parallel()
	provider := &mockBlockTMProvider{
		match: tools.TMBlockMatch{TargetRuns: iconTargetRuns(), Score: 100, Exact: true},
		found: true,
	}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	tgt := rb.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	require.Len(t, tgt.Runs, 2, "target keeps the entry's run structure")
	require.NotNil(t, tgt.Runs[0].Ph, "placeholder run preserved")
	assert.Equal(t, "=m0", tgt.Runs[0].Ph.Equiv)
	assert.Equal(t, " Installer", tgt.Runs[1].Text.Text)
	assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	assert.Equal(t, "tm", tgt.Origin.Kind)
	assert.InEpsilon(t, 1.0, tgt.Score, 0.001)

	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](rb, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "exact", tm.Type)
}

// TestTMLeverageBlockAwareAmbiguousSkips: an ambiguous match is recorded
// as a candidate but never filled — and the text path must not run either
// (it would resolve the tie by arbitrary pick).
func TestTMLeverageBlockAwareAmbiguousSkips(t *testing.T) {
	t.Parallel()
	provider := &mockBlockTMProvider{
		mockTMProvider: mockTMProvider{exact: map[string]string{"Install": "Installation"}},
		match:          tools.TMBlockMatch{TargetRuns: iconTargetRuns(), Score: 99, Exact: true, Ambiguous: true},
		found:          true,
	}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	assert.False(t, rb.HasTarget(model.LocaleFrench), "ambiguous match must not fill")
	alts := rb.AltTranslations()
	require.Len(t, alts, 1, "the ambiguous candidate is recorded for review")
	assert.Equal(t, " Installer", model.RunsText(alts[0].Target))
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](rb, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 99, tm.Score)
}

// TestTMLeverageBlockAwareSubThresholdFallsThrough: a block match below
// the fill threshold is recorded, then the differently-keyed text path
// still runs and can fill from a legacy plain-text entry.
func TestTMLeverageBlockAwareSubThresholdFallsThrough(t *testing.T) {
	t.Parallel()
	provider := &mockBlockTMProvider{
		// Text path keys on the text-only source (" Install" → codes drop).
		mockTMProvider: mockTMProvider{exact: map[string]string{" Install": "Installer"}},
		match:          tools.TMBlockMatch{TargetRuns: iconTargetRuns(), Score: 80, Exact: false},
		found:          true,
	}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, FillTarget: true, FillTargetThreshold: 95, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, "Installer", rb.TargetText(model.LocaleFrench), "text path filled after fall-through")
	// Both candidates are on record: the sub-threshold block match and the
	// text-path exact.
	assert.Len(t, rb.AltTranslations(), 2)
	tm, ok := model.AnnoAs[*tools.TMMatchAnnotation](rb, string(model.AnnoTMMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score, "text-path exact overwrites the sub-threshold annotation")
}

// TestTMLeverageBlockAwareIncompatibleCodes: a matched target carrying
// inline codes the block's source does not have is never spliced in; the
// text path takes over.
func TestTMLeverageBlockAwareIncompatibleCodes(t *testing.T) {
	t.Parallel()
	foreign := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "9", Type: "jsx:element", Data: "{=m9}", Equiv: "=m9"}},
		{Text: &model.TextRun{Text: "Installer"}},
		{PcClose: &model.PcCloseRun{ID: "9", Type: "jsx:element", Data: "{/=m9}", Equiv: "=m9"}},
	}
	provider := &mockBlockTMProvider{
		mockTMProvider: mockTMProvider{exact: map[string]string{" Install": "Installer"}},
		match:          tools.TMBlockMatch{TargetRuns: foreign, Score: 100, Exact: true},
		found:          true,
	}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	tgt := rb.Target(model.LocaleFrench)
	require.NotNil(t, tgt, "text path filled instead")
	require.Len(t, tgt.Runs, 1)
	assert.Equal(t, "Installer", tgt.Runs[0].Text.Text)
}

// TestTMLeverageBlockAwareCloneIsolation: the filled target runs are
// deep-copied — mutating them must not write through to the provider's
// stored entry.
func TestTMLeverageBlockAwareCloneIsolation(t *testing.T) {
	t.Parallel()
	stored := iconTargetRuns()
	provider := &mockBlockTMProvider{
		match: tools.TMBlockMatch{TargetRuns: stored, Score: 100, Exact: true},
		found: true,
	}
	cfg := &tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Provider: provider}
	tl := tools.NewTMLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	rb.Target(model.LocaleFrench).Runs[1].Text.Text = "MUTATED"
	assert.Equal(t, " Installer", stored[1].Text.Text, "TM entry runs unaffected by target edits")
}
