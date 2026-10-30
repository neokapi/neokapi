//go:build !js

package host

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The MCP translate tool runs the registry tool over a text the caller gives,
// and applies the redaction policy of the project the call acts on as `kapi
// exec translate` does.
func TestFrameworkMCPHandler_AppliesTheProjectsRedaction(t *testing.T) {
	isolateKapiEnv(t)
	onlyDemoProvider(t)
	recipe := redactProject(t, redactionYAML)
	app := &App{}
	app.InitRegistries()
	handler := app.frameworkMCPHandler("translate", "", targetWithheld)
	call := func(project string) (*frameworkToolOutput, string) {
		t.Helper()
		saw := providerSaw(t)
		args, err := json.Marshal(map[string]any{
			"text": "Write to Ada Lovelace today.", "target_lang": "fr", "provider": "demo", "project": project,
		})
		require.NoError(t, err)
		res, err := handler(context.Background(), &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: args}})
		require.NoError(t, err)
		out, ok := res.StructuredContent.(*frameworkToolOutput)
		require.True(t, ok)
		return out, saw()
	}

	out, sent := call(recipe)
	require.NotEmpty(t, sent)
	assert.NotContains(t, sent, redactedName, "the provider never reads the name the policy withholds")
	assert.Contains(t, out.Targets["fr"], redactedName, "the translation carries the name again")
	assert.Contains(t, out.Source, redactedName, "the source comes back as the caller gave it")
	assert.NotContains(t, out.Annotations, "redaction.secret", "no original rides out on the block")

	_, sent = call(filepath.Dir(redactProject(t, "")))
	assert.Contains(t, sent, redactedName, "a project with no policy sends the text as it is")
}
