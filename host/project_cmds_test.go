package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScaffoldRecipe_NeokapiI18nCleanLayout exercises the framework scaffold
// path end to end: resolve the neokapi-i18n preset into content + bindings,
// build the recipe YAML, then parse it back through the real loader. It proves
// `kapi init --framework neokapi-i18n` emits a valid recipe carrying the clean
// nested layout — source under i18n/src/, per-locale targets under i18n/{lang}/,
// with voice profile + terms source bound under i18n/.
func TestScaffoldRecipe_NeokapiI18nCleanLayout(t *testing.T) {
	content, err := FrameworkContent(preset.NeokapiI18nPresetName)
	require.NoError(t, err)
	require.Len(t, content, 1)
	assert.Equal(t, "i18n/src/**/*.kbf.json", content[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf.json", content[0].Target)

	voiceProfile, termsSource := FrameworkBindings(preset.NeokapiI18nPresetName)
	assert.Equal(t, "i18n/voice.yaml", voiceProfile)
	assert.Equal(t, "i18n/terms.json", termsSource)

	yaml := ScaffoldRecipe("MyApp", "en", []string{"de", "fr", "nb"}, content, voiceProfile, termsSource)

	// Parse the emitted recipe through the real loader — it must be valid.
	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, yaml, 0o644))

	proj, err := project.Load(recipePath)
	require.NoError(t, err, "scaffolded recipe must load + validate:\n%s", yaml)

	require.Len(t, proj.Collections, 1)
	assert.Equal(t, "i18n/src/**/*.kbf.json", proj.Collections[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf.json", proj.Collections[0].Target)

	require.NotNil(t, proj.Defaults.Voice)
	assert.Equal(t, "i18n/voice.yaml", proj.Defaults.Voice.ProfileFile)
	assert.Equal(t, "i18n/terms.json", proj.Defaults.TermsSource)
}

// TestInitProject is finding 9: the whole `kapi init` composition — write
// recipe, ensure the .kapi layout, stamp the state manifest — is one host call
// so the desktop can create a project the same way. It is idempotent.
func TestInitProject(t *testing.T) {
	dir := t.TempDir()

	res, err := InitProject(dir, InitOptions{Name: "MyApp", TargetLocales: []string{"fr"}})
	require.NoError(t, err)
	assert.False(t, res.AlreadyInitialized)
	assert.Equal(t, "MyApp", res.Name)

	// Recipe loads and the state layout exists.
	proj, err := project.Load(res.RecipePath)
	require.NoError(t, err)
	assert.Equal(t, "MyApp", proj.Name)
	assert.DirExists(t, res.StateDir)
	assert.FileExists(t, filepath.Join(res.StateDir, project.StateManifestFilename))

	// Idempotent: a second run adopts the existing recipe and leaves it be.
	before, err := os.ReadFile(res.RecipePath)
	require.NoError(t, err)
	res2, err := InitProject(dir, InitOptions{Name: "Renamed"})
	require.NoError(t, err)
	assert.True(t, res2.AlreadyInitialized)
	after, err := os.ReadFile(res.RecipePath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "an existing recipe is left untouched")
}

// TestInitProject_ContentScaffold: with no target locales and no framework, the
// on-brand content scaffold is written (not the translation one).
func TestInitProject_ContentScaffold(t *testing.T) {
	dir := t.TempDir()
	res, err := InitProject(dir, InitOptions{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(dir), res.Name, "name defaults to the dir basename")

	proj, err := project.Load(res.RecipePath)
	require.NoError(t, err)
	assert.Empty(t, proj.Defaults.TargetLanguages, "content scaffold declares no targets")
}

// TestScaffoldRecipe_CommentedExamplesLoad: the tutorial `kapi init` writes is
// the first recipe most people read, and its examples are copied verbatim. Each
// commented block must therefore load — an unqualified `channel: docs` did not,
// so the one example of a governed collection was an example of a load error.
func TestScaffoldRecipe_CommentedExamplesLoad(t *testing.T) {
	yaml := string(ScaffoldRecipe("MyApp", "en", []string{"fr"}, nil, "", ""))

	example := uncommentedYAML(t, yaml, "profiles:")
	require.Contains(t, example, "channel: acme/docs",
		"a channel names its profile — the loader refuses a bare one")

	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath,
		[]byte("version: v1\nname: MyApp\ndefaults:\n  source_language: en\n"+example), 0o644))

	_, err := project.Load(recipePath)
	require.NoError(t, err, "the scaffold's own example must load:\n%s", example)
}

// uncommentedYAML returns the commented recipe example that starts at the line
// beginning with start, uncommented. The block runs to the end of the comment
// run; the sentences interleaved with it are dropped, being unindented prose
// rather than the mapping keys and list items a recipe is made of.
func uncommentedYAML(t *testing.T, yaml, start string) string {
	t.Helper()
	var out []string
	collecting := false
	for line := range strings.SplitSeq(yaml, "\n") {
		body, isComment := strings.CutPrefix(line, "#")
		body = strings.TrimPrefix(body, " ")
		if !collecting {
			collecting = isComment && strings.HasPrefix(body, start)
			if collecting {
				out = append(out, body)
			}
			continue
		}
		if !isComment {
			break
		}
		if isRecipeLine(body) {
			out = append(out, body)
		}
	}
	require.NotEmpty(t, out, "the scaffold must carry a %q example", start)
	return strings.Join(out, "\n") + "\n"
}

// isRecipeLine reports whether an uncommented line is recipe YAML rather than
// the prose around it: blank, indented, a comment of its own, or a top-level key
// (which a recipe writes lower-case, and a sentence does not).
func isRecipeLine(s string) bool {
	if s == "" || s[0] == ' ' || s[0] == '#' {
		return true
	}
	key, _, ok := strings.Cut(s, ":")
	return ok && key != "" && key == strings.ToLower(key) && !strings.ContainsAny(key, " '\"")
}
