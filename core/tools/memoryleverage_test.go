package tools_test

import (
	"context"
	"testing"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMemoryProvider implements corememory.Provider for testing.
//
// It answers a block request and a text request alike, by flattening the block,
// because the real provider does: one interface, and "cannot answer" is a
// return value rather than a missing method.
type mockMemoryProvider struct {
	exact map[string]string     // source -> translation
	fuzzy map[string]fuzzyMatch // source -> {translation, score}
}

type fuzzyMatch struct {
	translation string
	score       int
}

func (m *mockMemoryProvider) Lookup(_ context.Context, req corememory.Request) (corememory.Match, bool) {
	key := req.Text
	if req.Block != nil {
		key = model.FlattenRuns(req.Block.Source)
	}
	if key == "" {
		return corememory.Match{}, false
	}
	if translation, ok := m.exact[key]; ok {
		return corememory.Match{
			TargetRuns: []model.Run{{Text: &model.TextRun{Text: translation}}},
			Score:      100,
			Exact:      true,
		}, true
	}
	// A verbatim request wants the same content or nothing; the fuzzy table is
	// not an answer to it.
	if req.Verbatim || req.MinScore >= 100 {
		return corememory.Match{}, false
	}
	match, ok := m.fuzzy[key]
	if !ok || match.score < req.MinScore {
		return corememory.Match{}, false
	}
	return corememory.Match{
		TargetRuns: []model.Run{{Text: &model.TextRun{Text: match.translation}}},
		Score:      match.score,
	}, true
}

func (m *mockMemoryProvider) PriorVersion(context.Context, corememory.VersionRequest) (corememory.Version, bool) {
	return corememory.Version{}, false
}

var _ corememory.Provider = (*mockMemoryProvider)(nil)

func TestMemoryLeverageTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         corememory.NullProvider{},
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	assert.Equal(t, "recycle", tl.Name())
	assert.Contains(t, tl.Description(), "content memory")
}

func TestMemoryLeverageToolExactMatch(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
	}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](resultBlock, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "exact", tm.Type)

	// Whole-block leverage is auditable too: target provenance + an
	// alt-translation annotation carrying the match metadata.
	tgt := resultBlock.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.OriginMemory, tgt.Origin.Kind)
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

// TestMemoryLeverageToolStampsGovernance: a target filled from content memory
// under a governing profile carries the current context's profile, its pinned
// version and a non-empty fingerprint — resolved at fill time, so a recycled
// target is as attributable and as drift-detectable as a translated one.
func TestMemoryLeverageToolStampsGovernance(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		exact: map[string]string{"Hello world": "Bonjour le monde"},
	}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
		Profile:        &profile.VoiceProfile{ID: "end-user-help", Name: "End-user help", Version: 7},
		TermRules:      []profile.TermRule{{Term: "cart", Replacement: "panier"}},
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

	tgt := result.Resource.(*model.Block).Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.OriginMemory, tgt.Origin.Kind)
	assert.Equal(t, "recycle", tgt.Origin.Tool)
	assert.Equal(t, "end-user-help", tgt.Origin.Profile)
	assert.Equal(t, "7", tgt.Origin.ProfileVersion)
	assert.NotEmpty(t, tgt.Origin.ContextFingerprint)
}

func TestMemoryLeverageToolFuzzyMatch(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour monde", resultBlock.TargetText(model.LocaleFrench))
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](resultBlock, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 85, tm.Score)
	assert.Equal(t, "fuzzy", tm.Type)
}

func TestMemoryLeverageToolNoMatch(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	_, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](resultBlock, string(model.AnnoMemoryMatch))
	assert.False(t, ok)
}

func TestMemoryLeverageToolExactOverFuzzy(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Exact match should win.
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](resultBlock, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "exact", tm.Type)
}

func TestMemoryLeverageToolNullProvider(t *testing.T) {
	t.Parallel()
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         corememory.NullProvider{},
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestMemoryLeverageToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
	}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestMemoryLeverageToolEmptySource(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		exact: map[string]string{
			"": "something",
		},
	}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Memory:         provider,
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestMemoryLeverageConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.MemoryLeverageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.MemoryLeverageConfig{Memory: corememory.NullProvider{}},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name:    "missing memory",
			cfg:     tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench},
			wantErr: true,
			errMsg:  "Memory",
		},
		{
			name:    "threshold out of range",
			cfg:     tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, Memory: corememory.NullProvider{}, FuzzyThreshold: 101},
			wantErr: true,
			errMsg:  "FuzzyThreshold",
		},
		{
			name: "valid config",
			cfg: tools.MemoryLeverageConfig{
				TargetLocale:   model.LocaleFrench,
				Memory:         corememory.NullProvider{},
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

func TestMemoryLeverageSegmentedAllExact(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{exact: map[string]string{
		seg1Src:    "Bonjour le monde. ",
		"Goodbye.": "Au revoir.",
	}}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := segBlock("tu1", "Hello world. ", "Goodbye.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	assert.Equal(t, "Bonjour le monde. Au revoir.", rb.TargetText(model.LocaleFrench))
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](rb, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "segmented-exact", tm.Type)
	assert.Equal(t, "2/2", tm.SegmentMatches)
	// Source runs are never rewritten by leverage.
	assert.Equal(t, "Hello world. Goodbye.", rb.SourceText())

	// The committed target carries provenance + score, not just text.
	tgt := rb.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, model.OriginMemory, tgt.Origin.Kind)
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
	v, ok := model.AnnoAs[*model.AltTranslations](b, tools.PropMemorySegmentAlts)
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

func TestMemoryLeverageSegmentedMixedExactFuzzy(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		exact: map[string]string{seg1Src: "Bonjour le monde. "},
		fuzzy: map[string]fuzzyMatch{"Goodbye.": {translation: "Au revoir.", score: 80}},
	}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := segBlock("tu1", "Hello world. ", "Goodbye.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	assert.Equal(t, "Bonjour le monde. Au revoir.", rb.TargetText(model.LocaleFrench))
	// Block score is the weakest leveraged segment.
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](rb, string(model.AnnoMemoryMatch))
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

func TestMemoryLeverageSegmentedPartialNoFill(t *testing.T) {
	t.Parallel()
	// Only the first sentence is in the content memory; the second misses entirely.
	provider := &mockMemoryProvider{exact: map[string]string{seg1Src: "Bonjour le monde. "}}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := segBlock("tu1", "Hello world. ", "Goodbye.")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	// Partial leverage must not write a half-translated target.
	assert.Empty(t, rb.TargetText(model.LocaleFrench))
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](rb, string(model.AnnoMemoryMatch))
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

// --- structure-aware (BlockMemoryProvider) leverage ---

// mockBlockMemoryProvider answers a block request with a canned match and
// delegates a text request to the embedded text mock.
//
// It has to say which explicitly, and that is the point. Under the old split it
// implemented two interfaces and was two different providers depending on which
// method the caller happened to reach — so a mock could answer a block one way
// and, if the caller took the other door, answer nothing at all without
// anything saying so.
type mockBlockMemoryProvider struct {
	mockMemoryProvider
	match corememory.Match
	found bool
	calls int
	// at records the context point the tool asked from, so a test can assert
	// that a fill names where it is happening.
	at string
}

func (m *mockBlockMemoryProvider) Lookup(ctx context.Context, req corememory.Request) (corememory.Match, bool) {
	if req.Block == nil {
		m.at = req.Point
		return m.mockMemoryProvider.Lookup(ctx, req)
	}
	m.calls++
	m.at = req.Point
	return m.match, m.found
}

// iconBlock builds a block shaped like a KBF icon button: a standalone
// placeholder run followed by text.
func iconBlock(id string) *model.Block {
	b := model.NewBlock(id, "")
	b.Source = []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:element", Data: "{=m0}", Equiv: "=m0"}},
		{Text: &model.TextRun{Text: " Install"}},
	}
	return b
}

// plainBlock builds a code-free block, for the cases that are about match
// tiers rather than inline-code integrity.
func plainBlock(id, text string) *model.Block {
	b := model.NewBlock(id, "")
	b.Source = []model.Run{{Text: &model.TextRun{Text: text}}}
	return b
}

func iconTargetRuns() []model.Run {
	return []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:element", Data: "{=m0}", Equiv: "=m0"}},
		{Text: &model.TextRun{Text: " Installer"}},
	}
}

// TestMemoryLeverageBlockAwareRunsFill: a structural exact match fills the
// target with the entry's RUNS — the placeholder survives as a model
// object, not flattened text.
func TestMemoryLeverageBlockAwareRunsFill(t *testing.T) {
	t.Parallel()
	provider := &mockBlockMemoryProvider{
		match: corememory.Match{TargetRuns: iconTargetRuns(), Score: 100, Exact: true},
		found: true,
	}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	tgt := rb.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	require.Len(t, tgt.Runs, 2, "target keeps the entry's run structure")
	require.NotNil(t, tgt.Runs[0].Ph, "placeholder run preserved")
	assert.Equal(t, "=m0", tgt.Runs[0].Ph.Equiv)
	assert.Equal(t, " Installer", tgt.Runs[1].Text.Text)
	assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	assert.Equal(t, model.OriginMemory, tgt.Origin.Kind)
	assert.InEpsilon(t, 1.0, tgt.Score, 0.001)

	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](rb, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score)
	assert.Equal(t, "exact", tm.Type)
}

// TestMemoryLeverageAsksFromWhereItIs: a fill has to name the point it is
// happening at, or the content memory cannot tell one collection's reviewed
// wording from another's and answers both with whichever it repeats most. The
// point comes from the flow's bindings and reaches the lookup unchanged.
func TestMemoryLeverageAsksFromWhereItIs(t *testing.T) {
	t.Parallel()
	provider := &mockBlockMemoryProvider{
		match: corememory.Match{TargetRuns: iconTargetRuns(), Score: 100, Exact: true},
		found: true,
	}
	at := memory.NewPoint("neokapi", "cli", "neokapi-cli")
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish,
		FuzzyThreshold: 70, Memory: provider, Point: at,
	}
	tl := tools.NewMemoryLeverageTool(cfg)

	processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	assert.Equal(t, at, provider.at, "the fill names the collection it is filling")
}

// TestMemoryLeverageBlockAwareAmbiguousSkips: an ambiguous match is recorded
// as a candidate but never filled — and the text path must not run either
// (it would resolve the tie by arbitrary pick).
func TestMemoryLeverageBlockAwareAmbiguousSkips(t *testing.T) {
	t.Parallel()
	provider := &mockBlockMemoryProvider{
		mockMemoryProvider: mockMemoryProvider{exact: map[string]string{"Install": "Installation"}},
		match:              corememory.Match{TargetRuns: iconTargetRuns(), Score: 99, Exact: true, Ambiguous: true},
		found:              true,
	}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	assert.False(t, rb.HasTarget(model.LocaleFrench), "ambiguous match must not fill")
	alts := rb.AltTranslations()
	require.Len(t, alts, 1, "the ambiguous candidate is recorded for review")
	assert.Equal(t, " Installer", model.RunsText(alts[0].Target))
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](rb, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 99, tm.Score)
}

// TestMemoryLeverageBlockAwareSubThresholdFallsThrough: a block match below
// the fill threshold is recorded, then the differently-keyed text path
// still runs and can fill from a legacy plain-text entry. The block is
// code-free, so the text path's flat fill loses nothing.
func TestTMLeverageBlockAwareSubThresholdFallsThrough(t *testing.T) {
	t.Parallel()
	provider := &mockBlockMemoryProvider{
		mockMemoryProvider: mockMemoryProvider{exact: map[string]string{"Install": "Installer"}},
		match:              corememory.Match{TargetRuns: []model.Run{{Text: &model.TextRun{Text: "Installation"}}}, Score: 80, Exact: false},
		found:              true,
	}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, FillTarget: true, FillTargetThreshold: 95, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: plainBlock("tu1", "Install")})
	rb := result.Resource.(*model.Block)

	assert.Equal(t, 1, provider.calls)
	assert.Equal(t, "Installer", rb.TargetText(model.LocaleFrench), "text path filled after fall-through")
	// Both candidates are on record: the sub-threshold block match and the
	// text-path exact.
	assert.Len(t, rb.AltTranslations(), 2)
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](rb, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 100, tm.Score, "text-path exact overwrites the sub-threshold annotation")
}

// TestMemoryLeverageBlockAwareIncompatibleCodes: a matched target carrying
// inline codes the block's source does not have is never spliced in; the
// text path takes over.
func TestMemoryLeverageBlockAwareIncompatibleCodes(t *testing.T) {
	t.Parallel()
	foreign := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "9", Type: "jsx:element", Data: "{=m9}", Equiv: "=m9"}},
		{Text: &model.TextRun{Text: "Installer"}},
		{PcClose: &model.PcCloseRun{ID: "9", Type: "jsx:element", Data: "{/=m9}", Equiv: "=m9"}},
	}
	provider := &mockBlockMemoryProvider{
		mockMemoryProvider: mockMemoryProvider{exact: map[string]string{"Install": "Installer"}},
		match:              corememory.Match{TargetRuns: foreign, Score: 100, Exact: true},
		found:              true,
	}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: plainBlock("tu1", "Install")})
	rb := result.Resource.(*model.Block)

	tgt := rb.Target(model.LocaleFrench)
	require.NotNil(t, tgt, "text path filled instead")
	require.Len(t, tgt.Runs, 1)
	assert.Equal(t, "Installer", tgt.Runs[0].Text.Text)
}

// --- placeholder integrity: a recycled target must carry its source's codes ---

// TestTMLeverageNeverFillsLossyMatch is the regression guard for the defect
// this suite used to encode: the plain tier matches on *flattened* text, so a
// legacy TM entry stored without inline codes is an exact/near-exact match for
// a code-bearing source. Filling it dropped the placeholder, and the UI
// rendered a sentence with a hole where the count belonged.
//
// The table walks both fill paths (structure-aware LookupBlock and the
// flattened text fallback) against both outcomes: a candidate that lost a code
// must never be filled, and a candidate that carries the source's codes must
// still fill exactly as before.
func TestTMLeverageNeverFillsLossyMatch(t *testing.T) {
	t.Parallel()

	// A count placeholder followed by text — the FormatsPage.tsx shape:
	// `{documentedCount} documented formats`.
	countBlock := func(id string) *model.Block {
		b := model.NewBlock(id, "")
		b.Source = []model.Run{
			{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{documentedCount}", Equiv: "documentedCount"}},
			{Text: &model.TextRun{Text: " documented formats"}},
		}
		return b
	}
	faithfulRuns := []model.Run{
		{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{documentedCount}", Equiv: "documentedCount"}},
		{Text: &model.TextRun{Text: " dokumenterte formater"}},
	}
	lossyRuns := []model.Run{{Text: &model.TextRun{Text: " dokumenterte formater"}}}

	tests := []struct {
		name string
		// blockMatch, when non-nil, is served by the structure-aware path.
		blockMatch []model.Run
		// exactText, when non-empty, is served by the flattened text path.
		exactText string
		wantText  string
		// wantPlaceholder asserts the filled target keeps the source's Ph run.
		wantPlaceholder bool
	}{
		{
			name:            "structure-aware match keeping the placeholder fills",
			blockMatch:      faithfulRuns,
			wantText:        " dokumenterte formater",
			wantPlaceholder: true,
		},
		{
			name:       "structure-aware match that dropped the placeholder is rejected",
			blockMatch: lossyRuns,
		},
		{
			name:      "text-path match is rejected because it cannot carry the placeholder",
			exactText: " dokumenterte formater",
		},
		{
			name:       "lossy block match does not let the text path fill either",
			blockMatch: lossyRuns,
			exactText:  " dokumenterte formater",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &mockBlockMemoryProvider{}
			if tc.exactText != "" {
				provider.mockMemoryProvider = mockMemoryProvider{exact: map[string]string{" documented formats": tc.exactText}}
			}
			if tc.blockMatch != nil {
				provider.match = corememory.Match{TargetRuns: tc.blockMatch, Score: 99, Exact: true}
				provider.found = true
			}
			cfg := &tools.MemoryLeverageConfig{
				TargetLocale:        model.LocaleID("nb"),
				SourceLocale:        model.LocaleEnglish,
				FuzzyThreshold:      70,
				FillTarget:          true,
				FillTargetThreshold: 95,
				Memory:              provider,
			}
			tl := tools.NewMemoryLeverageTool(cfg)

			result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: countBlock("tu1")})
			rb := result.Resource.(*model.Block)

			if tc.wantText == "" {
				assert.False(t, rb.HasTarget(model.LocaleID("nb")),
					"a candidate missing the source's placeholder must not be filled — the locale falls back to source")
				require.NotEmpty(t, rb.AltTranslations(),
					"the rejected candidate is still recorded for review")
				return
			}

			tgt := rb.Target(model.LocaleID("nb"))
			require.NotNil(t, tgt)
			assert.Equal(t, tc.wantText, model.RunsText(tgt.Runs))
			if tc.wantPlaceholder {
				require.NotNil(t, tgt.Runs[0].Ph, "placeholder run preserved")
				assert.Equal(t, "documentedCount", tgt.Runs[0].Ph.Equiv)
			}
		})
	}
}

// TestTMLeverageSegmentedFillRejectsCodeLoss: the segment path assembles a
// plain-text target from per-segment TM hits, so it cannot carry an inline
// code the block's source holds. The segment matches stay on record, but the
// lossy assembly is never committed as the block target.
func TestTMLeverageSegmentedFillRejectsCodeLoss(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{exact: map[string]string{
		seg1Src:    "Hei verden. ",
		"Goodbye.": "Farvel.",
	}}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleID("nb"), SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	block := segBlock("tu1", seg1Src, "Goodbye.")
	// Append an icon placeholder to the source: the segment overlay still
	// covers the text, but the assembled plain target would drop the code.
	block.Source = append(block.Source, model.Run{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:element", Data: "{=m0}", Equiv: "=m0"}})

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	assert.False(t, rb.HasTarget(model.LocaleID("nb")), "lossy segment assembly must not fill")
	assert.NotNil(t, segAlt(rb, 0), "segment matches stay recorded for review")
}

// TestTMLeverageBlockAwareCloneIsolation: the filled target runs are
// deep-copied — mutating them must not write through to the provider's
// stored entry.
func TestMemoryLeverageBlockAwareCloneIsolation(t *testing.T) {
	t.Parallel()
	stored := iconTargetRuns()
	provider := &mockBlockMemoryProvider{
		match: corememory.Match{TargetRuns: stored, Score: 100, Exact: true},
		found: true,
	}
	cfg := &tools.MemoryLeverageConfig{TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider}
	tl := tools.NewMemoryLeverageTool(cfg)

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	rb := result.Resource.(*model.Block)

	rb.Target(model.LocaleFrench).Runs[1].Text.Text = "MUTATED"
	assert.Equal(t, " Installer", stored[1].Text.Text, "content-memory entry runs unaffected by target edits")
}

// ctxCapturingProvider records the context each lookup was called with.
type ctxCapturingProvider struct {
	exact string
	block bool
	got   []context.Context
}

func (p *ctxCapturingProvider) Lookup(ctx context.Context, req corememory.Request) (corememory.Match, bool) {
	p.got = append(p.got, ctx)
	if req.Block != nil {
		if !p.block {
			return corememory.Match{}, false
		}
		return corememory.Match{
			TargetRuns: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}},
			Score:      100,
			Exact:      true,
		}, true
	}
	if p.exact == "" || !req.Verbatim {
		return corememory.Match{}, false
	}
	return corememory.Match{
		TargetRuns: []model.Run{{Text: &model.TextRun{Text: p.exact}}},
		Score:      100,
		Exact:      true,
	}, true
}

func (p *ctxCapturingProvider) PriorVersion(ctx context.Context, _ corememory.VersionRequest) (corememory.Version, bool) {
	p.got = append(p.got, ctx)
	return corememory.Version{}, false
}

// TestMemoryLeverageLookupsReceiveTheRunContext: a content-memory lookup is
// I/O — a SQLite query, or a network round-trip for a non-local provider — so
// cancelling a run has to reach it. These lookups used to be handed
// context.Background(), which cannot be cancelled and carries no deadline.
//
// The assertion is not that a context arrives (a placeholder would satisfy
// that) but that cancelling the caller's context cancels what the provider
// received.
func TestMemoryLeverageLookupsReceiveTheRunContext(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		provider *ctxCapturingProvider
	}{
		// Exercises LookupBlock, the structure-aware path.
		{"block lookup", &ctxCapturingProvider{block: true}},
		// A provider whose block lookup misses falls through to the
		// text-based LookupExact / LookupFuzzy pair.
		{"text lookup", &ctxCapturingProvider{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tl := tools.NewMemoryLeverageTool(&tools.MemoryLeverageConfig{
				TargetLocale:   model.LocaleFrench,
				SourceLocale:   model.LocaleEnglish,
				FuzzyThreshold: 70,
				Memory:         tc.provider,
			})

			ctx, cancel := context.WithCancel(context.Background())
			in := make(chan *model.Part, 1)
			out := make(chan *model.Part, 1)
			in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b1", "Hello world")}
			close(in)
			require.NoError(t, tl.Process(ctx, in, out))
			close(out)
			require.NotNil(t, <-out)

			require.NotEmpty(t, tc.provider.got, "the provider must have been consulted")
			for i, got := range tc.provider.got {
				require.NotNil(t, got, "lookup %d received no context", i)
				select {
				case <-got.Done():
					t.Fatalf("lookup %d received an already-cancelled context", i)
				default:
				}
			}

			// The decisive step: cancelling the caller's context must cancel
			// what the provider was given. context.Background() would not
			// react here.
			cancel()
			for i, got := range tc.provider.got {
				select {
				case <-got.Done():
				default:
					t.Fatalf("lookup %d received a context detached from the run", i)
				}
			}
		})
	}
}
