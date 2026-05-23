package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
	out, err := runExtractCmd(t, recipe, "--redact-rules", rulesPath, "--no-tm")
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
	redactionDir := filepath.Join(real, ".kapi", "cache", "redaction")
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
	mergeOut, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-tm-update")
	require.NoError(t, err, "merge stdout: %s", mergeOut)

	mergedPath := filepath.Join(real, "src", "locales", "fr-FR", "app.json")
	data, err := os.ReadFile(mergedPath)
	require.NoError(t, err, "expected merged file at %s", mergedPath)
	merged := string(data)
	assert.Contains(t, merged, "Bonjour", "translation missing")
	assert.Contains(t, merged, "Mr Bean", "redacted original was not restored on merge")
	assert.NotContains(t, merged, "REDACTED", "placeholder leaked into merged output")
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

	out, err := runExtractCmd(t, recipe, "--redact-rules", rulesPath, "--no-tm")
	require.NoError(t, err, "extract stdout: %s", out)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	xliffPath := filepath.Join(real, "out", entries[0].Name())
	xliffData, err := os.ReadFile(xliffPath)
	require.NoError(t, err)
	m := xliffSourceInnerRe.FindStringSubmatch(string(xliffData))
	require.Len(t, m, 2)
	editXLIFFTarget(t, xliffPath, strings.Replace(m[1], "Hello", "Bonjour", 1))

	mergeOut, err := runMergeCmd(t, recipe, "-i", xliffPath, "--no-tm-update", "--no-restore")
	require.NoError(t, err, "merge stdout: %s", mergeOut)

	mergedPath := filepath.Join(real, "src", "locales", "fr-FR", "app.json")
	data, err := os.ReadFile(mergedPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "REDACTED", "with --no-restore the placeholder should remain")
	assert.NotContains(t, string(data), "Mr Bean", "original must not appear when restore is skipped")
}
