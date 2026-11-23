package keychain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// The test keyring is in place before any test runs, so this package's first
// use keeps what is already in it. Installed on first use, it replaced the
// keyring and lost a secret stored before that use, which made a test pass
// or fail on which test reached the keychain first.
func TestTheFirstUseKeepsWhatTheKeyringHolds(t *testing.T) {
	require.NoError(t, keyring.Set("svc", "user", "kept"))

	got, err := Get("svc", "user")
	require.NoError(t, err, "the first use of this package must not empty the keyring")
	assert.Equal(t, "kept", got)

	require.NoError(t, Set("svc", "other", "second"))
	require.NoError(t, Delete("svc", "user"))
	_, err = Get("svc", "user")
	require.ErrorIs(t, err, ErrNotFound)
	got, err = keyring.Get("svc", "other")
	require.NoError(t, err)
	assert.Equal(t, "second", got, "this package and the keyring it stands in front of agree")
}
