package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
)

// Two files whose blocks share an ID ("greeting" in every JSON file that
// greets) withhold a different name each under one redaction policy. The
// batch vault keeps each original under its file, so a merge of either file
// restores its own name.

// vaultNames is the name each file of the vault fixture withholds.
var vaultNames = map[string]string{
	"src/locales/en/a.json": "Alice Archer",
	"src/locales/en/b.json": "Bruno Black",
}

// vaultProject is a project with two JSON files under one redaction policy
// that withholds both names.
func vaultProject(t *testing.T) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	for rel, name := range vaultNames {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hello `+name+`"}`+"\n"), 0o644))
	}
	rules := &redaction.RulesFile{Rules: []redaction.Rule{
		{Term: "Alice Archer", Category: redaction.CategoryPerson},
		{Term: "Bruno Black", Category: redaction.CategoryPerson},
	}}
	require.NoError(t, rules.Save(filepath.Join(dir, "redact.yaml")))
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "VaultPerDocument",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Redaction:       &project.RedactionSpec{Enabled: true, Rules: "redact.yaml"},
		},
		Collections: []project.Collection{{
			Path:   "src/locales/en/*.json",
			Format: &project.FormatSpec{Name: "json"},
			Target: "src/locales/{lang}/*.json",
		}},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	a.Quiet = true
	return a, recipe, dir
}

// extractXLIFF runs kapi extract --format xliff2 over the project into
// <dir>/out and returns the files it wrote.
func extractXLIFF(t *testing.T, a *App, recipe, dir string) []string {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "extract")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	addKpzExtractFlags(cmd, recipe)
	require.NoError(t, cmd.Flags().Set("no-memory", "true"))
	cmd.Flags().String("format", ExtractFormatXLIFF2, "")
	cmd.Flags().String("xliff-version", "", "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("redact", false, "")
	cmd.Flags().String("redact-rules", "", "")
	require.NoError(t, a.RunExtract(cmd))
	out, err := filepath.Glob(filepath.Join(dir, "out", "*.xliff"))
	require.NoError(t, err)
	return out
}

var xliffSourceRe = regexp.MustCompile(`(?s)<source>(.*?)</source>`)

// translateXLIFF fills each unit's target with its source, the placeholder
// kept and "Hello" put into French, as a translator returns it.
func translateXLIFF(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := xliffSourceRe.ReplaceAllStringFunc(string(data), func(src string) string {
		inner := xliffSourceRe.FindStringSubmatch(src)[1]
		return src + "<target>" + strings.Replace(inner, "Hello", "Bonjour", 1) + "</target>"
	})
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
}

// mergeFile runs kapi merge -i path.
func mergeFile(t *testing.T, a *App, recipe, path string) {
	t.Helper()
	cmd, _, stderr := mergeCommand(t, recipe)
	cmd.Flags().StringArray("input", nil, "")
	cmd.Flags().Bool("no-restore", false, "")
	require.NoError(t, cmd.Flags().Set("input", path))
	require.NoError(t, cmd.Flags().Set("no-memory-update", "true"))
	require.NoError(t, a.RunMerge(cmd), stderr.String())
}

// batchVault returns the one batch vault the extraction wrote.
func batchVault(t *testing.T, dir string) string {
	t.Helper()
	vaults, err := filepath.Glob(filepath.Join(project.LayoutAt(dir).VaultDir(), project.VaultBatchesDirName, "*.json"))
	require.NoError(t, err)
	require.Len(t, vaults, 1)
	return vaults[0]
}

func TestExtractMergeRestoresEachFilesOwnRedactedName(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	files := extractXLIFF(t, a, recipe, dir)
	require.Len(t, files, len(vaultNames))
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, name := range vaultNames {
			assert.NotContains(t, string(data), name, "the extracted file withholds every name")
		}
	}

	v, err := redaction.OpenFileVault(batchVault(t, dir))
	require.NoError(t, err)
	assert.Len(t, v.All(), len(vaultNames), "the batch vault keeps one name per file")
	for doc, name := range vaultNames {
		got, ok := v.Get(doc, "tu1", "rdx1")
		require.True(t, ok, doc)
		assert.Equal(t, name, got.Original, doc)
	}

	for _, f := range files {
		translateXLIFF(t, f)
		mergeFile(t, a, recipe, f)
	}
	for _, base := range []string{"a.json", "b.json"} {
		data, err := os.ReadFile(filepath.Join(dir, "src", "locales", "fr", base))
		require.NoError(t, err, "merge writes %s", base)
		name := vaultNames["src/locales/en/"+base]
		assert.Equal(t, `{"greeting":"Bonjour `+name+`"}`, strings.TrimSpace(string(data)), base)
	}
}

// A batch vault written before the vault kept each file's name holds its
// entries under block and token alone. A merge still restores from it.
func TestMergeRestoresFromAVersion1BatchVault(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "src", "locales", "en", "b.json")))
	files := extractXLIFF(t, a, recipe, dir)
	require.Len(t, files, 1)

	path := batchVault(t, dir)
	v, err := redaction.OpenFileVault(path)
	require.NoError(t, err)
	type v1Value struct {
		Token    string `json:"token"`
		Category string `json:"category"`
		Disp     string `json:"disp,omitempty"`
		Original string `json:"original"`
		Locale   string `json:"locale,omitempty"`
		BlockID  string `json:"blockId,omitempty"`
	}
	var old []v1Value
	for _, rv := range v.All() {
		old = append(old, v1Value{rv.Token, rv.Category, rv.Disp, rv.Original, string(rv.Locale), rv.BlockID})
	}
	data, err := json.Marshal(map[string]any{"version": 1, "values": old})
	require.NoError(t, err)
	require.NotContains(t, string(data), `"doc"`)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	translateXLIFF(t, files[0])
	mergeFile(t, a, recipe, files[0])
	got, err := os.ReadFile(filepath.Join(dir, "src", "locales", "fr", "a.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"greeting":"Bonjour Alice Archer"}`, strings.TrimSpace(string(got)))
}
