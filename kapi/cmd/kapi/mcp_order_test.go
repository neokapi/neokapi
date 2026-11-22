package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tools/list answers in the same order, byte for byte, on every server: an
// agent that caches or diffs the list sees no churn between runs.
func TestMCPToolsListIsStable(t *testing.T) {
	list := func() []byte {
		app := &cli.App{}
		app.InitRegistries()
		server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
		cli.ApplyMCPToolFactories(server, app)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		serverSession, err := server.Connect(ctx, serverTransport, nil)
		require.NoError(t, err)
		defer serverSession.Close()
		session, err := mcp.NewClient(&mcp.Implementation{Name: "order-test", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
		require.NoError(t, err)
		defer session.Close()

		var tools []*mcp.Tool
		params := &mcp.ListToolsParams{}
		for {
			res, err := session.ListTools(ctx, params)
			require.NoError(t, err)
			tools = append(tools, res.Tools...)
			if res.NextCursor == "" {
				break
			}
			params.Cursor = res.NextCursor
		}
		data, err := json.Marshal(tools)
		require.NoError(t, err)
		return data
	}
	first := list()
	for range 19 {
		assert.Equal(t, string(first), string(list()))
	}
}
