package cli

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mergeProjectFixture builds a project with a JSON source + locale target
// template suitable for the extract→merge round-trip.
func mergeProjectFixture(t *testing.T, dir string) string {
	t.Helper()
	recipe := filepath.Join(dir, "app.kapi")
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "MergeTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
		},
		Collections: []project.Collection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "src/locales/{lang}/*.json",
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	return recipe
}

// runMergeCmd drives the merge command with the given flags and returns
// combined stdout/stderr.
func runMergeCmd(t *testing.T, recipe string, flags ...string) (string, error) {
	t.Helper()
	a := newExtractApp(t)
	cmd := NewMergeCmd(a, MergeCmdOptions{})
	args := append([]string{"--project", recipe}, flags...)
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

// editXLIFFTarget edits the first <segment> in an XLIFF 2 file to include
// <target>translation</target> (inserted after <source>), simulating a
// translator's return. The kapi extract writer does not emit an empty
// <target>; each block's target is populated by the translator.
func editXLIFFTarget(t *testing.T, path, translation string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(raw)

	// If a <target> already exists, replace its inner text.
	if open := strings.Index(s, "<target>"); open != -1 {
		close := strings.Index(s[open:], "</target>")
		require.GreaterOrEqual(t, close, 0)
		close += open
		updated := s[:open] + "<target>" + translation + s[close:]
		require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))
		return
	}

	// Otherwise insert a <target> right after the first </source>.
	idx := strings.Index(s, "</source>")
	require.GreaterOrEqual(t, idx, 0, "no </source> found; input was:\n%s", s)
	insertAt := idx + len("</source>")
	updated := s[:insertAt] + "<target>" + translation + "</target>" + s[insertAt:]
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))

	// Sanity: still parses as XML.
	var v any
	require.NoError(t, xml.Unmarshal([]byte(updated), &v))
}

func TestMerge_RoundTripAppliesTranslatorTarget(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := mergeProjectFixture(t, real)
	writeJSONSource(t, real, "src/locales/en/app.json", `{"greeting":"Hello"}`)

	// Extract.
	out, err := runExtractCmd(t, recipe)
	require.NoError(t, err, "extract stdout: %s", out)

	// Edit the XLIFF to simulate a translator's return.
	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	xliffPath := filepath.Join(real, "out", entries[0].Name())
	editXLIFFTarget(t, xliffPath, "Bonjour")

	// Merge.
	mergeOut, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-memory-update")
	require.NoError(t, err, "merge stdout: %s", mergeOut)

	// The translated output should land at src/locales/fr-FR/app.json
	// per the recipe's Target template.
	mergedPath := filepath.Join(real, "src", "locales", "fr-FR", "app.json")
	data, err := os.ReadFile(mergedPath)
	require.NoError(t, err, "expected merged file at %s", mergedPath)
	assert.Contains(t, string(data), "Bonjour")
}

func TestMerge_MultipleInputsInOnePass(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "MultiInput",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
		},
		Collections: []project.Collection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "src/locales/{lang}/*.json",
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello"}`)

	// Extract to both fr and de.
	_, err = runExtractCmd(t, recipe)
	require.NoError(t, err)

	// Edit both xliffs.
	outDir := filepath.Join(real, "out")
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	var frFile, deFile string
	for _, e := range entries {
		if strings.Contains(e.Name(), "fr-FR") {
			frFile = filepath.Join(outDir, e.Name())
		}
		if strings.Contains(e.Name(), "de-DE") {
			deFile = filepath.Join(outDir, e.Name())
		}
	}
	require.NotEmpty(t, frFile)
	require.NotEmpty(t, deFile)
	editXLIFFTarget(t, frFile, "Bonjour")
	editXLIFFTarget(t, deFile, "Hallo")

	// Merge both in one invocation.
	out, err := runMergeCmd(t, recipe, "-i", frFile, "-i", deFile, "--no-memory-update")
	require.NoError(t, err, "merge stdout: %s", out)

	frContent, err := os.ReadFile(filepath.Join(real, "src", "locales", "fr-FR", "app.json"))
	require.NoError(t, err)
	assert.Contains(t, string(frContent), "Bonjour")
	deContent, err := os.ReadFile(filepath.Join(real, "src", "locales", "de-DE", "app.json"))
	require.NoError(t, err)
	assert.Contains(t, string(deContent), "Hallo")
}

func TestMerge_StaleSourceIsSkippedNotApplied(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := mergeProjectFixture(t, real)
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello"}`)

	_, err = runExtractCmd(t, recipe)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	xliffPath := filepath.Join(real, "out", entries[0].Name())
	editXLIFFTarget(t, xliffPath, "Bonjour")

	// Modify the source AFTER extract so the block's source text differs
	// from the XLIFF's <source>. This is the per-block stale case.
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello world"}`)

	out, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-memory-update")
	require.NoError(t, err, "merge stdout: %s", out)
	assert.Contains(t, out, "stale=1")

	// No fr-FR output should have been written (we only had 1 block).
	_, err = os.Stat(filepath.Join(real, "src", "locales", "fr-FR", "app.json"))
	// File may exist (writer emits structure from skeleton) but target
	// text should not contain "Bonjour".
	if err == nil {
		data, _ := os.ReadFile(filepath.Join(real, "src", "locales", "fr-FR", "app.json"))
		assert.NotContains(t, string(data), "Bonjour")
	}
}

func TestMerge_ConflictPolicyExistingWins(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ExistingWins",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Merge:           project.MergeDefaults{ConflictPolicy: project.ConflictPolicyExistingWins},
		},
		Collections: []project.Collection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
				Target: "src/locales/{lang}/*.json",
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello"}`)

	// Extract.
	_, err = runExtractCmd(t, recipe)
	require.NoError(t, err)

	// Pre-existing translated output that merge must NOT overwrite.
	existingPath := filepath.Join(real, "src", "locales", "fr-FR", "app.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(existingPath), 0o755))
	require.NoError(t, os.WriteFile(existingPath, []byte(`{"k":"Déjà traduit"}`), 0o644))

	// Translator returns "Bonjour".
	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	xliffPath := filepath.Join(real, "out", entries[0].Name())
	editXLIFFTarget(t, xliffPath, "Bonjour")

	// The unit was extracted when the project held no French for it, and the
	// French written since wins under existing-wins: the returned target is
	// skipped and the file keeps what it holds.
	out, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-memory-update")
	require.NoError(t, err, "merge stdout: %s", out)
	assert.Contains(t, out, "existing-wins")
	assert.Contains(t, out, "applied=0 stale=0 skipped=1")
	data, err := os.ReadFile(existingPath)
	require.NoError(t, err)
	assert.Equal(t, `{"k":"Déjà traduit"}`, string(data))
}

// TestMerge_SourceEditedAfterExtractIsStaleWithoutTheManifest pins that a
// returned file carries what each unit was extracted against: a unit whose
// source changed since is reported stale and left out, the other units land,
// and none of it needs the extraction manifest, which is deleted here.
func TestMerge_SourceEditedAfterExtractIsStaleWithoutTheManifest(t *testing.T) {
	for _, tc := range []struct {
		format    string
		translate func(t *testing.T, path string)
	}{
		{format: "xliff2", translate: func(t *testing.T, path string) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			s := strings.Replace(string(raw), "<source>Hello</source>", "<source>Hello</source><target>Bonjour</target>", 1)
			s = strings.Replace(s, "<source>Goodbye</source>", "<source>Goodbye</source><target>Au revoir</target>", 1)
			require.NoError(t, os.WriteFile(path, []byte(s), 0o644))
		}},
		{format: "po", translate: func(t *testing.T, path string) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			s := strings.Replace(string(raw), "msgid \"Hello\"\nmsgstr \"\"", "msgid \"Hello\"\nmsgstr \"Bonjour\"", 1)
			s = strings.Replace(s, "msgid \"Goodbye\"\nmsgstr \"\"", "msgid \"Goodbye\"\nmsgstr \"Au revoir\"", 1)
			require.NoError(t, os.WriteFile(path, []byte(s), 0o644))
		}},
	} {
		t.Run(tc.format, func(t *testing.T) {
			real, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			recipe := mergeProjectFixture(t, real)
			writeJSONSource(t, real, "src/locales/en/app.json", `{"a":"Hello","b":"Goodbye"}`)
			_, err = runExtractCmd(t, recipe, "--format", tc.format)
			require.NoError(t, err)
			entries, err := os.ReadDir(filepath.Join(real, "out"))
			require.NoError(t, err)
			require.Len(t, entries, 1)
			returned := filepath.Join(real, "out", entries[0].Name())
			tc.translate(t, returned)

			layout, err := project.LayoutFor(recipe)
			require.NoError(t, err)
			require.NoError(t, os.RemoveAll(project.ExtractionsRoot(layout)), "the extraction manifest is lost")
			writeJSONSource(t, real, "src/locales/en/app.json", `{"a":"Hello there","b":"Goodbye"}`)

			out, err := runMergeCmd(t, recipe, "-i", returned, "--no-memory-update")
			require.NoError(t, err, "merge stdout: %s", out)
			assert.Contains(t, out, "applied=1 stale=1 skipped=0")
			data, err := os.ReadFile(filepath.Join(real, "src", "locales", "fr-FR", "app.json"))
			require.NoError(t, err)
			assert.Equal(t, `{"a":"Hello there","b":"Au revoir"}`, string(data))
		})
	}
}
