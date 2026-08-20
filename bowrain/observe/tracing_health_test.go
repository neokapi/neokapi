package observe

import (
	"context"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// The load balancer's probe is served, and produces nothing.
//
// Both halves matter. Skipping the transaction must not skip the handler: a
// health route that stops answering takes the service out of the target group.
func TestTracing_HealthProbeIsServedButNotTraced(t *testing.T) {
	tr := capture(t)

	rec := serve(t, healthRouteHTTP, healthRouteHTTP,
		func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	assert.Equal(t, http.StatusOK, rec.Code, "the probe is still answered")
	assert.Empty(t, tr.transactions(), "and costs no transaction")
}

// The skip is the health route and nothing adjacent to it.
//
// `/api/admin/health` is the ctrl Health page — an authenticated page a person
// loads, which fans out to the worker and the database and is exactly the kind
// of thing worth timing. It shares a word with the probe and nothing else.
func TestTracing_AdjacentHealthRouteIsStillTraced(t *testing.T) {
	tr := capture(t)

	serve(t, "/api/admin/health", "/api/admin/health",
		func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "GET /api/admin/health", txns[0].Transaction)
}

// The gRPC health method is skipped by the interceptor rather than by the
// absence of a handler.
//
// grpc-go answers an unregistered method without running interceptors, so this
// path is inert while no health service is registered. The test registers one,
// which is what makes it a regression test: it asserts the interceptor itself
// declines the method, so registering the service for real cannot silently
// start producing a transaction every fifteen seconds.
func TestGRPCTracing_HealthProbeIsServedButNotTraced(t *testing.T) {
	tr := capture(t)

	handled := false
	_, err := GRPCTracingUnaryInterceptor()(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: healthMethodGRPC},
		func(context.Context, any) (any, error) { handled = true; return nil, nil })

	require.NoError(t, err)
	assert.True(t, handled, "the probe is still answered")
	assert.Empty(t, tr.transactions(), "and costs no transaction")
}

func TestIsHealthProbe(t *testing.T) {
	for _, tc := range []struct {
		route string
		want  bool
	}{
		{healthRouteHTTP, true},
		{healthMethodGRPC, true},
		// The ctrl Health page, and a workspace-scoped route that merely ends
		// in the same word. Neither is a probe.
		{"/api/admin/health", false},
		{"/api/v1/:ws/:id/sync/health", false},
		{"/api/v1/health/detail", false},
		{"", false},
	} {
		t.Run(tc.route, func(t *testing.T) {
			assert.Equal(t, tc.want, isHealthProbe(tc.route))
		})
	}
}
