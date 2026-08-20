package observe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A request's transaction carries the dimensions a person filters by.
//
// Duration with no subject answers nothing anyone asks. The questions are "is it
// slow for this customer", "is it only the trial plan", "is it this project" —
// and each needs its dimension on the trace or it cannot be asked at all.
func TestTagScope_HTTPCarriesTheFilterableDimensions(t *testing.T) {
	tr := capture(t)

	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(TracingMiddleware())
	// Stands in for the auth middleware, which resolves the workspace and its
	// plan and runs INSIDE the tracing middleware.
	e.GET("/api/v1/:ws/:id/dashboard/:ref", func(c echo.Context) error {
		c.Set("workspace_id", "ws_4a1")
		c.Set("workspace_plan", "enterprise")
		c.Set("user_id", "usr_88")
		return c.NoContent(http.StatusOK)
	})
	e.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/v1/acme/p_9f3/dashboard/main", nil))

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "ws_4a1", txns[0].Tags["workspace_id"])
	assert.Equal(t, "p_9f3", txns[0].Tags["project_id"])
	assert.Equal(t, "enterprise", txns[0].Tags["plan"])
	assert.Equal(t, "dashboard", txns[0].Tags["feature"])
	assert.Equal(t, "usr_88", txns[0].Tags["user_id"])

	// And none of it leaked into the name, which is what has to stay one row.
	assert.Equal(t, "GET /api/v1/:ws/:id/dashboard/:ref", txns[0].Transaction)
}

// The dimensions are read after the handler, because that is the only time
// they exist.
//
// The auth middleware sets the workspace and plan and runs inside the tracing
// middleware, so reading them on the way in finds nothing. This asserts the
// order rather than the values: a future refactor that tags on entry would
// produce traces that look complete and filter to nothing.
func TestTagScope_ReadsDimensionsSetByInnerMiddleware(t *testing.T) {
	tr := capture(t)

	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(TracingMiddleware())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("workspace_id", "ws_set_by_inner_middleware")
			c.Set("workspace_plan", "pro")
			return next(c)
		}
	})
	e.GET("/api/v1/:ws/projects", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/v1/acme/projects", nil))

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "ws_set_by_inner_middleware", txns[0].Tags["workspace_id"])
	assert.Equal(t, "pro", txns[0].Tags["plan"])
}

// A dimension that is absent is absent, not empty.
//
// Writing "" would make a filter for a missing plan match every unauthenticated
// request, which is the opposite of what someone asking that question wants.
func TestTagScope_SkipsEmptyDimensions(t *testing.T) {
	tr := capture(t)

	ctx, end := Transaction(context.Background(), "queue.task", "translation")
	TagScope(ctx, Scope{WorkspaceID: "ws_1"})
	end(nil)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "ws_1", txns[0].Tags["workspace_id"])
	assert.NotContains(t, txns[0].Tags, "plan")
	assert.NotContains(t, txns[0].Tags, "project_id")
	assert.NotContains(t, txns[0].Tags, "user_id")
}

// Tagging outside a transaction is a no-op rather than a panic: background code
// and tests reach this path.
func TestTagScope_WithoutATransaction(t *testing.T) {
	tr := capture(t)
	TagScope(context.Background(), Scope{WorkspaceID: "ws_1", Plan: "free"})
	assert.Empty(t, tr.transactions())
}

// The feature is derived from the route, so it cannot drift from what is served.
func TestFeatureFromRoute(t *testing.T) {
	for _, tc := range []struct{ route, want string }{
		{"/api/v1/:ws/:id/dashboard/:ref", "dashboard"},
		{"/api/v1/:ws/projects", "projects"},
		{"/api/v1/:ws/:id/collections/:ref/:cid", "collections"},
		{"/api/v1/:ws/:id/sync/status", "sync"},
		{"/api/v1/health", "health"},
		// Nothing but parameters after the prefix: these serve the workspace
		// itself, which is the honest name for them.
		{"/api/v1/:ws/:id", "workspace"},
		{"/api/v1/:ws", "workspace"},
		{"", ""},
	} {
		t.Run(tc.route, func(t *testing.T) {
			assert.Equal(t, tc.want, FeatureFromRoute(tc.route))
		})
	}
}
