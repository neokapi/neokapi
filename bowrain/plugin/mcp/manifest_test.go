package bowrainmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// servedTools reads back what registerBowrainTools puts on a server, over an
// in-memory session.
func servedTools(t *testing.T) map[string]*mcp.Tool {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi-bowrain", Version: "test"}, nil)
	registerBowrainTools(server, bowrainTestApp())

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "probe", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = clientSession.Close() })

	out := map[string]*mcp.Tool{}
	for tool, err := range clientSession.Tools(ctx, nil) {
		require.NoError(t, err)
		out[tool.Name] = tool
	}
	return out
}

func loadManifest(t *testing.T) *manifest.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "cmd", "kapi-bowrain", "manifest.json"))
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)
	return m
}

// The manifest's `mcp_tools` is what joins kapi's surface through the
// `kapi mcp` proxy. The proxy skips a declared tool the plugin's server does
// not offer, with a note on stderr, so a declaration that drifts from the
// registrations fails here instead. The declared set is the curated one: a
// tool joins it by a decision recorded in this list, and the two registered
// tools left off it say why at their registration.
func TestManifestMCPToolsAreServed(t *testing.T) {
	m := loadManifest(t)
	served := servedTools(t)

	var declared []string
	for _, tool := range m.Capabilities.MCPTools {
		declared = append(declared, tool.Name)
		assert.Contains(t, served, tool.Name,
			"the manifest declares %q but registerBowrainTools does not serve it", tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"project_status", "project_push", "project_pull",
		"project_ls", "concept_story", "experiment_status",
	}, declared, "the declared set is a curation decision; change it together with this list")

	for _, name := range []string{"project_config", "concept_search"} {
		assert.Contains(t, served, name, "%s is served from the plugin's own server", name)
		assert.NotContains(t, declared, name, "%s stays off kapi's surface", name)
	}
}

// A plugin tool that shares a name with one kapi serves never reaches the
// surface: the proxy keeps kapi's and skips the plugin's, with a note on
// stderr. The generated MCP reference is kapi's full tool list, every surface
// included, so a served name found there is a collision whether or not it is
// declared.
func TestPluginToolsDoNotCollideWithKapiTools(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "packages", "reference-data", "data", "mcp-tools.json"))
	require.NoError(t, err)
	var ref struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(data, &ref))
	require.NotEmpty(t, ref.Tools, "sanity: the generated reference lists kapi's tools")

	kapi := map[string]bool{}
	for _, tool := range ref.Tools {
		kapi[tool.Name] = true
	}
	for name := range servedTools(t) {
		assert.False(t, kapi[name], "%s is a tool kapi already serves; give the plugin's its own name", name)
	}
}
