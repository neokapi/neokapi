package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinComposedFlows_DerivedFromRegistry(t *testing.T) {
	composed := builtinComposedFlows()
	require.NotEmpty(t, composed, "should have at least one composed flow")

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
		for _, def := range flow.BuiltInFlows() {
			if def.ID == cf.Name {
				toolCount := 0
				for _, n := range def.Nodes {
					if n.Type == flow.NodeTool {
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
	assert.Len(t, names, len(composed))
	for _, cf := range composed {
		assert.True(t, names[cf.Name], "composed flow %q should be in names map", cf.Name)
	}
}

func TestDefaultParallelBlocks_AIFlows(t *testing.T) {
	app := newTestApp()
	pb := app.defaultParallelBlocks("translate")
	assert.Greater(t, pb, 0, "translate should have parallel blocks")

	pb = app.defaultParallelBlocks("translate-qa")
	assert.Greater(t, pb, 0, "translate-qa should have parallel blocks")
}

func TestDefaultParallelBlocks_CPUFlows(t *testing.T) {
	app := newTestApp()
	assert.Equal(t, 0, app.defaultParallelBlocks("pseudo-translate"))
	assert.Equal(t, 0, app.defaultParallelBlocks("word-count"))
}

func TestDefaultParallelBlocks_Unknown(t *testing.T) {
	app := newTestApp()
	assert.Equal(t, 0, app.defaultParallelBlocks("nonexistent"))
}

func TestAllBuiltInFlowToolsResolvable(t *testing.T) {
	app := newTestApp()
	for _, def := range flow.BuiltInFlows() {
		for _, n := range def.Nodes {
			if n.Type != "tool" {
				continue
			}
			assert.True(t, app.ToolReg.Has(registry.ToolID(n.Name)),
				"flow %q references tool %q which is not in ToolRegistry",
				def.ID, n.Name)
		}
	}
}
