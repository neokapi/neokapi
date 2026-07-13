package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addConnectorAs posts a connector of the given type as a workspace on the given
// plan, and reports the status code. It stops at the plan gate: the connector
// service is nil, so a request that passes the gate fails afterwards with 503 —
// which is exactly the signal we want (503 = "the gate let it through").
func addConnectorAs(t *testing.T, plan billing.Plan, connectorType string) int {
	t.Helper()
	s := &Server{}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/connectors",
		strings.NewReader(`{"type":"`+connectorType+`","config":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageConnectors)
	c.Set("workspace_plan", string(plan))

	require.NoError(t, s.HandleAddConnector(c))
	return rec.Code
}

// Git connectors are a Pro feature in the plan matrix and on the pricing page —
// but no route ever enforced it, so a Free workspace could add one. The paid
// plan's headline connector was free.
func TestAddConnector_GitRequiresPaidPlan(t *testing.T) {
	code := addConnectorAs(t, billing.PlanFree, "git")

	assert.Equal(t, http.StatusForbidden, code, "a Free workspace must not be able to add a git connector")
}

func TestAddConnector_GitAllowedOnPro(t *testing.T) {
	code := addConnectorAs(t, billing.PlanPro, "git")

	assert.NotEqual(t, http.StatusForbidden, code, "git connectors are included in Pro")
}

// Connector types that are not sold as a plan feature stay available to everyone:
// the gate must not invent a paywall around types nobody advertises as paid.
func TestAddConnector_UngatedTypesAreNotPaywalled(t *testing.T) {
	for _, typ := range []string{"wordpress", "figma", "hubspot"} {
		code := addConnectorAs(t, billing.PlanFree, typ)
		assert.NotEqual(t, http.StatusForbidden, code, "connector type %q is not a paid feature", typ)
	}
}

// A deployment with no billing configured (self-hosted) has no plan on the
// context and gates nothing — same rule PlanGuard follows.
func TestAddConnector_NoPlanMeansNoGate(t *testing.T) {
	code := addConnectorAs(t, "", "git")

	assert.NotEqual(t, http.StatusForbidden, code, "self-hosted deployments gate no features")
}
