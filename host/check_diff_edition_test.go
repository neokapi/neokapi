package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// A diff-scoped check that cannot tell a PO catalog's entries apart names each
// one by key and edition and ends with the command that checks the file whole,
// and a translation file the recipe writes for a source is named as that
// source's edition with the command that checks it.
func TestDiffCheck_NamesTheEditionAndTheCommandThatRuns(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "locales", "nb"), 0o755))
	po := "msgid \"\"\nmsgstr \"\"\n\"Language: nb\\n\"\n\nmsgctxt \"button\"\nmsgid \"Book\"\nmsgstr \"Bestill\"\n\n" +
		"msgctxt \"library\"\nmsgid \"Book\"\nmsgstr \"Bok\"\n"
	writeCheckInput(t, dir, "locales/nb/messages.po", po)
	writeCheckInput(t, dir, "locales/en.json", `{"a":"Book a time"}`+"\n")
	writeCheckInput(t, dir, "locales/nb.json", `{"a":"Book a time"}`+"\n")
	recipe := writeCheckInput(t, dir, "kapi.yaml", "version: v1\ndefaults:\n  source_language: en\n  target_languages: [nb]\ncollections:\n"+
		"  - name: web\n    content:\n      - path: locales/nb/*.po\n        format:\n          name: po\n"+
		"  - name: app\n    content:\n      - path: locales/en.json\n        target: locales/{lang}.json\n")
	patch := writeCheckInput(t, dir, "change.diff",
		"diff --git a/locales/nb/messages.po b/locales/nb/messages.po\n--- a/locales/nb/messages.po\n+++ b/locales/nb/messages.po\n"+
			"@@ -7 +7 @@\n-msgstr \"Bok\"\n+msgstr \"Bestill\"\n"+
			"diff --git a/locales/nb.json b/locales/nb.json\n--- a/locales/nb.json\n+++ b/locales/nb.json\n@@ -1 +1 @@\n-{\"a\":\"Bestill en time\"}\n+{\"a\":\"Book a time\"}\n")
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", recipe)
	t.Chdir(dir)

	cmd := diffCommand(t)
	require.NoError(t, cmd.Flags().Set("diff-file", patch))
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)

	catalog := scopeEntry(t, report, "locales/nb/messages.po")
	assert.Equal(t, check.ScopeDidNotRun, catalog.Status)
	assert.Contains(t, catalog.Reason, "block button/Book@nb")
	assert.Contains(t, catalog.Reason, "; to check the whole file, run `kapi check locales/nb/messages.po`")

	edition := scopeEntry(t, report, "locales/nb.json")
	assert.Equal(t, check.ScopeOutOfScope, edition.Status)
	assert.Equal(t, "the nb edition of locales/en.json, and a check of a diff covers source content: "+
		"to check the translation, run `kapi check locales/en.json --target locales/nb.json --target-lang nb`", edition.Reason)
}
