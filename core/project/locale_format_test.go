package project

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// locale_format is the disk boundary and nothing else: it decides how a locale
// is spelled in a path, while the recipe, the resolved runtime and every answer
// stay canonical BCP-47.

func TestResolveTargetPathInRendersTheDeclaredStyle(t *testing.T) {
	cases := []struct {
		name   string
		format string
		lang   string
		want   string
	}{
		{"default is bcp-47", "", "nb-NO", "app/nb-NO.json"},
		{"explicit bcp-47", "bcp-47", "nb-NO", "app/nb-NO.json"},
		{"posix", "posix", "nb-NO", "app/nb_NO.json"},
		{"posix leaves a bare language alone", "posix", "nb", "app/nb.json"},
		{"posix on a script tag", "posix", "zh-Hans", "app/zh_Hans.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTargetPathIn("app/en.json", "", "app/{lang}.json", "app/en.json", tc.lang, tc.format)
			assert.Equal(t, tc.want, got)
		})
	}
}

// {lang} is projected wherever it appears in the pattern, so a target that
// mirrors its sources under a per-locale directory gets the declared style in
// the directory name as well as in a filename. The Project Settings hint says
// "file and directory names"; this is the half that is not a filename.
func TestResolveTargetPathInProjectsDirectorySegments(t *testing.T) {
	cases := []struct {
		name   string
		target string
		want   string
	}{
		{"directory segment", "i18n/{lang}/messages.json", "i18n/nb_NO/messages.json"},
		// A directory target mirrors the source tree beneath it; the source's
		// own fixed prefix ("app/") is the base that gets trimmed.
		{"directory mirror", "i18n/{lang}/", "i18n/nb_NO/en.json"},
		{"both segments", "i18n/{lang}/app.{lang}.json", "i18n/nb_NO/app.nb_NO.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTargetPathIn("app/en.json", "", tc.target, "app/en.json", "nb-NO", "posix")
			assert.Equal(t, tc.want, got)
		})
	}
}

// A locale is projected on the way out and canonicalized on the way back, so a
// posix project's own files answer to the locale its recipe declares.
func TestLocaleFormatRoundTrips(t *testing.T) {
	for _, declared := range []model.LocaleID{"nb-NO", "zh-Hans", "pt-BR"} {
		t.Run(string(declared), func(t *testing.T) {
			onDisk := ResolveTargetPathIn("app/en.json", "", "app/{lang}.json", "app/en.json", string(declared), "posix")

			// The path is written in the declared style.
			assert.Contains(t, onDisk, "_")

			// And read back, it is the locale the recipe declares.
			seg := onDisk[len("app/") : len(onDisk)-len(".json")]
			assert.Equal(t, declared, LocaleFromPath(seg),
				"a locale recovered from a path comes back canonical")
		})
	}
}

func TestPathLocaleProjectsThroughTheContext(t *testing.T) {
	posix := &ProjectContext{LocaleFormat: "posix"}
	assert.Equal(t, "nb_NO", posix.PathLocale("nb-NO"))

	bcp := &ProjectContext{LocaleFormat: "bcp-47"}
	assert.Equal(t, "nb-NO", bcp.PathLocale("nb-NO"))

	var none *ProjectContext
	assert.Equal(t, "nb-NO", none.PathLocale("nb-NO"), "no project means no projection")
}

// The whole point: what a project declares, what it writes, and what it answers
// are three different questions, and only the middle one depends on the style.
func TestPosixProjectKeepsCanonicalIdentity(t *testing.T) {
	path := writeRecipe(t, `version: v1
name: Posix
defaults:
  source_language: en
  target_languages: [nb_NO]
  locale_format: posix
collections:
  - name: App
    content:
      - path: app/en.json
        target: app/{lang}.json
`)
	proj, err := Load(path)
	require.NoError(t, err)
	ctx := NewProjectContext(proj, path)

	// Identity is canonical.
	require.Equal(t, []model.LocaleID{"nb-NO"}, ctx.TargetLocales)

	// The file it writes is not.
	item := ContentItem{Path: "app/en.json", Target: "app/{lang}.json"}
	assert.Equal(t, "app/nb_NO.json", ctx.TargetPath(item, "app/en.json", ctx.TargetLocales[0]))

	// And a project that declares nothing writes the canonical name.
	plain := &ProjectContext{}
	assert.Equal(t, "app/nb-NO.json", plain.TargetPath(item, "app/en.json", "nb-NO"))
}
