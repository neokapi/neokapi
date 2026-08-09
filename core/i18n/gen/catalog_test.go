package gen

import (
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leonelquinteros/gotext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureRecipe is a two-collection recipe in the shape of the dogfood one:
// an engine-style collection with an extraction rule, and a bare-json CLI-style
// collection where every string is a catalog entry.
const fixtureRecipe = `version: v1
name: fixture
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: engine
    base: core/i18n
    content:
      - path: builtins/metadata.json
        format:
          name: json
          config:
            extractAllPairs: false
            useKeyAsName: true
            extractionRules: "(displayName|description|title|label)$"
        target: catalogs/{lang}.json
  - name: cli
    base: host/i18n
    content:
      - path: commands.json
        format:
          name: json
        target: catalogs/{lang}.json
`

const fixtureMetadata = `{
  "tools": {
    "translate": {
      "displayName": "AI Translate",
      "category": "translation",
      "description": "Translate content with an LLM provider",
      "properties": {
        "model": {
          "title": "Model",
          "description": "Model identifier"
        }
      }
    }
  }
}
`

const fixtureCommands = `{
  "cli": {
    "commands": {
      "kapi": {
        "version": {
          "short": "Show version information"
        }
      }
    }
  }
}
`

// writeFixture lays out a repository root with the recipe and both source
// documents, plus whatever translated catalogs the case supplies (keyed by
// path relative to the root).
func writeFixture(t *testing.T, catalogs map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"kapi.yaml":                        fixtureRecipe,
		"core/i18n/builtins/metadata.json": fixtureMetadata,
		"host/i18n/commands.json":          fixtureCommands,
	}
	maps.Copy(files, catalogs)
	for path, body := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	return root
}

func compile(t *testing.T, root string) ([]CatalogSummary, string) {
	t.Helper()
	var log strings.Builder
	summaries, err := CompileCollections(root, []string{"engine", "cli"}, &log)
	require.NoError(t, err)
	return summaries, log.String()
}

func parseMO(t *testing.T, path string) *gotext.Mo {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	catalog := gotext.NewMo()
	catalog.Parse(data)
	return catalog
}

// TestCompileCatalogs_PairsByKeyPath is the contract: msgid is the English
// source, msgstr the translation, and msgctxt the key path both documents
// share — which is also the scope the runtime looks a string up under.
func TestCompileCatalogs_PairsByKeyPath(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"core/i18n/catalogs/nb.json": `{
  "tools": {
    "translate": {
      "displayName": "KI-oversettelse",
      "category": "translation",
      "description": "Oversett innhold med en LLM-leverandør",
      "properties": {
        "model": {
          "title": "Modell",
          "description": "Modellidentifikator"
        }
      }
    }
  }
}
`,
		"host/i18n/catalogs/nb.json": `{
  "cli": {
    "commands": {
      "kapi": {
        "version": {
          "short": "Vis versjonsinformasjon"
        }
      }
    }
  }
}
`,
	})

	summaries, _ := compile(t, root)
	require.Len(t, summaries, 2)

	engine := parseMO(t, filepath.Join(root, "core/i18n/catalogs/nb.mo"))
	tests := []struct {
		scope  string
		source string
		want   string
	}{
		{"tools.translate.displayName", "AI Translate", "KI-oversettelse"},
		{"tools.translate.description", "Translate content with an LLM provider", "Oversett innhold med en LLM-leverandør"},
		{"tools.translate.properties.model.title", "Model", "Modell"},
		{"tools.translate.properties.model.description", "Model identifier", "Modellidentifikator"},
	}
	for _, tc := range tests {
		t.Run(tc.scope, func(t *testing.T) {
			assert.True(t, engine.IsTranslatedC(tc.source, tc.scope),
				"%s must be translated in the compiled catalog", tc.scope)
			assert.Equal(t, tc.want, engine.GetC(tc.source, tc.scope))
		})
	}

	// A key the extraction rule excludes is not a catalog entry, even though
	// the translated document carries it.
	assert.False(t, engine.IsTranslatedC("translation", "tools.translate.category"),
		"category is not translatable text — it must not reach the catalog")

	cli := parseMO(t, filepath.Join(root, "host/i18n/catalogs/nb.mo"))
	assert.Equal(t, "Vis versjonsinformasjon",
		cli.GetC("Show version information", "cli.commands.kapi.version.short"))
}

// TestCompileCatalogs_Deterministic guards the gate that makes a generated
// artifact reviewable at all: the same inputs must produce the same bytes, so
// a rebuild is a no-op rather than a diff.
func TestCompileCatalogs_Deterministic(t *testing.T) {
	catalogs := map[string]string{
		"core/i18n/catalogs/nb.json": `{"tools":{"translate":{"displayName":"KI-oversettelse","description":"Oversett innhold med en LLM-leverandør","properties":{"model":{"title":"Modell","description":"Modellidentifikator"}}}}}`,
		"host/i18n/catalogs/nb.json": `{"cli":{"commands":{"kapi":{"version":{"short":"Vis versjonsinformasjon"}}}}}`,
	}
	rootA := writeFixture(t, catalogs)
	rootB := writeFixture(t, catalogs)

	compile(t, rootA)
	compile(t, rootB)

	for _, rel := range []string{"core/i18n/catalogs/nb.mo", "host/i18n/catalogs/nb.mo"} {
		a, err := os.ReadFile(filepath.Join(rootA, filepath.FromSlash(rel)))
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(rootB, filepath.FromSlash(rel)))
		require.NoError(t, err)
		assert.Equal(t, a, b, "%s is not byte-stable across runs", rel)
	}
}

// TestCompileCatalogs_MissingTranslationFallsBackToSource pins the drift
// policy: a string the locale has not translated — or has copied verbatim —
// is left out, so the runtime returns the English source. It is counted, never
// enforced.
func TestCompileCatalogs_MissingTranslationFallsBackToSource(t *testing.T) {
	root := writeFixture(t, map[string]string{
		// displayName is copied verbatim, description is translated, the two
		// model properties are absent from the document entirely.
		"core/i18n/catalogs/nb.json": `{"tools":{"translate":{"displayName":"AI Translate","description":"Oversett innhold med en LLM-leverandør"}}}`,
	})

	summaries, log := compile(t, root)
	require.Len(t, summaries, 1, "the cli collection has no committed catalog")

	engine := summaries[0]
	assert.Equal(t, "engine", engine.Collection)
	assert.Equal(t, 1, engine.Translated)
	assert.Equal(t, 3, engine.Pending,
		"a verbatim copy and two absent strings are all pending")

	catalog := parseMO(t, filepath.Join(root, "core/i18n/catalogs/nb.mo"))
	assert.False(t, catalog.IsTranslatedC("AI Translate", "tools.translate.displayName"),
		"a translation identical to its source is not a translation")
	assert.False(t, catalog.IsTranslatedC("Model", "tools.translate.properties.model.title"))
	assert.True(t, catalog.IsTranslatedC(
		"Translate content with an LLM provider", "tools.translate.description"))

	assert.Contains(t, log, "host/i18n/catalogs/nb.json is not committed yet",
		"an untranslated locale is reported, not an error")
	assert.NoFileExists(t, filepath.Join(root, "host/i18n/catalogs/nb.mo"))
}

// TestCompileCatalogs_UndeclaredLocaleStillCompiles covers the qps probe
// locale: a committed catalog is compiled because it is there, not because the
// recipe lists the language as a target.
func TestCompileCatalogs_UndeclaredLocaleStillCompiles(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"core/i18n/catalogs/qps.json": `{"tools":{"translate":{"displayName":"▒ ÀÎ Ţŕàñšļàţé ▒"}}}`,
	})

	summaries, _ := compile(t, root)
	require.Len(t, summaries, 1)
	assert.Equal(t, "qps", string(summaries[0].Locale))

	catalog := parseMO(t, filepath.Join(root, "core/i18n/catalogs/qps.mo"))
	assert.Equal(t, "▒ ÀÎ Ţŕàñšļàţé ▒",
		catalog.GetC("AI Translate", "tools.translate.displayName"))
}

// TestCompileCatalogs_RewriteIsSkippedWhenUnchanged keeps a no-op rebuild from
// invalidating everything downstream of the catalog.
func TestCompileCatalogs_RewriteIsSkippedWhenUnchanged(t *testing.T) {
	root := writeFixture(t, map[string]string{
		"core/i18n/catalogs/nb.json": `{"tools":{"translate":{"displayName":"KI-oversettelse"}}}`,
	})
	moPath := filepath.Join(root, "core/i18n/catalogs/nb.mo")

	compile(t, root)
	first, err := os.Stat(moPath)
	require.NoError(t, err)

	compile(t, root)
	second, err := os.Stat(moPath)
	require.NoError(t, err)
	assert.Equal(t, first.ModTime(), second.ModTime(),
		"an unchanged catalog must not be rewritten")
}

// TestCompileCatalogs_UnknownCollectionIsAnError separates the two failure
// classes: a recipe that does not declare the collection the binary embeds is
// broken configuration, unlike a locale nobody has translated yet.
func TestCompileCatalogs_UnknownCollectionIsAnError(t *testing.T) {
	root := writeFixture(t, nil)
	_, err := CompileCollections(root, []string{"nonexistent"}, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `declares no collection "nonexistent"`)
}

// TestCatalogCollections_MatchTheDogfoodRecipe asserts the compiler's list and
// the recipe agree — the whole point of reading the configuration rather than
// restating it.
func TestCatalogCollections_MatchTheDogfoodRecipe(t *testing.T) {
	root := repoRoot(t)
	summaries, err := CompileCatalogs(root, io.Discard)
	require.NoError(t, err)
	require.NotEmpty(t, summaries)

	byCollection := map[string]bool{}
	for _, s := range summaries {
		byCollection[s.Collection] = true
		assert.Positive(t, s.Translated, "%s carries no translations", s.Path)
	}
	for _, name := range CatalogCollections {
		assert.True(t, byCollection[name], "no catalog compiled for %s", name)
	}
}

// repoRoot walks up from the test's working directory to the checkout root —
// the one directory holding both the workspace file and the dogfood recipe.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		_, recipeErr := os.Stat(filepath.Join(dir, "kapi.yaml"))
		_, workErr := os.Stat(filepath.Join(dir, "go.work"))
		if recipeErr == nil && workErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no kapi.yaml above %s", dir)
		dir = parent
	}
}
