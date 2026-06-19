package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProfileResolver implements brand.ProfileResolver for testing.
type mockProfileResolver struct {
	profile *brand.VoiceProfile
}

func (m *mockProfileResolver) ResolveProfile(_ context.Context, _ brand.ResolveContext) (*brand.VoiceProfile, error) {
	return m.profile, nil
}

func TestBrandVoiceCheckToolFindings(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[{"dimension":"tone","severity":"major","message":"Too informal for brand guidelines","suggestion":"Use a more professional tone"}]}`,
			Model:   "test",
		}, nil
	}

	profile := &brand.VoiceProfile{
		ID: "test-voice",
		Tone: brand.ToneProfile{
			Personality: []string{"professional", "friendly"},
			Formality:   "formal",
		},
	}

	tool := tools.NewBrandVoiceCheckTool(mock, profile)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hey dude, check out this awesome thing!")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// Check findings.
	findingsStr := resultBlock.Properties["brand-voice-findings"]
	require.NotEmpty(t, findingsStr)

	var findings []brand.BrandVoiceFinding
	err = json.Unmarshal([]byte(findingsStr), &findings)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, string(brand.DimensionTone), findings[0].Category)
	assert.Equal(t, brand.SeverityMajor, findings[0].Severity)

	// Check score.
	scoreStr := resultBlock.Properties["brand-voice-score"]
	require.NotEmpty(t, scoreStr)

	var score brand.BrandComplianceScore
	err = json.Unmarshal([]byte(scoreStr), &score)
	require.NoError(t, err)
	assert.Equal(t, "test-voice", score.ProfileID)
	assert.Less(t, score.Overall, 100)
}

func TestBrandVoiceCheckToolPromptConstruction(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	var capturedPrompt string
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		for _, m := range messages {
			if m.Role == "user" {
				capturedPrompt = m.Text()
			}
		}
		return &aiprovider.ChatResponse{
			Content: `{"findings":[]}`,
			Model:   "test",
		}, nil
	}

	profile := &brand.VoiceProfile{
		ID: "prompt-test",
		Tone: brand.ToneProfile{
			Personality: []string{"warm", "knowledgeable"},
			Formality:   "neutral",
			Emotion:     "warm",
		},
		Style: brand.StyleRules{
			ActiveVoice:    true,
			SentenceLength: "short",
			PersonPOV:      "second",
			Contractions:   "sometimes",
		},
		Examples: []brand.VoiceExample{
			{
				Before:      "The system will process your request.",
				After:       "We'll process your request right away.",
				Explanation: "Use active voice and contractions for warmth",
			},
		},
	}

	tool := tools.NewBrandVoiceCheckTool(mock, profile)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Welcome to our platform")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	<-out

	// Verify prompt includes profile elements.
	assert.Contains(t, capturedPrompt, "warm, knowledgeable")
	assert.Contains(t, capturedPrompt, "neutral")
	assert.Contains(t, capturedPrompt, "active voice")
	assert.Contains(t, capturedPrompt, "second")
	assert.Contains(t, capturedPrompt, "The system will process your request.")
	assert.Contains(t, capturedPrompt, "We'll process your request right away.")
	assert.Contains(t, capturedPrompt, "Welcome to our platform")

	// Verify schema.
	require.Len(t, mock.ChatStructuredCalls, 1)
	assert.Equal(t, "brand_voice_findings", mock.ChatStructuredCalls[0].Schema.Name)
	assert.True(t, mock.ChatStructuredCalls[0].Schema.Strict)
}

func TestBrandVoiceCheckToolScoreCalculation(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[
				{"dimension":"tone","severity":"minor","message":"slightly informal","suggestion":"adjust tone"},
				{"dimension":"style","severity":"major","message":"passive voice","suggestion":"use active voice"},
				{"dimension":"clarity","severity":"minor","message":"could be clearer","suggestion":"simplify"}
			]}`,
			Model: "test",
		}, nil
	}

	profile := &brand.VoiceProfile{ID: "score-test"}
	tool := tools.NewBrandVoiceCheckTool(mock, profile)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "The data was processed by the system in an efficient manner")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	var score brand.BrandComplianceScore
	err = json.Unmarshal([]byte(resultBlock.Properties["brand-voice-score"]), &score)
	require.NoError(t, err)

	// minor=1, major=5, minor=1 → total penalty = 7 → overall = 93
	assert.Equal(t, 93, score.Overall)
	assert.Equal(t, "score-test", score.ProfileID)
	assert.Len(t, score.Findings, 3)
}

func TestBrandVoiceCheckToolSkipsEmptyText(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	profile := &brand.VoiceProfile{ID: "skip-test"}
	tool := tools.NewBrandVoiceCheckTool(mock, profile)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "  ")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	<-out
	assert.Empty(t, mock.ChatStructuredCalls)
}

func TestBrandVoiceCheckToolAddsAnnotation(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[{"dimension":"brand_compliance","severity":"minor","message":"missing trademark","suggestion":"add TM symbol"}]}`,
			Model:   "test",
		}, nil
	}

	profile := &brand.VoiceProfile{ID: "ann-test"}
	tool := tools.NewBrandVoiceCheckTool(mock, profile)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Try Bowrain today")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	ann, ok := resultBlock.Anno("brand-voice")
	require.True(t, ok)

	bva := ann.(*brand.BrandVoiceAnnotation)
	assert.Equal(t, "ann-test", bva.ProfileID)
	assert.Len(t, bva.Findings, 1)
}

func TestBrandVoiceCheckToolWithResolver(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[{"dimension":"tone","severity":"minor","message":"slightly informal","suggestion":"adjust"}]}`,
			Model:   "test",
		}, nil
	}

	profile := &brand.VoiceProfile{
		ID:   "resolved-profile",
		Name: "Resolved",
		Tone: brand.ToneProfile{Formality: "formal"},
	}

	resolver := &mockProfileResolver{profile: profile}
	rc := brand.ResolveContext{ExplicitProfileID: "resolved-profile"}

	tool := tools.NewBrandVoiceCheckToolWithResolver(mock, resolver, rc)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hey check this out")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// Verify it used the resolved profile.
	ann, ok := resultBlock.Anno("brand-voice")
	require.True(t, ok)
	bva := ann.(*brand.BrandVoiceAnnotation)
	assert.Equal(t, "resolved-profile", bva.ProfileID)
}

func TestBrandVoiceCheckToolWithResolverNilProfile(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[]}`,
			Model:   "test",
		}, nil
	}

	// Resolver returns nil profile — tool still processes but with empty context.
	resolver := &mockProfileResolver{profile: nil}
	rc := brand.ResolveContext{}

	tool := tools.NewBrandVoiceCheckToolWithResolver(mock, resolver, rc)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Hello world")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// Tool still processes — profile ID will be empty.
	var score brand.BrandComplianceScore
	err = json.Unmarshal([]byte(resultBlock.Properties["brand-voice-score"]), &score)
	require.NoError(t, err)
	assert.Empty(t, score.ProfileID)
	assert.Equal(t, 100, score.Overall)
}

func TestBrandVoiceCheckToolNoFindings(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[]}`,
			Model:   "test",
		}, nil
	}

	profile := &brand.VoiceProfile{ID: "clean-test"}
	tool := tools.NewBrandVoiceCheckTool(mock, profile)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Welcome to our platform")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)

	// Score should be present and perfect.
	var score brand.BrandComplianceScore
	err = json.Unmarshal([]byte(resultBlock.Properties["brand-voice-score"]), &score)
	require.NoError(t, err)
	assert.Equal(t, 100, score.Overall)

	// No findings property.
	assert.Empty(t, resultBlock.Properties["brand-voice-findings"])
}
