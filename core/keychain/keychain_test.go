package keychain

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A test binary's keyring is in memory: a secret set there is read back, and
// Reset empties it.
func TestATestBinaryKeepsSecretsInMemory(t *testing.T) {
	Reset()
	require.NoError(t, Set("kapi-test", "user", "s3cret"))
	got, err := Get("kapi-test", "user")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", got)

	Reset()
	_, err = Get("kapi-test", "user")
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, Set("kapi-test", "user", "again"))
	require.NoError(t, Delete("kapi-test", "user"))
	_, err = Get("kapi-test", "user")
	require.ErrorIs(t, err, ErrNotFound)
}

// Reset in a binary `go test` did not build panics: the real keychain is
// never emptied.
func TestResetRefusesOutsideATestBinary(t *testing.T) {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skip("no subprocesses")
	}
	bin := filepath.Join(t.TempDir(), "reset")
	out, err := exec.Command("go", "build", "-trimpath", "-o", bin, "./testdata/reset").CombinedOutput()
	require.NoError(t, err, "%s", out)

	out, err = exec.Command(bin).CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(out), "keychain: Reset is for test binaries only")
}
