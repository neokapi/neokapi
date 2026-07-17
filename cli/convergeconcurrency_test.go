package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUp_SingleFile_ConcurrentLocales: a single-content-file project (both
// locale workers drive RunSingleFile over the SAME source, each with a
// whole-run block-store session) converges concurrently — the desktop's
// coverage-project shape.
func TestUp_SingleFile_ConcurrentLocales(t *testing.T) {
	for range 3 {
		a := demoProviderApp(t)
		dir := t.TempDir()
		real, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Join(real, "locales"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(real, "locales", "en.json"),
			[]byte(`{"greeting":"Hello","farewell":"Goodbye","cta":"Buy now"}`), 0o644))
		proj := &project.KapiProject{
			Version: "v1",
			Name:    "Coverage",
			Defaults: project.Defaults{
				SourceLanguage:  "en-US",
				TargetLanguages: []model.LocaleID{"fr-FR", "de-DE", "nb-NO", "ja-JP"},
			},
			Content: []project.ContentCollection{{
				Name:  "App",
				Items: []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
			}},
		}
		recipe := filepath.Join(real, "project.kapi")
		require.NoError(t, project.Save(recipe, proj))
		require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))

		out, upErr := runUp(t, a, recipe)
		require.NoError(t, upErr, out)
		assert.Contains(t, out, "Up to date: every gated scope is shippable", out)
		for _, loc := range []string{"fr-FR", "de-DE", "nb-NO", "ja-JP"} {
			_, serr := os.Stat(filepath.Join(real, "locales", loc+".json"))
			require.NoError(t, serr, "must materialize %s", loc)
		}
	}
}

// TestUp_BuiltinDefaultFlow_ConcurrentLocales: the built-in default flow
// (recycle → translate) converges multiple locales CONCURRENTLY within one
// pass — the path the desktop's "Bring up to date" takes. Regression guard for
// the per-locale fan-out: shared TM open, shared block-store sessions, and the
// shared parse cache must all tolerate concurrent locale workers (a failure
// here historically surfaced as a "send on closed channel" panic through
// ExecuteWithChannels' Begin-error path).
func TestUp_BuiltinDefaultFlow_ConcurrentLocales(t *testing.T) {
	for range 3 { // a few rounds — the failure was interleaving-dependent
		a := demoProviderApp(t)
		recipe, root := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE", "fr-FR", "ja-JP"}, gate.Gate{"translated": {Pct: 100}})
		proj, err := project.Load(recipe)
		require.NoError(t, err)
		proj.Defaults.Flow = ""
		proj.Flows = nil
		require.NoError(t, project.Save(recipe, proj))

		out, upErr := runUp(t, a, recipe)
		require.NoError(t, upErr, out)
		assert.Contains(t, out, "Up to date: every gated scope is shippable", out)
		_ = root
	}
}
