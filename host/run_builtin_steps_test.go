package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A built-in flow run over a project's collections takes the recipe-flow path,
// so its definition has to render as the steps that path runs: the tool nodes
// in chain order, each node's own config kept, and the run flags a file run
// honours seeded under it.
func TestBuiltInFlowSteps_RendersTheChainWithRunFlags(t *testing.T) {
	spec := builtInFlowSteps("translate", map[string]any{"provider": "demo"})
	require.NotNil(t, spec)

	var tools []string
	for _, s := range spec.Steps {
		tools = append(tools, s.Tool)
	}
	assert.Equal(t, []string{"recycle", "translate", "qa"}, tools,
		"the chain runs left to right, as buildFlowTools assembles it")

	for _, s := range spec.Steps {
		assert.Equal(t, "demo", s.Config["provider"], "step %s carries the run flags", s.Tool)
	}
	assert.Equal(t, true, spec.Steps[1].Config["skipMatched"],
		"the node's own config survives beside the run flags")
}

func TestBuiltInFlowSteps_NodeConfigWinsOverRunFlags(t *testing.T) {
	def := builtInFlow("translate")
	require.NotNil(t, def)
	for i := range def.Nodes {
		if def.Nodes[i].Name == "translate" {
			def.Nodes[i].Config = map[string]any{"provider": "ollama"}
		}
	}
	// The helper reads the catalog, so drive the merge the same way it does.
	nodes := orderedToolNodes(def)
	merged := mergeFlowNodeConfig(map[string]any{"provider": "demo"}, nodes[1].Config)
	assert.Equal(t, "ollama", merged["provider"])
}

func TestBuiltInFlowSteps_UnknownFlowIsNil(t *testing.T) {
	assert.Nil(t, builtInFlowSteps("no-such-flow", nil))
}
