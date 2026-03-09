package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gokapi/gokapi/core/ai/provider"
	"github.com/gokapi/gokapi/core/ai/tools"
	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAITranslateToolSetsTarget(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.TranslateFunc = func(ctx context.Context, req provider.TranslateRequest) (*provider.TranslateResponse, error) {
		return &provider.TranslateResponse{
			Translation: "Bonjour le monde",
			Confidence:  0.92,
			Model:       "test-model",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello World")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, 1, len(mock.TranslateCalls))
}

func TestAITranslateToolSkipsNonTranslatable(t *testing.T) {
	mock := provider.NewMockProvider()

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	assert.Equal(t, 0, len(mock.TranslateCalls))
}

func TestAITranslateToolSkipsMatchedWhenConfigured(t *testing.T) {
	mock := provider.NewMockProvider()

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		SkipMatched:  true,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, 0, len(mock.TranslateCalls))
}

func TestAITranslateToolWithGlossary(t *testing.T) {
	mock := provider.NewMockProvider()
	var capturedReq provider.TranslateRequest
	mock.TranslateFunc = func(ctx context.Context, req provider.TranslateRequest) (*provider.TranslateResponse, error) {
		capturedReq = req
		return &provider.TranslateResponse{
			Translation: "Translated",
			Confidence:  0.9,
			Model:       "test",
		}, nil
	}

	glossary := map[string]string{"hello": "bonjour"}
	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Glossary:     glossary,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	<-out

	assert.Equal(t, glossary, capturedReq.Glossary)
}

func TestAITranslateToolSetsAnnotation(t *testing.T) {
	mock := provider.NewMockProvider()

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	ann, ok := resultBlock.Annotations["alt-translations"]
	require.True(t, ok)
	alt := ann.(*model.AltTranslation)
	assert.Equal(t, "ai:mock", alt.Origin)
	assert.Equal(t, "ai", alt.MatchType)
	assert.Equal(t, model.LocaleFrench, alt.Locale)
}

func TestAITranslateToolInFlow(t *testing.T) {
	mock := provider.NewMockProvider()

	translateTool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	// Send multiple parts including non-block types
	layer := &model.Layer{ID: "doc1", Format: "test"}
	in <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
	in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1", Name: "comment"}}
	in <- &model.Part{Type: model.PartLayerEnd, Resource: layer}
	close(in)

	err := translateTool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}

	assert.Len(t, parts, 5)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartBlock, parts[1].Type)
	assert.Equal(t, model.PartBlock, parts[2].Type)
	assert.Equal(t, model.PartData, parts[3].Type)
	assert.Equal(t, model.PartLayerEnd, parts[4].Type)

	block1 := parts[1].Resource.(*model.Block)
	assert.True(t, block1.HasTarget(model.LocaleFrench))
	assert.Equal(t, 2, len(mock.TranslateCalls))
}

func TestAIQACheckToolAddsProperties(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []provider.Message) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{
			Content: `[{"type":"fluency","severity":"warning","description":"awkward phrasing","suggestion":"rephrase"}]`,
			Model:   "test",
		}, nil
	}

	tool := tools.NewAIQACheckTool(mock, tools.AIQAConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Checks:       []string{"fluency"},
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	issuesStr := resultBlock.Properties["qa-issues"]
	assert.NotEmpty(t, issuesStr)

	var issues []provider.QAIssue
	err = json.Unmarshal([]byte(issuesStr), &issues)
	require.NoError(t, err)
	assert.Len(t, issues, 1)
	assert.Equal(t, "fluency", issues[0].Type)
	assert.Equal(t, "mock", resultBlock.Properties["qa-provider"])
}

func TestAIQACheckToolSkipsUntranslated(t *testing.T) {
	mock := provider.NewMockProvider()

	tool := tools.NewAIQACheckTool(mock, tools.AIQAConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	<-out
	assert.Equal(t, 0, len(mock.ChatCalls))
}

func TestAITerminologyToolExtractsTerms(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []provider.Message) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{
			Content: `[{"term":"API","definition":"Application Programming Interface","domain":"technology"}]`,
			Model:   "test",
		}, nil
	}

	tool := tools.NewAITerminologyTool(mock, tools.AITerminologyConfig{
		Locale: model.LocaleEnglish,
		Domain: "technology",
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "The API endpoint returns JSON")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	termsStr := resultBlock.Properties["terminology"]
	assert.NotEmpty(t, termsStr)

	var terms []tools.TermEntry
	err = json.Unmarshal([]byte(termsStr), &terms)
	require.NoError(t, err)
	assert.Len(t, terms, 1)
	assert.Equal(t, "API", terms[0].Term)
}

func TestAIReviewToolAddsReview(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []provider.Message) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{
			Content: "Score: 9\nAssessment: Accurate translation.\nSuggestion: none",
			Model:   "test",
		}, nil
	}

	tool := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	assert.Contains(t, resultBlock.Properties["review"], "Score: 9")
	assert.Equal(t, "mock", resultBlock.Properties["review-provider"])
}

func TestAIReviewToolSkipsUntranslated(t *testing.T) {
	mock := provider.NewMockProvider()

	tool := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	<-out
	assert.Equal(t, 0, len(mock.ChatCalls))
}

func TestAITranslateBatchMode(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []provider.Message) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{
			Content: "[1] Bonjour le monde\n[2] Bienvenue\n[3] Paramètres",
			Model:   "test-model",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BatchSize:    10,
		Concurrency:  2,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello World")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Welcome")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu3", "Settings")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	require.Len(t, parts, 3)

	// All 3 blocks translated in a single Chat call (batch).
	assert.Equal(t, 1, len(mock.ChatCalls), "should use 1 batch Chat call, not 3 Translate calls")
	assert.Equal(t, 0, len(mock.TranslateCalls), "should not call Translate in batch mode")

	assert.Equal(t, "Bonjour le monde", parts[0].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, "Bienvenue", parts[1].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, "Paramètres", parts[2].Resource.(*model.Block).TargetText(model.LocaleFrench))
}

func TestAITranslateBatchSplitsIntoBatches(t *testing.T) {
	var mu sync.Mutex
	mock := provider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []provider.Message) (*provider.ChatResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		// Parse which segments were requested and return translations.
		content := messages[len(messages)-1].Content
		var resp strings.Builder
		for i := 1; i <= 10; i++ {
			marker := fmt.Sprintf("[%d]", i)
			if strings.Contains(content, marker) {
				fmt.Fprintf(&resp, "[%d] translated-%d\n", i, i)
			}
		}
		return &provider.ChatResponse{Content: resp.String(), Model: "test"}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BatchSize:    2, // 5 blocks → 3 batches (2+2+1)
		Concurrency:  3,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	for i := 0; i < 5; i++ {
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Text %d", i))}
	}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	require.Len(t, parts, 5)

	// With batch size 2, we get 3 batches: [2, 2, 1].
	// The single-item batch (size 1) uses handleBlock → Translate(), not Chat().
	assert.Equal(t, 2, len(mock.ChatCalls), "5 blocks / batch_size=2 → 2 full batches via Chat")
	assert.Equal(t, 1, len(mock.TranslateCalls), "1 remaining block uses single Translate")

	// All blocks should have targets.
	for _, p := range parts {
		block := p.Resource.(*model.Block)
		assert.True(t, block.HasTarget(model.LocaleFrench), "block %s should have target", block.ID)
	}
}

func TestAITranslateBatchPreservesNonBlockParts(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []provider.Message) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{
			Content: "[1] Bonjour\n[2] Monde",
			Model:   "test",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BatchSize:    10,
		Concurrency:  2,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	layer := &model.Layer{ID: "doc1", Format: "test"}
	in <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1", Name: "comment"}}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
	in <- &model.Part{Type: model.PartLayerEnd, Resource: layer}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}

	// Order and types preserved.
	require.Len(t, parts, 5)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartBlock, parts[1].Type)
	assert.Equal(t, model.PartData, parts[2].Type)
	assert.Equal(t, model.PartBlock, parts[3].Type)
	assert.Equal(t, model.PartLayerEnd, parts[4].Type)

	assert.Equal(t, "Bonjour", parts[1].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, "Monde", parts[3].Resource.(*model.Block).TargetText(model.LocaleFrench))
}

func TestAITranslateBatchSkipsNonTranslatable(t *testing.T) {
	mock := provider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []provider.Message) (*provider.ChatResponse, error) {
		return &provider.ChatResponse{
			Content: "[1] Bonjour",
			Model:   "test",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BatchSize:    10,
		Concurrency:  1,
	})

	ctx := context.Background()
	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	b1 := model.NewBlock("tu1", "Hello")
	b2 := model.NewBlock("tu2", "Skip me")
	b2.Translatable = false
	in <- &model.Part{Type: model.PartBlock, Resource: b1}
	in <- &model.Part{Type: model.PartBlock, Resource: b2}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	require.Len(t, parts, 2)

	// Only 1 translatable block → single Translate call (not batch Chat).
	assert.Equal(t, 1, len(mock.TranslateCalls))
	assert.Equal(t, 0, len(mock.ChatCalls))
	assert.True(t, parts[0].Resource.(*model.Block).HasTarget(model.LocaleFrench))
	assert.False(t, parts[1].Resource.(*model.Block).HasTarget(model.LocaleFrench))
}

func TestParseBatchResponse(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		want     []string
	}{
		{
			name:     "clean response",
			content:  "[1] Bonjour\n[2] Monde\n[3] Paramètres",
			expected: 3,
			want:     []string{"Bonjour", "Monde", "Paramètres"},
		},
		{
			name:     "with extra whitespace",
			content:  "  [1]   Bonjour  \n  [2]   Monde  \n",
			expected: 2,
			want:     []string{"Bonjour", "Monde"},
		},
		{
			name:     "with preamble text",
			content:  "Here are the translations:\n\n[1] Bonjour\n[2] Monde",
			expected: 2,
			want:     []string{"Bonjour", "Monde"},
		},
		{
			name:     "missing entry",
			content:  "[1] Bonjour\n[3] Paramètres",
			expected: 3,
			want:     []string{"Bonjour", "", "Paramètres"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tools.ParseBatchResponse(tt.content, tt.expected)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProviderMockDefaultBehavior(t *testing.T) {
	mock := provider.NewMockProvider()

	ctx := context.Background()
	resp, err := mock.Translate(ctx, provider.TranslateRequest{
		Source:       "Hello",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Translation, "[fr]")
	assert.Contains(t, resp.Translation, "Hello")

	chatResp, err := mock.Chat(ctx, []provider.Message{
		{Role: "user", Content: "Test message"},
	})
	require.NoError(t, err)
	assert.Contains(t, chatResp.Content, "Mock response")
}
