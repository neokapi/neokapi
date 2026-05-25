//go:build cgo && !windows

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestICUTokenizer(t *testing.T) {
	db, err := Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Create an FTS5 table using the ICU tokenizer.
	_, err = db.ExecContext(ctx, `CREATE VIRTUAL TABLE icu_test USING fts5(
		content, tokenize='icu'
	)`)
	require.NoError(t, err, "FTS5 ICU tokenizer should be available")

	// Insert multilingual content.
	_, err = db.ExecContext(ctx, `INSERT INTO icu_test(content) VALUES
		('中国 经济 发展 报告'),
		('การ ทดสอบ ภาษา ไทย ใน ระบบ'),
		('日本語 の テスト です'),
		('Hello world'),
		('Bonjour le monde')
	`)
	require.NoError(t, err)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"Chinese", "中国", 1},
		{"Thai", "ภาษา", 1},
		{"Japanese", "テスト", 1},
		{"English", "hello", 1},
		{"French", "monde", 1},
		{"No match", "xyz123", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var count int
			err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM icu_test WHERE icu_test MATCH ?`, tc.query).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, tc.want, count)
		})
	}
}
