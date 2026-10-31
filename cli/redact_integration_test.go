package cli

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var xliffSourceInnerRe = regexp.MustCompile(`(?s)<source>(.*?)</source>`)

// TestExtractRedact_MergeRestores is the end-to-end external-workflow test:
// extract --redact must keep the secret out of the emitted XLIFF (the secret
// lives only in the local vault), and merge must restore it into the merged
// target.
func TestExtractRedact_MergeRestores(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := mergeProjectFixture(t, real)

	writeJSONSource(t, real, "src/locales/en/app.json", `{"greeting":"Hello Mr Bean"}`)

	// Dedicated redaction rules file.
	rulesPath := filepath.Join(real, "redaction.yaml")
	require.NoError(t, os.WriteFile(rulesPath, []byte(
		"version: v1\nrules:\n  - term: \"Mr Bean\"\n    category: person\n"), 0o644))

	// Extract with redaction.
	out, err := runExtractCmd(t, recipe, "--redact-rules", rulesPath, "--no-memory")
	require.NoError(t, err, "extract stdout: %s", out)

	// The emitted XLIFF must NOT contain the secret, but must carry the
	// placeholder.
	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	xliffPath := filepath.Join(real, "out", entries[0].Name())
	xliffData, err := os.ReadFile(xliffPath)
	require.NoError(t, err)
	xliff := string(xliffData)
	assert.NotContains(t, xliff, "Mr Bean", "secret leaked into the extracted XLIFF")
	assert.Contains(t, xliff, "REDACTED", "placeholder missing from XLIFF")

	// The original lives only in the local vault sidecar.
	redactionDir := filepath.Join(project.LayoutAt(real).VaultDir(), project.VaultBatchesDirName)
	vaultEntries, err := os.ReadDir(redactionDir)
	require.NoError(t, err, "vault sidecar dir missing")
	require.Len(t, vaultEntries, 1)
	vaultData, err := os.ReadFile(filepath.Join(redactionDir, vaultEntries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(vaultData), "Mr Bean", "vault must hold the original")

	// Simulate a translator that preserves the placeholder: target = source
	// inner content with the leading word translated.
	m := xliffSourceInnerRe.FindStringSubmatch(xliff)
	require.Len(t, m, 2, "no <source> found in XLIFF:\n%s", xliff)
	targetInner := strings.Replace(m[1], "Hello", "Bonjour", 1)
	editXLIFFTarget(t, xliffPath, targetInner)

	// Merge — should restore "Mr Bean" from the vault into the merged target.
	mergeOut, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-memory-update")
	require.NoError(t, err, "merge stdout: %s", mergeOut)

	mergedPath := filepath.Join(real, "src", "locales", "fr-FR", "app.json")
	data, err := os.ReadFile(mergedPath)
	require.NoError(t, err, "expected merged file at %s", mergedPath)
	merged := string(data)
	assert.Contains(t, merged, "Bonjour", "translation missing")
	assert.Contains(t, merged, "Mr Bean", "redacted original was not restored on merge")
	assert.NotContains(t, merged, "REDACTED", "placeholder leaked into merged output")
}

// TestExtractRedact_ACollectionsPolicyApplies: a policy declared on the
// collection, with nothing under defaults and no flag, redacts the files the
// collection holds on a plain `kapi extract`, and merge restores them.
func TestExtractRedact_ACollectionsPolicyApplies(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := mergeProjectFixture(t, real)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Collections[0].Redaction = &project.RedactionSpec{Enabled: true, Rules: "redaction.yaml"}
	require.NoError(t, project.Save(recipe, proj))
	writeJSONSource(t, real, "src/locales/en/app.json", `{"greeting":"Hello Mr Bean"}`)
	require.NoError(t, os.WriteFile(filepath.Join(real, "redaction.yaml"), []byte(
		"version: v1\nrules:\n  - term: \"Mr Bean\"\n    category: person\n"), 0o644))

	out, err := runExtractCmd(t, recipe, "--no-memory")
	require.NoError(t, err, "extract stdout: %s", out)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	xliffPath := filepath.Join(real, "out", entries[0].Name())
	xliffData, err := os.ReadFile(xliffPath)
	require.NoError(t, err)
	assert.NotContains(t, string(xliffData), "Mr Bean", "the collection's policy keeps the value out of the XLIFF")
	assert.Contains(t, string(xliffData), "REDACTED")

	m := xliffSourceInnerRe.FindStringSubmatch(string(xliffData))
	require.Len(t, m, 2)
	editXLIFFTarget(t, xliffPath, strings.Replace(m[1], "Hello", "Bonjour", 1))
	mergeOut, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-memory-update")
	require.NoError(t, err, "merge stdout: %s", mergeOut)
	data, err := os.ReadFile(filepath.Join(real, "src", "locales", "fr-FR", "app.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Bonjour Mr Bean")
}

// TestExtractRedact_NoRestoreFlag verifies --no-restore leaves the placeholder
// in the merged output (originals stay only in the vault).
func TestExtractRedact_NoRestoreKeepsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := mergeProjectFixture(t, real)
	writeJSONSource(t, real, "src/locales/en/app.json", `{"greeting":"Hello Mr Bean"}`)
	rulesPath := filepath.Join(real, "redaction.yaml")
	require.NoError(t, os.WriteFile(rulesPath, []byte(
		"version: v1\nrules:\n  - term: \"Mr Bean\"\n    category: person\n"), 0o644))

	out, err := runExtractCmd(t, recipe, "--redact-rules", rulesPath, "--no-memory")
	require.NoError(t, err, "extract stdout: %s", out)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	xliffPath := filepath.Join(real, "out", entries[0].Name())
	xliffData, err := os.ReadFile(xliffPath)
	require.NoError(t, err)
	m := xliffSourceInnerRe.FindStringSubmatch(string(xliffData))
	require.Len(t, m, 2)
	editXLIFFTarget(t, xliffPath, strings.Replace(m[1], "Hello", "Bonjour", 1))

	mergeOut, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-memory-update", "--no-restore")
	require.NoError(t, err, "merge stdout: %s", mergeOut)

	mergedPath := filepath.Join(real, "src", "locales", "fr-FR", "app.json")
	data, err := os.ReadFile(mergedPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "REDACTED", "with --no-restore the placeholder should remain")
	assert.NotContains(t, string(data), "Mr Bean", "original must not appear when restore is skipped")
}

// An ad-hoc .kpz workspace extracted with --redact-rules holds a placeholder
// where the rules withhold a value, the original goes to the vault beside the
// package, and kapi merge restores it. --redact with no rules file writes
// nothing.
func TestExtractRedact_KpzWorkspaceRoundTrip(t *testing.T) {
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello Mr Bean"}`), 0o644))
	rules := filepath.Join(dir, "redaction.yaml")
	require.NoError(t, os.WriteFile(rules, []byte("version: v1\nrules:\n  - term: \"Mr Bean\"\n    category: person\n"), 0o644))
	work := filepath.Join(dir, "work.kpz")

	run := func(cmd interface {
		SetArgs([]string)
		SetOut(io.Writer)
		SetErr(io.Writer)
		Execute() error
	}, args ...string) error {
		cmd.SetArgs(args)
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		return cmd.Execute()
	}

	a := newExtractApp(t)
	err := run(NewExtractCmd(a, ExtractCmdOptions{}), src, "-o", work, "--target-lang", "fr", "--redact")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--redact-rules")
	assert.NoFileExists(t, work, "no workspace is written without a rules file")

	require.NoError(t, run(NewExtractCmd(a, ExtractCmdOptions{}), src, "-o", work, "--target-lang", "fr", "--redact-rules", rules, "--with-source"))
	zr, err := zip.OpenReader(work)
	require.NoError(t, err)
	defer zr.Close()
	sawSource := false
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, rc.Close())
		require.NoError(t, err)
		assert.NotContains(t, string(data), "Mr Bean", "%s withholds the name", f.Name)
		if f.Name == "source/app.json" {
			sawSource = true
			assert.Equal(t, `{"greeting":"Hello [REDACTED:Person]"}`, strings.TrimSpace(string(data)))
		}
	}
	assert.True(t, sawSource, "--with-source carries the redacted file")
	vault, err := os.ReadFile(work + ".vault")
	require.NoError(t, err, "the vault sits beside the package")
	assert.Contains(t, string(vault), "Mr Bean")

	outDir := filepath.Join(dir, "translated")
	require.NoError(t, run(NewMergeCmd(a, MergeCmdOptions{}), work, "-o", outDir))
	merged, err := os.ReadFile(filepath.Join(outDir, "app.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"greeting":"Hello Mr Bean"}`, strings.TrimSpace(string(merged)), "merge restores the withheld name")
}
