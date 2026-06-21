package backend

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInjectCredentialPicker_GroupAware verifies the credential field is attached
// to whichever group holds the provider fields — including a ToolGroup member
// group with a namespaced id (e.g. "ai:provider") — so the picker inherits that
// group's visibility instead of falling back to ungrouped (always-on).
func TestInjectCredentialPicker_GroupAware(t *testing.T) {
	app := &App{credentials: credentials.NewStore(filepath.Join(t.TempDir(), "creds.json"))}

	// A composed tool-group schema: the provider fields live in a namespaced
	// member group ("ai:provider") gated by the discriminator.
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode":     map[string]any{"type": "string"},
			"provider": map[string]any{"type": "string"},
			"apiKey":   map[string]any{"type": "string"},
			"model":    map[string]any{"type": "string"},
		},
		"ui:groups": []any{
			map[string]any{"id": "qa", "fields": []any{"mode"}},
			map[string]any{"id": "ai:provider", "fields": []any{"provider", "apiKey", "model"},
				"ui:visible": map[string]any{"field": "mode", "eq": "ai"}},
		},
	}

	app.injectCredentialPicker(schema)

	props := schema["properties"].(map[string]any)
	require.Contains(t, props, "credential", "credential field injected")

	// Manual provider fields gated on an empty credential.
	for _, f := range []string{"provider", "apiKey", "model"} {
		vis, ok := props[f].(map[string]any)["ui:visible"].(map[string]any)
		require.True(t, ok, "%s has ui:visible", f)
		assert.Equal(t, "credential", vis["field"])
	}

	// The credential field was added to the member group (ai:provider), not left
	// ungrouped — so it shows only when that AI backend is selected.
	groups := schema["ui:groups"].([]any)
	var aiFields []any
	for _, g := range groups {
		gm := g.(map[string]any)
		if gm["id"] == "ai:provider" {
			aiFields = gm["fields"].([]any)
		}
	}
	require.NotNil(t, aiFields)
	assert.Equal(t, any("credential"), aiFields[0], "credential prepended to the provider group")
}
