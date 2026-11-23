package keychain

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// The test keyring is in place before any test runs, so this package's first
// use keeps what is already in it. Installed on first use, it replaced the
// keyring and lost a secret stored before that use, which made a test pass
// or fail on which test reached the keychain first. The secret is stored
// through the library, which is how a store before this package's first use
// looks. It is the first test in this file, and tests run in source order,
// so it runs before any other test touches the package.
func TestAFirstUseKeepsWhatTheKeyringHolds(t *testing.T) {
	require.NoError(t, keyring.Set("svc", "user", "kept"))

	got, err := Get("svc", "user")
	require.NoError(t, err, "the first use of this package must not empty the keyring")
	assert.Equal(t, "kept", got)

	require.NoError(t, Set("svc", "other", "second"))
	got, err = keyring.Get("svc", "other")
	require.NoError(t, err)
	assert.Equal(t, "second", got, "this package and the keyring it stands in front of agree")
}

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
