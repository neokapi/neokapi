package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/output"
)

// `kapi ls` names the format of the item that claims a file's values, whichever
// item the recipe lists first. A comments-only item that also matches the file
// supplies no format for it.
func TestLs_NamesTheFormatOfTheItemThatClaimsTheValues(t *testing.T) {
	const values = `  - name: docs
    content:
      - path: "docs/*.md"
        format:
          name: mdx
`
	const comments = `  - name: comments
    content:
      - path: "docs/**/*.md"
        comments:
          only: true
`
	for name, commentsFirst := range map[string]bool{"the comments-only item first": true, "the value item first": false} {
		t.Run(name, func(t *testing.T) {
			a := processOnlyApp(t)
			real, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			collections := values + comments
			if commentsFirst {
				collections = comments + values
			}
			recipe := filepath.Join(real, "kapi.yaml")
			require.NoError(t, os.WriteFile(recipe, []byte("version: v1\nname: LsClaimOrder\ndefaults:\n  source_language: en-US\ncollections:\n"+collections), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Join(real, "docs"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(real, "docs", "guide.md"), []byte("# Guide\n\nRead this first.\n"), 0o644))

			cmd := NewLsCmd(a)
			cmd.SetArgs([]string{"--project", recipe, "--output-format", "json"})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			require.NoError(t, cmd.Execute(), out.String())

			var listed output.LsOutput
			require.NoError(t, json.Unmarshal(out.Bytes(), &listed), out.String())
			assert.Equal(t, []output.LsEntry{{Path: "docs/guide.md", Format: "mdx"}}, listed.Files)
		})
	}
}
