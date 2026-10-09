package pgtest

import (
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Many sessions anchoring the same absent extension at once all succeed, and
// the extension ends up in public once. This is the shape of the integration
// lane, where every test binary's first NewTestDB races the others on one
// database; without the advisory lock one of them lost with 23505.
//
// The extension under test is one nothing else in the suite uses, so the test
// can drop it first and race its creation for real, while pg_trgm stays in
// place for every other test sharing the server.
func TestAnchorExtensionInPublic_ConcurrentCreatorsAllSucceed(t *testing.T) {
	NewTestDB(t)
	const ext = "citext"

	_, err := sharedDB.Exec("DROP EXTENSION IF EXISTS " + ext)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = sharedDB.Exec("DROP EXTENSION IF EXISTS " + ext) })

	const creators = 16
	errs := make(chan error, creators)
	var wg sync.WaitGroup
	for range creators {
		wg.Go(func() {
			errs <- anchorExtensionInPublic(sharedDB, ext)
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var schema string
	var n int
	require.NoError(t, sharedDB.QueryRow(`SELECT n.nspname, count(*) OVER ()
		FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace
		WHERE e.extname = $1`, ext).Scan(&schema, &n))
	assert.Equal(t, "public", schema)
	assert.Equal(t, 1, n)
}

// A creator that races a session which took no lock sees the catalog's own
// answer, and that answer means the extension is there.
func TestExtensionAlreadyExists(t *testing.T) {
	assert.True(t, extensionAlreadyExists(&pgconn.PgError{Code: "23505"}))
	assert.True(t, extensionAlreadyExists(&pgconn.PgError{Code: "42710"}))
	assert.True(t, extensionAlreadyExists(errors.Join(errors.New("wrapped"), &pgconn.PgError{Code: "42710"})))
	assert.False(t, extensionAlreadyExists(&pgconn.PgError{Code: "42501"}))
	assert.False(t, extensionAlreadyExists(errors.New("not a pg error")))
	assert.False(t, extensionAlreadyExists(nil))
}
