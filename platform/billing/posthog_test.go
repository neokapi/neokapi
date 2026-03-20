package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostHogClient_NoOp(t *testing.T) {
	// Empty API key → no-op client.
	client, err := NewPostHogClient("", "")
	require.NoError(t, err)
	assert.Nil(t, client.client, "no-op client should have nil inner client")

	// These should not panic.
	client.CaptureEvent("user-1", "test_event", map[string]any{"key": "value"})
	client.Identify("user-1", map[string]any{"email": "test@example.com"})
	assert.NoError(t, client.Close())
}

func TestPostHogClient_WithKey(t *testing.T) {
	client, err := NewPostHogClient("phc_test_key", "https://test.posthog.com")
	require.NoError(t, err)
	assert.NotNil(t, client.client, "client with API key should have non-nil inner client")

	// Capture and identify should not panic.
	client.CaptureEvent("user-1", "test_event", map[string]any{"key": "value"})
	client.Identify("user-1", map[string]any{"email": "test@example.com"})
	assert.NoError(t, client.Close())
}

func TestPostHogClient_DefaultHost(t *testing.T) {
	client, err := NewPostHogClient("phc_test_key", "")
	require.NoError(t, err)
	assert.NotNil(t, client.client)
	assert.NoError(t, client.Close())
}
