package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kpzWorkspace extracts one JSON document into a fresh .kpz workspace that
// records the given target languages (none when targets is empty), with the
// workspace cache in a throwaway directory.
func kpzWorkspace(t *testing.T, targets string) (*App, string) {
	t.Helper()
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello, write to someone@example.com"}`), 0o644))

	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	work := filepath.Join(dir, "work.kpz")
	require.NoError(t, a.ExtractToKpz(context.Background(), []string{src}, work, targets, "", false))
	return a, work
}

// runToolOnKpz runs one registered tool over the workspace the way
// `kapi exec <tool> work.kpz` does when the run names no target language.
func runToolOnKpz(a *App, work, toolName string, config map[string]any) error {
	return a.RunToolOnFiles(context.Background(), ToolRunConfig{
		ToolName: toolName,
		Files:    []string{work},
		NewTool: func() (tool.Tool, error) {
			return a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, "")
		},
	})
}

func kpzRecordedTargets(t *testing.T, a *App, work string) []string {
	t.Helper()
	c, err := a.ensureKpzCache(context.Background(), work)
	require.NoError(t, err)
	return recipeTargetLangs(c.meta.Recipe)
}

// A monolingual tool runs on a workspace that records no target language, over
// the source alone, as it does on a plain file. `kapi exec redact` offers no
// --target-lang, so a transform that insisted on one could never run.
func TestKpzTransformRunsAMonolingualToolWithNoTarget(t *testing.T) {
	for _, tc := range []struct {
		tool   string
		config map[string]any
	}{
		{"redact", nil},
		{"encoding-detect", nil},
		// case-transform takes a target when a run names one and works on the
		// source otherwise.
		{"case-transform", map[string]any{"mode": "upper"}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			a, work := kpzWorkspace(t, "")
			require.NoError(t, runToolOnKpz(a, work, tc.tool, tc.config))
			assert.Empty(t, kpzRecordedTargets(t, a, work), "a source-only pass records no target locale")
		})
	}
}

// A bilingual tool still needs a target language, and the error names the
// --target-lang flag its command offers.
func TestKpzTransformRequiresATargetForABilingualTool(t *testing.T) {
	a, work := kpzWorkspace(t, "")
	err := runToolOnKpz(a, work, "whitespace-correct", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target-lang is required")
}

// A monolingual tool on a workspace that records a target works in that
// target, and adds no locale of its own.
func TestKpzTransformMonolingualToolKeepsTheRecordedTargets(t *testing.T) {
	a, work := kpzWorkspace(t, "fr")
	require.NoError(t, runToolOnKpz(a, work, "redact", nil))
	assert.Equal(t, []string{"fr"}, kpzRecordedTargets(t, a, work))
	assert.Equal(t, "fr", a.TargetLang)
}
