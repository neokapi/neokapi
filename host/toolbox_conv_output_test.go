package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/registry"
)

// A conversion that fails leaves a file the user already had at the output
// path exactly as it was. The json writer creates its output at SetOutput,
// and the kbf writer only records the path and writes at Close; both must
// leave the file alone.
func TestConvFailureKeepsAnExistingOutput(t *testing.T) {
	for _, tc := range []struct {
		format registry.FormatID
		ext    string
	}{
		{"json", ".json"},
		{"kbf", ".kbf.json"},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			app := newToolboxApp(t)
			app.FormatFlag = string(tc.format)
			dir := t.TempDir()
			in := writeToolboxFile(t, dir, "bad"+tc.ext, "{ not valid")
			out := writeToolboxFile(t, dir, "keep"+tc.ext, "USER DATA")

			err := app.convertDocument(context.Background(), in, tc.format, "", out)
			require.Error(t, err)
			got, rerr := os.ReadFile(out)
			require.NoError(t, rerr, "the existing output must survive the failed conversion")
			assert.Equal(t, "USER DATA", string(got))
			assertNoStagedOutput(t, dir)
		})
	}
}

// An input named through a symbolic link is the same file as its target, so
// a failed conversion whose output names the target never removes it.
func TestConvFailureNeverRemovesTheInputThroughALink(t *testing.T) {
	app := newToolboxApp(t)
	app.FormatFlag = "json"
	dir := t.TempDir()
	real := writeToolboxFile(t, dir, "real.json", "{ not valid")
	link := filepath.Join(dir, "link.json")
	require.NoError(t, os.Symlink(real, link))

	require.Error(t, app.convertDocument(context.Background(), link, "json", "", real))
	got, err := os.ReadFile(real)
	require.NoError(t, err, "the input must survive the failed conversion")
	assert.Equal(t, "{ not valid", string(got))
	assertNoStagedOutput(t, dir)
}

// A conversion that succeeds replaces the output, and one written through a
// symbolic link replaces the file the link names and keeps the link.
func TestConvSuccessWritesThroughALink(t *testing.T) {
	app := newToolboxApp(t)
	dir := t.TempDir()
	in := writeToolboxFile(t, dir, "en.json", `{"a": "Hello"}`)
	target := writeToolboxFile(t, dir, "target.json", "OLD")
	link := filepath.Join(dir, "out.json")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, app.convertDocument(context.Background(), in, "json", "", link))
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.JSONEq(t, `{"a": "Hello"}`, string(got))
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link stays a link")
	assertNoStagedOutput(t, dir)
}

// assertNoStagedOutput fails when a conversion's temporary output is left in
// dir.
func assertNoStagedOutput(t *testing.T, dir string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dir, ".kconv-*"))
	require.NoError(t, err)
	assert.Empty(t, left, "no temporary output is left behind")
}
