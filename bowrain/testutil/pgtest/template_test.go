package pgtest_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The template is this binary's declared starting schema. It is deliberately
// not a real migration set: what these tests are about is whether what the
// template built is what a test finds, and one table says that as well as two
// hundred would while making a failure readable.
func TestMain(m *testing.M) {
	pgtest.UseTemplate(func(db *storage.PgDB) error {
		_, err := db.Exec(`CREATE TABLE from_template (id int PRIMARY KEY, note text)`)
		return err
	})
	os.Exit(m.Run())
}

// A test starts from the schema the template built, without having built it.
//
// This is the whole point: the migrations run once for the binary, and each
// test inherits the result rather than paying ~279ms of DDL to reproduce a
// schema identical to the one the previous test just dropped.
func TestTemplate_TestStartsFromTheDeclaredSchema(t *testing.T) {
	db := pgtest.NewTestDB(t)

	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM from_template`).Scan(&n))
	assert.Zero(t, n, "the table is there and empty")
}

// Two tests do not see each other's rows.
//
// Copying a template is only a saving if it is still isolation. A shared
// database would pass the test above and fail here, and it would fail as a
// mysterious ordering-dependent assertion somewhere else in the suite rather
// than here — which is why this is asserted directly.
func TestTemplate_WritesDoNotEscapeTheTest(t *testing.T) {
	first := pgtest.NewTestDB(t)
	_, err := first.Exec(`INSERT INTO from_template (id, note) VALUES (1, 'first')`)
	require.NoError(t, err)

	second := pgtest.NewTestDB(t)
	var n int
	require.NoError(t, second.QueryRow(`SELECT count(*) FROM from_template`).Scan(&n))
	assert.Zero(t, n, "a second database must not hold the first's row")

	// And the first still does — the copy is not shared in either direction.
	require.NoError(t, first.QueryRow(`SELECT count(*) FROM from_template`).Scan(&n))
	assert.Equal(t, 1, n)
}

// Both databases are live at once and both carry the extensions.
//
// The schema path had to anchor pg_trgm in `public` by hand, because an
// extension installs INTO a schema and each test's own schema was dropped with
// it. A template is a database, extensions travel with the copy, and the
// regression that motivated that hack cannot recur — but only if the template
// actually has them, which is what this asserts.
func TestTemplate_EveryCopyCarriesTheExtensions(t *testing.T) {
	first := pgtest.NewTestDB(t)
	second := pgtest.NewTestDB(t)

	for name, db := range map[string]*storage.PgDB{"first": first, "second": second} {
		var reachable bool
		require.NoError(t,
			db.QueryRow(`SELECT count(*) > 0 FROM pg_opclass WHERE opcname = 'gin_trgm_ops'`).Scan(&reachable),
			"%s: query the operator class", name)
		assert.True(t, reachable, "%s: gin_trgm_ops must be reachable", name)
	}
}

// The database goes away with the test that asked for it.
//
// A database-per-test that never drops fills a developer's server and, in CI,
// the disk the container is on. The name is recorded here while the inner test
// runs and checked after it has finished.
func TestTemplate_DatabaseIsDroppedWithItsTest(t *testing.T) {
	var name string

	t.Run("inner", func(t *testing.T) {
		db := pgtest.NewTestDB(t)
		require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&name))
		require.NotEmpty(t, name)
	})

	// The subtest is over, so its cleanup has run.
	outer := pgtest.NewTestDB(t)
	var still bool
	require.NoError(t,
		outer.QueryRow(`SELECT count(*) > 0 FROM pg_database WHERE datname = $1`, name).Scan(&still))
	assert.False(t, still, "the inner test's database %q outlived it", name)
}
