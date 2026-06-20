package credentials

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingTool captures the config its factory received and acts as a pass-
// through tool so it can actually run a Part end-to-end with no network.
type recordingTool struct {
	apiKey   string
	provider string
}

func (r *recordingTool) Name() string                      { return "recording" }
func (r *recordingTool) Description() string               { return "records resolved config" }
func (r *recordingTool) Config() tool.ToolConfig           { return nil }
func (r *recordingTool) SetConfig(_ tool.ToolConfig) error { return nil }
func (r *recordingTool) Process(_ context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		out <- p
	}
	return nil
}

// TestEnvFallback_FullInjectResolveRun exercises the real production path: a
// tool requiring "credentials" registered with the genuine ResolveCredentials
// preprocessor (wired exactly as cli/App.Init does), with only an env var
// present. It asserts the env key flows through inject → preprocess → factory,
// and that the constructed tool runs over a Part with no network.
func TestEnvFallback_FullInjectResolveRun(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fullpath")

	store := newTestStore(t)

	reg := registry.NewToolRegistry()
	rec := &recordingTool{}
	reg.RegisterWithSchema("translate", func() tool.Tool { return rec }, &schema.ComponentSchema{
		Title: "AI Translate",
		ToolMeta: &schema.ToolMeta{
			ID:       "translate",
			Requires: []string{"credentials"},
		},
	})
	reg.SetConfigFactory("translate", func(config map[string]any, _ string) (tool.Tool, error) {
		rec.apiKey, _ = config["apiKey"].(string)
		rec.provider, _ = config["provider"].(string)
		return rec, nil
	})
	// Wire the real resolver, exactly as cli.App.Init / kapi-desktop do.
	reg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		return ResolveCredentials(store, toolName, requires, config)
	})

	// No inline key, no --credential — only the env var is present.
	built, err := reg.NewToolWithConfig("translate", map[string]any{"provider": "anthropic"}, "fr")
	require.NoError(t, err)
	require.NotNil(t, built)

	assert.Equal(t, "sk-ant-fullpath", rec.apiKey, "env var must reach the tool's config factory")
	assert.Equal(t, "anthropic", rec.provider)

	// And the tool actually runs (pass-through, no network).
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	part := &model.Part{Type: model.PartLayerStart}
	in <- part
	close(in)
	require.NoError(t, built.Process(context.Background(), in, out))
	close(out)
	got := <-out
	assert.Same(t, part, got)
}

// TestEnvFallback_FullPathMTProvider verifies the same full path for the
// unified translate tool routed to an MT engine via config["provider"]:
// DEEPL_API_KEY is injected from the environment.
func TestEnvFallback_FullPathMTProvider(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("DEEPL_API_KEY", "dl-fullpath")

	store := newTestStore(t)

	reg := registry.NewToolRegistry()
	rec := &recordingTool{}
	reg.RegisterWithSchema("translate", func() tool.Tool { return rec }, &schema.ComponentSchema{
		Title: "Translate",
		ToolMeta: &schema.ToolMeta{
			ID:       "translate",
			Requires: []string{"credentials"},
		},
	})
	reg.SetConfigFactory("translate", func(config map[string]any, _ string) (tool.Tool, error) {
		rec.apiKey, _ = config["apiKey"].(string)
		rec.provider, _ = config["provider"].(string)
		return rec, nil
	})
	reg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		return ResolveCredentials(store, toolName, requires, config)
	})

	// The MT engine is selected via config["provider"]; the resolver injects
	// DEEPL_API_KEY for it.
	built, err := reg.NewToolWithConfig("translate", map[string]any{"provider": "deepl"}, "fr")
	require.NoError(t, err)
	require.NotNil(t, built)

	assert.Equal(t, "dl-fullpath", rec.apiKey, "DEEPL_API_KEY must reach the translate tool's factory")
	assert.Equal(t, "deepl", rec.provider, "provider carried from config")
}
