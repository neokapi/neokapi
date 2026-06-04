package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coretool "github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAITranslateToolSetsTarget(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		return &aiprovider.TranslateResponse{
			Translation: "Bonjour le monde",
			Confidence:  0.92,
			Model:       "test-model",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello World")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Len(t, mock.TranslateCalls, 1)
}

func TestAITranslateToolSkipsNonTranslatable(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	assert.Empty(t, mock.TranslateCalls)
}

func TestAITranslateToolSkipsMatchedWhenConfigured(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		SkipMatched:  true,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour", resultBlock.TargetText(model.LocaleFrench))
	assert.Empty(t, mock.TranslateCalls)
}

func TestAITranslateToolWithGlossary(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	var capturedReq aiprovider.TranslateRequest
	mock.TranslateFunc = func(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		capturedReq = req
		return &aiprovider.TranslateResponse{
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

	ctx := t.Context()
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

func TestAITranslateToolInjectsBrandVoice(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	var capturedReq aiprovider.TranslateRequest
	mock.TranslateFunc = func(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		capturedReq = req
		return &aiprovider.TranslateResponse{Translation: "Bonjour", Confidence: 0.9, Model: "test"}, nil
	}

	profile := &brand.VoiceProfile{
		Name: "Friendly",
		Tone: brand.ToneProfile{Personality: []string{"warm"}, Formality: "casual"},
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{{Term: "utilize", Replacement: "use"}},
		},
	}
	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Profile:      profile,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	close(in)
	require.NoError(t, tool.Process(ctx, in, out))
	<-out

	require.NotEmpty(t, capturedReq.VoiceGuide, "voice guide must reach the provider request")
	assert.Contains(t, capturedReq.VoiceGuide, "warm")
	// Directives() must surface the forbidden-term swap to the model.
	assert.Contains(t, capturedReq.Directives(), `"utilize" → "use"`)
}

func TestAITranslateToolSetsAnnotation(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	resultBlock := result.Resource.(*model.Block)
	ann, ok := resultBlock.Annotations["alt-translations"]
	require.True(t, ok)
	alt := ann.(*model.AltTranslation)
	assert.Equal(t, "ai:mock", alt.Origin)
	assert.Equal(t, model.MatchAI, alt.MatchType)
	assert.Equal(t, model.LocaleFrench, alt.Locale)
}

func TestAITranslateToolInFlow(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	translateTool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
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
	assert.Len(t, mock.TranslateCalls, 2)
}

func TestAIQACheckToolAddsProperties(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"issues":[{"type":"fluency","severity":"warning","description":"awkward phrasing","suggestion":"rephrase"}]}`,
			Model:   "test",
		}, nil
	}

	tool := tools.NewAIQACheckTool(mock, tools.AIQAConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Checks:       []string{"fluency"},
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	resultBlock := result.Resource.(*model.Block)

	// ai-qa maps the model's structured output onto the unified check findings.
	findings := check.Findings(coretool.NewBlockView(resultBlock))
	require.Len(t, findings, 1)
	assert.Equal(t, "fluency", findings[0].Category)
	assert.Equal(t, check.SeverityMinor, findings[0].Severity) // "warning" → minor
	assert.Equal(t, "awkward phrasing", findings[0].Message)
	assert.Equal(t, "rephrase", findings[0].Suggestion)
	assert.Equal(t, "mock", resultBlock.Properties["qa-provider"])
}

func TestAIQACheckToolSkipsUntranslated(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	tool := tools.NewAIQACheckTool(mock, tools.AIQAConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	<-out
	assert.Empty(t, mock.ChatCalls)
	assert.Empty(t, mock.ChatStructuredCalls)
}

func TestAITerminologyToolExtractsTerms(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"terms":[{"term":"API","definition":"Application Programming Interface","domain":"technology"}]}`,
			Model:   "test",
		}, nil
	}

	tool := tools.NewAITerminologyTool(mock, tools.AITerminologyConfig{
		Locale: model.LocaleEnglish,
		Domain: "technology",
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "The API endpoint returns JSON")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
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
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: "Score: 9\nAssessment: Accurate translation.\nSuggestion: none",
			Model:   "test",
		}, nil
	}

	tool := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	resultBlock := result.Resource.(*model.Block)
	assert.Contains(t, resultBlock.Properties["review"], "Score: 9")
	assert.Equal(t, "mock", resultBlock.Properties["review-provider"])
}

func TestAIReviewToolSkipsUntranslated(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	tool := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	<-out
	assert.Empty(t, mock.ChatCalls)
}

// batchJSON builds a structured batch response JSON string.
func batchJSON(translations map[int]string) string {
	type item struct {
		Index int    `json:"index"`
		Text  string `json:"text"`
	}
	result := struct {
		Translations []item `json:"translations"`
	}{}
	for idx, text := range translations {
		result.Translations = append(result.Translations, item{Index: idx, Text: text})
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func TestAITranslateBatchMode(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: batchJSON(map[int]string{1: "Bonjour le monde", 2: "Bienvenue", 3: "Paramètres"}),
			Model:   "test-model",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale:     model.LocaleEnglish,
		TargetLocale:     model.LocaleFrench,
		BatchSize:        10,
		BatchConcurrency: 2,
	})

	ctx := t.Context()
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

	// All 3 blocks translated in a single ChatStructured call (batch).
	assert.Len(t, mock.ChatStructuredCalls, 1, "should use 1 batch ChatStructured call")
	assert.Empty(t, mock.TranslateCalls, "should not call Translate in batch mode")

	assert.Equal(t, "Bonjour le monde", parts[0].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, "Bienvenue", parts[1].Resource.(*model.Block).TargetText(model.LocaleFrench))
	assert.Equal(t, "Paramètres", parts[2].Resource.(*model.Block).TargetText(model.LocaleFrench))
}

func TestAITranslateBatchSplitsIntoBatches(t *testing.T) {
	var mu sync.Mutex
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		// Parse which segments were requested and return translations.
		content := messages[len(messages)-1].Content
		translations := make(map[int]string)
		for i := 1; i <= 10; i++ {
			marker := fmt.Sprintf("[%d]", i)
			if strings.Contains(content, marker) {
				translations[i] = fmt.Sprintf("translated-%d", i)
			}
		}
		return &aiprovider.ChatResponse{Content: batchJSON(translations), Model: "test"}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale:     model.LocaleEnglish,
		TargetLocale:     model.LocaleFrench,
		BatchSize:        2, // 5 blocks → 3 batches (2+2+1)
		BatchConcurrency: 3,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	for i := range 5 {
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
	// The single-item batch (size 1) uses handleBlock → Translate(), not ChatStructured().
	assert.Len(t, mock.ChatStructuredCalls, 2, "5 blocks / batch_size=2 → 2 full batches via ChatStructured")
	assert.Len(t, mock.TranslateCalls, 1, "1 remaining block uses single Translate")

	// All blocks should have targets.
	for _, p := range parts {
		block := p.Resource.(*model.Block)
		assert.True(t, block.HasTarget(model.LocaleFrench), "block %s should have target", block.ID)
	}
}

func TestAITranslateBatchPreservesNonBlockParts(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: batchJSON(map[int]string{1: "Bonjour", 2: "Monde"}),
			Model:   "test",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale:     model.LocaleEnglish,
		TargetLocale:     model.LocaleFrench,
		BatchSize:        10,
		BatchConcurrency: 2,
	})

	ctx := t.Context()
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
	mock := aiprovider.NewMockProvider()
	// With only 1 translatable block, it falls through to single Translate.
	// No ChatStructured call expected.

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale:     model.LocaleEnglish,
		TargetLocale:     model.LocaleFrench,
		BatchSize:        10,
		BatchConcurrency: 1,
	})

	ctx := t.Context()
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

	// Only 1 translatable block → single Translate call (not batch ChatStructured).
	assert.Len(t, mock.TranslateCalls, 1)
	assert.Empty(t, mock.ChatStructuredCalls)
	assert.True(t, parts[0].Resource.(*model.Block).HasTarget(model.LocaleFrench))
	assert.False(t, parts[1].Resource.(*model.Block).HasTarget(model.LocaleFrench))
}

func TestAITranslateBatchStructuredSchema(t *testing.T) {
	// Verify that the schema passed to ChatStructured has the expected structure.
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		assert.Equal(t, "batch_translations", schema.Name)
		assert.True(t, schema.Strict)
		props, ok := schema.Schema["properties"].(map[string]any)
		require.True(t, ok)
		_, hasTrans := props["translations"]
		assert.True(t, hasTrans, "schema should have 'translations' property")
		return &aiprovider.ChatResponse{
			Content: batchJSON(map[int]string{1: "Hola", 2: "Mundo"}),
			Model:   "test",
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale:     model.LocaleEnglish,
		TargetLocale:     "es",
		BatchSize:        10,
		BatchConcurrency: 1,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)

	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	require.Len(t, parts, 2)
	assert.Len(t, mock.ChatStructuredCalls, 1)
}

func TestProviderMockDefaultBehavior(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	ctx := t.Context()
	resp, err := mock.Translate(ctx, aiprovider.TranslateRequest{
		Source:         "Hello",
		SourceLanguage: model.LocaleEnglish,
		TargetLocale:   model.LocaleFrench,
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Translation, "[fr]")
	assert.Contains(t, resp.Translation, "Hello")

	chatResp, err := mock.Chat(ctx, []aiprovider.Message{
		{Role: "user", Content: "Test message"},
	})
	require.NoError(t, err)
	assert.Contains(t, chatResp.Content, "Mock response")

	structResp, err := mock.ChatStructured(ctx, []aiprovider.Message{
		{Role: "user", Content: "Test"},
	}, aiprovider.JSONSchema{
		Name: "test",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{"type": "array"},
			},
		},
	})
	require.NoError(t, err)
	// Default mock returns empty arrays for array properties.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(structResp.Content), &parsed))
	assert.NotNil(t, parsed)
}
