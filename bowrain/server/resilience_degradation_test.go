package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/apierror"
	"github.com/neokapi/neokapi/bowrain/resilience"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEchoCtx(t *testing.T) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/thing", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("request_id", "req-77")
	return c, rec
}

// TestUnavailableErr_DegradesRatherThanFails is the API half of the graceful
// degradation contract: an AI provider that is circuit-broken must reach the
// client as a distinct, actionable state — not a 500 the user can only stare at.
func TestUnavailableErr_DegradesRatherThanFails(t *testing.T) {
	c, rec := newEchoCtx(t)

	err := &resilience.UnavailableError{
		Dependency: "ai:bedrock",
		Kind:       resilience.KindAI,
		RetryAfter: 25 * time.Second,
	}
	resp, ok := unavailableErr(c, err)
	require.True(t, ok, "an open-circuit rejection must be recognised")
	require.NoError(t, resp)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"503, not 500: nothing broke, a known-down dependency was not called")
	assert.Equal(t, "25", rec.Header().Get("Retry-After"),
		"the client needs to know how long to wait")

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, apierror.CodeAIUnavailable, body["error"])
	assert.Equal(t, "ai:bedrock", body["dependency"])
	assert.Equal(t, float64(25), body["retry_after_seconds"])
	assert.Equal(t, "req-77", body["reference"], "still drillable to logs")

	msg, _ := body["message"].(string)
	assert.Contains(t, msg, "temporarily unavailable")
	assert.Contains(t, msg, "queued",
		"the sentence must tell the user the work is not lost")
}

func TestUnavailableErr_NonAIDependencyUsesItsOwnCode(t *testing.T) {
	c, rec := newEchoCtx(t)

	_, ok := unavailableErr(c, &resilience.UnavailableError{
		Dependency: "connector:figma",
		Kind:       resilience.KindConnector,
		RetryAfter: 30 * time.Second,
	})
	require.True(t, ok)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, apierror.CodeDependencyUnavailable, body["error"])
	assert.Contains(t, body["message"], "connector:figma")
}

func TestUnavailableErr_WrappedErrorIsStillRecognised(t *testing.T) {
	// The rejection crosses several layers between the provider guard and the
	// handler; unwrapping must survive that.
	c, _ := newEchoCtx(t)
	wrapped := errors.Join(
		errors.New("translate blocks"),
		&resilience.UnavailableError{Dependency: "ai:bedrock", Kind: resilience.KindAI, RetryAfter: time.Second},
	)
	_, ok := unavailableErr(c, wrapped)
	assert.True(t, ok)
}

func TestUnavailableErr_LeavesOtherErrorsAlone(t *testing.T) {
	c, rec := newEchoCtx(t)

	resp, ok := unavailableErr(c, errors.New("openai: API error 401: bad key"))
	assert.False(t, ok, "an ordinary error must fall through to the caller's handling")
	assert.NoError(t, resp)
	assert.Equal(t, http.StatusOK, rec.Code, "nothing should have been written")
	assert.Empty(t, rec.Body.String())

	_, ok = unavailableErr(c, nil)
	assert.False(t, ok)
}

func TestUnavailableErr_RetryAfterIsAlwaysUsable(t *testing.T) {
	// A sub-second estimate must still produce a header a client can parse and
	// act on, never "0" (retry immediately, defeating the breaker).
	c, rec := newEchoCtx(t)
	_, ok := unavailableErr(c, &resilience.UnavailableError{
		Dependency: "ai:bedrock", Kind: resilience.KindAI, RetryAfter: 5 * time.Millisecond,
	})
	require.True(t, ok)

	secs, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, secs, 1)
}

// TestReadiness_ReportsOpenCircuitAsDegraded proves the health surface cannot
// claim "ready" while the request path is shedding calls.
func TestReadiness_ReportsOpenCircuitAsDegraded(t *testing.T) {
	// The readiness check consults the process-wide registry, so trip a breaker
	// there and restore it afterwards.
	t.Cleanup(func() { resilience.Default().Configure(resilience.Overrides{}) })
	resilience.Default().Configure(resilience.Overrides{
		MinimumRequests: intPtr(4),
		FailureRatio:    floatPtr(0.5),
		CooldownSeconds: intPtr(60),
	})
	b := resilience.Get("ai:test-readiness", resilience.KindAI)
	for range 4 {
		_ = b.Do(context.Background(), func(context.Context) error {
			return errors.New("upstream API error 503")
		})
	}
	require.Equal(t, resilience.StateOpen, b.State())

	s := &Server{}
	components, status := s.readiness(context.Background())

	cb, present := components["circuit_breakers"]
	require.True(t, present, "an open circuit must appear as a health component")
	assert.Equal(t, "down", cb.Status)
	assert.Contains(t, cb.Error, "ai:test-readiness")
	assert.NotEqual(t, "ready", status,
		"the platform must not report ready while it is declining calls")
}

// TestHandleAdminHealth_IncludesBreakerStates covers the ctrl Health payload.
func TestHandleAdminHealth_IncludesBreakerStates(t *testing.T) {
	resilience.Get("ai:test-health-payload", resilience.KindAI)

	s := &Server{}
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/admin/health", nil), rec)

	require.NoError(t, s.HandleAdminHealth(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var body AdminHealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	var found *resilience.Status
	for i := range body.Breakers {
		if body.Breakers[i].Dependency == "ai:test-health-payload" {
			found = &body.Breakers[i]
		}
	}
	require.NotNil(t, found, "the breaker the code is enforcing must be visible to the operator")
	assert.Equal(t, "ai", found.Kind)
	assert.Equal(t, string(resilience.StateClosed), found.State)
	assert.True(t, found.Healthy)
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
