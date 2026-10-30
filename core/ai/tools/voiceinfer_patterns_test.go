package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A drafted voice is a proposal, so every pattern in it reports until a person
// marks it `fails: true`. The schema asks the model for nothing that decides it.
func TestInferredPatternsReport(t *testing.T) {
	var res inferLLMResult
	require.NoError(t, json.Unmarshal([]byte(`{"style":{"prohibited_patterns":[
		{"regex":"\\bsynergy\\b","description":"Corporate filler"},
		{"regex":"!{2,}","description":"Stacked exclamation marks"},
		{"regex":"  ","description":"Ignored: blank regex is dropped"}
	]}}`), &res))

	draft, _ := mapInferResult(res, InferOptions{ProfileName: "Draft"})
	require.Len(t, draft.Style.ProhibitedPatterns, 2)
	for _, pat := range draft.Style.ProhibitedPatterns {
		assert.False(t, pat.Failing(), "pattern %q is drafted to report", pat.Regex)
		assert.False(t, pat.Fails, "pattern %q is drafted without fails", pat.Regex)
	}
}

func TestInferSchemaAsksForNoSeverity(t *testing.T) {
	raw, err := json.Marshal(voiceInferLLMSchema())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "severity")
}
