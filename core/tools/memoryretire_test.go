package tools_test

import (
	"context"
	"testing"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryLeverageDefaultsHoldUntilTheReplacementLands: the thresholds are
// still 70/95, and that is deliberate.
//
// Fuzzy fill is retiring, but the two floors do different jobs and the
// measurement says so. The fill floor at 95 governs the band where an author's
// cosmetic edits land — a trailing period on a real sentence scores 98, not 78
// — and dropping it before a prior version reaches the prompt would replace a
// working behaviour with nothing. The fuzzy floor at 70 governs the band that
// is recorded and read by nothing.
//
// scripts/coordinatereport's edit ladder measures both on every run, so the
// flip is evidence rather than a judgement call. See
// TestMemoryLeverageExactOnlyWhenAsked for the retired behaviour, already
// reachable.
func TestMemoryLeverageDefaultsHoldUntilTheReplacementLands(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.TargetLocale = model.LocaleFrench
	cfg.SourceLocale = model.LocaleEnglish
	cfg.Memory = provider

	assert.Equal(t, 70, cfg.FuzzyThreshold, "the inert band is still looked up, for now")
	assert.Equal(t, 95, cfg.FillTargetThreshold, "and cosmetic edits still fill")

	tl := tools.NewMemoryLeverageTool(cfg)
	block := model.NewBlock("tu1", "Hello world")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

	// 85 is below the fill floor but above the lookup floor: recorded, not
	// filled. That is the band the retirement is actually about.
	out := result.Resource.(*model.Block)
	assert.Empty(t, out.TargetText(model.LocaleFrench), "85 is below the fill floor")
	tm, recorded := model.AnnoAs[*tools.MemoryMatchAnnotation](out, string(model.AnnoMemoryMatch))
	require.True(t, recorded)
	assert.Equal(t, 85, tm.Score)
}

// TestMemoryLeverageExactOnlyWhenAsked: setting both floors to 100 reaches the
// retired behaviour today, so the cutover is a config change rather than a code
// change when the replacement is wired.
func TestMemoryLeverageExactOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 98},
		},
	}
	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.TargetLocale = model.LocaleFrench
	cfg.SourceLocale = model.LocaleEnglish
	cfg.Memory = provider
	cfg.FuzzyThreshold = 100
	cfg.FillTargetThreshold = 100

	tl := tools.NewMemoryLeverageTool(cfg)
	block := model.NewBlock("tu1", "Hello world")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

	out := result.Resource.(*model.Block)
	assert.Empty(t, out.TargetText(model.LocaleFrench))
	_, recorded := model.AnnoAs[*tools.MemoryMatchAnnotation](out, string(model.AnnoMemoryMatch))
	assert.False(t, recorded, "at 100 nothing below an exact match is asked for at all")
}

// TestMemoryLeverageFuzzyStillReachableWhenAsked: a recipe that lowers the
// threshold still gets the old behaviour, so the retirement is reversible per
// project and the path stays exercised until the replacement has been measured
// against it.
func TestMemoryLeverageFuzzyStillReachableWhenAsked(t *testing.T) {
	t.Parallel()
	provider := &mockMemoryProvider{
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.TargetLocale = model.LocaleFrench
	cfg.SourceLocale = model.LocaleEnglish
	cfg.Memory = provider
	cfg.FillTargetThreshold = 80

	tl := tools.NewMemoryLeverageTool(cfg)
	block := model.NewBlock("tu1", "Hello world")
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

	out := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour monde", out.TargetText(model.LocaleFrench))
	tm, ok := model.AnnoAs[*tools.MemoryMatchAnnotation](out, string(model.AnnoMemoryMatch))
	require.True(t, ok)
	assert.Equal(t, 85, tm.Score)
}

// exactAtProvider records the point a lookup was asked from, so the test can
// assert the plain-text path now carries one.
type exactAtProvider struct {
	mockMemoryProvider
	askedAt string
}

func (p *exactAtProvider) Lookup(ctx context.Context, req corememory.Request) (corememory.Match, bool) {
	if req.Block != nil {
		// Block requests are not what this provider is for; the test asks about
		// the flattened path, which the tool reaches after this returns nothing.
		return corememory.Match{}, false
	}
	p.askedAt = req.Point
	trans, ok := p.exact[req.Text]
	if !ok {
		return corememory.Match{}, false
	}
	return corememory.Match{
		TargetRuns: []model.Run{{Text: &model.TextRun{Text: trans}}},
		Score:      100,
		Exact:      true,
	}, true
}

func (p *exactAtProvider) PriorVersion(context.Context, corememory.VersionRequest) (corememory.Version, bool) {
	return corememory.Version{}, false
}

// TestMemoryLeverageExactCarriesThePoint: the plain-text exact lookup is given
// the point the fill is happening at.
//
// The block path has always had it; the text path never did, so an exact answer
// approved somewhere else was offered here with nothing to say so. Closing that
// is the other half of narrowing the interface to the exact case.
func TestMemoryLeverageExactCarriesThePoint(t *testing.T) {
	t.Parallel()
	provider := &exactAtProvider{}
	provider.exact = map[string]string{"Hello world": "Bonjour le monde"}

	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.TargetLocale = model.LocaleFrench
	cfg.SourceLocale = model.LocaleEnglish
	cfg.Memory = provider
	cfg.Point = "acme\x1fsupport\x1facme-help"

	tl := tools.NewMemoryLeverageTool(cfg)
	block := model.NewBlock("tu1", "Hello world")
	processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})

	assert.Equal(t, cfg.Point, provider.askedAt,
		"an exact answer approved elsewhere is still an answer from elsewhere")
}
