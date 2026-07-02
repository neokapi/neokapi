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

// TestProvidersListed verifies the framework ships no classic MT engines — the
// translation core is LLM-first. Plugins hosting MT engines append to
// tools.Providers to surface them in `kapi translate --provider <id>`.
func TestProvidersListed(t *testing.T) {
	assert.Empty(t, tools.Providers, "no built-in MT engines: the translation core is LLM-only")
}

// TestNewMTTranslateFromConfigBuildsRegisteredEngine verifies a registered MT
// engine (here the offline demo provider) builds from a config map through its
// bound config factory. The reported tool name is the unified `translate` (the
// engine is an implementation detail of --provider).
func TestNewMTTranslateFromConfigBuildsRegisteredEngine(t *testing.T) {
	factory := tools.NewMTTranslateFromConfig(mtprovider.Demo)
	tl, err := factory(map[string]any{"apiKey": "test-key"}, "fr")
	require.NoError(t, err, "registered engine should build from config")
	require.NotNil(t, tl)
	assert.Equal(t, "translate", tl.Name(), "engine reports the unified tool name")
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
