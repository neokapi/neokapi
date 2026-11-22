package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flagProject writes a recipe with one JSON collection over dir's
// messages_en.json and the given flows, and returns the recipe path.
func flagProject(t *testing.T, dir string, flows map[string]*flow.StepsSpec) string {
	t.Helper()
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:     project.CurrentVersion,
		Name:        "flageffects",
		Defaults:    project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []project.Collection{{Path: "messages_en.json", Target: "messages_{lang}.json"}},
		Flows:       flows,
	}))
	return recipe
}

// runCmd runs a fresh `kapi run` with flags and args.
func runCmd(t *testing.T, a *App, args []string, flags ...string) (string, error) {
	t.Helper()
	cmd := NewRunCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(t.Context())
	for i := 0; i+1 < len(flags); i += 2 {
		require.NoError(t, cmd.Flags().Set(flags[i], flags[i+1]))
	}
	err := cmd.RunE(cmd, args)
	return out.String(), err
}

// A batch run (two files) fans block processing out across --parallel-blocks
// goroutines, as a single-file run does.
func TestRunParallelBlocksReachesABatch(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	var body strings.Builder
	body.WriteString("{")
	for i := range 8 {
		if i > 0 {
			body.WriteString(",")
		}
		fmt.Fprintf(&body, `"k%d": "Text %d"`, i, i)
	}
	body.WriteString("}")
	files := []string{filepath.Join(dir, "a_en.json"), filepath.Join(dir, "b_en.json")}
	for _, f := range files {
		require.NoError(t, os.WriteFile(f, []byte(body.String()), 0o644))
	}

	var active, peak atomic.Int32
	a := processOnlyApp(t)
	a.ToolReg.Register("concurrency-probe", func() tool.Tool {
		return &tool.BaseTool{ToolName: "concurrency-probe", Annotate: func(tool.BlockView) error {
			cur := active.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return nil
		}}
	})
	recipe := flagProject(t, dir, map[string]*flow.StepsSpec{
		"probe": {Steps: []flow.FlowStep{{Tool: "concurrency-probe"}}},
	})

	out, err := runCmd(t, a, []string{"probe"}, "project", recipe, "input", files[0], "input", files[1],
		"concurrency", "1", "parallel-blocks", "4", "output", filepath.Join(dir, "out")+"/")
	require.NoError(t, err, out)
	assert.Greater(t, peak.Load(), int32(1), "blocks of one file ran concurrently")
}

// A project run that keeps its output in the store writes --trace.
func TestRunTraceOnAProcessOnlyRun(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "messages_en.json"), []byte(`{"greeting": "Hello"}`), 0o644))
	recipe := flagProject(t, dir, map[string]*flow.StepsSpec{
		"mine": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
	})
	trace := filepath.Join(dir, "trace.json")

	out, err := runCmd(t, processOnlyApp(t), []string{"mine"}, "project", recipe, "trace", trace)
	require.NoError(t, err, out)
	assert.Contains(t, out, "Committed overlays", "the run kept its output in the store")
	assert.FileExists(t, trace)
}

// The translate-qa flow's judge takes the provider the run resolves, so
// --provider reaches it as it reaches translate.
func TestTranslateReviewFlowJudgeTakesTheRunProvider(t *testing.T) {
	pf, err := ResolveProjectFlow(&project.KapiProject{}, t.TempDir(), "translate-qa", map[string]any{"provider": "demo"})
	require.NoError(t, err)
	require.NotNil(t, pf)
	for _, s := range pf.Spec.Steps {
		assert.Equal(t, "demo", s.Config["provider"], "%s takes --provider", s.Tool)
	}
}
