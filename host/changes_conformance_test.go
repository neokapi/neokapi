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
			// A catalog whose French translation is the catalog its target
			// template names: the suite removes the French from po/fr.po, and
			// po/en.po keeps its bytes.
			"po/en.po": conformanceCatalog("en", ""),
			"po/fr.po": conformanceCatalog("fr", "Bonjour"),
			// A JSON catalog whose French translation is a file of its own:
			// the suite removes the French greeting from locales/fr.json.
			"locales/en.json": `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n",
			"locales/fr.json": `{"greeting": "Bonjour", "farewell": "Au revoir"}` + "\n",
		}, func(p *project.KapiProject) {
			p.Collections[0].Content = append(p.Collections[0].Content, project.ContentItem{
				Path: "po/en.po", Format: &project.FormatSpec{Name: "po"}, Target: "po/{lang}.po",
			}, project.ContentItem{
				Path: "locales/en.json", Target: "locales/{lang}.json",
			})
		})
		t.Cleanup(a.Shutdown)
		root := filepath.Dir(recipe)
		var hook func(string)
		svc, err := a.ChangeService(t.Context(), ChangeServiceOptions{Project: recipe, Origin: "test", TargetLocale: "fr", BeforeSettle: func(doc string) {
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
			Translated:      "po/en.po",
			TranslationFile: "po/fr.po",
			Translations:    []changetest.Translation{{Doc: "locales/en.json", File: "locales/fr.json"}},
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

// conformanceCatalog is a PO catalog in lang with one message, translated as
// msgstr.
func conformanceCatalog(lang, msgstr string) string {
	return "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Language: " + lang + "\\n\"\n\nmsgid \"Hello there\"\nmsgstr \"" + msgstr + "\"\n"
}
