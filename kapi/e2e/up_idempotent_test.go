//go:build e2e

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/storage"
)

const upTwiceRecipe = `version: v1
name: up-twice-e2e
defaults:
  source_language: en
  target_languages: [fr, de]
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
  - name: docs
    path: docs/en/*.md
    target: docs/{lang}/*.md
`

const upTwiceGuide = `# Getting started

Install the tool, then run it in your project.

- Open the **settings** page.
- Choose a [language](https://example.com/languages).
`

// upTwiceHeldRecipe adds a language whose ship gate the demo provider's drafts
// cannot clear: nl needs established wording, which only a person gives. Its
// drafts wait for review in the work directory and no nl file is written.
const upTwiceHeldRecipe = `version: v1
name: up-twice-held-e2e
defaults:
  source_language: en
  target_languages: [fr, de, nl]
  materialize: on-converge
ship_gates:
  - when: {locales: [nl]}
    gate: {established: 100}
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
  - name: docs
    path: docs/en/*.md
    target: docs/{lang}/*.md
`

// upTwiceScopeHeldRecipe gates one collection of one language: nl/docs needs
// established wording and nl/app has no gate. A language is delivered only
// when every scope of it ships, so no nl file is written, and every nl draft,
// the app's included, waits for review.
const upTwiceScopeHeldRecipe = `version: v1
name: up-twice-scope-held-e2e
defaults:
  source_language: en
  target_languages: [nl]
  materialize: on-converge
ship_gates:
  - when: {collections: [docs], locales: [nl]}
    gate: {established: 100}
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
  - name: docs
    path: docs/en/*.md
    target: docs/{lang}/*.md
`

// A second `kapi up` over a project the first one converged does nothing: it
// changes no byte of the project tree, its state under .kapi/ included, or of
// the workspace under the data root, where the content memory, terms and
// context graph live. The one exception is the two "last opened" stamps of
// the workspace registry (workspaceStamps), so the registry database is
// compared row by row; every other file is compared byte for byte. A run that
// rewrites a target, a cache entry or a store with equal content but a new
// timestamp inside it fails here.
//
// The first run must have done the work and report what it produced. The
// second run's summary reports nothing produced. Where a language waits for
// review, the second run passes over it again and reports the drafts waiting,
// apart from produced.
func TestUpTwiceIsANoOp(t *testing.T) {
	t.Run("converged", func(t *testing.T) {
		first, second := upTwice(t, upTwiceRecipe, []string{"fr", "de"}, nil)
		require.True(t, first.Converged, "the first run converges")
		require.Len(t, first.Locales, 2)
		for _, l := range first.Locales {
			assert.Equal(t, 100, l.Pct.Translated, "%s is fully translated", l.Locale)
		}
		assert.Positive(t, first.Produced, "the first run's summary reports what it wrote")
		assert.Zero(t, first.WaitingForReview)

		assert.True(t, second.Converged, "the second run is converged")
		assert.Zero(t, second.Passes, "the second run has nothing to do")
		assert.Zero(t, second.Produced, "the second run's summary reports nothing produced")
		assert.Zero(t, second.WaitingForReview)
	})

	t.Run("drafts wait for review", func(t *testing.T) {
		first, second := upTwice(t, upTwiceHeldRecipe, []string{"fr", "de"}, []string{"nl"})
		require.False(t, first.Converged, "nl waits for review")
		require.Len(t, first.Locales, 3)
		assert.Positive(t, first.Produced, "the first run's summary reports what it wrote")
		require.Positive(t, first.WaitingForReview, "the first run holds the nl drafts for review")

		assert.False(t, second.Converged, "nl still waits for review")
		assert.Positive(t, second.Passes, "the second run passes over nl again")
		assert.Zero(t, second.Produced, "the second run's summary reports nothing produced")
		assert.Equal(t, first.WaitingForReview, second.WaitingForReview,
			"the second run's summary reports the same drafts waiting for review")
		for _, ev := range second.passes {
			assert.Zero(t, ev.ProducedDelta, "pass %d of the second run wrote nothing", ev.Pass)
			assert.Equal(t, first.WaitingForReview, ev.WaitingForReview, "pass %d reports the drafts waiting", ev.Pass)
		}
	})

	t.Run("one scope of a language waits for review", func(t *testing.T) {
		first, second := upTwice(t, upTwiceScopeHeldRecipe, nil, []string{"nl"})
		units := first.units["nl"]
		require.Positive(t, units)
		for _, run := range []*upTwiceResult{first, second} {
			assert.Zero(t, run.Produced, "no nl file is written, so nothing is produced")
			assert.Equal(t, units, run.WaitingForReview,
				"every nl draft waits for review, and none is also counted as produced")
		}
	})
}

// upTwiceResult is one run's result record and its pass_done events.
type upTwiceResult struct {
	Type             string `json:"type"`
	Pass             int    `json:"pass"`
	Passes           int    `json:"passes"`
	Converged        bool   `json:"converged"`
	Produced         int    `json:"produced"`
	ProducedDelta    int    `json:"produced_delta"`
	WaitingForReview int    `json:"waiting_for_review"`
	Locales          []struct {
		Locale string `json:"locale"`
		Pct    struct {
			Translated int `json:"translated"`
		} `json:"pct"`
	} `json:"locales"`
	Locale string `json:"locale"`
	Units  int    `json:"units"`
	passes []upTwiceResult
	// units is each locale's unit count, from its locale_done events.
	units map[string]int
}

// upTwice runs `kapi up` twice over a project with recipe and returns both
// runs' results. It asserts that the first run translated the targets of
// each delivered language, that no target of a withheld language exists after
// either run, and that the second run changed no file of the project or the
// data root.
func upTwice(t *testing.T, recipe string, delivered, withheld []string) (first, second *upTwiceResult) {
	t.Helper()
	dir, dataRoot, up := newUpProject(t, recipe)
	assertWithheld := func() {
		t.Helper()
		for _, lang := range withheld {
			for _, path := range []string{"src/messages_" + lang + ".json", "docs/" + lang + "/guide.md"} {
				assert.NoFileExists(t, filepath.Join(dir, path), "%s waits for review, so no file of it is written", lang)
			}
		}
	}

	first = up()
	require.GreaterOrEqual(t, first.Passes, 1, "the first run did a pass")
	assertTranslated(t, dir, delivered...)
	assertWithheld()
	beforeTree, beforeData, beforeRows := snapshotUpProject(t, dir, dataRoot)

	second = up()
	assertWithheld()
	assertUpProjectUnchanged(t, dir, dataRoot, beforeTree, beforeData, beforeRows)
	return first, second
}

// newUpProject writes a project with recipe, a JSON catalog and a Markdown
// guide, and returns its directory, its own data root, and a function that
// runs `kapi up` over it and returns the run's result.
func newUpProject(t *testing.T, recipe string) (dir, dataRoot string, up func() *upTwiceResult) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err = filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "en"), 0o755))
	source, err := os.ReadFile(filepath.Join(testdata, "messages_en.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "messages_en.json"), source, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "en", "guide.md"), []byte(upTwiceGuide), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(recipe), 0o644))

	// The built-in default flow translates with the configured provider; the
	// demo provider answers offline, with no key. The data root is this test's
	// own, so its snapshot sees only what these two runs wrote.
	up = func() *upTwiceResult {
		t.Helper()
		cmd := exec.Command(kapiBin, "up", "-p", "kapi.yaml", "--json")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...),
			"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=demo", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "kapi up failed:\n%s", out)
		var result *upTwiceResult
		var passes []upTwiceResult
		units := map[string]int{}
		for line := range strings.Lines(string(out)) {
			var e upTwiceResult
			if json.Unmarshal([]byte(line), &e) != nil {
				continue
			}
			switch e.Type {
			case "result":
				result = &e
			case "pass_done":
				passes = append(passes, e)
			case "locale_done":
				units[e.Locale] = e.Units
			}
		}
		require.NotNil(t, result, "the run reports a result:\n%s", out)
		result.passes, result.units = passes, units
		return result
	}
	return dir, dataRoot, up
}

// assertTranslated checks that each language's targets exist and carry the
// demo provider's translated text.
func assertTranslated(t *testing.T, dir string, langs ...string) {
	t.Helper()
	for _, lang := range langs {
		for _, path := range []string{"src/messages_" + lang + ".json", "docs/" + lang + "/guide.md"} {
			content, err := os.ReadFile(filepath.Join(dir, path))
			require.NoError(t, err)
			assert.Contains(t, string(content), "⟦"+lang+"⟧", "%s carries translated text", path)
		}
	}
}

// snapshotUpProject records the project tree, the data root and the rows of
// the workspace registry.
func snapshotUpProject(t *testing.T, dir, dataRoot string) (tree, data map[string]string, rows map[string][]string) {
	t.Helper()
	tree, data = snapshotTree(t, dir), snapshotTree(t, dataRoot)
	require.Contains(t, data, workspaceDB, "the run wrote its workspace under the data root")
	rows = workspaceRows(t, filepath.Join(dataRoot, workspaceDB))
	require.NotEmpty(t, rows["workspace_projects"], "the run registered the project")
	return tree, data, rows
}

// assertUpProjectUnchanged checks that a run changed no file of the project
// or the data root, and no row of the workspace registry beyond its
// last-opened stamps.
func assertUpProjectUnchanged(t *testing.T, dir, dataRoot string, beforeTree, beforeData map[string]string, beforeRows map[string][]string) {
	t.Helper()
	assertUnchanged(t, "project", beforeTree, snapshotTree(t, dir))
	afterData := snapshotTree(t, dataRoot)
	delete(beforeData, workspaceDB)
	delete(afterData, workspaceDB)
	assertUnchanged(t, "data root", beforeData, afterData)
	assert.Equal(t, beforeRows, workspaceRows(t, filepath.Join(dataRoot, workspaceDB)),
		"the run changed a row of the workspace registry beyond its last-opened stamps")
}

// A language held for review on one run and released on the next (here the
// gate is removed, as a reviewer's approval would clear it) is delivered by
// the second run after its loop, from the drafts the first run made. The
// second run reports those units as produced and nothing waiting; a third
// run reports nothing and changes nothing.
func TestUpDeliversHeldDraftsWhenReleased(t *testing.T) {
	dir, dataRoot, up := newUpProject(t, upTwiceHeldRecipe)
	nlFiles := []string{"src/messages_nl.json", "docs/nl/guide.md"}

	first := up()
	require.Positive(t, first.WaitingForReview, "nl waits for review")
	for _, path := range nlFiles {
		require.NoFileExists(t, filepath.Join(dir, path))
	}
	units := first.units["nl"]
	require.Positive(t, units)

	released := strings.Replace(upTwiceHeldRecipe, "ship_gates:\n  - when: {locales: [nl]}\n    gate: {established: 100}\n", "", 1)
	require.NotEqual(t, upTwiceHeldRecipe, released)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(released), 0o644))

	second := up()
	assertTranslated(t, dir, "nl")
	assert.Equal(t, units, second.Produced, "the nl units delivered after the loop are produced")
	assert.Zero(t, second.WaitingForReview)
	beforeTree, beforeData, beforeRows := snapshotUpProject(t, dir, dataRoot)

	third := up()
	assert.Zero(t, third.Produced)
	assert.Zero(t, third.WaitingForReview)
	assertUpProjectUnchanged(t, dir, dataRoot, beforeTree, beforeData, beforeRows)
}

// assertUnchanged reports every file the second run created, removed or
// changed under one root.
func assertUnchanged(t *testing.T, root string, before, after map[string]string) {
	t.Helper()
	for path, sum := range before {
		if got, ok := after[path]; !ok {
			t.Errorf("the second run removed %s from the %s", path, root)
		} else if got != sum {
			t.Errorf("the second run changed %s in the %s", path, root)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("the second run created %s in the %s", path, root)
		}
	}
}

// workspaceStamps are the columns of the workspace registry that record when
// a project was last opened: workspace_projects.last_active and
// workspace_checkouts.seen_at. Every run opens the project, so every run
// stamps them; core/workspace writes them on purpose without moving the
// operation log, so that looking at a project changes nothing a reader of the
// workspace answers from. They are the only values this test lets change.
var workspaceStamps = map[string]string{
	"workspace_projects":  "last_active",
	"workspace_checkouts": "seen_at",
}

// workspaceDB is the registry database under the data root, compared by its
// rows rather than its bytes because of workspaceStamps.
const workspaceDB = "workspaces/default/workspace.db"

// workspaceRows reads every row of every table of the database at path, with
// the workspaceStamps columns blanked, from a copy so the original is never
// opened.
func workspaceRows(t *testing.T, path string) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	if _, err := os.Stat(path + "-wal"); err == nil {
		t.Fatalf("%s has an uncheckpointed log; compare after the process that wrote it exits", path)
	}
	cp := filepath.Join(t.TempDir(), "workspace.db")
	require.NoError(t, os.WriteFile(cp, data, 0o600))
	db, err := storage.OpenReadOnly(cp)
	require.NoError(t, err)
	defer db.Close()

	var tables []string
	rows, err := db.QueryContext(t.Context(), `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	require.NoError(t, err)
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	out := map[string][]string{}
	for _, table := range tables {
		rs, err := db.QueryContext(t.Context(), `SELECT * FROM "`+table+`"`)
		require.NoError(t, err)
		cols, err := rs.Columns()
		require.NoError(t, err)
		for rs.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			require.NoError(t, rs.Scan(ptrs...))
			fields := make([]string, len(cols))
			for i, c := range cols {
				if workspaceStamps[table] == c {
					fields[i] = c + "=<stamp>"
					continue
				}
				fields[i] = fmt.Sprintf("%s=%v", c, vals[i])
			}
			out[table] = append(out[table], strings.Join(fields, " "))
		}
		require.NoError(t, rs.Err())
		require.NoError(t, rs.Close())
		slices.Sort(out[table])
	}
	return out
}

// snapshotTree maps every regular file under root to the SHA-256 of its bytes.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		sums[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	require.NoError(t, err)
	return sums
}
