package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// explainProjectFixture writes a `.kapi` project with a JSON content pattern
// (per-locale target template), a project-defined "sidecars" flow (one
// recycle step — the shape of the dogfood tm-recycle flow from #1295) and a
// "pseudo" flow (offline pseudo-translate, used as the executing control).
// Returns the recipe path and the resolved project root.
func explainProjectFixture(t *testing.T, targets []model.LocaleID) (recipe, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe = filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ExplainTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: targets,
		},
		Collections: []project.Collection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "src/locales/{lang}/*.json",
			},
		},
		Flows: map[string]*flow.StepsSpec{
			"sidecars": {
				Steps: []flow.FlowStep{
					{Tool: "recycle", Config: map[string]any{"fillTarget": true, "fillTargetThreshold": 100}},
				},
			},
			"pseudo": {
				Steps: []flow.FlowStep{
					{Tool: "pseudo-translate"},
				},
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))

	writeJSONSource(t, real, "src/locales/en/a.json", `{"greeting":"Hello, world."}`)
	writeJSONSource(t, real, "src/locales/en/b.json", `{"farewell":"Goodbye."}`)
	return recipe, real
}

// assertNoRunSideEffects asserts the project tree carries none of the
// artefacts a real run would produce: no per-locale output files and no
// project cache (block store / parse cache under .kapi/work/cache).
func assertNoRunSideEffects(t *testing.T, root string, locales ...string) {
	t.Helper()
	for _, l := range locales {
		p := filepath.Join(root, "src", "locales", l)
		_, err := os.Stat(p)
		assert.True(t, os.IsNotExist(err), "explain must not write output files, found %s", p)
	}
	cache := project.LayoutAt(root).CacheDir()
	_, err := os.Stat(cache)
	assert.True(t, os.IsNotExist(err), "explain must not create the project cache (%s)", cache)
	// Every subsystem shares the one store now, and opening it runs their
	// migrations — so "must not create the content memory" is the same
	// assertion as "must not create the store", and only the latter can
	// still fail.
	store := project.LayoutAt(root).StorePath()
	_, err = os.Stat(store)
	assert.True(t, os.IsNotExist(err), "explain must not create the project store (%s)", store)
}

// TestRunProjectFlow_ExplainIsDryRun reproduces #1295: `kapi run
// <project-flow> --explain` must print the plan and return without executing
// tools or writing any output — including the multi-file, all-locales
// project path (no --target-lang → one pass per project target).
func TestRunProjectFlow_ExplainIsDryRun(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := explainProjectFixture(t, []model.LocaleID{"fr-FR", "de-DE"})

	out, err := runRunCmd(t, a, recipe, "sidecars", "--explain")
	require.NoError(t, err, "output: %s", out)

	// The plan uses the source → sink binding vocabulary explainBindings
	// renders, one line per resolved input, plus the locale passes.
	assert.Contains(t, out, "flow sidecars: file("+filepath.Join(root, "src", "locales", "en", "a.json")+") → file")
	assert.Contains(t, out, "flow sidecars: file("+filepath.Join(root, "src", "locales", "en", "b.json")+") → file")
	assert.Contains(t, out, "locales: fr-FR, de-DE")

	assertNoRunSideEffects(t, root, "fr-FR", "de-DE")
}

// TestRunProjectFlow_ExplainSingleInput covers the single-input variant: an
// explicit -i plus --target-lang with --explain plans that one file and
// writes nothing (not even the process-only block store).
func TestRunProjectFlow_ExplainSingleInput(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := explainProjectFixture(t, []model.LocaleID{"fr-FR"})
	src := filepath.Join(root, "src", "locales", "en", "a.json")

	out, err := runRunCmd(t, a, recipe, "sidecars", "-i", src, "--target-lang", "fr-FR", "--explain")
	require.NoError(t, err, "output: %s", out)

	assert.Contains(t, out, "flow sidecars: file("+src+") → file")
	assert.Contains(t, out, "locales: fr-FR")

	assertNoRunSideEffects(t, root, "fr-FR")
}

// TestRunProjectFlow_ExplainHonorsOutputFlag: an explicit -o shows up as the
// resolved sink, still without writing it.
func TestRunProjectFlow_ExplainHonorsOutputFlag(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := explainProjectFixture(t, []model.LocaleID{"fr-FR"})
	src := filepath.Join(root, "src", "locales", "en", "a.json")
	dst := filepath.Join(root, "out.json")

	out, err := runRunCmd(t, a, recipe, "sidecars", "-i", src, "-o", dst, "--target-lang", "fr-FR", "--explain")
	require.NoError(t, err, "output: %s", out)

	assert.Contains(t, out, "flow sidecars: file("+src+") → file("+dst+")")
	_, statErr := os.Stat(dst)
	assert.True(t, os.IsNotExist(statErr), "explain must not write -o output")
}

// TestRunProjectFlow_WithoutExplainWrites is the control: the same fixture
// without --explain executes the flow and writes the per-locale outputs —
// proving the dry-run assertions above would catch a real execution.
func TestRunProjectFlow_WithoutExplainWrites(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := explainProjectFixture(t, []model.LocaleID{"fr-FR"})

	out, err := runRunCmd(t, a, recipe, "pseudo", "--target-lang", "fr-FR")
	require.NoError(t, err, "output: %s", out)

	for _, name := range []string{"a.json", "b.json"} {
		p := filepath.Join(root, "src", "locales", "fr-FR", name)
		_, statErr := os.Stat(p)
		assert.NoError(t, statErr, "expected the real run to write %s", p)
	}
}
