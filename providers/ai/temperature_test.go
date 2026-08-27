package aiprovider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Config.Temperature was accepted by every provider and sent by two.
//
// Anthropic, OpenAI, Azure and Gemini took the field and dropped it, so a
// caller asking for determinism silently got whatever the API defaults to,
// which for Anthropic is 1.0. Nothing failed. The only evidence was that every
// eval in the repo was sampling at maximum variance and none of them could say
// what they had run at.
//
// The field is a *float64 because 0 is a real request: greedy decoding is what
// an eval that wants to be reproducible should ask for. As a bare float64 with
// `omitempty` it was indistinguishable from unset, and Bedrock still spelled
// that as `if cfg.Temperature > 0` — the one value you could not ask for.

// captureBody stands in for a provider's API and hands back the request body.
func captureBody(t *testing.T, reply string) (*httptest.Server, *[]byte) {
	t.Helper()
	var seen []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = b
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

const (
	anthropicReply = `{"content":[{"type":"text","text":"ok"}],"model":"m","usage":{"input_tokens":1,"output_tokens":1}}`
	openaiReply    = `{"choices":[{"message":{"content":"ok"}}],"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1}}`
	geminiReply    = `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"modelVersion":"m"}`
)

// TestEveryProviderSendsTemperature is the regression guard. A provider added
// later that forgets to thread the field will fail here rather than silently
// sampling at the API default forever.
func TestEveryProviderSendsTemperature(t *testing.T) {
	cases := []struct {
		name  string
		reply string
		// path into the request body where the value should land.
		field string
		build func(cfg Config) LLMProvider
	}{
		{
			name:  "anthropic",
			reply: anthropicReply,
			field: "temperature",
			build: func(cfg Config) LLMProvider { return NewAnthropicProvider(cfg) },
		},
		{
			name:  "openai",
			reply: openaiReply,
			field: "temperature",
			build: func(cfg Config) LLMProvider { return NewOpenAIProvider(cfg) },
		},
		{
			name:  "gemini",
			reply: geminiReply,
			field: "generationConfig.temperature",
			build: func(cfg Config) LLMProvider { return NewGeminiProvider(cfg) },
		},
		{
			name:  "ollama",
			reply: `{"message":{"content":"ok"},"done":true}`,
			field: "options.temperature",
			build: func(cfg Config) LLMProvider { return NewOllamaProvider(cfg) },
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 0.3 rather than a round number, so a hardcoded default cannot
			// pass by coincidence.
			t.Run("an explicit value reaches the wire", func(t *testing.T) {
				srv, seen := captureBody(t, c.reply)
				p := c.build(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Temperature: new(0.3)})
				_, _ = p.Chat(context.Background(), []Message{TextMessage(RoleUser, "hi")})
				require.NotEmpty(t, *seen, "provider sent no request")
				assert.InDelta(t, 0.3, fieldAt(t, *seen, c.field), 1e-9)
			})

			// The value an eval actually wants, and the one a bare float64
			// could never express.
			t.Run("zero is reachable", func(t *testing.T) {
				srv, seen := captureBody(t, c.reply)
				p := c.build(Config{BaseURL: srv.URL, Model: "m", APIKey: "k", Temperature: new(0.0)})
				_, _ = p.Chat(context.Background(), []Message{TextMessage(RoleUser, "hi")})
				require.NotEmpty(t, *seen)
				assert.InDelta(t, 0.0, fieldAt(t, *seen, c.field), 1e-9,
					"temperature 0 asks for greedy decoding; dropping it as empty is the bug")
			})
		})
	}
}

// TestUnsetTemperatureIsOmitted: leaving it nil must not pin a value of our
// choosing on the caller's behalf. Ollama is the documented exception — a local
// model gets 0.2 because its default is unusable for translation.
func TestUnsetTemperatureIsOmitted(t *testing.T) {
	for _, c := range []struct {
		name  string
		reply string
		build func(cfg Config) LLMProvider
	}{
		{"anthropic", anthropicReply, func(cfg Config) LLMProvider { return NewAnthropicProvider(cfg) }},
		{"openai", openaiReply, func(cfg Config) LLMProvider { return NewOpenAIProvider(cfg) }},
		{"gemini", geminiReply, func(cfg Config) LLMProvider { return NewGeminiProvider(cfg) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, seen := captureBody(t, c.reply)
			p := c.build(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
			_, _ = p.Chat(context.Background(), []Message{TextMessage(RoleUser, "hi")})
			require.NotEmpty(t, *seen)
			assert.NotContains(t, string(*seen), `"temperature"`,
				"an unset temperature must leave the provider's default alone")
		})
	}
}

// fieldAt reads a dotted path out of a JSON body.
func fieldAt(t *testing.T, body []byte, path string) float64 {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))

	var cur any = doc
	for _, part := range splitDots(path) {
		m, ok := cur.(map[string]any)
		require.True(t, ok, "path %q: %q is not an object in %s", path, part, body)
		cur, ok = m[part]
		require.True(t, ok, "path %q: no %q in %s", path, part, body)
	}
	v, ok := cur.(float64)
	require.True(t, ok, "path %q is not a number in %s", path, body)
	return v
}

func splitDots(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == '.' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
