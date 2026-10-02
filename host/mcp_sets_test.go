package host

import (
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMCPToolSets(t *testing.T) {
	sets, err := ParseMCPToolSets(nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{MCPSetWriting: true}, sets, "no flag serves the writing set")

	sets, err = ParseMCPToolSets([]string{"writing,translation"})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{MCPSetWriting: true, MCPSetTranslation: true}, sets)

	sets, err = ParseMCPToolSets([]string{"Review", " all "})
	require.NoError(t, err)
	assert.Equal(t, AllMCPToolSets(), sets)

	_, err = ParseMCPToolSets([]string{"writing,editing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown tool set "editing"`)
	assert.Contains(t, err.Error(), "writing, content, translation, review, all", "the refusal lists the sets")
}

// No tool sits in two sets: `--tools content` and `--tools writing` are
// different decisions, and a tool in both would make one of them a lie.
func TestMCPToolSetsDoNotOverlap(t *testing.T) {
	seen := map[string]string{}
	for set, tools := range mcpToolSets {
		for _, tool := range tools {
			if other, dup := seen[tool]; dup {
				t.Errorf("%s is in both %s and %s", tool, other, set)
			}
			seen[tool] = set
		}
	}
}

// listSurface lists the tools and resource templates a server built with the
// given sets serves.
func listSurface(t *testing.T, sets map[string]bool) ([]string, []string) {
	t.Helper()
	app := &App{MCPSurface: MCPSurface{Sets: sets}}
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	ApplyMCPToolFactories(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "sets-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	var tools, templates []string
	for tool, err := range session.Tools(t.Context(), nil) {
		require.NoError(t, err)
		tools = append(tools, tool.Name)
	}
	for tmpl, err := range session.ResourceTemplates(t.Context(), nil) {
		require.NoError(t, err)
		templates = append(templates, tmpl.URITemplate)
	}
	return tools, templates
}

func TestMCPToolSetsGateTheSurface(t *testing.T) {
	isolateCheckExecution(t)

	// The host registers every tool of the writing set, so a host-only server
	// serves the whole set.
	all, _ := listSurface(t, nil)
	tools, templates := listSurface(t, map[string]bool{MCPSetWriting: true})
	slices.Sort(tools)
	want := MCPToolSetTools(MCPSetWriting)
	slices.Sort(want)
	assert.Equal(t, want, tools, "the writing set serves its tools and no other")
	assert.ElementsMatch(t, []string{contextLocationTemplate, contextProfileTemplate}, templates)

	tools, templates = listSurface(t, map[string]bool{MCPSetTranslation: true})
	assert.NotContains(t, tools, "check_file")
	assert.Contains(t, tools, "up")
	assert.Empty(t, templates, "the context:// resources belong to the writing set")

	assert.Contains(t, all, "check_text", "a server built without a selection serves every set")
	assert.Contains(t, all, "check_file")
}

// The default surface reaches the edit contract: read_blocks reads a
// document's blocks, apply_edits sends a change set, and describe_format says
// what a format supports. An agent in a project kapi init wired, which names
// no set or adds only translation, has all three.
func TestMCPWritingSetServesTheEditContract(t *testing.T) {
	writing := MCPToolSetTools(MCPSetWriting)
	for _, name := range []string{"read_blocks", "apply_edits", "describe_format"} {
		assert.Contains(t, writing, name)
		assert.NotContains(t, MCPToolSetTools(MCPSetContent), name, "a tool sits in one set")
	}
	assert.Equal(t, []string{"review_queue", "review_block"}, MCPToolSetTools(MCPSetReview),
		"an agent records a pre-review through apply_edits, so the review set only reads")
}
