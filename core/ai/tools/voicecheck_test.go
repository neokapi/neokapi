package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProfileResolver implements coreprofile.ProfileResolver for testing.
type mockProfileResolver struct {
	profile *coreprofile.VoiceProfile
}

func (m *mockProfileResolver) ResolveProfile(_ context.Context, _ coreprofile.ResolveContext) (*coreprofile.VoiceProfile, error) {
	return m.profile, nil
}

func TestVoiceCheckToolFindings(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[{"dimension":"tone","severity":"major","message":"Too informal for brand guidelines","suggestion":"Use a more professional tone"}]}`,
			Model:   "test",
		}, nil
	}

	profile := &coreprofile.VoiceProfile{
		ID: "test-voice",
		Tone: coreprofile.ToneProfile{
			Personality: []string{"professional", "friendly"},
			Formality:   "formal",
		},
	}

	tool := tools.NewVoiceCheckTool(mock, profile)

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
	findingsStr := resultBlock.Properties["voice-findings"]
	require.NotEmpty(t, findingsStr)

	var findings []coreprofile.VoiceFinding
	err = json.Unmarshal([]byte(findingsStr), &findings)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, string(coreprofile.DimensionTone), findings[0].Category)
	assert.Equal(t, coreprofile.SeverityMajor, findings[0].Severity)

	// Check score.
	scoreStr := resultBlock.Properties["voice-score"]
	require.NotEmpty(t, scoreStr)

	var score coreprofile.ComplianceScore
	err = json.Unmarshal([]byte(scoreStr), &score)
	require.NoError(t, err)
	assert.Equal(t, "test-voice", score.ProfileID)
	assert.Less(t, score.Overall, 100)
}

func TestVoiceCheckToolPromptConstruction(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	var capturedSystem, capturedUser string
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		for _, m := range messages {
			switch m.Role {
			case aiprovider.RoleSystem:
				capturedSystem = m.Text()
			case aiprovider.RoleUser:
				capturedUser = m.Text()
			}
		}
		return &aiprovider.ChatResponse{
			Content: `{"findings":[]}`,
			Model:   "test",
		}, nil
	}

	profile := &coreprofile.VoiceProfile{
		ID: "prompt-test",
		Tone: coreprofile.ToneProfile{
			Personality: []string{"warm", "knowledgeable"},
			Formality:   "neutral",
			Emotion:     "warm",
		},
		Style: coreprofile.StyleRules{
			ActiveVoice:    true,
			SentenceLength: "short",
			PersonPOV:      "second",
			Contractions:   "sometimes",
		},
		Examples: []coreprofile.VoiceExample{
			{
				Before:      "The system will process your request.",
				After:       "We'll process your request right away.",
				Explanation: "Use active voice and contractions for warmth",
			},
		},
	}

	tool := tools.NewVoiceCheckTool(mock, profile)

	ctx := t.Context()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("tu1", "Welcome to our platform")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tool.Process(ctx, in, out)
	require.NoError(t, err)
	<-out

	// The voice guide is instruction, so it belongs in the system turn...
	assert.Contains(t, capturedSystem, "warm, knowledgeable")
	assert.Contains(t, capturedSystem, "neutral")
	assert.Contains(t, capturedSystem, "active voice")
	assert.Contains(t, capturedSystem, "second")
	assert.Contains(t, capturedSystem, "The system will process your request.")
	assert.Contains(t, capturedSystem, "We'll process your request right away.")

	// ...and the text under review is content, so the user turn holds it and
	// nothing else.
	assert.Equal(t, "Welcome to our platform", capturedUser)

	// Verify schema.
	require.Len(t, mock.ChatStructuredCalls, 1)
	assert.Equal(t, "brand_voice_findings", mock.ChatStructuredCalls[0].Schema.Name)
	assert.True(t, mock.ChatStructuredCalls[0].Schema.Strict)
}

func TestVoiceCheckToolScoreCalculation(t *testing.T) {
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

	profile := &coreprofile.VoiceProfile{ID: "score-test"}
	tool := tools.NewVoiceCheckTool(mock, profile)

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

	var score coreprofile.ComplianceScore
	err = json.Unmarshal([]byte(resultBlock.Properties["voice-score"]), &score)
	require.NoError(t, err)

	// minor=1, major=5, minor=1 → total penalty = 7 → overall = 93
	assert.Equal(t, 93, score.Overall)
	assert.Equal(t, "score-test", score.ProfileID)
	assert.Len(t, score.Findings, 3)
}

func TestVoiceCheckToolSkipsEmptyText(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	profile := &coreprofile.VoiceProfile{ID: "skip-test"}
	tool := tools.NewVoiceCheckTool(mock, profile)

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

func TestVoiceCheckToolAddsAnnotation(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[{"dimension":"brand_compliance","severity":"minor","message":"missing trademark","suggestion":"add content memory symbol"}]}`,
			Model:   "test",
		}, nil
	}

	profile := &coreprofile.VoiceProfile{ID: "ann-test"}
	tool := tools.NewVoiceCheckTool(mock, profile)

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

	ann, ok := resultBlock.Anno("voice")
	require.True(t, ok)

	bva := ann.(*coreprofile.VoiceAnnotation)
	assert.Equal(t, "ann-test", bva.ProfileID)
	assert.Len(t, bva.Findings, 1)
}

func TestVoiceCheckToolWithResolver(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[{"dimension":"tone","severity":"minor","message":"slightly informal","suggestion":"adjust"}]}`,
			Model:   "test",
		}, nil
	}

	profile := &coreprofile.VoiceProfile{
		ID:   "resolved-profile",
		Name: "Resolved",
		Tone: coreprofile.ToneProfile{Formality: "formal"},
	}

	resolver := &mockProfileResolver{profile: profile}
	rc := coreprofile.ResolveContext{ExplicitProfileID: "resolved-profile"}

	tool := tools.NewVoiceCheckToolWithResolver(mock, resolver, rc)

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
	ann, ok := resultBlock.Anno("voice")
	require.True(t, ok)
	bva := ann.(*coreprofile.VoiceAnnotation)
	assert.Equal(t, "resolved-profile", bva.ProfileID)
}

func TestVoiceCheckToolWithResolverNilProfile(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[]}`,
			Model:   "test",
		}, nil
	}

	// Resolver returns nil profile — tool still processes but with empty context.
	resolver := &mockProfileResolver{profile: nil}
	rc := coreprofile.ResolveContext{}

	tool := tools.NewVoiceCheckToolWithResolver(mock, resolver, rc)

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
	var score coreprofile.ComplianceScore
	err = json.Unmarshal([]byte(resultBlock.Properties["voice-score"]), &score)
	require.NoError(t, err)
	assert.Empty(t, score.ProfileID)
	assert.Equal(t, 100, score.Overall)
}

func TestVoiceCheckToolNoFindings(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"findings":[]}`,
			Model:   "test",
		}, nil
	}

	profile := &coreprofile.VoiceProfile{ID: "clean-test"}
	tool := tools.NewVoiceCheckTool(mock, profile)

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
	var score coreprofile.ComplianceScore
	err = json.Unmarshal([]byte(resultBlock.Properties["voice-score"]), &score)
	require.NoError(t, err)
	assert.Equal(t, 100, score.Overall)

	// No findings property.
	assert.Empty(t, resultBlock.Properties["voice-findings"])
}
