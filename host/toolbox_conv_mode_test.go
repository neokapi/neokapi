//go:build unix

package host

import (
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A new output is created under the umask, so a person whose umask is 077
// gets a file only they can read.
func TestConvNewOutputFollowsTheUmask(t *testing.T) {
	prev := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(prev) })

	app := newToolboxApp(t)
	dir := t.TempDir()
	in := writeToolboxFile(t, dir, "en.json", `{"a": "Hello"}`)
	out := dir + "/out.json"

	require.NoError(t, app.convertDocument(context.Background(), in, "json", "", out))
	info, err := os.Stat(out)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// An output that exists keeps its mode, whatever the umask would take off.
func TestConvExistingOutputKeepsItsMode(t *testing.T) {
	prev := syscall.Umask(0o077)
	t.Cleanup(func() { syscall.Umask(prev) })

	app := newToolboxApp(t)
	dir := t.TempDir()
	in := writeToolboxFile(t, dir, "en.json", `{"a": "Hello"}`)
	out := writeToolboxFile(t, dir, "out.json", "OLD")
	require.NoError(t, os.Chmod(out, 0o664))

	require.NoError(t, app.convertDocument(context.Background(), in, "json", "", out))
	info, err := os.Stat(out)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o664), info.Mode().Perm())
}
