package flow_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dataflowReg(t *testing.T) *registry.ToolRegistry {
	t.Helper()
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	return reg
}

// Every built-in flow must satisfy its own data-flow contract.
func TestValidateDataFlow_BuiltInFlowsPass(t *testing.T) {
	t.Parallel()
	reg := dataflowReg(t)
	for _, def := range flow.BuiltInFlows() {
		t.Run(def.ID, func(t *testing.T) {
			assert.NoError(t, def.ValidateDataFlow(reg), "built-in flow %q must satisfy its IO contract", def.ID)
		})
	}
}

func TestValidateDataFlow_NilRegistrySkips(t *testing.T) {
	t.Parallel()
	def := flow.FlowDefinition{
		ID:   "x",
		Name: "x",
		Nodes: []flow.FlowNode{
			{ID: "qa", Type: flow.NodeTool, Name: "qa-check"},
		},
		Binding: &flow.FlowBinding{Source: "file"},
	}
	assert.NoError(t, def.ValidateDataFlow(nil))
}

// qa-check requires a target. Against a monolingual file source with no
// upstream translate step, the flow is rejected.
func TestValidateDataFlow_QAOnFileSourceRejected(t *testing.T) {
	t.Parallel()
	reg := dataflowReg(t)
	def := flow.FlowDefinition{
		ID:   "qa-only",
		Name: "QA only",
		Nodes: []flow.FlowNode{
			{ID: "qa", Type: flow.NodeTool, Name: "qa-check"},
		},
		Binding: &flow.FlowBinding{Source: "file"},
	}
	err := def.ValidateDataFlow(reg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "qa-check")
	assert.Contains(t, err.Error(), "target@target")
}

// The same qa-check is valid against a bilingual interchange source, which
// provides the target.
func TestValidateDataFlow_QAOnInterchangeSourcePasses(t *testing.T) {
	t.Parallel()
	reg := dataflowReg(t)
	def := flow.FlowDefinition{
		ID:   "qa-xliff",
		Name: "QA xliff",
		Nodes: []flow.FlowNode{
			{ID: "qa", Type: flow.NodeTool, Name: "qa-check"},
		},
		Binding: &flow.FlowBinding{Source: "xliff"},
	}
	assert.NoError(t, def.ValidateDataFlow(reg))
}

// A translate step upstream produces the target qa-check needs, so the flow is
// valid even against a monolingual file source.
func TestValidateDataFlow_TranslateThenQAPasses(t *testing.T) {
	t.Parallel()
	reg := dataflowReg(t)
	def := flow.FlowDefinition{
		ID:   "pseudo-qa",
		Name: "Pseudo then QA",
		Nodes: []flow.FlowNode{
			{ID: "p", Type: flow.NodeTool, Name: "pseudo-translate"},
			{ID: "qa", Type: flow.NodeTool, Name: "qa-check"},
		},
		Edges: []flow.FlowEdge{
			{ID: "e", Source: "p", Target: "qa"},
		},
		Binding: &flow.FlowBinding{Source: "file"},
	}
	assert.NoError(t, def.ValidateDataFlow(reg))
}

// Reversed order (qa before the translate that produces its target) is rejected
// against a file source — the contract catches the ordering bug.
func TestValidateDataFlow_QABeforeTranslateRejected(t *testing.T) {
	t.Parallel()
	reg := dataflowReg(t)
	def := flow.FlowDefinition{
		ID:   "qa-then-pseudo",
		Name: "QA then pseudo",
		Nodes: []flow.FlowNode{
			{ID: "qa", Type: flow.NodeTool, Name: "qa-check"},
			{ID: "p", Type: flow.NodeTool, Name: "pseudo-translate"},
		},
		Edges: []flow.FlowEdge{
			{ID: "e", Source: "qa", Target: "p"},
		},
		Binding: &flow.FlowBinding{Source: "file"},
	}
	assert.Error(t, def.ValidateDataFlow(reg))
}

// Unknown (plugin) tools are skipped, not rejected.
func TestValidateDataFlow_UnknownToolSkipped(t *testing.T) {
	t.Parallel()
	reg := dataflowReg(t)
	def := flow.FlowDefinition{
		ID:   "plugin",
		Name: "plugin flow",
		Nodes: []flow.FlowNode{
			{ID: "x", Type: flow.NodeTool, Name: "some-plugin-tool"},
		},
		Binding: &flow.FlowBinding{Source: "file"},
	}
	assert.NoError(t, def.ValidateDataFlow(reg))
}
