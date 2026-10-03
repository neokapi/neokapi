package filehome_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// TestRead_CreatesNoTemporaryFile pins that a read, joined with an edition's
// own file or not, writes no file: the skeleton its reader is given keeps
// nothing, and an MDX document's markdown spans keep theirs in memory. The
// temporary directory is made read-only, so a read that created a file there
// fails.
func TestRead_CreatesNoTemporaryFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the test cannot write")
	}
	f := newTargetFixture(t, map[string]string{
		"guide.md":    "# Install\n\nRun the **installer**.\n",
		"fr/guide.md": "# Installer\n\nLancez l'**installateur**.\n",
		"page.mdx":    "import Note from './note'\n\n# Title\n\nSome *text* here.\n\n<Note>\n\nA **note** inside.\n\n</Note>\n",
	})
	tmp := t.TempDir()
	require.NoError(t, os.Chmod(tmp, 0o500))
	t.Cleanup(func() { _ = os.Chmod(tmp, 0o700) })
	t.Setenv("TMPDIR", tmp)

	ctx := context.Background()
	fr := mustEdition(t, "fr")
	tests := []struct {
		name     string
		doc      string
		editions []model.EditionKey
		joined   string
	}{
		{name: "a Markdown document", doc: "guide.md"},
		{name: "an MDX document", doc: "page.mdx"},
		{name: "a document joined with its translation's file", doc: "guide.md", editions: []model.EditionKey{fr}, joined: "Installer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := f.svc.Read(ctx, change.ReadRequest{Doc: tt.doc, Editions: tt.editions})
			require.NoError(t, err)
			require.NotEmpty(t, page.Blocks)
			if tt.joined != "" {
				var texts []string
				for _, b := range page.Blocks {
					texts = append(texts, b.Editions["fr"].Text)
				}
				assert.Contains(t, texts, tt.joined, "the read joins the translation")
			}
			entries, err := os.ReadDir(tmp)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}
