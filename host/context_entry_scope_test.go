package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

const bookCatalog = `msgid ""
msgstr ""
"Language: nb\n"

msgctxt "button"
msgid "Book"
msgstr "Bestill"

msgctxt "library"
msgid "Book"
msgstr "Bok"
`

// bookProject is a project whose Norwegian catalog translates "Book" two
// ways, by the entry's context.
func bookProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "nb"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "nb", "messages.po"), []byte(bookCatalog), 0o600))
	require.NoError(t, os.WriteFile(recipeOf(root), []byte("version: v1\nid: "+projectIDFor("book")+"\nname: book\n"+
		"defaults:\n  source_language: en\n  target_languages: [nb]\ncollections:\n"+
		"  - name: web\n    content:\n      - path: locales/nb/*.po\n        format:\n          name: po\n"), 0o600))
	return root
}

// A change made at one entry of a catalog implies no rule for the whole file
// when another entry holds the wording the rule would avoid: "Bestill, not
// Bok" made at the button would report the library's "Bok". The correction is
// recorded as evidence of that entry, and the rule is refused with the entry
// it would report.
func TestContextRules_KeepTheEntryAChangeWasMadeAt(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := bookProject(t)
	at := []contextop.Evidence{{Path: "locales/nb/messages.po", Unit: "button/Book"}}

	_, err := app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), From: "Bok", To: "Bestill", Evidence: at, Suggest: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `locales/nb/messages.po holds "Bok" at library/Book as well as at button/Book`)
	assert.Contains(t, err.Error(), "record the correction without the rule")

	_, err = app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), Term: "Bestill", InsteadOf: []string{"Bok"}, Evidence: at,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "record the observation without the rule")

	op, err := app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), From: "Bok", To: "Bestill", Evidence: at,
	})
	require.NoError(t, err, "the correction itself is evidence of the entry")
	assert.Equal(t, "button/Book", op.Evidence[0].Unit)
	assert.Nil(t, op.Subject.Term)

	// Evidence that names no entry is weighed as the file, as before.
	_, err = app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), From: "Bok", To: "Bestill",
		Evidence: []contextop.Evidence{{Path: "locales/nb/messages.po"}}, Suggest: true,
	})
	assert.NoError(t, err)
}
