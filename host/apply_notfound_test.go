package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apply_edits reports the same bucket, and an entry in it makes ok false.
func TestApplyEditsMCPReportsNotFound(t *testing.T) {
	app := newToolboxApp(t)
	file := filepath.Join(t.TempDir(), "page.json")
	const source = `{"title":"Before"}`
	require.NoError(t, os.WriteFile(file, []byte(source), 0o600))

	_, out, err := app.applyEditsMCP(t.Context(), contextop.Actor{Kind: contextop.ActorAgent, Name: "test", Session: "s1"},
		applyEditsInput{Changeset: []changeEntry{{Kind: kindContent, File: file, ID: "no-such-block", Text: "After"}}})
	require.NoError(t, err)
	assert.False(t, out.OK)
	assert.Equal(t, []string{file + ":no-such-block"}, out.NotFound)
	assert.Empty(t, out.Applied)

	got, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, source, string(got))
}

// apply_edits reports a change to a block that is not editable, such as a
// Markdown code block, in not_editable, writes nothing for it and makes ok
// false, while the same entry with the block's own text reads as applied
// already.
func TestApplyEditsMCPReportsNotEditable(t *testing.T) {
	const doc = "Run the tool.\n\n```sh\nkapi up\n```\n"
	tests := []struct {
		name string
		text func(read string) string
		ok   bool
	}{
		{name: "the code block's own text", text: func(read string) string { return read }, ok: true},
		{name: "a changed text", text: func(string) string { return "kapi down" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newToolboxApp(t)
			file := filepath.Join(t.TempDir(), "doc.md")
			require.NoError(t, os.WriteFile(file, []byte(doc), 0o600))
			codeID, codeText := "", ""
			_, rerr := app.StreamEditableBlocksAs(t.Context(), file, "", func(_ int, b *model.Block) error {
				if s, ok := b.Structure(); ok && s.Role == "code" {
					codeID, codeText = b.ID, model.RunsEditText(b.SourceRuns())
				}
				return nil
			})
			require.NoError(t, rerr)
			require.NotEmpty(t, codeID)

			_, out, err := app.applyEditsMCP(t.Context(), contextop.Actor{Kind: contextop.ActorAgent, Name: "test", Session: "s1"},
				applyEditsInput{Changeset: []changeEntry{{Kind: kindContent, File: file, ID: codeID, Text: tt.text(codeText)}}})
			require.NoError(t, err)
			assert.Equal(t, tt.ok, out.OK)
			assert.Empty(t, out.NotFound)
			if tt.ok {
				assert.Equal(t, []string{codeID}, out.Skipped)
				assert.Empty(t, out.NotEditable)
			} else {
				assert.Equal(t, []string{codeID}, out.NotEditable)
			}

			got, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, doc, string(got))
		})
	}
}
