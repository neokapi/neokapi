package aiprovider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureAnthropic stands up a stub Messages API and returns the request body
// the provider actually sent.
func captureAnthropic(t *testing.T, call func(p *AnthropicProvider) error) anthropicRequest {
	t.Helper()

	var (
		got       anthropicRequest
		decodeErr error
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never fail the test from inside the handler goroutine: record the error
		// and assert on it from the test goroutine (testifylint go-require).
		body, err := io.ReadAll(r.Body)
		if err == nil {
			err = json.Unmarshal(body, &got)
		}
		if err != nil {
			decodeErr = err
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		resp := anthropicResponse{
			Model: "claude-sonnet-4-20250514",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "ok"},
				{Type: "tool_use", Name: "structured_output", Input: map[string]any{"ok": true}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropicProvider(Config{BaseURL: srv.URL, APIKey: "test-key"})
	require.NoError(t, call(p))
	require.NoError(t, decodeErr)
	return got
}

// The Messages API accepts only "user" and "assistant" in messages[] and takes
// system instructions in a top-level parameter. Sending a system-role message
// inside messages[] is a 400 — which is what entity-extract and media-refine
// used to do on the default provider.
func TestAnthropicLiftsSystemToTopLevel(t *testing.T) {
	t.Parallel()

	got := captureAnthropic(t, func(p *AnthropicProvider) error {
		_, err := p.Chat(t.Context(), []Message{
			TextMessage(RoleSystem, "You are a localization specialist."),
			TextMessage(RoleUser, "Extract entities."),
		})
		return err
	})

	assert.Equal(t, "You are a localization specialist.", got.System)
	require.Len(t, got.Messages, 1)
	assert.Equal(t, RoleUser, got.Messages[0].Role)
	for _, m := range got.Messages {
		assert.NotEqual(t, RoleSystem, m.Role, "system role must never appear in messages[]")
	}
}

// ChatStructured shares the same request builder, so it must lift system too.
func TestAnthropicChatStructuredLiftsSystem(t *testing.T) {
	t.Parallel()

	got := captureAnthropic(t, func(p *AnthropicProvider) error {
		_, err := p.ChatStructured(t.Context(), []Message{
			TextMessage(RoleSystem, "You are a translation reviewer."),
			TextMessage(RoleUser, "Check this."),
		}, JSONSchema{Name: "structured_output", Schema: map[string]any{"type": "object"}})
		return err
	})

	assert.Equal(t, "You are a translation reviewer.", got.System)
	require.Len(t, got.Messages, 1)
	assert.Equal(t, RoleUser, got.Messages[0].Role)
}

// Multiple system messages join rather than the last one winning.
func TestAnthropicJoinsMultipleSystemMessages(t *testing.T) {
	t.Parallel()

	got := captureAnthropic(t, func(p *AnthropicProvider) error {
		_, err := p.Chat(t.Context(), []Message{
			TextMessage(RoleSystem, "First."),
			TextMessage(RoleSystem, "Second."),
			TextMessage(RoleUser, "Go."),
		})
		return err
	})

	assert.Equal(t, "First.\n\nSecond.", got.System)
	assert.Len(t, got.Messages, 1)
}

// A prompt with no system message must not emit an empty system parameter.
func TestAnthropicOmitsEmptySystem(t *testing.T) {
	t.Parallel()

	got := captureAnthropic(t, func(p *AnthropicProvider) error {
		_, err := p.Chat(t.Context(), []Message{TextMessage(RoleUser, "Hi.")})
		return err
	})

	assert.Empty(t, got.System)
	assert.Len(t, got.Messages, 1)
}
