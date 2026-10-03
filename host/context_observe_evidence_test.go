package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// An observation says a term was seen in a file. One whose file holds
// neither the term nor a form it avoids is refused, and so is a form to avoid
// that is a description of the term rather than a form of it.
func TestRecordContextObservation_HoldsTheEvidenceToTheFile(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-evidence")
	observe := func(term string, insteadOf ...string) error {
		_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: agentIn("s1"), Project: recipeOf(root), Term: term, InsteadOf: insteadOf,
			Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
		})
		return err
	}

	err := observe("Harbor Help", "HarborHelp")
	require.Error(t, err, "config/app.yaml holds no Harbor Help")
	assert.Contains(t, err.Error(), `config/app.yaml holds neither "Harbor Help" nor a form it avoids`)

	err = observe("Quickcast", "Quickcast product name variant")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds the term \"Quickcast\" with more words")

	assert.NoError(t, observe("Quickcast", "Quick cast"), "the file holds the form it avoids")
}

// A file holds a term when a reader of it sees the term: across a line a
// writer wrapped, spelled with a character reference or a JSON escape, or in
// a format whose bytes are compressed. A file no reader opens is passed over.
func TestRecordContextObservation_SeesTheTermAsAReaderDoes(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-evidence-read")
	write := func(name string, body []byte) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, body, 0o600))
	}
	write("docs/wrapped.md", []byte("# Support\n\nAsk the desk, or open Harbor\nHelp from the menu.\n"))
	write("site/menu.html", []byte("<html><body><p>Meet us at Caf&eacute; Harbor today.</p></body></html>\n"))
	write("locales/menu.json", []byte("{\"title\": \"Caf\\u00e9 Harbor\"}\n"))
	write("docs/guide.docx", docxOf(t, "Open Harbor Help from the menu."))
	write("assets/logo.bin", []byte{0x00, 0x01, 0xff, 0xfe, 0x00, 0x89, 0x50, 0x4e, 0x47})

	observe := func(path, term string, insteadOf ...string) error {
		_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: agentIn("s1"), Project: recipeOf(root), Term: term, InsteadOf: insteadOf,
			Evidence: []contextop.Evidence{{Path: path}},
		})
		return err
	}
	for _, c := range []struct{ path, term, avoid string }{
		{"docs/wrapped.md", "Harbor Help", "HarborHelp"},
		{"site/menu.html", "Café Harbor", "Cafe Harbor"},
		{"locales/menu.json", "Café Harbor", "Cafe Harbor"},
		{"docs/guide.docx", "Harbor Help", "HarborHelp"},
		{"assets/logo.bin", "Harbor Help", "HarborHelp"},
	} {
		assert.NoError(t, observe(c.path, c.term, c.avoid), c.path)
	}

	err := observe("site/menu.html", "Quickcast", "Quick cast")
	require.Error(t, err, "the page holds neither form")
	assert.Contains(t, err.Error(), `site/menu.html holds neither "Quickcast" nor a form it avoids`)
}

func TestDescribesTerm(t *testing.T) {
	assert.True(t, describesTerm("Harbor Help", "Harbor Help product name variant"))
	assert.False(t, describesTerm("Harbor Help", "HarborHelp"))
	assert.False(t, describesTerm("Harbor Help", "the Harbor Help"), "one word more can be a form")
	assert.False(t, describesTerm("sign in", "log in"))
}
