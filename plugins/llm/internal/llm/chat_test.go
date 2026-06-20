package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapRole(t *testing.T) {
	assert.Equal(t, roleModel, mapRole("assistant"))
	assert.Equal(t, roleModel, mapRole("model"))
	assert.Equal(t, roleUser, mapRole("user"))
	assert.Equal(t, roleUser, mapRole("something"))
}

func TestRenderPromptBasic(t *testing.T) {
	got := renderPrompt([]Message{
		{Role: "user", Text: "Hello"},
	})
	assert.True(t, strings.HasPrefix(got, tokBOS), "starts with bos")
	assert.Contains(t, got, tokStartTurn+"user\nHello"+tokEndTurn)
	assert.True(t, strings.HasSuffix(got, tokStartTurn+"model\n"), "primes a model turn")
}

func TestRenderPromptFoldsSystemIntoFirstUserTurn(t *testing.T) {
	got := renderPrompt([]Message{
		{Role: "system", Text: "Be terse."},
		{Role: "user", Text: "Hi"},
		{Role: "assistant", Text: "Hey"},
		{Role: "user", Text: "Again"},
	})
	// System is merged into the FIRST user turn only.
	assert.Contains(t, got, tokStartTurn+"user\nBe terse.\n\nHi"+tokEndTurn)
	assert.Contains(t, got, tokStartTurn+"model\nHey"+tokEndTurn)
	// The later user turn is not prefixed with the system text again.
	assert.Contains(t, got, tokStartTurn+"user\nAgain"+tokEndTurn)
	assert.Equal(t, 1, strings.Count(got, "Be terse."))
}

func TestRenderPromptMultipleSystemMessagesConcatenate(t *testing.T) {
	got := renderPrompt([]Message{
		{Role: "system", Text: "One."},
		{Role: "system", Text: "Two."},
		{Role: "user", Text: "Q"},
	})
	assert.Contains(t, got, "One.\n\nTwo.\n\nQ")
}

func TestCleanOutput(t *testing.T) {
	assert.Equal(t, "Hello", cleanOutput("  Hello"+tokEndTurn))
	assert.Equal(t, "Hello", cleanOutput("Hello"))
	assert.Equal(t, "Bonjour", cleanOutput(tokStartTurn+"model\nBonjour"+tokEndTurn))
}
