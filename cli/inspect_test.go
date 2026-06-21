package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runInspectFixture writes content to a temp file of the given name and runs
// `kapi inspect` over it, returning captured stdout.
func runInspectFixture(t *testing.T, name, content string, args ...string) string {
	t.Helper()
	app := newAppForTest(t)
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cmd := app.NewInspectCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(args, path))
	require.NoError(t, cmd.Execute())
	return out.String()
}

func TestInspect_AnchoredBlocks(t *testing.T) {
	out := runInspectFixture(t, "en.json", `{"greeting":"Hello","farewell":"Bye"}`)

	var blocks []inspectBlock
	require.NoError(t, json.Unmarshal([]byte(out), &blocks))
	require.Len(t, blocks, 2)

	for _, b := range blocks {
		require.NotEmpty(t, b.Text)
		// The anchor is the content hash of the block's text: stable and
		// reproducible, so an agent can retrieve and write back to it.
		assert.Equal(t, model.ComputeContentHash(b.Text), b.ContentHash)
		assert.NotEmpty(t, b.ID)
	}
}

func TestInspect_JSONLStreamsOnePerLine(t *testing.T) {
	out := runInspectFixture(t, "en.json", `{"a":"one","b":"two","c":"three"}`, "--jsonl")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3, "three blocks => three JSONL lines")
	for _, ln := range lines {
		var b inspectBlock
		require.NoError(t, json.Unmarshal([]byte(ln), &b), "each line is a JSON object")
		assert.NotEmpty(t, b.ContentHash)
		assert.NotEmpty(t, b.Text)
	}
}

// Markdown carries structural roles, so a heading block reports its role.
func TestInspect_StructuralRole(t *testing.T) {
	out := runInspectFixture(t, "page.md", "# Title\n\nA paragraph.\n")

	var blocks []inspectBlock
	require.NoError(t, json.Unmarshal([]byte(out), &blocks))

	var heading *inspectBlock
	for i := range blocks {
		if blocks[i].Text == "Title" {
			heading = &blocks[i]
		}
	}
	require.NotNil(t, heading, "heading block not found")
	assert.Equal(t, model.RoleHeading, heading.Role)
}

// Number is a 1-based counter that increments across all input files, and File
// records which input each block came from — the output contract for piping.
func TestInspect_NumberAndFileAcrossFiles(t *testing.T) {
	app := newAppForTest(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	require.NoError(t, os.WriteFile(a, []byte(`{"x":"one"}`), 0o644))
	require.NoError(t, os.WriteFile(b, []byte(`{"y":"two","z":"three"}`), 0o644))

	cmd := app.NewInspectCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{a, b})
	require.NoError(t, cmd.Execute())

	var blocks []inspectBlock
	require.NoError(t, json.Unmarshal(out.Bytes(), &blocks))
	require.Len(t, blocks, 3)

	for i, blk := range blocks {
		assert.Equal(t, i+1, blk.Number, "Number is a global 1-based counter across files")
		assert.NotEmpty(t, blk.File, "every record carries its source file")
	}
	assert.Equal(t, displayName(a), blocks[0].File)
	assert.Equal(t, displayName(b), blocks[1].File)
	assert.Equal(t, displayName(b), blocks[2].File)
}
