package tools_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coretool "github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkBlockRoute runs the LLM check tool over one translated block with a scripted
// structured response and returns the resulting block.
func checkBlockRoute(t *testing.T, fn func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error)) *model.Block {
	t.Helper()
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = fn

	tl := tools.NewAICheckTool(mock, tools.AICheckConfig{
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

func checkScripted(content string) func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	return func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{Content: content, Model: "test"}, nil
	}
}

// TestAICheck_SeverityMapping pins the LLM-severity → core/check mapping:
// error→major, warning→minor, info and anything unknown → neutral.
func TestAICheck_SeverityMapping(t *testing.T) {
	cases := []struct {
		llm  string
		want check.Severity
	}{
		{"error", check.SeverityMajor},
		{"warning", check.SeverityMinor},
		{"info", check.SeverityNeutral},
		{"bogus", check.SeverityNeutral},
	}
	for _, tc := range cases {
		t.Run(tc.llm, func(t *testing.T) {
			block := checkBlockRoute(t, checkScripted(fmt.Sprintf(
				`{"issues":[{"type":"accuracy","severity":%q,"description":"issue","suggestion":"fix"}]}`, tc.llm,
			)))
			findings := check.Findings(coretool.NewBlockView(block))
			require.Len(t, findings, 1)
			assert.Equal(t, tc.want, findings[0].Severity)
		})
	}
}

// TestAICheck_MalformedResponseDegrades: a non-JSON model response must not
// fail the pipeline — it degrades to a single neutral "parse-error" finding
// that carries the raw content for inspection.
func TestAICheck_MalformedResponseDegrades(t *testing.T) {
	raw := "I could not analyze this translation, sorry."
	block := checkBlockRoute(t, checkScripted(raw))

	findings := check.Findings(coretool.NewBlockView(block))
	require.Len(t, findings, 1)
	assert.Equal(t, "parse-error", findings[0].Category)
	assert.Equal(t, check.SeverityNeutral, findings[0].Severity, "a parse failure carries no penalty")
	assert.Equal(t, raw, findings[0].Message)
	assert.Equal(t, "mock", block.Properties["qa-provider"])
}

// TestAICheck_NoIssues: an empty issues array yields no findings but still
// records the provider and checks metadata.
func TestAICheck_NoIssues(t *testing.T) {
	block := checkBlockRoute(t, checkScripted(`{"issues":[]}`))

	findings := check.Findings(coretool.NewBlockView(block))
	assert.Empty(t, findings)
	assert.Equal(t, "mock", block.Properties["qa-provider"])
	assert.Equal(t, "terminology,fluency,accuracy", block.Properties["qa-checks"],
		"default check set is recorded")
}

// TestAICheck_ErrorPropagation: a provider failure surfaces as a tool
// error wrapped with the tool name.
func TestAICheck_ErrorPropagation(t *testing.T) {
	provErr := errors.New("model unavailable")
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return nil, provErr
	}

	tl := tools.NewAICheckTool(mock, tools.AICheckConfig{
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
	assert.Contains(t, err.Error(), "qa:")
}
