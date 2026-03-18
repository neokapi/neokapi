package mcp

import (
	platagent "github.com/neokapi/neokapi/platform/agent"
)

// PolicyDecision is the result of evaluating a tool against workspace policy.
type PolicyDecision string

const (
	PolicyAllow   PolicyDecision = "allow"
	PolicyDeny    PolicyDecision = "deny"
	PolicyApprove PolicyDecision = "approve" // needs human approval
)

// ToolPolicy evaluates tool access against per-workspace AgentConfig.
type ToolPolicy struct {
	config *platagent.AgentConfig
}

// NewToolPolicy creates a ToolPolicy from an AgentConfig.
func NewToolPolicy(cfg *platagent.AgentConfig) *ToolPolicy {
	return &ToolPolicy{config: cfg}
}

// Check evaluates whether a tool should be allowed, denied, or require approval.
// Evaluation order:
//  1. DeniedTools blacklist (overrides everything)
//  2. RequireApproval list
//  3. AllowedTools whitelist (empty = all available)
func (p *ToolPolicy) Check(toolName string) PolicyDecision {
	if p.config == nil || !p.config.Enabled {
		return PolicyDeny
	}

	// Denied tools always take precedence.
	for _, t := range p.config.DeniedTools {
		if t == toolName {
			return PolicyDeny
		}
	}

	// Check if tool requires human approval.
	for _, t := range p.config.RequireApproval {
		if t == toolName {
			return PolicyApprove
		}
	}

	// If an allow list is configured, the tool must be on it.
	if len(p.config.AllowedTools) > 0 {
		for _, t := range p.config.AllowedTools {
			if t == toolName {
				return PolicyAllow
			}
		}
		return PolicyDeny
	}

	return PolicyAllow
}

// FilterTools returns only the tools that are allowed or require approval
// according to the policy.
func (p *ToolPolicy) FilterTools(toolNames []string) []string {
	var allowed []string
	for _, name := range toolNames {
		decision := p.Check(name)
		if decision == PolicyAllow || decision == PolicyApprove {
			allowed = append(allowed, name)
		}
	}
	return allowed
}
