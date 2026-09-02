package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviewBlock runs the review tool over one translated block with a scripted
// chat response and returns the resulting block.
func reviewBlock(t *testing.T, chat func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error)) *model.Block {
	t.Helper()
	mock := aiprovider.NewMockProvider()
	mock.ChatFunc = chat

	tl := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	require.NoError(t, tl.Process(t.Context(), in, out))
	return (<-out).Resource.(*model.Block)
}

func scripted(content string) func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	return func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: content, Model: "test"}, nil
	}
}

// TestAIReviewTool_StructuredContract: a model that honors the JSON contract
// yields a canonical {score, findings} payload in the "review" property and
// the integer score in "review-score".
func TestAIReviewTool_StructuredContract(t *testing.T) {
	block := reviewBlock(t, scripted(
		`{"score": 87, "findings": [{"severity": "minor", "message": "awkward phrasing", "suggestion": "Bonjour, le monde"}]}`,
	))

	var result tools.ReviewResult
	require.NoError(t, json.Unmarshal([]byte(block.Properties["review"]), &result),
		"the review property must hold the canonical JSON contract")
	assert.Equal(t, 87, result.Score)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "minor", result.Findings[0].Severity)
	assert.Equal(t, "awkward phrasing", result.Findings[0].Message)
	assert.Equal(t, "Bonjour, le monde", result.Findings[0].Suggestion)

	assert.Equal(t, "87", block.Properties["review-score"])
	assert.Equal(t, "mock", block.Properties["review-provider"])
}

// TestAIReviewTool_TransportNoiseTolerated: code fences and prose around the
// JSON value still parse (local models pad structured output routinely).
func TestAIReviewTool_TransportNoiseTolerated(t *testing.T) {
	block := reviewBlock(t, scripted(
		"Here is my review:\n```json\n{\"score\": 95, \"findings\": []}\n```\nLet me know if you need more detail.",
	))

	var result tools.ReviewResult
	require.NoError(t, json.Unmarshal([]byte(block.Properties["review"]), &result))
	assert.Equal(t, 95, result.Score)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "95", block.Properties["review-score"])
}

// TestAIReviewTool_ProseFallback: a model answering in free text (no JSON)
// keeps working — the raw prose lands in "review" unchanged and no score
// property is set. This is the backward-lenient path.
func TestAIReviewTool_ProseFallback(t *testing.T) {
	prose := "Score: 9\nAssessment: Accurate translation.\nSuggestion: none"
	block := reviewBlock(t, scripted(prose))

	assert.Equal(t, prose, block.Properties["review"])
	_, hasScore := block.Properties["review-score"]
	assert.False(t, hasScore, "prose fallback must not fabricate a score")
	assert.Equal(t, "mock", block.Properties["review-provider"])
}

// TestAIReviewTool_MalformedJSONFallsBack: a broken JSON payload (truncated
// object) must not fail the pipeline; it degrades to the prose fallback.
func TestAIReviewTool_MalformedJSONFallsBack(t *testing.T) {
	malformed := `{"score": 87, "findings": [`
	block := reviewBlock(t, scripted(malformed))

	assert.Equal(t, malformed, block.Properties["review"])
	_, hasScore := block.Properties["review-score"]
	assert.False(t, hasScore)
}

// TestAIReviewTool_ErrorPropagation: a provider failure surfaces as a tool
// error, wrapped with the tool name.
func TestAIReviewTool_ErrorPropagation(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	provErr := errors.New("rate limited")
	mock.ChatFunc = func(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		return nil, provErr
	}

	tl := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tl.Process(t.Context(), in, out)
	require.Error(t, err)
	require.ErrorIs(t, err, provErr)
	assert.Contains(t, err.Error(), "review:")
}

// TestParseReviewResult pins the lenient parse behavior of the contract:
// clamping, severity normalization, finding filtering, and the error that
// triggers the prose fallback.
func TestParseReviewResult(t *testing.T) {
	t.Run("clamps score into 0-100", func(t *testing.T) {
		over, err := tools.ParseReviewResult(`{"score": 150, "findings": []}`)
		require.NoError(t, err)
		assert.Equal(t, 100, over.Score)

		under, err := tools.ParseReviewResult(`{"score": -3, "findings": []}`)
		require.NoError(t, err)
		assert.Equal(t, 0, under.Score)
	})

	t.Run("normalizes severities", func(t *testing.T) {
		res, err := tools.ParseReviewResult(
			`{"score": 50, "findings": [
				{"severity": "CRITICAL", "message": "wrong meaning"},
				{"severity": "blocker", "message": "unknown level"},
				{"message": "no level at all"}
			]}`,
		)
		require.NoError(t, err)
		require.Len(t, res.Findings, 3)
		assert.Equal(t, "critical", res.Findings[0].Severity)
		assert.Equal(t, "info", res.Findings[1].Severity, "unknown severity normalizes to info")
		assert.Equal(t, "info", res.Findings[2].Severity, "missing severity normalizes to info")
	})

	t.Run("drops findings without a message", func(t *testing.T) {
		res, err := tools.ParseReviewResult(
			`{"score": 70, "findings": [{"severity": "major", "message": "  "}, {"severity": "minor", "message": "real"}]}`,
		)
		require.NoError(t, err)
		require.Len(t, res.Findings, 1)
		assert.Equal(t, "real", res.Findings[0].Message)
	})

	t.Run("fractional scores truncate", func(t *testing.T) {
		res, err := tools.ParseReviewResult(`{"score": 87.9, "findings": []}`)
		require.NoError(t, err)
		assert.Equal(t, 87, res.Score)
	})

	t.Run("no JSON at all errors", func(t *testing.T) {
		_, err := tools.ParseReviewResult("The translation looks fine to me.")
		require.Error(t, err)
	})
}

// judgeSystemPrompt runs one block through a review tool and returns the system
// turn the provider was sent.
func judgeSystemPrompt(t *testing.T, tl tool.Tool, mock *aiprovider.MockProvider, sent *[]aiprovider.Message) string {
	t.Helper()
	mock.ChatFunc = func(_ context.Context, msgs []aiprovider.Message) (*aiprovider.ChatResponse, error) {
		*sent = msgs
		return &aiprovider.ChatResponse{Content: `{"score": 80, "findings": []}`, Model: "test"}, nil
	}
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	<-out
	require.NotEmpty(t, *sent)
	return (*sent)[0].Text()
}

// TestAIReviewTool_JudgesAgainstTheGovernance: the judge is told the voice
// profile and the terminology in force, so a target that reads well and uses a
// retired term is reported rather than scored clean. Asked only about accuracy
// and fluency, a reviewer cannot report the thing the checks beside it report.
func TestAIReviewTool_JudgesAgainstTheGovernance(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	var sent []aiprovider.Message

	tl := tools.NewAIReviewTool(mock, tools.AIReviewConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		Profile: &profile.VoiceProfile{
			ID:   "house",
			Name: "House Style",
			Tone: profile.ToneProfile{Formality: "formal"},
		},
		TermRules: []profile.TermRule{
			{Term: "content memory", Replacement: "mémoire de contenu"},
			{Term: "no replacement", Replacement: ""},
		},
	})

	system := judgeSystemPrompt(t, tl, mock, &sent)
	assert.Contains(t, system, "Voice profile the translation must comply with:")
	assert.Contains(t, system, "formal")
	assert.Contains(t, system, "Terminology the translation must use:")
	assert.Contains(t, system, "content memory → mémoire de contenu")
	assert.NotContains(t, system, "no replacement",
		"a rule with no replacement says nothing to reach past, and TermRuleMap drops it")
}

// TestAIReviewFromConfig_TakesTheProfileHandle: the voice profile is a live
// handle, so it reaches the tool through the config map rather than the JSON
// round trip the rest of the config takes. This is the path a run's config
// assembly hands the judge, and it is where a silently dropped handle would
// leave the judge scoring a bare pair again.
func TestAIReviewFromConfig_TakesTheProfileHandle(t *testing.T) {
	cfg := map[string]any{
		"sourceLocale": string(model.LocaleEnglish),
		"provider":     "anthropic",
		"apiKey":       "test-key",
		"profile":      &profile.VoiceProfile{ID: "house", Name: "House Style", Tone: profile.ToneProfile{Formality: "formal"}},
		"term_rules":   []profile.TermRule{{Term: "content memory", Replacement: "mémoire de contenu"}},
	}
	built, err := tools.NewAIReviewFromConfig(cfg, string(model.LocaleFrench))
	require.NoError(t, err)
	require.NotNil(t, built)
	assert.NotContains(t, cfg, "profile",
		"the handle is lifted out before the JSON round trip, which cannot carry it")
	assert.Contains(t, cfg, "term_rules",
		"the rules are plain data and travel the round trip")
}
