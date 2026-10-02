//go:build js && wasm

package storage

import (
	"database/sql"
	"path/filepath"
	"strings"
)

// The browser build's driver is the official SQLite WebAssembly build
// (@sqlite.org/sqlite-wasm), reached through the bridge the host installs on
// globalThis as __kapiSQL before the Go program starts
// (packages/engine/src/sqlite.ts). Every call is a synchronous syscall/js call:
// Go and SQLite share a thread, so a statement runs to completion inside the
// call and no goroutine runs meanwhile.
//
// Databases live in SQLite's memdb VFS, in the module's memory, named by their
// absolute path. They are shared by every connection to one name, ATTACH by
// path reaches them, and nothing outlives the tab.

func init() { sql.Register(sqliteDriver, jsDriver{}) }

// sqliteDriver is the database/sql name the bridge driver registers under.
const sqliteDriver = "sqlite-wasm"

// driverProfile is what the bridge gives a store. One connection per file,
// because a second connection's busy wait would spin the only thread; no WAL,
// because memdb keeps its journal in memory; no lock another process honours,
// because there is no other process; and nothing durable.
var driverProfile = Profile{
	Driver:   "sqlite-wasm",
	MaxConns: 1,
}

// busyTimeoutMS is zero: SQLite's busy handler sleeps on the only thread, so
// the lock it waits for can never be released meanwhile. A conflict reports
// "database is locked" at once instead of after a frozen page.
const busyTimeoutMS = 0

// cacheSize is the page cache per connection, in the PRAGMA's negative-KiB
// spelling: 8 MiB. The database itself already sits in the module's memory.
const cacheSize = "-8192"

// FTSWordTokenizer is the FTS5 tokenizer for word-search tables. The
// WebAssembly build carries SQLite's built-in tokenizers (unicode61, ascii,
// porter, trigram) and no ICU, so word search splits on Unicode word
// boundaries without segmenting scripts that do not mark them (CJK, Thai).
const FTSWordTokenizer = "unicode61"

// sqliteDSN builds the DSN the bridge driver parses: an absolute database name,
// then the per-connection pragmas in the `_pragma=NAME(VALUE)` spelling modernc
// uses, then `_txlock=immediate` when Options.ImmediateTx asks for it.
//
// The name is made absolute against the working directory, because memdb
// shares a database between connections by its name: `memory.db` opened from
// two directories must be two databases, as it is on disk.
func sqliteDSN(dbPath string, opts Options) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	name, query, _ := strings.Cut(dbPath, "?")
	name = absDBPath(name)
	params := []string{
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(0)",
		"_pragma=temp_store(MEMORY)",
	}
	if opts.ImmediateTx {
		params = append(params, "_txlock=immediate")
	}
	if query != "" {
		params = append([]string{query}, params...)
	}
	return name + "?" + strings.Join(params, "&")
}

// absDBPath is the name a database is held under: the absolute, cleaned path.
func absDBPath(p string) string {
	p = strings.TrimPrefix(p, "file:")
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

func dbExists(path string) (bool, error) {
	b, err := sqlBridge()
	if err != nil {
		return false, err
	}
	return b.Call("exists", absDBPath(path)).Bool(), nil
}

func dbRemove(path string) error {
	_, err := bridgeCall("remove", absDBPath(path))
	return err
}

func dbRename(from, to string) error {
	_, err := bridgeCall("rename", absDBPath(from), absDBPath(to))
	return err
}

func dbList(dir string) ([]string, error) {
	b, err := sqlBridge()
	if err != nil {
		return nil, err
	}
	names := b.Call("list", absDBPath(dir))
	out := make([]string, names.Length())
	for i := range out {
		out[i] = names.Index(i).String()
	}
	return out, nil
}
