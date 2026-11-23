package storage_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/storage"
)

// NormalizeSQL drops comments and collapses whitespace, and keeps every byte
// inside a literal, a quoted identifier or a dollar-quoted body.
func TestNormalizeSQL(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"comments and whitespace": {
			"-- why this table exists; see the note\nCREATE TABLE a (\n\tx INTEGER, -- the count\n\ty TEXT /* a note */\n);\n",
			"CREATE TABLE a ( x INTEGER, y TEXT );",
		},
		"a comment marker inside a string literal": {
			"INSERT INTO t VALUES ('a -- not a comment', '/* nor this */')",
			"INSERT INTO t VALUES ('a -- not a comment', '/* nor this */')",
		},
		"a doubled quote inside a literal": {
			"SELECT 'it''s -- kept'  -- dropped",
			"SELECT 'it''s -- kept'",
		},
		"whitespace inside a literal is kept": {
			"SELECT 'two  spaces\nand a newline'",
			"SELECT 'two  spaces\nand a newline'",
		},
		"an escape string": {
			`SELECT E'a\' -- kept' -- dropped`,
			`SELECT E'a\' -- kept'`,
		},
		"a quoted identifier": {
			`CREATE TABLE "odd -- name" (x INT)`,
			`CREATE TABLE "odd -- name" (x INT)`,
		},
		"a dollar-quoted body": {
			"CREATE FUNCTION f() RETURNS void AS $$\n  -- a comment the function keeps\n  SELECT 1;\n$$ LANGUAGE sql; -- dropped",
			"CREATE FUNCTION f() RETURNS void AS $$\n  -- a comment the function keeps\n  SELECT 1;\n$$ LANGUAGE sql;",
		},
		"a tagged dollar-quoted body": {
			"DO $body$ BEGIN /* kept */ END $body$",
			"DO $body$ BEGIN /* kept */ END $body$",
		},
		"a positional parameter is not a dollar quote": {
			"UPDATE t SET a = $1 -- one\n WHERE b = $2",
			"UPDATE t SET a = $1 WHERE b = $2",
		},
		"a nested block comment": {
			"SELECT /* outer /* inner */ still outer */ 1",
			"SELECT 1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, storage.NormalizeSQL(tc.in))
		})
	}
}

// A comment reworded inside a shipped migration leaves its identity as it
// was; a change to what executes does not.
func TestMigrationIdentity_IgnoresHowTheSQLIsTyped(t *testing.T) {
	m := storage.Migration{Version: 63, Description: "the reasons of a review decision", SQL: "\n\t\t\t-- The reasons a person chose; kept beside the note.\n\t\t\tALTER TABLE unit_decisions ADD COLUMN IF NOT EXISTS reasons JSONB NOT NULL DEFAULT '[]';\n\t\t"}
	reworded := m
	reworded.SQL = "-- The reasons a person chose, kept beside the note.\nALTER TABLE unit_decisions ADD COLUMN IF NOT EXISTS reasons JSONB NOT NULL DEFAULT '[]';"
	assert.Equal(t, storage.MigrationIdentity(m), storage.MigrationIdentity(reworded))

	changed := m
	changed.SQL = "ALTER TABLE unit_decisions ADD COLUMN IF NOT EXISTS reasons JSONB NOT NULL DEFAULT '{}';"
	assert.NotEqual(t, storage.MigrationIdentity(m), storage.MigrationIdentity(changed))
}

// The FTS word tokenizer differs between builds; a migration that names it
// through the placeholder has one identity in every build and executes with
// the build's tokenizer.
func TestMigrationIdentity_TheTokenizerPlaceholder(t *testing.T) {
	m := storage.Migration{Version: 1, Description: "fts", SQL: "CREATE VIRTUAL TABLE w USING fts5(x, tokenize='" + storage.FTSWordTokenizerPlaceholder + "')"}
	assert.Contains(t, storage.ExecutableSQL(m), "tokenize='"+storage.FTSWordTokenizer+"'")
	assert.NotContains(t, storage.MigrationIdentity(m), storage.FTSWordTokenizer)
	built := m
	built.SQL = storage.ExecutableSQL(m)
	assert.NotEqual(t, storage.MigrationIdentity(m), storage.MigrationIdentity(built), "the identity is taken over the placeholder, not the build's tokenizer")
}
