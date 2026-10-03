//go:build !js

package host

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// checkFileOverMCP calls the MCP check_file tool on one file of the project at
// root and returns its report.
func checkFileOverMCP(t *testing.T, root, file string) check.Report {
	t.Helper()
	app := &App{SourceLang: "en", mcpRecipePath: filepath.Join(root, "kapi.yaml")}
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerCheckMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "directives-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "check_file", Arguments: map[string]any{"file": filepath.Join(root, file)}})
	require.NoError(t, err)
	body, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	require.False(t, result.IsError, "%s", body)
	var report check.Report
	require.NoError(t, json.Unmarshal(body, &report))
	return report
}

// MCP check_file reads a named file through the same layer, under the
// directives of the project it sits in.
func TestMCPCheckFileSetsAsideDeclaredDirectives(t *testing.T) {
	t.Run("declared markers are never prose", func(t *testing.T) {
		report := checkFileOverMCP(t, directiveProject(t, declaresAll), "code/parse.go")
		assert.Equal(t, 5, report.Target.Blocks)
		assert.ElementsMatch(t, with(placed{file: "parse.go", block: "func/Parse#2", lines: lineRange(6, 6)}), doubledWords(report.Findings))

		report = checkFileOverMCP(t, directiveProject(t, declaresAll), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks, "two comments and two values")
		assert.Equal(t, []placed{{file: "app.yaml", block: "comment/greeting#2", lines: lineRange(3, 3)}}, doubledWords(report.Findings))
	})

	t.Run("must fail: without the declarations the marker lines are prose", func(t *testing.T) {
		report := checkFileOverMCP(t, directiveProject(t, declaresNothing), "code/parse.go")
		assert.Equal(t, 5, report.Target.Blocks)
		assert.ElementsMatch(t, with(
			placed{file: "parse.go", block: "func/Parse", lines: lineRange(3, 6)},
			placed{file: "parse.go", block: "func/Format", lines: lineRange(9, 9)},
		), doubledWords(report.Findings))
	})
}
