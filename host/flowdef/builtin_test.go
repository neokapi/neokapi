package flowdef

import (
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltInFlows(t *testing.T) {
	flows := BuiltInFlows()
	require.Len(t, flows, 10)

	ids := make(map[string]bool)
	for _, f := range flows {
		assert.NotEmpty(t, f.ID)
		assert.NotEmpty(t, f.Name)
		assert.Equal(t, registry.SourceBuiltIn, f.Source)
		require.NoError(t, f.Validate())
		ids[f.ID] = true
	}
	assert.True(t, ids["translate"])
	assert.True(t, ids["translate-qa"])
	assert.True(t, ids["pseudo-translate"])
	assert.True(t, ids["qa"])
	assert.True(t, ids["recycle"])
	assert.True(t, ids["secure-translate"])
	assert.True(t, ids["redact-pii"])
	assert.True(t, ids["audio-to-subtitles"])
	assert.True(t, ids["video-to-subtitles"])
	assert.True(t, ids["image-ocr-translate"])
}

// TestTranslateFlowTranslatesOnlyTheRemainder proves the built-in translate
// flow's AI step skips units the recycle step already filled. Its description
// promises "content memory reuse, then AI translate"; with the tool's own
// default the AI step re-translates every recycled unit, so the flow paid for a
// model call per unit and shipped the model's wording over the project's own
// approved wording.
func TestTranslateFlowTranslatesOnlyTheRemainder(t *testing.T) {
	var translateFlow *flow.FlowDefinition
	for i, f := range BuiltInFlows() {
		if f.ID == "translate" {
			translateFlow = &BuiltInFlows()[i]
			break
		}
	}
	require.NotNil(t, translateFlow, "built-in translate flow")

	var node *flow.FlowNode
	for i, n := range translateFlow.Nodes {
		if n.Name == "translate" {
			node = &translateFlow.Nodes[i]
			break
		}
	}
	require.NotNil(t, node, "translate node in the translate flow")
	assert.Equal(t, true, node.Config["skipMatched"])
}

// builtinGateReg builds a registry with the built-in and AI tools — the same
// population the CLI gates validate against.
func builtinGateReg(t *testing.T) *registry.ToolRegistry {
	t.Helper()
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	aitools.RegisterAll(reg)
	return reg
}

// Every built-in flow must satisfy its own data-flow contract.
func TestValidateDataFlow_BuiltInFlowsPass(t *testing.T) {
	t.Parallel()
	reg := builtinGateReg(t)
	for _, def := range BuiltInFlows() {
		t.Run(def.ID, func(t *testing.T) {
			assert.NoError(t, def.ValidateDataFlow(reg), "built-in flow %q must satisfy its IO contract", def.ID)
		})
	}
}

// Every built-in flow must pass its own placement gate.
func TestValidatePlacement_BuiltInFlowsPass(t *testing.T) {
	t.Parallel()
	reg := builtinGateReg(t)
	for _, def := range BuiltInFlows() {
		t.Run(def.ID, func(t *testing.T) {
			assert.NoError(t, def.CheckPlacement(reg), "built-in flow %q must pass the placement gate", def.ID)
		})
	}
}
