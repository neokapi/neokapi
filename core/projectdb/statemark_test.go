//go:build unix

package projectdb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
)

// A run with KAPI_TRUST_STATE=1 over state from elsewhere records nothing: the
// next run without the variable is refused as the first would have been. The
// vault here arrived in a checkout kapi made, so the directory's own mark
// verifies.
func TestARunTheTrustOverrideAllowedRecordsNothing(t *testing.T) {
	l := newLayout(t)
	db, err := projectdb.Open(t.Context(), l)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.NoError(t, os.MkdirAll(l.VaultDir(), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(l.VaultDir(), "redaction.json"), []byte(`{"from":"elsewhere"}`), 0o600))
	t.Setenv(project.TrustStateEnv, "1")
	db, err = projectdb.Open(t.Context(), l)
	require.NoError(t, err, "the person vouches for this run")
	require.NoError(t, db.Close())

	t.Setenv(project.TrustStateEnv, "")
	prev := project.StateNotice
	project.StateNotice = func(string) {}
	t.Cleanup(func() { project.StateNotice = prev })
	_, err = projectdb.Open(t.Context(), l)
	var unverified *project.UnverifiedStateError
	require.ErrorAs(t, err, &unverified, "nothing the first run saw was recorded")
	assert.Contains(t, unverified.Entries, ".kapi/vault")
}

// A project store from another checkout copied onto this one's, in place,
// records the other checkout's mark, and is set aside.
func TestAStoreFromAnotherCheckoutIsSetAside(t *testing.T) {
	a := newLayout(t)
	db, err := projectdb.Open(t.Context(), a)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	b := newLayout(t)
	db, err = projectdb.Open(t.Context(), b)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	data, err := os.ReadFile(a.StorePath())
	require.NoError(t, err)
	for _, sidecar := range []string{"-wal", "-shm"} {
		_ = os.Remove(b.StorePath() + sidecar)
	}
	f, err := os.OpenFile(b.StorePath(), os.O_WRONLY|os.O_TRUNC, 0)
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var notices []string
	prev := project.StateNotice
	project.StateNotice = func(msg string) { notices = append(notices, msg) }
	t.Cleanup(func() { project.StateNotice = prev })
	db, err = projectdb.Open(t.Context(), b)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.Len(t, notices, 1)
	assert.Contains(t, notices[0], ".kapi/work was not written by kapi on this machine")
	id, ok, err := db.Meta(t.Context(), projectdb.MetaStateMarkID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, project.StateMarkID(b), id, "the store opened again records this checkout's mark")
}
