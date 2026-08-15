package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrackEvent_NilClient(t *testing.T) {
	s := &Server{} // PostHogClient is nil
	// Should not panic.
	s.trackEvent("user-1", "test_event", map[string]any{"key": "value"})
}

func TestTrackEvent_WithClient(t *testing.T) {
	client, err := analytics.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	// Should not panic — events are enqueued but not sent (test key).
	s.trackEvent("user-1", "project_created", map[string]any{
		"project_id": "proj-123",
	})
}

func TestTrackUserLogin_Signup(t *testing.T) {
	client, err := analytics.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	// CreatedAt just now → treated as signup.
	s.trackUserLogin(acquisitionContext(""), "user-1", "new@example.com", time.Now())
	// No assertions on external state — just verifying no panics.
}

func TestTrackUserLogin_ExistingUser(t *testing.T) {
	client, err := analytics.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	// CreatedAt in the past → treated as login.
	s.trackUserLogin(acquisitionContext(""), "user-1", "existing@example.com", time.Now().Add(-24*time.Hour))
}

func TestTrackUserLogin_NilClient(t *testing.T) {
	s := &Server{}
	// Should not panic with nil client, nor with no request context at all —
	// the device-code flow reaches this without a browser behind it.
	s.trackUserLogin(nil, "user-1", "test@example.com", time.Now())
}

// acquisitionContext builds a request context carrying the acquisition cookie,
// or none when the value is empty.
func acquisitionContext(cookie string) echo.Context {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil)
	if cookie != "" {
		req.Header.Set("Cookie", acquisitionCookie+"="+cookie)
	}
	return echo.New().NewContext(req, httptest.NewRecorder())
}

func TestAcquisitionSource(t *testing.T) {
	tests := []struct {
		name   string
		cookie string
		want   string
	}{
		{"no cookie", "", ""},
		{"campaign the landing stamped", "bowrain-landing", "bowrain-landing"},
		{"a campaign name", "newsletter", "newsletter"},
		{"a referring host", "news.ycombinator.com", "news.ycombinator.com"},
		{"case is normalized", "Bowrain-Landing", "bowrain-landing"},
		{"percent-encoded value", "docs%2Ebowrain%2Ecloud", "docs.bowrain.cloud"},
		{"whitespace is trimmed", "%20newsletter%20", "newsletter"},
		{"a value with a space is dropped, not repaired", "two%20words", ""},
		{"markup is dropped", "%3Cscript%3E", ""},
		{"a path is dropped", "example.com%2Fpath", ""},
		{"an over-long value is dropped", strings.Repeat("a", maxAcquisitionSource+1), ""},
		{"the longest allowed value survives", strings.Repeat("a", maxAcquisitionSource), strings.Repeat("a", maxAcquisitionSource)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, acquisitionSource(acquisitionContext(tt.cookie)))
		})
	}
}

func TestAcquisitionSourceWithoutContext(t *testing.T) {
	assert.Empty(t, acquisitionSource(nil))
}

func TestTrackEvent_NilProperties(t *testing.T) {
	client, err := analytics.NewPostHogClient("phc_test", "https://test.posthog.com")
	require.NoError(t, err)
	defer client.Close()

	s := &Server{PostHogClient: client}
	s.trackEvent("user-1", "test_event", nil)
	// If we reached here without a panic, the test passes.
}
