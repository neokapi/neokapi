package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLocaleOutputPath covers the default (no -o / no --output-dir) output path
// resolution: swap the source locale in the input path when present, else drop
// the file under a {lang}/ directory beside the input.
func TestLocaleOutputPath(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		sourceLang string
		targetLang string
		want       string // slash form; converted per-OS below
	}{
		// No locale marker → {lang}/ beside the input.
		{"flat cwd", "messages.json", "en", "fr", "fr/messages.json"},
		{"flat subdir", "config/app.json", "en", "fr", "config/fr/app.json"},

		// Source locale as a directory segment → swapped, file untouched.
		{"locale dir", "locales/en/messages.json", "en", "fr", "locales/fr/messages.json"},
		{"locale dir nested", "src/locales/en/app.json", "en", "fr-FR", "src/locales/fr-FR/app.json"},
		{"deepest dir wins", "en/pkg/en/app.json", "en", "fr", "en/pkg/fr/app.json"},

		// Source locale as a filename token → swapped beside the input.
		{"suffix dot", "app.en.json", "en", "fr", "app.fr.json"},
		{"suffix underscore", "app_en.arb", "en", "fr", "app_fr.arb"},
		{"suffix dash", "app-en.json", "en", "fr", "app-fr.json"},
		{"bare locale file", "en.json", "en", "fr", "fr.json"},
		{"suffix in subdir", "i18n/strings.en.json", "en", "de", "i18n/strings.de.json"},

		// Region tolerance: primary subtag matches across en ↔ en-US.
		{"region dir, short source", "locales/en/app.json", "en-US", "fr-FR", "locales/fr-FR/app.json"},
		{"region source, short dir", "locales/en/app.json", "en", "fr", "locales/fr/app.json"},
		{"region dir exact", "locales/en-US/app.json", "en-US", "fr-FR", "locales/fr-FR/app.json"},

		// Directory match is preferred over a filename match.
		{"dir preferred over file", "locales/en/en.json", "en", "fr", "locales/fr/en.json"},

		// No false positives: "app"/"index" are not locales.
		{"no false positive", "app.json", "en", "fr", "fr/app.json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := localeOutputPath(filepath.FromSlash(tc.input), tc.sourceLang, tc.targetLang)
			assert.Equal(t, filepath.FromSlash(tc.want), got)
		})
	}
}

func TestLocaleMatches(t *testing.T) {
	assert.True(t, localeMatches("en", "en"))
	assert.True(t, localeMatches("EN", "en"))    // case-insensitive
	assert.True(t, localeMatches("en", "en-US")) // primary subtag of source
	assert.True(t, localeMatches("en-US", "en-US"))
	assert.False(t, localeMatches("en-US", "en")) // not a substring match
	assert.False(t, localeMatches("english", "en"))
	assert.False(t, localeMatches("app", "en"))
	assert.False(t, localeMatches("", "en"))
}

// TestExpandOutputPathDirPlaceholder verifies the {dir} placeholder added for
// parity with the flow path, and the --output-dir-style "DIR/{lang}/" template.
func TestExpandOutputPathDirPlaceholder(t *testing.T) {
	tmp := t.TempDir()
	input := filepath.Join(tmp, "locales", "messages.json")
	commonDir := filepath.Join(tmp, "locales") + string(filepath.Separator)

	// {dir} expands to the input's directory.
	got := expandOutputPath(filepath.Join("{dir}", "{lang}", "{name}.{ext}"), input, commonDir, "fr")
	assert.Equal(t, filepath.Join(tmp, "locales", "fr", "messages.json"), got)

	// A trailing-separator dir template (as produced by --output-dir) joins the
	// file relative to the common dir under DIR/{lang}/.
	outDir := filepath.Join(tmp, "build")
	got = expandOutputPath(filepath.Join(outDir, "{lang}")+string(filepath.Separator), input, commonDir, "de")
	assert.Equal(t, filepath.Join(outDir, "de", "messages.json"), got)
}
