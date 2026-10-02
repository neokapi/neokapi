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

	// extract_content is registered by the kapi binary's own factories
	// (kapi/mcptools), so a host-only server serves the rest of the set.
	all, _ := listSurface(t, nil)
	tools, templates := listSurface(t, map[string]bool{MCPSetWriting: true})
	slices.Sort(tools)
	var want []string
	for _, name := range MCPToolSetTools(MCPSetWriting) {
		if slices.Contains(all, name) {
			want = append(want, name)
		}
	}
	slices.Sort(want)
	assert.Equal(t, want, tools, "the writing set serves its tools and no other")
	assert.Contains(t, tools, "apply_edits", "the writing set serves the structured edit path")
	assert.ElementsMatch(t, []string{contextLocationTemplate, contextProfileTemplate}, templates)

	tools, templates = listSurface(t, map[string]bool{MCPSetTranslation: true})
	assert.NotContains(t, tools, "check_file")
	assert.Contains(t, tools, "up")
	assert.Empty(t, templates, "the context:// resources belong to the writing set")

	assert.Contains(t, all, "check_text", "a server built without a selection serves every set")
	assert.Contains(t, all, "check_file")
}

// The default surface reaches the structured edit path: extract_content reads
// a file's blocks and apply_edits writes them back. An agent in a project kapi
// init wired, which names no set or adds only translation, has both.
func TestMCPWritingSetServesTheEditPath(t *testing.T) {
	writing := MCPToolSetTools(MCPSetWriting)
	for _, name := range []string{"extract_content", "apply_edits"} {
		assert.Contains(t, writing, name)
		assert.NotContains(t, MCPToolSetTools(MCPSetContent), name, "a tool sits in one set")
	}
}

// apply_edits declares each comment file's check as an object rather than
// repeating the kapi.check/v2 report schema check_file already declares in the
// same set, which would double the size of a tool every writing session reads.
func TestApplyEditsOutputSchemaLeavesTheCheckToCheckFile(t *testing.T) {
	s := applyEditsOutputSchema()
	comments := s.Properties["comments"]
	require.NotNil(t, comments)
	require.NotNil(t, comments.Items)
	held := comments.Items.Properties["check"]
	require.NotNil(t, held)
	assert.Empty(t, held.Properties, "the report's fields are check_file's to declare")
	assert.Contains(t, held.Description, "check_file")
	assert.Contains(t, s.Properties, "not_found", "the rest of the result is declared as inferred")
	assert.Contains(t, comments.Items.Properties, "edits")
}
