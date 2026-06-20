package tools_test

import (
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/mt/tools"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProvidersListed verifies the canonical MT engine list the unified
// `translate` tool dispatches to. There is no longer a per-engine command —
// these ids select the engine via `kapi translate --provider <id>`.
func TestProvidersListed(t *testing.T) {
	ids := make([]string, 0, len(tools.Providers))
	for _, p := range tools.Providers {
		ids = append(ids, string(p.ID))
		assert.NotEmpty(t, p.Label, "provider %q should carry a label", p.ID)
	}
	assert.ElementsMatch(t, []string{
		"deepl", "google", "microsoft", "modernmt", "mymemory",
	}, ids)
}

// TestNewMTTranslateFromConfigBuildsEachEngine verifies every MT engine builds
// from a config map through its bound config factory. The reported tool name is
// the unified `translate` (the engine is an implementation detail of --provider).
func TestNewMTTranslateFromConfigBuildsEachEngine(t *testing.T) {
	for _, p := range tools.Providers {
		factory := tools.NewMTTranslateFromConfig(p.ID)
		tl, err := factory(map[string]any{"apiKey": "test-key"}, "fr")
		require.NoErrorf(t, err, "engine %q should build from config", p.ID)
		require.NotNilf(t, tl, "engine %q should be non-nil", p.ID)
		assert.Equalf(t, "translate", tl.Name(), "engine %q reports the unified tool name", p.ID)
	}
}

// TestMTTranslateDemoRun exercises the Process/target-set behaviour
// deterministically and offline against the demo provider.
func TestMTTranslateDemoRun(t *testing.T) {
	mtprovider.SetDemoNoticeWriter(io.Discard)

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
