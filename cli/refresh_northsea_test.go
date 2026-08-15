package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Context refresh — the second visit. A project that already carries a
// committed context points its assistant at the repository again, because the
// material moved: a surface appeared that the recipe never declared, and the
// new material calls a product by a name the vocabulary has never been told
// about.
//
// The property under test is that refresh is a PROPOSAL. Reading the drift
// writes nothing; the recipe, the voice profile and the vocabulary change only
// where an approved change-set says so, and each change reaches the gate.
//
// The fixture is samples/northsea itself rather than a hand-written recipe, so
// the flow the skill teaches is exercised against the sample a reader follows;
// a sample that stops supporting it fails here.

// northseaCopy copies the committed sample into a throwaway directory and
// returns the recipe path and root. The copy runs under the dogfood isolation
// contract (CLAUDE.md): every root this run could otherwise inherit is pinned
// to a throwaway dir and project discovery is off.
func northseaCopy(t *testing.T) (recipe, root string) {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "samples", "northsea"))
	require.NoError(t, err)

	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	root = filepath.Join(real, "northsea")
	require.NoError(t, os.CopyFS(root, os.DirFS(src)))

	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	// A voice-store write with no --name/--file resolves to the WORKING
	// directory (host.resolveResourcePath), which for a test is the package
	// directory. Standing in the copy is also how a user runs this.
	t.Chdir(root)

	return filepath.Join(root, "kapi.yaml"), root
}

// seedDrift is the second visit's material: a support surface Northsea
// launched after the recipe was written, written in the vocabulary the company
// now uses. "Tideguard" is the name the record has never been told about.
func seedDrift(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "support"), 0o755))
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, "support", name), []byte(body), 0o644))
	}
	write("faq.md", `# Support — common questions

## Why did Tideguard raise an alert for a movement I already approved?

Tideguard compares the forecast with the constraints of the berth continuously,
so an approved movement is re-evaluated whenever the forecast changes.

## Can I subscribe to a berth rather than a vessel?

Subscriptions follow the vessel. A vessel that is reassigned keeps its
subscribers, so the duty officer who was watching it still hears from Tideguard.
`)
	write("escalation.md", `# Support — escalation

An alert that nobody acknowledges within the escalation window is repeated to
the terminal supervisor. Tideguard records every repeat with the reading that
raised it.
`)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// TestRefresh_NorthseaDrift walks the refresh flow end to end on the sample:
// read the drift, propose, apply what was approved, verify at the gate.
func TestRefresh_NorthseaDrift(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := northseaCopy(t)

	upOut, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upOut)

	seedDrift(t, root)

	governance := map[string]string{
		recipe: readFile(t, recipe),
		filepath.Join(root, ".kapi", "voice.yaml"): readFile(t, filepath.Join(root, ".kapi", "voice.yaml")),
		filepath.Join(root, ".kapi", "terms.json"): readFile(t, filepath.Join(root, ".kapi", "terms.json")),
	}

	// --- Propose. Every read below is part of drafting the change-set.
	untracked, err := runCLI(t, NewLsCmd(a), "--project", recipe, "--untracked")
	require.NoError(t, err, untracked)
	assert.Contains(t, untracked, "support/faq.md")
	assert.Contains(t, untracked, "support/escalation.md")
	assert.Contains(t, untracked, "README.md",
		"the report is what is undeclared, not what deserves declaring — the reviewer decides which")
	assert.NotContains(t, untracked, "docs/berths.md", "content a collection already governs is not a discovery")
	assert.NotContains(t, untracked, "kapi.yaml", "the recipe is governance, not an undeclared surface")

	baseline, err := runCLI(t, NewContextCmd(a), "search", "Tidewatch", "--project", recipe)
	require.NoError(t, err, baseline)
	assert.Contains(t, baseline, "Tidewatch", "the baseline is what the record says today")

	newName, err := runCLI(t, NewContextCmd(a), "search", "Tideguard", "--project", recipe)
	require.NoError(t, err, newName)
	assert.NotContains(t, newName, "preferred",
		"the name the new material uses is not in the record yet — that is the delta to propose")

	for path, before := range governance {
		assert.Equal(t, before, readFile(t, path),
			"proposing changed %s; a refresh proposes and never overwrites", filepath.Base(path))
	}

	// --- Approve. The user accepts the new surface at the documentation point.
	addOut, err := runCLI(t, NewAddCmd(a), "support/**/*.md",
		"--project", recipe, "--name", "northsea-support", "--channel", "northsea/docs")
	require.NoError(t, err, addOut)

	recipeNow := readFile(t, recipe)
	assert.Contains(t, recipeNow, "northsea-support")
	assert.Contains(t, recipeNow, "# Northsea — one product, one language, three surfaces.",
		"the recipe is edited rather than re-emitted, so its comments survive an approved change")

	nowTracked, err := runCLI(t, NewLsCmd(a), "--project", recipe, "--untracked", "support")
	require.NoError(t, err, nowTracked)
	assert.Contains(t, nowTracked, "Every readable file is tracked by a collection.")

	// The file the reviewer declined stays where it was: undeclared, and still
	// reported. Nothing is adopted because it was discovered.
	declined, err := runCLI(t, NewLsCmd(a), "--project", recipe, "--untracked")
	require.NoError(t, err, declined)
	assert.Contains(t, declined, "README.md")

	// --- Approve. The rename lands as term and voice entries in one change-set.
	changeset := filepath.Join(root, "refresh.jsonl")
	require.NoError(t, os.WriteFile(changeset, []byte(
		`{"kind":"term","op":"upsert","term":"Tideguard","locale":"en-GB","status":"preferred"}`+"\n"+
			`{"kind":"term","op":"upsert","term":"Tidewatch","locale":"en-GB","status":"deprecated","replacement":"Tideguard"}`+"\n"+
			`{"kind":"voice","op":"add-rule","list":"preferred","term":"Tideguard"}`+"\n"), 0o644))

	applyOut, err := runCLI(t, NewApplyCmd(a), changeset, "--project", recipe)
	require.NoError(t, err, applyOut)
	assert.NotContains(t, applyOut, "error", applyOut)

	terms := readFile(t, filepath.Join(root, ".kapi", "terms.json"))
	assert.Contains(t, terms, "Tideguard")
	assert.Contains(t, terms, "deprecated")

	voice := readFile(t, filepath.Join(root, ".kapi", "voice.yaml"))
	assert.Contains(t, voice, "Tideguard")
	assert.Contains(t, voice, "# Northsea house voice.",
		"the voice profile is edited rather than re-emitted, so its comments survive an approved change")

	// --- Verify. A newly retired name starts flagging the surfaces that still
	// carry it, which is what makes the applied decision observable.
	upAgain, err := runCLI(t, NewUpCmd(a), "--project", recipe)
	require.NoError(t, err, upAgain)

	checkOut, err := runCLI(t, NewCheckCmd(a), "--project", recipe, "--no-fail")
	require.NoError(t, err, checkOut)
	assert.Contains(t, checkOut, "Tidewatch", "the retired name is reported where the old surfaces still use it")
}
