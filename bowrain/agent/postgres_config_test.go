package agent

// Real Postgres-backed coverage for the agent's tool policy. The three policy
// columns all fail open when empty — an empty deny-list denies nothing, an
// empty approval list asks for nothing, and an empty allow-list means "all
// available" (server/mcp/tool_policy.go) — so a column that cannot be read must
// not be allowed to read as an unrestricted agent. Skipped in -short unless
// BOWRAIN_TEST_POSTGRES_URL is set.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

func newAgentStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(pgtest.NewTestDB(t))
	require.NoError(t, err)
	return s
}

func TestAgentConfigRoundTripsToolPolicy(t *testing.T) {
	s := newAgentStore(t)
	ctx := t.Context()

	want := &platagent.AgentConfig{
		WorkspaceID:     "ws-policy",
		Enabled:         true,
		AllowedTools:    []string{"search", "read"},
		DeniedTools:     []string{"shell"},
		RequireApproval: []string{"publish"},
		MaxConcurrent:   2,
	}
	require.NoError(t, s.SaveAgentConfig(ctx, want))

	got, err := s.GetAgentConfig(ctx, "ws-policy")
	require.NoError(t, err)
	assert.Equal(t, want.AllowedTools, got.AllowedTools)
	assert.Equal(t, want.DeniedTools, got.DeniedTools)
	assert.Equal(t, want.RequireApproval, got.RequireApproval)
}

// A policy column that cannot be parsed must stop the read. Returning the
// zero value would hand back an agent with nothing denied and everything
// allowed, indistinguishable from one deliberately configured that way.
func TestAgentConfigRefusesUnparseableToolPolicy(t *testing.T) {
	tests := []struct {
		name   string
		column string
	}{
		{name: "allowed tools", column: "allowed_tools"},
		{name: "denied tools", column: "denied_tools"},
		{name: "approval list", column: "require_approval"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newAgentStore(t)
			ctx := t.Context()

			require.NoError(t, s.SaveAgentConfig(ctx, &platagent.AgentConfig{
				WorkspaceID:  "ws-corrupt",
				Enabled:      true,
				DeniedTools:  []string{"shell"},
				AllowedTools: []string{"search"},
			}))

			// A JSON scalar: valid JSON, so the column's own type check passes,
			// but not the list the policy is.
			_, err := s.db.ExecContext(ctx,
				`UPDATE agent_config SET `+tc.column+` = '"not-a-list"' WHERE workspace_id = $1`, "ws-corrupt")
			require.NoError(t, err)

			got, err := s.GetAgentConfig(ctx, "ws-corrupt")
			require.Error(t, err, "an unreadable tool policy must not resolve to an unrestricted agent")
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tc.column)
		})
	}
}

// An absent row is not a corrupt one: it is a workspace that has never
// configured the agent, and it still gets the defaults.
func TestAgentConfigMissingRowFallsBackToDefaults(t *testing.T) {
	s := newAgentStore(t)

	got, err := s.GetAgentConfig(t.Context(), "ws-never-configured")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "ws-never-configured", got.WorkspaceID)
}
