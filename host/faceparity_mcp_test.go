//go:build !js

package host_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpSession connects a client to a real server carrying kapi's MCP surface, so
// the tools and resources are exercised through their own dispatch: the tool
// names, the URI templates and the mime types are part of the face.
func mcpSession(t *testing.T, a *host.App) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	host.ApplyMCPToolFactories(server, a)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	client := mcp.NewClient(&mcp.Implementation{Name: "face-parity", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// mcpContextAt reads the by-location resource and decodes its JSON rendering.
func mcpContextAt(t *testing.T, p facetest.Project) facetest.ContextFacts {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()
	session := mcpSession(t, a)

	uri := "context://" + p.ContextPath + "?format=json"
	res, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)

	var answer host.ContextAnswer
	require.NoError(t, json.Unmarshal([]byte(res.Contents[0].Text), &answer))
	return facetest.ContextFactsFrom(&answer)
}

// mcpContextSearch calls the by-content tool and decodes its structured result.
func mcpContextSearch(t *testing.T, p facetest.Project) facetest.SearchFacts {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()
	session := mcpSession(t, a)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "context_search",
		Arguments: map[string]any{
			"query": p.SearchQuery,
			"limit": p.SearchLimit,
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "context_search failed: %+v", res.Content)

	var out host.ContextSearchResult
	require.NoError(t, json.Unmarshal(structuredJSON(t, res), &out))
	return facetest.SearchFactsFrom(&out)
}

// mcpCheckFacts runs the check tool over the fixture's document.
func mcpCheckFacts(t *testing.T, p facetest.Project) facetest.CheckFacts {
	t.Helper()
	a := &host.App{}
	a.InitRegistries()
	session := mcpSession(t, a)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "check_file",
		Arguments: map[string]any{
			"file":         p.CheckPath,
			"profile_file": ".kapi/voice.yaml",
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "check_file failed: %+v", res.Content)

	var report check.Report
	require.NoError(t, json.Unmarshal(structuredJSON(t, res), &report))
	return facetest.CheckFactsFrom(report)
}

// Status is answered at two faces, not three. The gap is pinned rather than
// left for a future reader to rediscover: `kapi status` prints the two-axis
// standing and the desktop renders it, and no MCP tool serves it. An agent
// asking where a project stands has to read the graph instead.
func TestFaceParity_StatusHasNoMCPFace(t *testing.T) {
	a := &host.App{}
	a.InitRegistries()
	session := mcpSession(t, a)

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	for _, tool := range res.Tools {
		assert.NotEqual(t, "status", tool.Name,
			"a status tool exists now: give it a leg in the parity record")
		assert.NotEqual(t, "project_status", tool.Name,
			"a status tool exists now: give it a leg in the parity record")
	}
}

// referenceAnswers computes what the host layer says about the fixture.
func referenceAnswers(t *testing.T, p facetest.Project) facetest.Answers {
	t.Helper()
	ctx := t.Context()
	a := &host.App{}
	a.InitRegistries()

	// context <path>
	atReq := host.ContextPointRequest{Path: p.ContextPath, Limit: p.ContextLimit}
	atCmd := hostCommand(t, ctx, "context", p)
	atSrc, atDone := a.ContextSourcesAt(atCmd, atReq)
	atAnswer, err := host.ResolveContextAt(ctx, atSrc, atReq)
	atDone()
	require.NoError(t, err)

	// context search
	searchReq := host.ContextSearchRequest{Query: p.SearchQuery, Limit: p.SearchLimit}
	searchCmd := hostCommand(t, ctx, "context-search", p)
	searchSrc, searchDone := a.ContextSearchSourcesFor(searchCmd, "", "")
	searchResult, err := host.SearchContext(ctx, searchSrc, searchReq)
	searchDone()
	require.NoError(t, err)

	return facetest.Answers{
		ContextAt:     facetest.ContextFactsFrom(atAnswer),
		ContextSearch: facetest.SearchFactsFrom(searchResult),
		Status:        hostStatusFacts(t, p),
		Check:         mcpCheckFacts(t, p),
	}
}

// TestFaceParity_MCPMatchesTheRecord is the MCP leg: the tools and resources
// answer what the record says, so the CLI and desktop suites comparing against
// the same record are comparing against MCP.
func TestFaceParity_MCPMatchesTheRecord(t *testing.T) {
	// The fixture becomes the working directory, so the record's own path is
	// resolved against the package's directory first.
	goldenPath, err := filepath.Abs(filepath.Join("facetest", facetest.GoldenPath()))
	require.NoError(t, err)

	p := facetest.Write(t)

	if *updateGolden {
		require.NoError(t, os.WriteFile(goldenPath, facetest.Marshal(t, referenceAnswers(t, p)), 0o644))
		t.Logf("wrote %s", goldenPath)
	}

	want := facetest.Golden(t)

	assert.Equal(t, want.ContextAt, mcpContextAt(t, p),
		"the context:// resource answers what the record says applies there")
	assert.Equal(t, want.ContextSearch, mcpContextSearch(t, p),
		"context_search answers what the record says is known")
	assert.Equal(t, want.Check, mcpCheckFacts(t, p),
		"check_file finds what the record says is wrong")
}

// The host layer and the record agree, so a change to the shared resolution
// that nobody re-recorded fails here rather than three modules away.
func TestFaceParity_TheRecordMatchesTheHostLayer(t *testing.T) {
	p := facetest.Write(t)
	got := referenceAnswers(t, p)
	want := facetest.Golden(t)

	assert.Equal(t, want.ContextAt, got.ContextAt)
	assert.Equal(t, want.ContextSearch, got.ContextSearch)
	assert.Equal(t, want.Status, got.Status)
	assert.Equal(t, want.Check, got.Check)
}
