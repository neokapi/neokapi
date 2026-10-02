package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// applyViaMCP applies a change-set the way the MCP apply_edits tool does.
func applyViaMCP(t *testing.T, app *App, entries []map[string]any) applyEditsMCPOutput {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: "test"}, nil)
	registerEditMCPTools(server, app)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "apply-encode-test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	changeset := make([]any, len(entries))
	for i, e := range entries {
		changeset[i] = e
	}
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "apply_edits", Arguments: map[string]any{"changeset": changeset},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "apply_edits: %v", res.Content)
	body, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out applyEditsMCPOutput
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

// The wording of a content entry is text. `kapi apply` and MCP apply_edits
// write it through the same round-trip, which encodes it for the file's
// format, so an agent's wording can never add markup to the document: inline
// codes travel as <x id="…"/> tokens, and a literal '<' or '&' is a character.
func TestApply_WritesEditedWordingAsText(t *testing.T) {
	const payload = "Hello <script>alert(1)</script> & goodbye"
	tests := []struct {
		name string
		file string
		src  string
		want string
	}{
		{
			name: "html",
			file: "inj.html",
			src:  `<html><body><p>Hello world</p></body></html>`,
			want: `<html><body><p>Hello &lt;script>alert(1)&lt;/script> &amp; goodbye</p></body></html>`,
		},
		{
			name: "markdown",
			file: "inj.md",
			src:  "Hello world\n",
			want: "Hello \\<script>alert(1)\\</script> & goodbye\n",
		},
	}
	for _, tc := range tests {
		for _, surface := range []string{"kapi apply", "apply_edits"} {
			t.Run(tc.name+"/"+surface, func(t *testing.T) {
				app := newToolboxApp(t)
				path := filepath.Join(t.TempDir(), tc.file)
				require.NoError(t, os.WriteFile(path, []byte(tc.src), 0o600))
				entries := []map[string]any{{
					"kind": "content", "file": path,
					"content_hash": model.ComputeContentHash("Hello world"), "text": payload,
				}}

				if surface == "kapi apply" {
					noProject(t)
					recs := inspectJSONL(t, app, path)
					require.Len(t, recs, 1)
					body := changeSetOf(t, map[string]any{"op": "set_content", "at": recs[0].Ref, "if_match": recs[0].Rev, "text": payload})
					res, err := applyJSON(t, app, NewEnvCommand(t.Context(), "apply"), body, ApplyOptions{})
					require.NoError(t, err)
					assert.Equal(t, change.OpApplied, res.Ops[0].Status)
				} else {
					assert.Len(t, applyViaMCP(t, app, entries).Applied, 1)
				}
				got, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, tc.want, string(got))
			})
		}
	}
}
