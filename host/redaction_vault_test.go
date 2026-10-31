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

// extractBilingual runs kapi extract --format format over the project into
// <dir>/out and returns the files it wrote.
func extractBilingual(t *testing.T, a *App, recipe, dir, format string) []string {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "extract")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	addKpzExtractFlags(cmd, recipe)
	require.NoError(t, cmd.Flags().Set("no-memory", "true"))
	cmd.Flags().String("format", format, "")
	cmd.Flags().String("xliff-version", "", "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("redact", false, "")
	cmd.Flags().String("redact-rules", "", "")
	require.NoError(t, a.RunExtract(cmd))
	out, err := filepath.Glob(filepath.Join(dir, "out", "*"))
	require.NoError(t, err)
	return out
}

var (
	xliffSourceRe = regexp.MustCompile(`(?s)<source>(.*?)</source>`)
	poEntryRe     = regexp.MustCompile(`(?m)^msgid ("[^"\n]+")\nmsgstr ""$`)
)

// translateReturned fills each unit's target with its source, the
// placeholder kept and "Hello" put into French, as a translator returns it.
func translateReturned(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var out string
	if strings.HasSuffix(path, ".po") {
		out = poEntryRe.ReplaceAllStringFunc(string(data), func(entry string) string {
			msgid := poEntryRe.FindStringSubmatch(entry)[1]
			return "msgid " + msgid + "\nmsgstr " + strings.Replace(msgid, "Hello", "Bonjour", 1)
		})
	} else {
		out = xliffSourceRe.ReplaceAllStringFunc(string(data), func(src string) string {
			inner := xliffSourceRe.FindStringSubmatch(src)[1]
			return src + "<target>" + strings.Replace(inner, "Hello", "Bonjour", 1) + "</target>"
		})
	}
	require.NotEqual(t, string(data), out, "the translation fills a target in %s", path)
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
	for _, format := range []string{ExtractFormatXLIFF2, extractFormatPO} {
		t.Run(format, func(t *testing.T) {
			a, recipe, dir := vaultProject(t)
			files := extractBilingual(t, a, recipe, dir, format)
			require.Len(t, files, len(vaultNames))
			for _, f := range files {
				data, err := os.ReadFile(f)
				require.NoError(t, err)
				for _, name := range vaultNames {
					assert.NotContains(t, string(data), name, "the extracted file withholds every name")
				}
				assert.Contains(t, string(data), "[REDACTED:Person]", "the extracted file shows where a name was withheld")
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
				translateReturned(t, f)
				mergeFile(t, a, recipe, f)
			}
			for _, base := range []string{"a.json", "b.json"} {
				data, err := os.ReadFile(filepath.Join(dir, "src", "locales", "fr", base))
				require.NoError(t, err, "merge writes %s", base)
				name := vaultNames["src/locales/en/"+base]
				assert.Equal(t, `{"greeting":"Bonjour `+name+`"}`, strings.TrimSpace(string(data)), base)
			}
		})
	}
}

// A batch vault written before the vault kept each file's name holds its
// entries under block and token alone. A merge still restores from it.
func TestMergeRestoresFromAVersion1BatchVault(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "src", "locales", "en", "b.json")))
	files := extractBilingual(t, a, recipe, dir, ExtractFormatXLIFF2)
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

	translateReturned(t, files[0])
	mergeFile(t, a, recipe, files[0])
	got, err := os.ReadFile(filepath.Join(dir, "src", "locales", "fr", "a.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"greeting":"Bonjour Alice Archer"}`, strings.TrimSpace(string(got)))
}

// A batch vault a newer kapi wrote is in a format this binary cannot read,
// so a merge that needs it fails before it writes the target file.
func TestMergeRefusesABatchVaultANewerKapiWrote(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "src", "locales", "en", "b.json")))
	files := extractBilingual(t, a, recipe, dir, ExtractFormatXLIFF2)
	require.Len(t, files, 1)
	path := batchVault(t, dir)
	require.NoError(t, os.WriteFile(path, []byte(`{"version": 4, "values": []}`), 0o600))

	translateReturned(t, files[0])
	cmd, stdout, _ := mergeCommand(t, recipe)
	cmd.Flags().StringArray("input", nil, "")
	cmd.Flags().Bool("no-restore", false, "")
	require.NoError(t, cmd.Flags().Set("input", files[0]))
	require.NoError(t, cmd.Flags().Set("no-memory-update", "true"))
	require.Error(t, a.RunMerge(cmd))
	var res struct {
		Files []struct {
			Error string `json:"error"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &res), stdout.String())
	require.Len(t, res.Files, 1)
	assert.Contains(t, res.Files[0].Error, "vault "+path+" is version 4 and this kapi reads up to version 3: a newer kapi wrote this vault")
	assert.NoFileExists(t, filepath.Join(dir, "src", "locales", "fr", "a.json"), "the merge writes no target file")
}

// kpzExtractCommand is kapi extract --format kpz over the project at recipe,
// writing into outDir.
func kpzExtractCommand(t *testing.T, recipe, outDir string) *EnvCommand {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "extract")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	addKpzExtractFlags(cmd, recipe)
	cmd.Flags().Bool("redact", false, "")
	cmd.Flags().String("redact-rules", "", "")
	require.NoError(t, cmd.Flags().Set("out-dir", outDir))
	return cmd
}

// kpzPackages returns the packages in outDir.
func kpzPackages(t *testing.T, outDir string) []string {
	t.Helper()
	written, err := filepath.Glob(filepath.Join(outDir, "*.kpz"))
	require.NoError(t, err)
	return written
}

// withoutRedactionPolicy removes the project's defaults.redaction.
func withoutRedactionPolicy(t *testing.T, recipe string) {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Redaction = nil
	require.NoError(t, project.Save(recipe, proj))
}

// Under a redaction policy, and under --redact-rules, kapi extract --format
// kpz writes packages that withhold every name and record the batch vault
// that keeps the names. With no policy the packages record none.
func TestExtractKpzUnderARedactionPolicy(t *testing.T) {
	t.Run("the recipe declares defaults.redaction", func(t *testing.T) {
		a, recipe, dir := vaultProject(t)
		out := t.TempDir()
		require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
		assertRedactedPackages(t, dir, kpzPackages(t, out))
	})
	t.Run("--redact-rules on a project that declares none", func(t *testing.T) {
		a, recipe, dir := vaultProject(t)
		withoutRedactionPolicy(t, recipe)
		out := t.TempDir()
		cmd := kpzExtractCommand(t, recipe, out)
		require.NoError(t, cmd.Flags().Set("redact-rules", filepath.Join(dir, "redact.yaml")))
		require.NoError(t, a.RunExtractKpz(cmd))
		assertRedactedPackages(t, dir, kpzPackages(t, out))
	})
	t.Run("with no policy the packages record no vault", func(t *testing.T) {
		a, recipe, _ := vaultProject(t)
		withoutRedactionPolicy(t, recipe)
		out := t.TempDir()
		require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
		written := kpzPackages(t, out)
		require.Len(t, written, len(vaultNames))
		for _, p := range written {
			pkg, err := LoadWorkspace(p)
			require.NoError(t, err)
			assert.Nil(t, pkg.Redaction, p)
		}
	})
}

// assertRedactedPackages checks that every package withholds every name in
// every member, records one batch vault, and that the vault, named by its
// ID under .kapi/vault/batches/, keeps each file's name.
func assertRedactedPackages(t *testing.T, dir string, packages []string) {
	t.Helper()
	require.Len(t, packages, len(vaultNames))
	vaultID := ""
	for _, p := range packages {
		assertPackageWithholds(t, p, vaultNames)
		pkg, err := LoadWorkspace(p)
		require.NoError(t, err)
		require.NotNil(t, pkg.Redaction, "%s records that it is redacted", p)
		if vaultID == "" {
			vaultID = pkg.Redaction.VaultID
		}
		assert.Equal(t, vaultID, pkg.Redaction.VaultID, "one extraction writes one vault")
	}
	v, err := redaction.OpenFileVault(project.LayoutAt(dir).RedactionSidecarPath(vaultID))
	require.NoError(t, err)
	assert.Equal(t, vaultID, v.ID(), "the vault carries the ID the packages name")
	for doc, name := range vaultNames {
		got, ok := v.Get(doc, "tu1", "rdx1")
		require.True(t, ok, doc)
		assert.Equal(t, name, got.Original, doc)
	}
}

// A PO entry has no inline codes, so the writer carries a withheld value as
// its visible placeholder, and the reader returns that placeholder for merge
// to restore. Other inline codes stay out of the entry, as before.
func TestWritePOExtractCarriesTheRedactionPlaceholder(t *testing.T) {
	b := &model.Block{ID: "tu1", Translatable: true}
	b.SetSourceRuns([]model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: &model.PlaceholderRun{ID: "rdx1", Type: redaction.PlaceholderType(redaction.CategoryPerson), Data: "[REDACTED:Person]", Disp: "[REDACTED:Person]"}},
		{Text: &model.TextRun{Text: ", see "}},
		{Ph: &model.PlaceholderRun{ID: "x1", Type: "fmt:br", Data: "<br/>", Disp: "<br/>"}},
	})
	b.SetTargetRuns("fr", []model.Run{
		{Text: &model.TextRun{Text: "Bonjour "}},
		{Ph: &model.PlaceholderRun{ID: "rdx1", Type: redaction.PlaceholderType(redaction.CategoryPerson), Disp: "[REDACTED:Person]"}},
	})
	path := filepath.Join(t.TempDir(), "a.po")
	var buf bytes.Buffer
	require.NoError(t, WritePOExtract(&buf, "fr", "batch", "src/a.json", "sha256:x", []*model.Block{b}))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))

	po, err := ReadPOForMerge(path)
	require.NoError(t, err)
	require.Len(t, po.Blocks, 1)
	assert.Equal(t, "Hello [REDACTED:Person], see ", po.Blocks[0].MsgID)
	assert.Equal(t, "Bonjour [REDACTED:Person]", po.Blocks[0].MsgStr)
}
