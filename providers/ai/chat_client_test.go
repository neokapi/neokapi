package aiprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A reasoning model can take minutes on one batch, so every chat provider's
// client allows chatRequestTimeout instead of the 30-second HTTP default.
func TestChatProvidersAllowLongModelRequests(t *testing.T) {
	assert.Equal(t, chatRequestTimeout, NewOpenAIProvider(Config{APIKey: "k"}).client.Timeout)
	assert.Equal(t, chatRequestTimeout, NewAnthropicProvider(Config{APIKey: "k"}).client.Timeout)
	assert.Equal(t, chatRequestTimeout, NewAzureOpenAIProvider(Config{APIKey: "k"}).client.Timeout)
	assert.Equal(t, chatRequestTimeout, NewOllamaProvider(Config{}).client.Timeout)
}
