package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BCP-47 is the only locale representation inside kapi. A recipe may be written
// in whatever style its author uses; a run, a store lookup and a coverage tally
// all see the canonical tag.

func writeRecipe(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

const posixRecipe = `version: v1
name: Posix
defaults:
  source_language: en_US
  target_languages: [nb_NO, pt_BR]
collections:
  - name: App
    content:
      - path: app/en.json
        target: app/{lang}.json
`

func TestProjectContextCanonicalizesDeclaredLocales(t *testing.T) {
	path := writeRecipe(t, posixRecipe)
	proj, err := Load(path)
	require.NoError(t, err)

	// The recipe is left exactly as the author wrote it.
	assert.Equal(t, model.LocaleID("en_US"), proj.Defaults.SourceLanguage,
		"loading does not restyle the user's file")
	assert.Equal(t, []model.LocaleID{"nb_NO", "pt_BR"}, proj.Defaults.TargetLanguages)

	// Everything derived from it is canonical.
	ctx := NewProjectContext(proj, path)
	assert.Equal(t, model.LocaleID("en-US"), ctx.SourceLocale)
	assert.Equal(t, []model.LocaleID{"nb-NO", "pt-BR"}, ctx.TargetLocales)
}

func TestResolvedLanguagesAreCanonical(t *testing.T) {
	defaults := Defaults{
		SourceLanguage:  "en_US",
		TargetLanguages: []model.LocaleID{"nb_NO"},
	}

	t.Run("from defaults", func(t *testing.T) {
		item := &ContentItem{}
		assert.Equal(t, model.LocaleID("en-US"), item.ResolvedSourceLanguage(nil, defaults))
		assert.Equal(t, []model.LocaleID{"nb-NO"}, item.ResolvedTargetLanguages(nil, defaults))
	})

	t.Run("from the collection", func(t *testing.T) {
		coll := &Collection{SourceLanguage: "DE_de", TargetLanguages: []model.LocaleID{"fr_CA"}}
		item := &ContentItem{}
		assert.Equal(t, model.LocaleID("de-DE"), item.ResolvedSourceLanguage(coll, defaults))
		assert.Equal(t, []model.LocaleID{"fr-CA"}, item.ResolvedTargetLanguages(coll, defaults))
	})

	t.Run("from the item", func(t *testing.T) {
		coll := &Collection{SourceLanguage: "de_DE"}
		item := &ContentItem{SourceLanguage: "es_419", TargetLanguages: []model.LocaleID{"zh_Hans"}}
		assert.Equal(t, model.LocaleID("es-419"), item.ResolvedSourceLanguage(coll, defaults))
		assert.Equal(t, []model.LocaleID{"zh-Hans"}, item.ResolvedTargetLanguages(coll, defaults))
	})

	t.Run("the caller's slice is untouched", func(t *testing.T) {
		declared := []model.LocaleID{"nb_NO"}
		d := Defaults{TargetLanguages: declared}
		got := (&ContentItem{}).ResolvedTargetLanguages(nil, d)
		assert.Equal(t, []model.LocaleID{"nb-NO"}, got)
		assert.Equal(t, []model.LocaleID{"nb_NO"}, declared, "normalizing must not write through")
	})
}

// A locale nobody can spell produces no files, no memory hits and no findings,
// and reads as 0% translated exactly as a real locale would. The recipe is
// where that is caught.
func TestLoadRejectsLocalesThatAreNotLocales(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		field string
	}{
		{
			name:  "source language",
			field: "defaults.source_language",
			body: `version: v1
defaults:
  source_language: engish
`,
		},
		{
			name:  "target language",
			field: "defaults.target_languages[1]",
			body: `version: v1
defaults:
  source_language: en
  target_languages: [nb, xx_YY]
`,
		},
		{
			name:  "collection source",
			field: "collections[0].source_language",
			body: `version: v1
collections:
  - name: App
    source_language: "!!!"
    path: app/en.json
`,
		},
		{
			name:  "per-locale defaults key",
			field: "defaults.locales.nope",
			body: `version: v1
defaults:
  source_language: en
  locales:
    nope:
      tools: {}
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeRecipe(t, tc.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.field)
			assert.Contains(t, err.Error(), "invalid locale")
		})
	}
}

// Styles a recipe may legitimately be written in, and the pseudo-locale the
// repo's own catalogs use, all load.
func TestLoadAcceptsEveryWrittenStyle(t *testing.T) {
	for _, tag := range []string{"nb_NO", "nb-NO", "NB-no", "en", "es-419", "zh_Hans", "qps", "qps-Ploc", "fil"} {
		t.Run(tag, func(t *testing.T) {
			body := "version: v1\ndefaults:\n  source_language: en\n  target_languages: [" + tag + "]\n"
			_, err := Load(writeRecipe(t, body))
			require.NoError(t, err, "%q is a locale a recipe may declare", tag)
		})
	}
}
