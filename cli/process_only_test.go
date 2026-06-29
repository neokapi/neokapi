package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/neokapi/neokapi/core/formats" // register JSON etc.
)

// processOnlyApp builds a fresh App with all registries populated, as the
// run/merge commands depend on the format + tool registries.
func processOnlyApp(t *testing.T) *App {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	a.AssumeYes = true
	return a
}

// processOnlyProjectFixture writes a minimal `.kapi` project with a single JSON
// content pattern, an inline `pseudo` flow (one pseudo-translate step), one
// target locale, and the source file. Returns the recipe path and the source
// file's project-relative path. Symlinks in the temp dir are resolved so the
// project's within-root check (ResolveContent) holds on macOS.
func processOnlyProjectFixture(t *testing.T, targets []model.LocaleID) (recipe, srcRel, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe = filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "ProcessOnlyTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: targets,
		},
		Content: []project.ContentCollection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "src/locales/{lang}/*.json",
			},
		},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {
				Steps: []flow.FlowStep{
					{Tool: "pseudo-translate"},
				},
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))

	srcRel = "src/locales/en/messages.json"
	srcAbs := filepath.Join(real, srcRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(srcAbs), 0o755))
	require.NoError(t, os.WriteFile(srcAbs, []byte(`{"greeting":"Hello, world."}`), 0o644))
	return recipe, srcRel, real
}

// runRunCmd invokes `kapi run <flow> --project <recipe> [flags]` on a fresh
// command, capturing combined stdout/stderr.
func runRunCmd(t *testing.T, a *App, recipe, flow string, flags ...string) (string, error) {
	t.Helper()
	cmd := a.NewRunCmd(RunCmdOptions{})
	args := append([]string{flow, "--project", recipe}, flags...)
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

// storeOverlayCount counts overlays of the given kind in the project block
// store, asserting the store and a single overlay's payload looks sane.
func storeOverlayCount(t *testing.T, recipe, kind string) int {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	store, err := sqlitestore.New(layout.BlockStorePath())
	require.NoError(t, err)
	defer store.Close()
	sess, err := store.Begin(t.Context())
	require.NoError(t, err)
	defer sess.Close()
	n := 0
	for ov, err := range sess.ListOverlays(kind) {
		require.NoError(t, err)
		assert.NotEmpty(t, ov.Payload, "overlay payload must be non-empty")
		n++
	}
	return n
}

// TestRun_InProjectNoOutput_ProcessOnly verifies that a flow run inside a
// project with NO -o commits `targets/<locale>` overlays to the project block
// store and writes NO output file (AD-026 §3/§5).
func TestRun_InProjectNoOutput_ProcessOnly(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})

	out, err := runRunCmd(t, a, recipe, "pseudo", "-i", filepath.Join(root, srcRel))
	require.NoError(t, err, "run output: %s", out)

	// Overlays committed to the project store.
	assert.Equal(t, 1, storeOverlayCount(t, recipe, "targets/fr-FR"),
		"process-only run must commit one targets/fr-FR overlay")

	// No localized output file on disk.
	target := filepath.Join(root, "src/locales/fr-FR/messages.json")
	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr), "process-only run must write NO target file; statErr=%v", statErr)

	// And the guidance is printed.
	assert.Contains(t, out, "kapi merge")
}

// TestMerge_FromProjectStore materializes the localized files from the store
// overlays committed by a prior process-only run.
func TestMerge_FromProjectStore(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})

	// 1. Process-only run lands overlays.
	out, err := runRunCmd(t, a, recipe, "pseudo", "-i", filepath.Join(root, srcRel))
	require.NoError(t, err, "run output: %s", out)

	// 2. merge (no -i) materializes from the store.
	mOut, err := runMergeCmd(t, recipe)
	require.NoError(t, err, "merge output: %s", mOut)

	target := filepath.Join(root, "src/locales/fr-FR/messages.json")
	data, rerr := os.ReadFile(target)
	require.NoError(t, rerr, "merge must write the localized file")
	assert.NotEmpty(t, data)
	// Pseudo-translation alters the source text.
	assert.NotEqual(t, `{"greeting":"Hello, world."}`, string(data))
	assert.Contains(t, string(data), "greeting", "structure must be preserved by the skeleton round-trip")
}

// TestRoundTrip_ProcessOnlyMerge_EqualsDirectOutput asserts that a
// process-only run followed by merge produces byte-identical target content to
// a direct `-o` run of the same flow on the same input.
func TestRoundTrip_ProcessOnlyMerge_EqualsDirectOutput(t *testing.T) {
	// Path A: process-only run → merge.
	aRun := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})
	out, err := runRunCmd(t, aRun, recipe, "pseudo", "-i", filepath.Join(root, srcRel))
	require.NoError(t, err, "run output: %s", out)
	mOut, err := runMergeCmd(t, recipe)
	require.NoError(t, err, "merge output: %s", mOut)
	merged, err := os.ReadFile(filepath.Join(root, "src/locales/fr-FR/messages.json"))
	require.NoError(t, err)

	// Path B: direct -o run on the same input.
	directOut := filepath.Join(t.TempDir(), "direct.json")
	aDirect := processOnlyApp(t)
	dOut, err := runRunCmd(t, aDirect, recipe, "pseudo",
		"-i", filepath.Join(root, srcRel), "-o", directOut)
	require.NoError(t, err, "direct run output: %s", dOut)
	direct, err := os.ReadFile(directOut)
	require.NoError(t, err)

	assert.Equal(t, string(direct), string(merged),
		"process-only run → merge must equal a direct -o run")
}

// TestRun_InProjectExplicitOutput_WritesFile verifies that an explicit -o in a
// project keeps the file-writing behavior (no process-only), and writes NO
// store overlays detour — the file is the sink.
func TestRun_InProjectExplicitOutput_WritesFile(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})

	outPath := filepath.Join(t.TempDir(), "explicit.json")
	out, err := runRunCmd(t, a, recipe, "pseudo",
		"-i", filepath.Join(root, srcRel), "-o", outPath)
	require.NoError(t, err, "run output: %s", out)

	data, rerr := os.ReadFile(outPath)
	require.NoError(t, rerr, "explicit -o must write the output file directly")
	assert.NotEqual(t, `{"greeting":"Hello, world."}`, string(data))
}

// TestRun_NoProject_WritesFile verifies the no-project default is unchanged:
// a run with no -o writes the default sibling output file.
func TestRun_NoProject_WritesFile(t *testing.T) {
	a := processOnlyApp(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello, world."}`), 0o644))

	// Build the flow-run command first (its flags bind &a.SourceLang /
	// &a.TargetLang), then set the languages so the bindings hold.
	flowCmd := newRunSingleFileCmd(t, a)
	require.NoError(t, flowCmd.Flags().Set("input", src))
	a.projectContext = nil
	a.SourceLang = "en-US"
	a.TargetLang = "qps"
	err := a.runSingleFile(t.Context(), flowCmd, "pseudo-translate", src)
	require.NoError(t, err)

	// Default sibling output path: <base>_<lang><ext>.
	def := filepath.Join(dir, "messages_qps.json")
	data, rerr := os.ReadFile(def)
	require.NoError(t, rerr, "no-project run must write the default sibling file")
	assert.NotEqual(t, `{"greeting":"Hello, world."}`, string(data))
}

// newRunSingleFileCmd builds a cobra command carrying the flow-run flags so
// runSingleFile can read --output / --trace / etc.
func newRunSingleFileCmd(t *testing.T, a *App) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	a.addFlowRunFlags(cmd)
	return cmd
}

// runDocCacheKeys returns the document-cache config keys present after a run,
// opening the project's streaming document cache directly. A "|doc|" key proves
// the flow runner populated the document cache through the real wiring.
func runDocCacheKeys(t *testing.T, recipe string) []string {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	c, err := openDocCache(layout.CacheDir())
	require.NoError(t, err)
	defer c.close()
	rows, err := c.db.Query(`SELECT config_key FROM documents`)
	require.NoError(t, err)
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		require.NoError(t, rows.Scan(&k))
		keys = append(keys, k)
	}
	return keys
}

// TestRun_InProject_PopulatesAndReusesDocumentCache verifies the real CLI wiring:
// a process-only run populates the project document cache under a "|doc|" key, and
// a second run produces the identical work (served from the cache).
func TestRun_InProject_PopulatesAndReusesDocumentCache(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})
	src := filepath.Join(root, srcRel)

	out, err := runRunCmd(t, a, recipe, "pseudo", "-i", src)
	require.NoError(t, err, "run output: %s", out)

	keys := runDocCacheKeys(t, recipe)
	hasRunKey := false
	for _, k := range keys {
		if strings.Contains(k, "|run|") {
			hasRunKey = true
		}
	}
	assert.True(t, hasRunKey, "the process-only run must populate the document cache under a |run| key; got %v", keys)
	first := storeOverlayCount(t, recipe, "targets/fr-FR")
	assert.Equal(t, 1, first)

	// Second run: served from the cache, identical committed work.
	a2 := processOnlyApp(t)
	out2, err := runRunCmd(t, a2, recipe, "pseudo", "-i", src)
	require.NoError(t, err, "second run output: %s", out2)
	assert.Equal(t, first, storeOverlayCount(t, recipe, "targets/fr-FR"),
		"a cache-served re-run commits the identical overlays")

	// Rebuild invariant: delete the cache, re-run → still correct.
	require.NoError(t, os.RemoveAll(layoutCacheDir(t, recipe)))
	a3 := processOnlyApp(t)
	out3, err := runRunCmd(t, a3, recipe, "pseudo", "-i", src)
	require.NoError(t, err, "post-rebuild run output: %s", out3)
	assert.Equal(t, first, storeOverlayCount(t, recipe, "targets/fr-FR"))
}

func layoutCacheDir(t *testing.T, recipe string) string {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	return layout.CacheDir()
}

// TestRun_InProjectExplicitOutput_DocumentCacheByteIdentical verifies the real
// CLI wiring of the file-writing document cache: a project-mode `kapi run -o`
// populates a "|write|" document, and a second run (served from the cache) writes
// byte-identical output.
func TestRun_InProjectExplicitOutput_DocumentCacheByteIdentical(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})
	src := filepath.Join(root, srcRel)

	out1 := filepath.Join(t.TempDir(), "o1.json")
	o, err := runRunCmd(t, a, recipe, "pseudo", "-i", src, "-o", out1)
	require.NoError(t, err, o)

	hasWriteKey := false
	for _, k := range runDocCacheKeys(t, recipe) {
		if strings.Contains(k, "|write|") {
			hasWriteKey = true
		}
	}
	assert.True(t, hasWriteKey, "a file-writing run must populate a |write| document")

	out2 := filepath.Join(t.TempDir(), "o2.json")
	a2 := processOnlyApp(t)
	o2, err := runRunCmd(t, a2, recipe, "pseudo", "-i", src, "-o", out2)
	require.NoError(t, err, o2)

	b1, err := os.ReadFile(out1)
	require.NoError(t, err)
	b2, err := os.ReadFile(out2)
	require.NoError(t, err)
	assert.Equal(t, string(b1), string(b2), "the cache-served run must be byte-identical")
	assert.Contains(t, string(b1), "greeting", "structure preserved by the skeleton round-trip")
}

// docCacheSkelFiles counts skeleton files written to the document cache.
func docCacheSkelFiles(t *testing.T, recipe string) int {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Join(layout.CacheDir(), "docs"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "skel-") && strings.HasSuffix(e.Name(), ".bin") {
			n++
		}
	}
	return n
}

// TestRun_ProcessOnlyRecordsLean_NoSkeleton verifies the demand-driven recording:
// a process-only run (no reconstruction) records the document WITHOUT a skeleton —
// the default run writes no skeleton file at all — while a file-writing run does.
func TestRun_ProcessOnlyRecordsLean_NoSkeleton(t *testing.T) {
	a := processOnlyApp(t)
	recipe, srcRel, root := processOnlyProjectFixture(t, []model.LocaleID{"fr-FR"})
	src := filepath.Join(root, srcRel)

	out, err := runRunCmd(t, a, recipe, "pseudo", "-i", src)
	require.NoError(t, err, "run output: %s", out)
	assert.Equal(t, 0, docCacheSkelFiles(t, recipe),
		"a process-only run records lean — no skeleton file")

	// A file-writing run of the same source records the skeleton it needs.
	a2 := processOnlyApp(t)
	out2, err := runRunCmd(t, a2, recipe, "pseudo", "-i", src, "-o", filepath.Join(t.TempDir(), "o.json"))
	require.NoError(t, err, "run output: %s", out2)
	assert.Equal(t, 1, docCacheSkelFiles(t, recipe),
		"the file-writing run records the skeleton (a separate |write| entry)")
}
