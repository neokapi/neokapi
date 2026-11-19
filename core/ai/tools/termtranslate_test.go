package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// TestProposeTermTranslations sends each concept once with its examples and
// the terms turned down, and keeps only answers that name a concept asked
// about, carry a term, and repeat nothing turned down.
func TestProposeTermTranslations(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	mock.ChatStructuredFunc = func(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
		return &aiprovider.ChatResponse{
			Content: `{"terms":[
				{"id":"c1","term":" arbeidsområde ","note":"the usual word"},
				{"id":"c1","term":"arbeidsflate","note":"a second answer for c1"},
				{"id":"c2","term":"Flyt","note":""},
				{"id":"c3","term":"","note":"cannot tell"},
				{"id":"zz","term":"oppfunnet","note":""}
			]}`,
			Usage: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 5},
		}, nil
	}
	got, usage, err := ProposeTermTranslations(t.Context(), mock, "en", "nb", []TermTranslateRequest{
		{ID: "c1", Term: "workspace", Definition: "a shared place", Examples: []string{"a", "b", "c", "d"}},
		{ID: "c2", Term: "flow", TurnedDown: []string{"flyt"}},
		{ID: "c3", Term: "harness"},
		{ID: "c4", Term: "  "},
	})
	require.NoError(t, err)
	assert.Equal(t, []TermTranslation{{ID: "c1", Term: "arbeidsområde", Note: "the usual word"}}, got)
	assert.Equal(t, 15, usage.TotalTokens())

	require.Len(t, mock.ChatStructuredCalls, 1)
	var b strings.Builder
	for _, m := range mock.ChatStructuredCalls[0].Messages {
		for _, p := range m.Parts {
			b.WriteString(p.Text)
		}
	}
	sent := b.String()
	assert.Contains(t, sent, "Norwegian")
	assert.Contains(t, sent, `"turned_down":["flyt"]`)
	assert.NotContains(t, sent, `"d"`, "examples are capped")
	assert.NotContains(t, sent, `"c4"`, "a concept with no source term is not sent")
}

// TestProposeTermTranslationsNothingToAsk makes no call for an empty list.
func TestProposeTermTranslationsNothingToAsk(t *testing.T) {
	mock := aiprovider.NewMockProvider()
	got, _, err := ProposeTermTranslations(t.Context(), mock, "en", "nb", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, mock.ChatStructuredCalls)
}
