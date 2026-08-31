package apierror

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCtx(t *testing.T, withRequestID bool) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/thing", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if withRequestID {
		c.Set("request_id", "req-42")
	}
	return c, rec
}

func TestWrite_EnvelopeShape(t *testing.T) {
	c, rec := newCtx(t, true)

	err := Write(c, http.StatusForbidden, "project_limit_reached", map[string]any{
		"current": 2,
		"limit":   1,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	// The historical wire contract is untouched: the stable code stays in
	// "error" and the structured detail fields stay at the top level.
	assert.Equal(t, "project_limit_reached", body["error"])
	assert.Equal(t, float64(2), body["current"])
	assert.Equal(t, float64(1), body["limit"])

	// The additive fields.
	assert.Equal(t, "req-42", body["reference"])
	msg, _ := body["message"].(string)
	assert.Contains(t, msg, "1 projects")
	assert.Contains(t, msg, "already has 2")
	assert.False(t, strings.Contains(msg, "!"), "messages are plain factual sentences")
}

func TestWrite_NoDetailsNoRequestID(t *testing.T) {
	c, rec := newCtx(t, false)

	require.NoError(t, Write(c, http.StatusUnauthorized, "not authenticated"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "not authenticated", body["error"])
	assert.Equal(t, "You are not signed in. Sign in and try again.", body["message"])
	_, hasRef := body["reference"]
	assert.False(t, hasRef, "no reference key when the middleware did not run")
}

func TestWrite_DetailKeysNeverOverrideEnvelope(t *testing.T) {
	c, rec := newCtx(t, true)

	require.NoError(t, Write(c, http.StatusBadRequest, "invalid_grant", map[string]any{
		"error":     "spoofed",
		"reference": "spoofed",
	}))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "invalid_grant", body["error"])
	assert.Equal(t, "req-42", body["reference"])
}

func TestMessageFor(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		details map[string]any
		want    string
	}{
		{
			name:    "parametrized upgrade_required",
			code:    "upgrade_required",
			details: map[string]any{"feature": "connectors-git", "minimum_plan": "pro"},
			want:    "This workspace's plan does not include this feature; it requires the pro plan or higher.",
		},
		{
			name:    "credits_exhausted with reset time",
			code:    "credits_exhausted",
			details: map[string]any{"resets_at": "2026-07-20T00:00:00Z"},
			want:    "This workspace has used its weekly credits; they reset at 2026-07-20T00:00:00Z.",
		},
		{
			name: "mapped sentence string",
			code: "slug is already in use",
			want: "That workspace URL is already taken. Choose a different one.",
		},
		{
			name: "project limit without details",
			code: "project_limit_reached",
			want: "A free workspace cannot hold more projects. Get in touch if you need more.",
		},
		{
			name: "unknown code falls back to itself so message is always present",
			code: "some dynamic error text",
			want: "some dynamic error text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MessageFor(tt.code, tt.details))
		})
	}
}
