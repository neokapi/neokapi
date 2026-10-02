package storage_test

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDriver_ValuesRoundTrip holds every build's driver to the values the
// stores write: integers past JavaScript's safe range, empty text and blobs
// apart from NULL, floats, booleans, and time.Time in a DATETIME column, which
// scans back as a time.Time.
func TestDriver_ValuesRoundTrip(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "values.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	ctx := t.Context()

	_, err = db.ExecContext(ctx, `CREATE TABLE v (
		big INTEGER, neg INTEGER, s TEXT, empty TEXT, b BLOB, eb BLOB, nb BLOB,
		f REAL, flag BOOLEAN, at DATETIME)`)
	require.NoError(t, err)

	at := time.Date(2026, 10, 2, 9, 30, 15, 123456789, time.UTC)
	res, err := db.ExecContext(ctx, `INSERT INTO v VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		int64(9007199254740993), int64(math.MinInt64), "héllo ✓", "", []byte{0, 1, 255}, []byte{}, []byte(nil),
		0.25, true, at)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	assert.Equal(t, int64(1), id)
	n, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	var (
		big, neg          int64
		s, empty          string
		b, eb, nb         []byte
		f                 float64
		flag              bool
		gotAt             time.Time
		emptyKind, nbKind string
	)
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT big, neg, s, empty, b, eb, nb, f, flag, at, typeof(eb), typeof(nb) FROM v`).
		Scan(&big, &neg, &s, &empty, &b, &eb, &nb, &f, &flag, &gotAt, &emptyKind, &nbKind))
	assert.Equal(t, int64(9007199254740993), big)
	assert.Equal(t, int64(math.MinInt64), neg)
	assert.Equal(t, "héllo ✓", s)
	assert.Empty(t, empty)
	assert.Equal(t, []byte{0, 1, 255}, b)
	assert.Equal(t, "blob", emptyKind, "an empty blob is a value, not NULL")
	assert.Equal(t, "null", nbKind, "a nil slice is NULL")
	assert.Nil(t, nb)
	assert.InDelta(t, 0.25, f, 0)
	assert.True(t, flag)
	assert.True(t, at.Equal(gotAt), "DATETIME round trip: got %v", gotAt)
}

// TestDriver_StatementsAndArguments: one Exec without arguments runs every
// statement in it, arguments bind by position, and rows arrive in full.
// Several statements with arguments in one Exec are left untested on purpose:
// mattn/go-sqlite3 hands each statement the next arguments and modernc hands
// each the first, so no store may write one.
func TestDriver_StatementsAndArguments(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "multi.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	ctx := t.Context()

	_, err = db.ExecContext(ctx, `CREATE TABLE a (v INTEGER); CREATE TABLE b (v INTEGER); INSERT INTO a VALUES (1);`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO b VALUES (?), (?)`, 2, 3)
	require.NoError(t, err)

	var sum int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT (SELECT SUM(v) FROM a) + (SELECT SUM(v) FROM b)`).Scan(&sum))
	assert.Equal(t, 6, sum)

	rows, err := db.QueryContext(ctx, `SELECT v FROM b ORDER BY v`)
	require.NoError(t, err)
	var got []int
	for rows.Next() {
		var v int
		require.NoError(t, rows.Scan(&v))
		got = append(got, v)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	assert.Equal(t, []int{2, 3}, got)

	// Many rows cross the browser bridge in batches; every one arrives.
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	for i := range 1000 {
		_, err := tx.ExecContext(ctx, `INSERT INTO a VALUES (?)`, i)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	var count int
	rows, err = db.QueryContext(ctx, `SELECT v FROM a`)
	require.NoError(t, err)
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Close())
	assert.Equal(t, 1001, count)

	// A failed statement reports SQLite's own message.
	_, err = db.ExecContext(ctx, `INSERT INTO missing VALUES (?)`, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such table: missing")
}

// TestDriver_FTS5: every build's SQLite carries FTS5 with the word-search
// tokenizer the stores name, and the trigram tokenizer.
func TestDriver_FTS5(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "fts.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	ctx := t.Context()

	_, err = db.ExecContext(ctx, `CREATE VIRTUAL TABLE w USING fts5(x, tokenize='`+storage.FTSWordTokenizer+`');
		CREATE VIRTUAL TABLE g USING fts5(x, tokenize='trigram');`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO w VALUES (?); INSERT INTO g VALUES (?);`,
		"Log in to continue", "Log in to continue")
	require.NoError(t, err)
	var hits int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM w WHERE w MATCH 'continue'`).Scan(&hits))
	assert.Equal(t, 1, hits)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM g WHERE g MATCH 'ntin'`).Scan(&hits))
	assert.Equal(t, 1, hits)
}

// TestDriverProfile names a driver and allows at least one connection.
func TestDriverProfile(t *testing.T) {
	p := storage.DriverProfile()
	assert.NotEmpty(t, p.Driver)
	assert.GreaterOrEqual(t, p.MaxConns, 1)
	if !p.Durable {
		assert.False(t, p.CrossProcessLock, "a driver whose databases die with the process has no other process to lock against")
	}
}
