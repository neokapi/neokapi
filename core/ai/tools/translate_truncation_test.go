package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// pushBlocks runs blocks through a tool and returns the produced parts.
func pushBlocks(t *testing.T, tl *tools.AITranslateTool, blocks ...*model.Block) ([]*model.Part, error) {
	t.Helper()
	in := make(chan *model.Part, len(blocks))
	out := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		in <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	return parts, err
}

// A cut-off reply is a fragment of a translation, and a fragment committed as
// the target is indistinguishable from a short translation — no error, no flag,
// nothing for a reviewer to notice. The single-block path has already asked for
// a budget sized to this block, so there is no smaller request left to make: it
// must fail rather than write.
func TestSingleBlockRefusesToCommitATruncatedTranslation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		streamed bool
	}{
		{name: "blocking Translate"},
		{name: "streaming ChatStream", streamed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mock := aiprovider.NewMockProvider()
			mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
				return &aiprovider.TranslateResponse{
					Translation: "Bonjour le mon",
					Model:       "test-model",
					Truncated:   true,
				}, nil
			}
			mock.ChatStreamFunc = func(_ context.Context, _ []aiprovider.Message, onEvent func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
				onEvent(aiprovider.ChatStreamEvent{Type: aiprovider.StreamEventContent, Content: "Bonjour le mon"})
				return &aiprovider.ChatResponse{Content: "Bonjour le mon", Model: "test-model", Truncated: true}, nil
			}

			cfg := tools.AITranslateConfig{
				SourceLocale: model.LocaleEnglish,
				TargetLocale: model.LocaleFrench,
				BatchSize:    1,
			}
			if tt.streamed {
				cfg.OnProgress = func(aiprovider.ProgressEvent) {}
			}
			tl := tools.NewAITranslateTool(mock, cfg)

			block := model.NewBlock("tu1", "Hello world")
			_, err := pushBlocks(t, tl, block)

			require.Error(t, err)
			require.ErrorIs(t, err, aiprovider.ErrOutputTruncated)
			assert.False(t, block.HasTarget(model.LocaleFrench),
				"a truncated reply must not be committed as the target")
		})
	}
}

// The inline-codes path reconstructs runs from the reply, so a truncated reply
// there does not merely shorten the text — it drops the tags that came after the
// cut, and writes a target the source can never round-trip through.
func TestInlineCodeBlockRefusesToCommitATruncatedTranslation(t *testing.T) {
	t.Parallel()

	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{Translation: "Cliquez <ph id=\"1", Model: "test-model", Truncated: true}, nil
	}

	tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BatchSize:    1,
	})

	block := model.NewBlock("tu1", "")
	block.SetSourceRuns([]model.Run{
		model.TextR("Click "),
		model.PhR(model.PlaceholderRun{ID: "1", Equiv: "icon"}),
		model.TextR(" here"),
	})

	_, err := pushBlocks(t, tl, block)
	require.Error(t, err)
	require.ErrorIs(t, err, aiprovider.ErrOutputTruncated)
	assert.False(t, block.HasTarget(model.LocaleFrench))
}

// packBlocks isolates a block into a batch of one exactly when its estimated
// output exceeds the whole call budget — so the blocks reaching the single-block
// path are the ones already known to be large. Letting them run on a provider's
// bare default is a guaranteed truncation of precisely the work that needed more
// room.
func TestSingleBlockPathGetsABudgetSizedForTheBlock(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var budgets []int
	var hadBudget []bool

	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		n, ok := aiprovider.MaxOutputTokensFrom(ctx)
		mu.Lock()
		budgets = append(budgets, n)
		hadBudget = append(hadBudget, ok)
		mu.Unlock()
		return &aiprovider.TranslateResponse{Translation: "traduit", Model: "test-model"}, nil
	}

	// Auto packing (no pinned batch size): the mock declares no limits, so the
	// call budget is the conservative one, and a block this long busts it alone.
	tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	long := strings.Repeat("Words that will need translating. ", 500) // ~4k est tokens
	_, err := pushBlocks(t, tl, model.NewBlock("tu1", long))
	require.NoError(t, err)

	require.Len(t, budgets, 1, "an oversized block is translated alone")
	assert.True(t, hadBudget[0], "the single-block path must carry an output budget")
	assert.Greater(t, budgets[0], aiprovider.ConservativeMaxOutputTokens,
		"a block that busts the call budget must ask for more than a provider's bare default")
}

// The budget only ever raises the cap. A reasoning model draws its thoughts from
// the same allowance as its answer, so sizing a short block's request down to
// what its words need would be a new way to truncate one.
func TestSingleBlockBudgetNeverTightensBelowTheProviderDefault(t *testing.T) {
	t.Parallel()

	var budget int
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(ctx context.Context, _ aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		budget, _ = aiprovider.MaxOutputTokensFrom(ctx)
		return &aiprovider.TranslateResponse{Translation: "Enregistrer", Model: "test-model"}, nil
	}

	tl := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BatchSize:    1,
	})

	_, err := pushBlocks(t, tl, model.NewBlock("tu1", "Save"))
	require.NoError(t, err)

	assert.GreaterOrEqual(t, budget, aiprovider.ConservativeMaxOutputTokens,
		"a short block must not be squeezed below what the provider would have allowed anyway")
}

// A truncated batch reply from a provider that reports it as a flag rather than
// an error must halve the batch and retry, not fail the run. Driven through the
// real Azure provider, because the flag it reports is the whole subject: without
// it the fragment reaches the JSON decoder and the errgroup takes the run down.
func TestTruncatedAzureBatchSplitsInsteadOfAborting(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var batchSizes []int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) || !assert.NotEmpty(t, req.Messages) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var payload struct {
			Segments []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"segments"`
		}
		if !assert.NoError(t, json.Unmarshal([]byte(req.Messages[len(req.Messages)-1].Content), &payload)) {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}

		mu.Lock()
		batchSizes = append(batchSizes, len(payload.Segments))
		mu.Unlock()

		// A batch of more than two segments is more than this deployment will
		// emit: the reply stops mid-object with finish_reason "length".
		if len(payload.Segments) > 2 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "gpt-4o",
				"choices": []map[string]any{{
					"finish_reason": "length",
					"message":       map[string]any{"content": `{"translations":[{"id":"s1","te`},
				}},
			})
			return
		}

		type item struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		var result struct {
			Translations []item `json:"translations"`
		}
		for _, seg := range payload.Segments {
			result.Translations = append(result.Translations, item{ID: seg.ID, Text: "traduit-" + seg.Text})
		}
		content, _ := json.Marshal(result)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4o",
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"content": string(content)},
			}},
		})
	}))
	defer srv.Close()

	provider := aiprovider.NewAzureOpenAIProvider(aiprovider.Config{
		BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o",
	})
	tl := tools.NewAITranslateTool(provider, tools.AITranslateConfig{
		SourceLocale:     model.LocaleEnglish,
		TargetLocale:     model.LocaleFrench,
		BatchSize:        4,
		BatchConcurrency: 1,
	})

	blocks := make([]*model.Block, 4)
	for i := range blocks {
		blocks[i] = model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Text %d", i))
	}

	_, err := pushBlocks(t, tl, blocks...)
	require.NoError(t, err, "a truncated batch is our batch being too big, not a failed run")

	for _, b := range blocks {
		assert.Equal(t, "traduit-"+b.SourceText(), b.TargetText(model.LocaleFrench),
			"every block in a split batch is still translated")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{4, 2, 2}, batchSizes,
		"the oversized batch is halved and each half retried")
}
