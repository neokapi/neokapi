package server

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPostHogClient captures events for testing.
type capturedEvent struct {
	DistinctID string
	Event      string
	Properties map[string]any
}

type capturedIdentify struct {
	DistinctID string
	Properties map[string]any
}

// testAnalyticsCollector wraps a real PostHogClient and captures events.
// We use the no-op PostHogClient from billing and intercept via trackEvent.

func TestTrackEvent_NilClient(t *testing.T) {
	s := &Server{} // PostHogClient is nil
	// Should not panic.
	s.trackEvent("user-1", "test_event", map[string]any{"key": "value"})
}

func TestTrackEvent_WithClient(t *testing.T) {
	client, err := billing.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	// Should not panic — events are enqueued but not sent (test key).
	s.trackEvent("user-1", "project_created", map[string]any{
		"project_id": "proj-123",
	})
}

func TestTrackUserLogin_Signup(t *testing.T) {
	client, err := billing.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	// CreatedAt just now → treated as signup.
	s.trackUserLogin("user-1", "new@example.com", time.Now())
	// No assertions on external state — just verifying no panics.
}

func TestTrackUserLogin_ExistingUser(t *testing.T) {
	client, err := billing.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	// CreatedAt in the past → treated as login.
	s.trackUserLogin("user-1", "existing@example.com", time.Now().Add(-24*time.Hour))
}

func TestTrackUserLogin_NilClient(t *testing.T) {
	s := &Server{}
	// Should not panic with nil client.
	s.trackUserLogin("user-1", "test@example.com", time.Now())
}

func TestTrackEvent_NilProperties(t *testing.T) {
	client, err := billing.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	s.trackEvent("user-1", "test_event", nil)
	assert.True(t, true) // reached without panic
}
