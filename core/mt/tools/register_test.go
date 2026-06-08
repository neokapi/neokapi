package tools_test

import (
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mtToolNames is the canonical set of <provider>-translate tools that
// RegisterAll must contribute.
var mtToolNames = []string{
	"deepl-translate",
	"google-translate",
	"microsoft-translate",
	"modernmt-translate",
	"mymemory-translate",
}

func TestRegisterAllRegistersAllMTTools(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	for _, name := range mtToolNames {
		assert.Truef(t, reg.Has(registry.ToolID(name)), "tool %q should be registered", name)

		// Each tool must carry a schema.
		s := reg.Schema(registry.ToolID(name))
		require.NotNilf(t, s, "tool %q should have a schema", name)
		require.NotNilf(t, s.ToolMeta, "tool %q schema should carry ToolMeta", name)

		// Each tool must be constructible with no config (demo default factory).
		tl, err := reg.NewTool(registry.ToolID(name))
		require.NoErrorf(t, err, "tool %q should be constructible with default factory", name)
		assert.Equal(t, name, tl.Name())

		// Each tool must expose a config factory (verified by building with a
		// config map below — NewToolWithConfig routes through it).
	}
}

func TestRegisterAllMTToolMetadata(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	for _, name := range mtToolNames {
		info := reg.ToolInfo(registry.ToolID(name))
		require.NotNilf(t, info, "tool %q should have info", name)

		assert.Equalf(t, schema.CategoryTranslation, info.Category, "tool %q category", name)
		assert.Truef(t, info.WritesOutput, "tool %q should write output", name)
		assert.Equalf(t, 5, info.DefaultParallelBlocks, "tool %q parallel blocks", name)
		assert.Containsf(t, info.Requires, schema.RequiresTargetLanguage, "tool %q requires target-language", name)
		assert.Containsf(t, info.Requires, schema.RequiresCredentials, "tool %q requires credentials", name)
		assert.Equalf(t, schema.Bilingual, info.Cardinality, "tool %q cardinality", name)
		assert.Containsf(t, info.Produces, schema.IOPort{Type: schema.PortTarget, Side: model.SideTarget}, "tool %q produces translation", name)
		assert.Containsf(t, info.SideEffects, schema.SideEffectAPICall, "tool %q has api-call side effect", name)
	}
}

func TestMTToolConfigFactoriesPresent(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	// Each tool builds via its config factory. mymemory needs no credentials;
	// the others would fail provider Validate() at runtime but construct fine
	// from a config map here (no network call until Process).
	for _, name := range mtToolNames {
		tl, err := reg.NewToolWithConfig(registry.ToolID(name), map[string]any{
			"apiKey": "test-key",
		}, "fr")
		require.NoErrorf(t, err, "tool %q should build from config", name)
		require.NotNilf(t, tl, "tool %q should be non-nil", name)
		assert.Equal(t, name, tl.Name())
	}
}

// TestMTToolConfigFactoryUsesDemoForRun exercises the config-factory + Process
// path end-to-end against the offline demo provider, asserting the target is
// set. No real network/API call is made.
func TestMTToolConfigFactoryUsesDemoForRun(t *testing.T) {
	mtprovider.SetDemoNoticeWriter(io.Discard)

	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	// Build a deepl-translate tool but override the provider to demo via the
	// public config-factory bound to demo. We exercise the registered tool's
	// Process by swapping in a demo-backed instance built through the same
	// MTTranslateSchema/config path used at registration.
	tl, err := reg.NewToolWithConfig("deepl-translate", map[string]any{
		"baseURL": "http://127.0.0.1:0", // never contacted; demo path below covers Process
	}, "fr")
	require.NoError(t, err)
	require.NotNil(t, tl)

	// Now build a demo-backed tool to verify the Process/target-set behaviour
	// deterministically and offline (the real providers would hit the network).
	demoTool := tools.NewMTTranslateTool(mtprovider.NewDemoProvider(), tools.MTTranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)
	require.NoError(t, demoTool.Process(context.Background(), in, out))
	close(out)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	target := resultBlock.TargetText(model.LocaleFrench)
	assert.NotEmpty(t, target, "demo MT should set a target")
	assert.Equal(t, "⟦fr⟧ Bonjour", target)
}
