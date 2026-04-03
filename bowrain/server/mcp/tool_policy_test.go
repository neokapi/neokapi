package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
)

func TestToolPolicyDisabledAgent(t *testing.T) {
	p := NewToolPolicy(&platagent.AgentConfig{Enabled: false})
	assert.Equal(t, PolicyDeny, p.Check("list_projects"))
}

func TestToolPolicyNilConfig(t *testing.T) {
	p := NewToolPolicy(nil)
	assert.Equal(t, PolicyDeny, p.Check("list_projects"))
}

func TestToolPolicyAllowAll(t *testing.T) {
	p := NewToolPolicy(&platagent.AgentConfig{Enabled: true})
	assert.Equal(t, PolicyAllow, p.Check("list_projects"))
	assert.Equal(t, PolicyAllow, p.Check("run_flow"))
}

func TestToolPolicyDenyList(t *testing.T) {
	p := NewToolPolicy(&platagent.AgentConfig{
		Enabled:     true,
		DeniedTools: []string{"connector_push"},
	})
	assert.Equal(t, PolicyDeny, p.Check("connector_push"))
	assert.Equal(t, PolicyAllow, p.Check("list_projects"))
}

func TestToolPolicyApprovalList(t *testing.T) {
	p := NewToolPolicy(&platagent.AgentConfig{
		Enabled:         true,
		RequireApproval: []string{"connector_push"},
	})
	assert.Equal(t, PolicyApprove, p.Check("connector_push"))
	assert.Equal(t, PolicyAllow, p.Check("list_projects"))
}

func TestToolPolicyDenyOverridesApproval(t *testing.T) {
	p := NewToolPolicy(&platagent.AgentConfig{
		Enabled:         true,
		DeniedTools:     []string{"connector_push"},
		RequireApproval: []string{"connector_push"},
	})
	assert.Equal(t, PolicyDeny, p.Check("connector_push"))
}

func TestToolPolicyAllowList(t *testing.T) {
	p := NewToolPolicy(&platagent.AgentConfig{
		Enabled:      true,
		AllowedTools: []string{"list_projects", "get_project"},
	})
	assert.Equal(t, PolicyAllow, p.Check("list_projects"))
	assert.Equal(t, PolicyAllow, p.Check("get_project"))
	assert.Equal(t, PolicyDeny, p.Check("run_flow"))
}

func TestToolPolicyFilterTools(t *testing.T) {
	p := NewToolPolicy(&platagent.AgentConfig{
		Enabled:         true,
		DeniedTools:     []string{"execute_script"},
		RequireApproval: []string{"connector_push"},
	})
	result := p.FilterTools([]string{"list_projects", "connector_push", "execute_script"})
	assert.Equal(t, []string{"list_projects", "connector_push"}, result)
}
