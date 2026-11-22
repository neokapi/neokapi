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

// A second `kapi up` over a project the first one converged does nothing: it
// reports zero passes and changes no byte of the project tree, its state
// under .kapi/ included, or of the workspace under the data root, where the
// content memory, terms and context graph live. The one exception is the two
// "last opened" stamps of the workspace registry (workspaceStamps), so the
// registry database is compared row by row; every other file is compared
// byte for byte. A run that rewrites a target, a cache entry or a store with
// equal content but a new timestamp inside it fails here.
//
// The first run must have done the work: every target exists, carries
// translated text (the demo provider marks it with the language, "⟦fr⟧"), and
// the run reports what it produced.
func TestUpTwiceIsANoOp(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "en"), 0o755))
	source, err := os.ReadFile(filepath.Join(testdata, "messages_en.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "messages_en.json"), source, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "en", "guide.md"), []byte(upTwiceGuide), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(upTwiceRecipe), 0o644))

	// The built-in default flow translates with the configured provider; the
	// demo provider answers offline, with no key. The data root is this test's
	// own, so its snapshot sees only what these two runs wrote.
	up := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(kapiBin, append([]string{"up", "-p", "kapi.yaml", "--json"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...),
			"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=demo", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "kapi up failed:\n%s", out)
		return string(out)
	}
	type upEvent struct {
		Type      string `json:"type"`
		Passes    int    `json:"passes"`
		Converged bool   `json:"converged"`
		Produced  int    `json:"produced"`
		Locales   []struct {
			Locale string `json:"locale"`
			Pct    struct {
				Translated int `json:"translated"`
			} `json:"pct"`
		} `json:"locales"`
	}
	events := func(out string) (result *upEvent, produced int) {
		for line := range strings.Lines(out) {
			var e upEvent
			if json.Unmarshal([]byte(line), &e) != nil {
				continue
			}
			switch e.Type {
			case "result":
				result = &e
			case "pass_done":
				produced = max(produced, e.Produced)
			}
		}
		return result, produced
	}

	first, produced := events(up())
	require.NotNil(t, first, "the first run reports a result")
	require.True(t, first.Converged, "the first run converges")
	require.GreaterOrEqual(t, first.Passes, 1, "the first run did a pass")
	require.Positive(t, produced, "the first run produced translations")
	require.Len(t, first.Locales, 2)
	for _, l := range first.Locales {
		assert.Equal(t, 100, l.Pct.Translated, "%s is fully translated", l.Locale)
	}
	for _, target := range []struct{ path, lang string }{
		{"src/messages_fr.json", "fr"}, {"src/messages_de.json", "de"},
		{"docs/fr/guide.md", "fr"}, {"docs/de/guide.md", "de"},
	} {
		content, err := os.ReadFile(filepath.Join(dir, target.path))
		require.NoError(t, err)
		assert.Contains(t, string(content), "⟦"+target.lang+"⟧", "%s carries translated text", target.path)
	}
	beforeTree, beforeData := snapshotTree(t, dir), snapshotTree(t, dataRoot)
	require.Contains(t, beforeData, workspaceDB, "the run wrote its workspace under the data root")
	beforeRows := workspaceRows(t, filepath.Join(dataRoot, workspaceDB))
	require.NotEmpty(t, beforeRows["workspace_projects"], "the run registered the project")

	second, _ := events(up())
	require.NotNil(t, second, "the second run reports a result")
	assert.True(t, second.Converged, "the second run is converged")
	assert.Zero(t, second.Passes, "the second run has nothing to do")

	assertUnchanged(t, "project", beforeTree, snapshotTree(t, dir))
	afterData := snapshotTree(t, dataRoot)
	delete(beforeData, workspaceDB)
	delete(afterData, workspaceDB)
	assertUnchanged(t, "data root", beforeData, afterData)
	assert.Equal(t, beforeRows, workspaceRows(t, filepath.Join(dataRoot, workspaceDB)),
		"the second run changed a row of the workspace registry beyond its last-opened stamps")
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
