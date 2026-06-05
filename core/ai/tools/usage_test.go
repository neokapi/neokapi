package tools_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateToolAccumulatesUsage(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	callCount := 0
	mock.TranslateFunc = func(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		callCount++
		return &aiprovider.TranslateResponse{
			Translation: "Translated",
			Confidence:  0.9,
			Model:       "test",
			Usage:       aiprovider.TokenUsage{InputTokens: 100, OutputTokens: 50},
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BatchSize:    1, // individual calls, not batched
	})

	// Process two blocks.
	ctx := t.Context()
	in := make(chan *model.Part, 2)
	out := make(chan *model.Part, 2)

	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)
	for range out {
	}

	assert.Equal(t, 2, callCount)
	usage := tool.TotalUsage()
	assert.Equal(t, 200, usage.InputTokens)
	assert.Equal(t, 100, usage.OutputTokens)
	assert.Equal(t, 300, usage.TotalTokens())
}

func TestTranslateToolResetUsage(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	// Default mock returns Usage{InputTokens: 10, OutputTokens: 20}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	<-out

	assert.Greater(t, tool.TotalUsage().TotalTokens(), 0)

	tool.ResetUsage()
	assert.Equal(t, 0, tool.TotalUsage().TotalTokens())
}

func TestTranslateToolBatchAccumulatesUsage(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: batchJSON(map[int]string{1: "Bonjour", 2: "Monde", 3: "Salut"}),
			Model:   "test",
			Usage:   aiprovider.TokenUsage{InputTokens: 500, OutputTokens: 200},
		}, nil
	}

	tool := tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale:     model.LocaleEnglish,
		TargetLocale:     model.LocaleFrench,
		BatchSize:        10,
		BatchConcurrency: 2,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 3)
	out := make(chan *model.Part, 3)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu3", "Hi")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	close(out)
	for range out {
	}

	usage := tool.TotalUsage()
	assert.Equal(t, 500, usage.InputTokens)
	assert.Equal(t, 200, usage.OutputTokens)
}

func TestQAToolAccumulatesUsage(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"issues":[]}`,
			Model:   "test",
			Usage:   aiprovider.TokenUsage{InputTokens: 200, OutputTokens: 30},
		}, nil
	}

	tool := tools.NewAIQACheckTool(mock, tools.AIQAConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
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
	<-out

	usage := tool.TotalUsage()
	assert.Equal(t, 200, usage.InputTokens)
	assert.Equal(t, 30, usage.OutputTokens)
}

func TestReviewToolAccumulatesUsage(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = func(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: "Score: 9\nAssessment: Good",
			Model:   "test",
			Usage:   aiprovider.TokenUsage{InputTokens: 150, OutputTokens: 25},
		}, nil
	}

	tool := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
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
	<-out

	usage := tool.TotalUsage()
	assert.Equal(t, 150, usage.InputTokens)
	assert.Equal(t, 25, usage.OutputTokens)
}

func TestTerminologyToolAccumulatesUsage(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"terms":[]}`,
			Model:   "test",
			Usage:   aiprovider.TokenUsage{InputTokens: 80, OutputTokens: 15},
		}, nil
	}

	tool := tools.NewAITerminologyTool(mock, tools.AITerminologyConfig{
		Locale: model.LocaleEnglish,
	})

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "The API endpoint")}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	<-out

	usage := tool.TotalUsage()
	assert.Equal(t, 80, usage.InputTokens)
	assert.Equal(t, 15, usage.OutputTokens)
}

func TestUsageReporterInterface(t *testing.T) {
	mock := aiprovider.NewMockProvider()

	// Verify all tools implement UsageReporter.
	var reporters []tools.UsageReporter
	reporters = append(reporters, tools.NewAITranslateTool(mock, tools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	reporters = append(reporters, tools.NewAIQACheckTool(mock, tools.AIQAConfig{
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	reporters = append(reporters, tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	reporters = append(reporters, tools.NewAITerminologyTool(mock, tools.AITerminologyConfig{
		Locale: model.LocaleEnglish,
	}))

	for _, r := range reporters {
		// All start at zero.
		assert.Equal(t, 0, r.TotalUsage().TotalTokens())
		// Reset on zero is harmless.
		r.ResetUsage()
		assert.Equal(t, 0, r.TotalUsage().TotalTokens())
	}
}

func TestSkippedBlocksProduceZeroUsage(t *testing.T) {
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
	<-out

	assert.Equal(t, 0, tool.TotalUsage().TotalTokens())
	assert.Empty(t, mock.TranslateCalls)
}
