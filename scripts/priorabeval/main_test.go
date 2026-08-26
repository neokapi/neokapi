package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests call no model. The eval spends model calls, so its results are
// committed rather than regenerated; what is worth testing on every build is
// the harness around them, which is where an eval usually goes wrong.

// TestTheJudgeCannotShareAFamilyWithTheModel: a model grading its own family
// prefers itself, measurably, so this is refused rather than warned about.
func TestTheJudgeCannotShareAFamilyWithTheModel(t *testing.T) {
	t.Parallel()

	_, err := Run(t.Context(), RunOpts{
		Model: target{provider: "claude-code", model: "sonnet"},
		Judge: target{provider: "anthropic", model: "claude-haiku-4-5"},
	})
	require.Error(t, err, "same family must not run at all")
	assert.Contains(t, err.Error(), "same family")
}

func TestModelFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider, model, want string
	}{
		{"claude-code", "sonnet", "anthropic"},
		{"anthropic", "claude-opus-5", "anthropic"},
		{"ollama", "gemma4:e2b", "google"},
		{"ollama", "llama3.2:3b", "meta"},
		{"ollama", "qwen3:1.7b", "alibaba"},
		{"openai", "gpt-5", "openai"},
		// The model name decides, not the provider: one provider serves many
		// families, so keying off the provider would let a Gemma served by
		// Ollama judge a Gemma served by Google.
		{"ollama", "", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, modelFamily(target{provider: tt.provider, model: tt.model}),
			"%s:%s", tt.provider, tt.model)
	}
}

// TestTheTwoPromptsDifferOnlyByTheReference is what makes the A/B an A/B. If
// the prompts differed in any other way, the eval would be measuring that.
func TestTheTwoPromptsDifferOnlyByTheReference(t *testing.T) {
	t.Parallel()

	for _, c := range abCases {
		without, with, err := promptsFor(c)
		require.NoError(t, err)

		a, b := renderMessages(without), renderMessages(with)
		if c.withheld {
			assert.Equal(t, a, b, "%s: a withheld case is the control, so both prompts must be identical", c.name)
			continue
		}
		assert.NotEqual(t, a, b, "%s: the reference must actually reach the model", c.name)
		assert.Contains(t, b, c.priorTarget, "%s: and it must be the approved translation", c.name)
		assert.NotContains(t, a, c.priorTarget, "%s: which the other arm must not see", c.name)
	}
}

// TestEveryCaseNamesADistinctWording: a case whose approved wording and drift
// overlap would score both at once and mean nothing.
func TestEveryCaseNamesADistinctWording(t *testing.T) {
	t.Parallel()

	for _, c := range abCases {
		require.NotEmpty(t, c.keep, "%s: nothing to check for", c.name)
		require.NotEmpty(t, c.drift, "%s: no alternative to check against", c.name)

		assert.True(t, containsAny(c.priorTarget, c.keep),
			"%s: the approved wording must appear in the approved translation", c.name)
		assert.False(t, containsAny(c.priorTarget, c.drift),
			"%s: and the drift must not", c.name)
	}
}

// TestContainsAnyMatchesWholeWords guards the bug this eval shipped with: the
// first version used strings.Contains, "handlekurven" contains "kurven", and
// every drift in the corpus scored as the approved wording surviving. The eval
// reported that the reference changed nothing.
func TestContainsAnyMatchesWholeWords(t *testing.T) {
	t.Parallel()

	assert.False(t, containsAny("Legg dette i handlekurven", []string{"kurven"}),
		"the drift is a different word, not a match")
	assert.True(t, containsAny("Legg denne i kurven", []string{"kurven"}))
	assert.True(t, containsAny("Åpne innstillingene for arbeidsområdet", []string{"arbeidsområde", "arbeidsområdet"}),
		"an inflected form is the same choice of word")
}

// TestCommittedDataMatchesTheCorpus: the committed results are from a real run
// and cannot be regenerated in a test, so what is checked is that they still
// describe this corpus. A case renamed or added without a re-run would leave the
// page showing results for a corpus that no longer exists.
func TestCommittedDataMatchesTheCorpus(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", DefaultOut))
	require.NoError(t, err, "the committed data is missing — run: go run ./scripts/priorabeval")

	var committed Report
	require.NoError(t, json.Unmarshal(raw, &committed))

	require.Len(t, committed.Cases, len(abCases),
		"the corpus changed since the last run — re-run: go run ./scripts/priorabeval -repeat 3")
	for i, c := range abCases {
		assert.Equal(t, c.name, committed.Cases[i].Name)
		assert.Equal(t, c.source, committed.Cases[i].Source)
	}

	assert.Positive(t, committed.Samples)
	assert.False(t, committed.JudgeValidated,
		"judge agreement with a person has not been measured, so the page must not present it as a finding")
}

// renderMessages flattens a prompt to comparable text.
func renderMessages(msgs []aiprovider.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Text())
		sb.WriteString("\n")
	}
	return sb.String()
}
