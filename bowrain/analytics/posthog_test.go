package analytics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPostHogClient_EmptyKey(t *testing.T) {
	client, err := NewPostHogClient("", "")
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Nil(t, client.client, "nil client means no-op mode")
}

func TestPostHogClient_NoOpWhenNilClient(t *testing.T) {
	client := &PostHogClient{client: nil}

	// These should not panic.
	client.CaptureEvent("user-1", "test_event", map[string]any{"key": "value"})
	client.Identify("user-1", map[string]any{"email": "test@example.com"})

	err := client.Close()
	assert.NoError(t, err)
}

func TestNewPostHogClient_DefaultHost(t *testing.T) {
	// We can't actually connect to PostHog in tests, but we can test
	// that providing an empty host uses the default.
	// The constructor will succeed because PostHog SDK accepts any config.
	client, err := NewPostHogClient("phc_test_key", "")
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.NotNil(t, client.client)
	_ = client.Close()
}

func TestNewPostHogClient_CustomHost(t *testing.T) {
	client, err := NewPostHogClient("phc_test_key", "https://custom.posthog.example.com")
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.NotNil(t, client.client)
	_ = client.Close()
}

func TestPostHogClient_CaptureWithProperties(t *testing.T) {
	client, err := NewPostHogClient("phc_test_key", "")
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	// Should not panic with various property types.
	client.CaptureEvent("user-1", "plan_changed", map[string]any{
		"old_plan":   "free",
		"new_plan":   "pro",
		"amount":     2500,
		"is_upgrade": true,
	})
}

func TestPostHogClient_IdentifyWithProperties(t *testing.T) {
	client, err := NewPostHogClient("phc_test_key", "")
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	// Should not panic.
	client.Identify("user-1", map[string]any{
		"email":        "test@bowrain.cloud",
		"workspace_id": "ws-1",
		"plan":         "pro",
	})
}
