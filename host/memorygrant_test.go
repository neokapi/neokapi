package host

import (
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// One grant, reaching every tool that asked.
//
// There used to be three injection routes, and each narrowed the built tool to
// recycle's concrete config before setting anything — so the capability could
// only ever serve one tool, and a second one had to invent a side channel to
// avoid being routed into an assertion it would fail. These tests are about the
// declaration side of that: which tools ask, and by which word.

// appWithTools builds an App holding the real built-in registry, so these
// assertions are about what the shipped tools declare rather than what a
// fixture was told to declare.
func appWithTools() *App {
	reg := registry.NewToolRegistry()
	libtools.RegisterAll(reg)
	aitools.RegisterAll(reg)
	return &App{ToolReg: reg}
}

func schemaFor(t *testing.T, tool string) *schema.ComponentSchema {
	t.Helper()
	s := appWithTools().ToolReg.Schema(registry.ToolID(tool))
	require.NotNil(t, s, "tool %q is not registered", tool)
	require.NotNil(t, s.ToolMeta, "tool %q has no metadata", tool)
	return s
}

// TestRecycleRequiresACorpusAndTranslateAcceptsOne pins the distinction the
// whole grant rests on.
//
// Recycle exists to fill from a content memory: with none it is a no-op, so it
// REQUIRES one. Translate reads a block's previously approved answer as prompt
// reference and translates fine without: it ACCEPTS one. Collapsing these makes
// a corpus mandatory for anything that merely benefits from one, which would
// stop a project without a store from building a translate step at all.
func TestRecycleRequiresACorpusAndTranslateAcceptsOne(t *testing.T) {
	t.Parallel()

	recycle := schemaFor(t, "recycle")
	assert.True(t, ToolRequires(recycle, schema.RequiresMemory),
		"recycle with no corpus is a no-op, which is a requirement")
	assert.False(t, ToolAccepts(recycle, schema.AcceptsMemory),
		"and it does not also merely accept one")

	translate := schemaFor(t, "translate")
	assert.True(t, ToolAccepts(translate, schema.AcceptsMemory),
		"translate uses a corpus when the run has one")
	assert.False(t, ToolRequires(translate, schema.RequiresMemory),
		"and must still build without one, or a project with no store loses translate entirely")
}

// TestRequiresMemoryStillMeansPurpose guards the inference host/upplan.go makes.
//
// upplan reads RequiresMemory on a target-producing step as "this step
// recycles". That is sound only while the flag means the tool's PURPOSE is the
// corpus rather than that it happens to read one. A tool that merely uses a
// corpus and declared Requires would be counted as a recycle step and reported
// as reuse in the plan.
func TestRequiresMemoryStillMeansPurpose(t *testing.T) {
	t.Parallel()

	a := appWithTools()
	var producersRequiringMemory []string
	for _, id := range a.ToolReg.Names() {
		s := a.ToolReg.Schema(id)
		if !toolProducesTarget(s) || !ToolRequires(s, schema.RequiresMemory) {
			continue
		}
		producersRequiringMemory = append(producersRequiringMemory, string(id))
	}

	assert.Equal(t, []string{"recycle"}, producersRequiringMemory,
		"a second target-producing tool that REQUIRES a corpus would be counted as reuse by upplan; "+
			"if it merely uses one, it belongs in Accepts")
}

// TestGrantSkipsAToolThatAskedForNothing: opening a store costs a file handle,
// and a tool that never reads one should not cause it.
func TestGrantSkipsAToolThatAskedForNothing(t *testing.T) {
	t.Parallel()

	a := appWithTools()
	config := map[string]any{"existing": "value"}

	out, cleanup, err := a.grantMemory("uppercase", config)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	cleanup()

	assert.Equal(t, config, out, "the config is handed back untouched")
	assert.NotContains(t, out, corememory.ConfigKey)
}

// TestGrantWithNoCommandIsNotAnError: a tool built outside a command context
// (a test, an embedded run) has no flags to resolve a store from. That is a
// run without a corpus, not a failure — recycle is then a no-op and translate
// is unaffected.
func TestGrantWithNoCommandIsNotAnError(t *testing.T) {
	t.Parallel()

	a := appWithTools()
	out, cleanup, err := a.grantMemory("recycle", map[string]any{})
	require.NoError(t, err)
	cleanup()
	assert.NotContains(t, out, corememory.ConfigKey)
}
