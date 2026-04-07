package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinComposedFlows_DerivedFromRegistry(t *testing.T) {
	composed := builtinComposedFlows()
	require.NotEmpty(t, composed, "should have at least one composed flow")

	// Every composed flow should exist in flow.BuiltInFlows().
	builtInIDs := make(map[string]bool)
	for _, def := range flow.BuiltInFlows() {
		builtInIDs[def.ID] = true
	}
	for _, cf := range composed {
		assert.True(t, builtInIDs[cf.Name],
			"composed flow %q should exist in flow.BuiltInFlows()", cf.Name)
	}
}

func TestBuiltinComposedFlows_OnlyMultiTool(t *testing.T) {
	composed := builtinComposedFlows()
	for _, cf := range composed {
		// Find the flow definition and count tools.
		for _, def := range flow.BuiltInFlows() {
			if def.ID == cf.Name {
				toolCount := 0
				for _, n := range def.Nodes {
					if n.Type == "tool" {
						toolCount++
					}
				}
				assert.GreaterOrEqual(t, toolCount, 2,
					"composed flow %q should have 2+ tool nodes, got %d", cf.Name, toolCount)
			}
		}
	}
}

func TestBuiltinComposedFlowNames_MatchesComposedFlows(t *testing.T) {
	names := builtinComposedFlowNames()
	composed := builtinComposedFlows()
	assert.Equal(t, len(composed), len(names))
	for _, cf := range composed {
		assert.True(t, names[cf.Name], "composed flow %q should be in names map", cf.Name)
	}
}

func TestDefaultParallelBlocks_AIFlows(t *testing.T) {
	// AI-powered flows should have parallel blocks > 0.
	pb := defaultParallelBlocks("ai-translate")
	assert.Greater(t, pb, 0, "ai-translate should have parallel blocks")

	pb = defaultParallelBlocks("ai-translate-qa")
	assert.Greater(t, pb, 0, "ai-translate-qa should have parallel blocks")
}

func TestDefaultParallelBlocks_CPUFlows(t *testing.T) {
	// CPU-bound flows should have 0 parallel blocks.
	assert.Equal(t, 0, defaultParallelBlocks("pseudo-translate"))
	assert.Equal(t, 0, defaultParallelBlocks("qa-check"))
	assert.Equal(t, 0, defaultParallelBlocks("segmentation"))
}

func TestDefaultParallelBlocks_Unknown(t *testing.T) {
	assert.Equal(t, 0, defaultParallelBlocks("nonexistent"))
}

func TestAllBuiltInFlowToolsResolvable(t *testing.T) {
	// Every tool node in every built-in flow should be resolvable via LookupToolCommand.
	for _, def := range flow.BuiltInFlows() {
		for _, n := range def.Nodes {
			if n.Type != "tool" {
				continue
			}
			td := LookupToolCommand(n.Name)
			assert.NotNil(t, td,
				"flow %q references tool %q which is not in BuiltinToolCommands",
				def.ID, n.Name)
		}
	}
}
