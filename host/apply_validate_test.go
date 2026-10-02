package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A content entry that names no block, and two entries that edit one block
// differently, make the change-set malformed. Each surface refuses it before
// writing anything, and `kapi apply` exits on the usage code. Two identical
// entries are one edit.
func TestApplyRefusesContentEntriesThatNameNoBlockOrCollide(t *testing.T) {
	const source = `{"greeting":"Hello world","note":"Keep me"}`
	hello := model.ComputeContentHash("Hello world")
	tests := []struct {
		name    string
		entries []changeEntry
		refusal string // a substring of the refusal; empty when the change-set applies
		want    string // the file after a change-set that applies
	}{
		{
			name:    "an entry with neither id nor content_hash",
			entries: []changeEntry{{Kind: kindContent, Text: "Bye"}},
			refusal: "content entry 1 in ",
		},
		{
			name: "two entries for one id with different text",
			entries: []changeEntry{
				{Kind: kindContent, ID: "greeting", ContentHash: hello, Text: "First"},
				{Kind: kindContent, ID: "greeting", ContentHash: hello, Text: "Second"},
			},
			refusal: `content entries 1 and 2 both edit the block with id "greeting"`,
		},
		{
			name: "two entries for one content_hash with different text",
			entries: []changeEntry{
				{Kind: kindContent, ContentHash: hello, Text: "First"},
				{Kind: kindContent, ContentHash: hello, Text: "Second"},
			},
			refusal: `content entries 1 and 2 both edit the block with content_hash "` + hello + `"`,
		},
		{
			name: "two entries for one id with different content_hash",
			entries: []changeEntry{
				{Kind: kindContent, ID: "greeting", ContentHash: hello, Text: "Hi planet"},
				{Kind: kindContent, ID: "greeting", ContentHash: model.ComputeContentHash("Goodbye"), Text: "Hi planet"},
			},
			refusal: `content entries 1 and 2 both edit the block with id "greeting"`,
		},
		{
			name: "two identical entries",
			entries: []changeEntry{
				{Kind: kindContent, ContentHash: hello, Text: "Hi planet"},
				{Kind: kindContent, ContentHash: hello, Text: "Hi planet"},
			},
			want: `{"greeting":"Hi planet","note":"Keep me"}`,
		},
	}
	for _, tt := range tests {
		for _, surface := range []string{"kapi apply", "apply_edits"} {
			t.Run(tt.name+"/"+surface, func(t *testing.T) {
				app := newToolboxApp(t)
				dir := t.TempDir()
				file := filepath.Join(dir, "en.json")
				require.NoError(t, os.WriteFile(file, []byte(source), 0o600))
				entries := make([]changeEntry, len(tt.entries))
				for i, e := range tt.entries {
					e.File = file
					entries[i] = e
				}

				var err error
				if surface == "kapi apply" {
					body, merr := json.Marshal(entries)
					require.NoError(t, merr)
					changeset := filepath.Join(dir, "edits.json")
					require.NoError(t, os.WriteFile(changeset, body, 0o600))
					cmd := NewEnvCommand(t.Context(), "apply")
					var stdout, stderr bytes.Buffer
					cmd.SetOut(&stdout)
					cmd.SetErr(&stderr)
					err = app.RunApply(cmd, changeset, false, "", true)
					if tt.refusal != "" {
						require.Error(t, err)
						assert.Equal(t, ExitUsage, ExitCode(cmd, err), "a malformed change-set is a usage error")
						assert.Empty(t, stdout.String(), "nothing was applied, so there is no report")
					}
				} else {
					_, _, err = app.applyEditsMCP(t.Context(), contextop.Actor{Kind: contextop.ActorAgent, Name: "test", Session: "s1"},
						applyEditsInput{Changeset: entries})
				}

				got, rerr := os.ReadFile(file)
				require.NoError(t, rerr)
				if tt.refusal == "" {
					require.NoError(t, err)
					assert.JSONEq(t, tt.want, string(got))
					return
				}
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.refusal)
				assert.Equal(t, source, string(got), "a refused change-set writes nothing")
			})
		}
	}
}
