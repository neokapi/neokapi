package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSrcBlock builds a translatable block with a single source TextRun.
func newSrcBlock(id, text string) *model.Block {
	b := model.NewRunsBlock(id, nil)
	b.SetSourceText(text)
	b.Translatable = true
	return b
}

// translateBlockDirect runs the tool's Produce handler over one block through a
// VariantView, the same entry the worker's single-block path uses.
func translateBlockDirect(t *testing.T, tl *tools.AITranslateTool, b *model.Block) {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	<-out
}

// TestAITranslate_DNTTermSurvivesVerbatim proves a do-not-translate term is
// PREVENTED from being translated in the AI path — not merely mentioned in the
// prompt (epic 019, item 4). The mock provider aggressively "translates"
// everything it is handed (lowercasing every word), which WOULD corrupt the
// brand name "Kapi" — but because the tool masks the protected span before the
// model sees it and restores it after, the exact term survives in the target.
func TestAITranslate_DNTTermSurvivesVerbatim(t *testing.T) {
	// A provider that mangles anything it receives: it lowercases every word.
	// Left to its own devices it would render "Kapi" as "kapi". The sentinel it
	// receives instead ("[[DNT_0]]") is passed through and restored verbatim.
	var sawSource string
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		sawSource = req.Source
		return &aiprovider.TranslateResponse{
			Translation: strings.ToLower(req.Source),
			Confidence:  0.9,
			Model:       "test-model",
		}, nil
	}

	tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Provider:     "anthropic",
		Model:        "claude-x",
		DNT:          []string{"Kapi"},
	})

	b := newSrcBlock("b1", "Welcome to Kapi")
	translateBlockDirect(t, tl, b)

	target := b.TargetText(model.LocaleFrench)
	assert.Contains(t, target, "Kapi", "the DNT term must survive verbatim in the target")
	assert.NotContains(t, target, "kapi", "the DNT term must not be lowercased/translated")
	// The model must never have seen the raw term — it saw the sentinel instead.
	assert.NotContains(t, sawSource, "Kapi", "the protected span must be masked before the model")
	assert.Contains(t, sawSource, "[[DNT_0]]", "the protected span must be masked with a sentinel")
}

// TestAITranslate_DNTForcesSingleBlock: configuring DNT terms forces per-block
// translation (batchSize=1) so the sentinel survives a single generation — the
// batch path cannot track a per-span sentinel across a packed JSON generation.
func TestAITranslate_DNTForcesSingleBlock(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	batchCalls := 0
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		batchCalls++
		return &aiprovider.TranslateResponse{Translation: req.Source, Model: "m"}, nil
	}
	tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Provider:     "anthropic",
		Model:        "claude-x",
		BatchSize:    50, // would batch — but DNT overrides to single
		DNT:          []string{"Acme"},
	})

	in := make(chan *model.Part, 2)
	out := make(chan *model.Part, 2)
	for _, txt := range []string{"Acme one", "Acme two"} {
		bb := newSrcBlock("", txt)
		in <- &model.Part{Type: model.PartBlock, Resource: bb}
	}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))

	// Two single-block Translate calls, not one batched ChatStructured call.
	assert.Equal(t, 2, batchCalls, "DNT terms must force one Translate call per block")
}

// TestAITranslate_DNTLongestFirst: overlapping DNT terms mask longest-first so a
// term that is a substring of another survives intact.
func TestAITranslate_DNTLongestFirst(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: strings.ToUpper(req.Source), Model: "m"}, nil
	}
	tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Provider:     "anthropic",
		Model:        "claude-x",
		DNT:          []string{"Kapi", "Kapi Desktop"},
	})
	b := newSrcBlock("b1", "Open Kapi Desktop now")
	translateBlockDirect(t, tl, b)
	assert.Contains(t, b.TargetText(model.LocaleFrench), "Kapi Desktop")
}
