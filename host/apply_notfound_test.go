package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A content entry whose id, or content_hash when it gives no id, names no
// block of its file wrote nothing. `kapi apply` lists it under not_found and
// exits on the gate code, as it does for a stale entry, while the entries that
// did match still land.
func TestApplyReportsAnEntryThatMatchesNoBlock(t *testing.T) {
	const source = `{"greeting":"Hello world","note":"Keep me"}`
	hello := model.ComputeContentHash("Hello world")
	tests := []struct {
		name    string
		diff    bool
		missing changeEntry
		want    string
	}{
		{
			name:    "an id no block has",
			missing: changeEntry{Kind: kindContent, ID: "no-such-block", Text: "x"},
			want:    "no-such-block",
		},
		{
			name:    "a content hash no block has",
			missing: changeEntry{Kind: kindContent, ContentHash: model.ComputeContentHash("Goodbye"), Text: "x"},
			want:    coretools.NotFoundHashPrefix + model.ComputeContentHash("Goodbye"),
		},
		{
			name:    "an id no block has, previewed with --diff",
			diff:    true,
			missing: changeEntry{Kind: kindContent, ID: "no-such-block", Text: "x"},
			want:    "no-such-block",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newToolboxApp(t)
			dir := t.TempDir()
			file := filepath.Join(dir, "en.json")
			require.NoError(t, os.WriteFile(file, []byte(source), 0o644))

			missing := tt.missing
			missing.File = file
			body, err := json.Marshal([]changeEntry{
				missing,
				{Kind: kindContent, File: file, ContentHash: hello, Text: "Hi planet"},
			})
			require.NoError(t, err)
			changeset := filepath.Join(dir, "edits.json")
			require.NoError(t, os.WriteFile(changeset, body, 0o644))

			cmd := NewEnvCommand(t.Context(), "apply")
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			err = app.RunApply(cmd, changeset, tt.diff, "", true)
			require.Error(t, err, "an entry that matched no block keeps the change-set from passing")
			assert.Equal(t, ExitGate, ExitCode(cmd, err))

			var out applyOutput
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), stdout.String())
			assert.Equal(t, []string{file + ":" + tt.want}, out.Content.NotFound)

			got, err := os.ReadFile(file)
			require.NoError(t, err)
			if tt.diff {
				assert.Equal(t, source, string(got), "--diff writes nothing")
				return
			}
			assert.Len(t, out.Content.Applied, 1, "the entry that matched still lands")
			assert.JSONEq(t, `{"greeting":"Hi planet","note":"Keep me"}`, string(got))
		})
	}
}

// The summary a person reads counts the entries that matched no block and names
// each one.
func TestPrintApplyReportNamesNotFound(t *testing.T) {
	var out applyOutput
	out.Content.Applied = []string{"p1"}
	out.Content.NotFound = []string{"docs/a.md:p9"}

	var buf bytes.Buffer
	printApplyReport(&buf, &out)

	assert.Equal(t,
		"content: 1 applied, 0 unchanged, 1 not found (no block has that id or content_hash, re-inspect)\n"+
			"content docs/a.md:p9: not found\n",
		buf.String())
	assert.False(t, out.ok())
}

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

// The summary counts the blocks an entry changed that the file marks as not
// editable, and such a block keeps the change-set from passing.
func TestPrintApplyReportCountsNotEditable(t *testing.T) {
	var out applyOutput
	out.Content.Skipped = []string{"tu1"}
	out.Content.NotEditable = []string{"tu3"}

	var buf bytes.Buffer
	printApplyReport(&buf, &out)

	assert.Equal(t,
		"content: 0 applied, 1 unchanged, 1 not editable (the file marks the block as content an edit does not change, such as code)\n",
		buf.String())
	assert.False(t, out.ok())
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
			_, recs := inspectRecords(t, app, file)
			codeID, codeText := "", ""
			for _, rec := range recs {
				if rec.Role == "code" {
					codeID, codeText = rec.ID, rec.Text
				}
			}
			require.NotEmpty(t, codeID, "%+v", recs)

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
