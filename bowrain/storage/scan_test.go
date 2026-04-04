package storage_test

import (
	"database/sql"
	"testing"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanRows(t *testing.T) {
	t.Parallel()

	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test_scan (id INTEGER PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO test_scan (id, name) VALUES (1, 'alpha'), (2, 'bravo'), (3, 'charlie')`)
	require.NoError(t, err)

	type row struct {
		ID   int
		Name string
	}

	rows, err := db.Query(`SELECT id, name FROM test_scan ORDER BY id`)
	require.NoError(t, err)

	results, err := storage.ScanRows(rows, func(s storage.Scanner) (row, error) {
		var r row
		err := s.Scan(&r.ID, &r.Name)
		return r, err
	})
	require.NoError(t, err)

	assert.Len(t, results, 3)
	assert.Equal(t, row{1, "alpha"}, results[0])
	assert.Equal(t, row{2, "bravo"}, results[1])
	assert.Equal(t, row{3, "charlie"}, results[2])
}

func TestScanRows_Empty(t *testing.T) {
	t.Parallel()

	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test_empty (id INTEGER)`)
	require.NoError(t, err)

	rows, err := db.Query(`SELECT id FROM test_empty`)
	require.NoError(t, err)

	results, err := storage.ScanRows(rows, func(s storage.Scanner) (int, error) {
		var id int
		err := s.Scan(&id)
		return id, err
	})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestScanRows_ScanError(t *testing.T) {
	t.Parallel()

	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test_err (name TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO test_err (name) VALUES ('hello')`)
	require.NoError(t, err)

	rows, err := db.Query(`SELECT name FROM test_err`)
	require.NoError(t, err)

	// Try to scan text into int — should produce error.
	_, err = storage.ScanRows(rows, func(s storage.Scanner) (int, error) {
		var id int
		err := s.Scan(&id)
		return id, err
	})
	assert.Error(t, err)
}

// Verify Scanner interface is satisfied by *sql.Row and *sql.Rows.
var (
	_ storage.Scanner = (*sql.Row)(nil)
	_ storage.Scanner = (*sql.Rows)(nil)
)
