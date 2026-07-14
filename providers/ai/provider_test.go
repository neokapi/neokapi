package aiprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The registry advertises each provider's default model — it is what `kapi models`
// shows a user, and what they get when they name no model. Those must be the same
// string.
//
// They were not. The registry went on advertising `gemini-3-flash-preview` (which
// the API now answers 404 "no longer available" for) and `claude-sonnet-4-20250514`
// (retired), while the constructors had moved on. The doc comment claimed the two
// were the same value; nothing checked it.
func TestRegistryDefaultsMatchWhatTheProviderActuallyUses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id   ProviderID
		want string
	}{
		{Anthropic, NewAnthropicProvider(Config{}).config.Model},
		{OpenAI, NewOpenAIProvider(Config{}).config.Model},
		{Gemini, NewGeminiProvider(Config{}).config.Model},
		{AzureOpenAI, NewAzureOpenAIProvider(Config{}).config.Model},
		{Ollama, NewOllamaProvider(Config{}).config.Model},
	}
	for _, tc := range tests {
		t.Run(string(tc.id), func(t *testing.T) {
			t.Parallel()
			info, ok := ProviderInfoFor(tc.id)
			require.True(t, ok)
			assert.Equal(t, tc.want, info.DefaultModel,
				"the model kapi advertises must be the model kapi uses — a stale advert sends a user's first call to a retired model")
		})
	}
}
