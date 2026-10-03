package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/project"
)

// TestChangeService_Conformance runs the conformance suite every home passes
// (core/change/changetest) against the service the host builds for a
// project: the file home over the recipe's layout, with the commit check, the
// policy and the recorder plugged in, so every edit that lands is also
// recorded in the workspace's log. `make test-wasm-stores` runs it under
// GOOS=js, where the files are the browser engine's and the log lives in the
// browser's SQLite. The service holds a sender between its stage and its
// commit at ChangeServiceOptions.BeforeSettle, so the cases that interleave
// two senders run here too.
func TestChangeService_Conformance(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		a, recipe := changeProject(t, project.ContentItem{Path: "docs/*.json"}, map[string]string{
			"docs/a.json": `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			"docs/b.json": `{"title": "Welcome"}` + "\n",
		})
		t.Cleanup(a.Shutdown)
		root := filepath.Dir(recipe)
		var hook func(string)
		svc, err := a.ChangeService(t.Context(), ChangeServiceOptions{Project: recipe, Origin: "test", BeforeSettle: func(doc string) {
			if hook != nil {
				hook(doc)
			}
		}})
		require.NoError(t, err)
		return changetest.Env{
			Service:         svc,
			SetBeforeSettle: func(fn func(string)) { hook = fn },
			DocA:            "docs/a.json",
			DocB:            "docs/b.json",
			Snapshot: func(t *testing.T, doc string) []byte {
				return []byte(readFile(t, recipe, doc))
			},
			Mode: func(t *testing.T, doc string) os.FileMode {
				info, err := os.Stat(filepath.Join(root, filepath.FromSlash(doc)))
				require.NoError(t, err)
				return info.Mode().Perm()
			},
		}
	})
}
