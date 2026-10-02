//go:build js && wasm

package storage

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall/js"
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
// path reaches them, and nothing outlives the tab. A database file at a name
// in the page's file system is read in on the first open (readFileIn).

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

// The namespace answers for the databases the bridge holds and for database
// files in the page's file system (one a person added, say), which the driver
// reads into memory the first time a connection opens them (see readFileIn).
// A database held in memory takes precedence over a file at its name.

func dbExists(path string) (bool, error) {
	b, err := sqlBridge()
	if err != nil {
		return false, err
	}
	name := absDBPath(path)
	if b.Call("exists", name).Bool() {
		return true, nil
	}
	return fileExists(name)
}

// dbRemove deletes the database from memory and the file at its name, so a
// removed database does not come back from the file on the next open.
func dbRemove(path string) error {
	name := absDBPath(path)
	if _, err := bridgeCall("remove", name); err != nil {
		return err
	}
	return removeFiles(name)
}

// dbRename moves a database held in memory, reading a database file in first,
// and deletes the files at both names: the source has moved, and the
// destination's file is replaced.
func dbRename(from, to string) error {
	src, dst := absDBPath(from), absDBPath(to)
	if err := readFileIn(src); err != nil {
		return err
	}
	if _, err := bridgeCall("rename", src, dst); err != nil {
		return err
	}
	if src == dst {
		return nil
	}
	if err := removeFiles(src); err != nil {
		return err
	}
	return removeFiles(dst)
}

func dbList(dir string) ([]string, error) {
	b, err := sqlBridge()
	if err != nil {
		return nil, err
	}
	root := absDBPath(dir)
	names := b.Call("list", root)
	out := make([]string, names.Length())
	for i := range out {
		out[i] = names.Index(i).String()
	}
	files, err := listFiles(root)
	if err != nil {
		return nil, err
	}
	out = append(out, files...)
	slices.Sort(out)
	return slices.Compact(out), nil
}

// readFileIn loads the database file at name into memory, the first time a
// connection opens a name the bridge does not hold. The file stays where it
// is, and writes go to the database in memory: like every browser database,
// the copy lasts as long as the tab.
//
// No file, or an empty one, leaves the bridge to start an empty database, as
// SQLite does with an empty file on disk. A file that is not a database fails
// as it does natively.
func readFileIn(name string) error {
	b, err := sqlBridge()
	if err != nil {
		return err
	}
	if b.Call("exists", name).Bool() {
		return nil
	}
	held, err := fileExists(name)
	if err != nil || !held {
		return err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return fmt.Errorf("storage: read %s: %w", name, err)
	}
	if len(data) == 0 {
		return nil
	}
	if !bytes.HasPrefix(data, sqliteMagic) {
		return &jsError{code: sqliteNotADB, msg: "file is not a database"}
	}
	if info, err := os.Stat(name + "-wal"); err == nil && info.Size() > 0 {
		return fmt.Errorf("storage: %s has a write-ahead log beside it, which the browser cannot read; "+
			"checkpoint it (PRAGMA wal_checkpoint(TRUNCATE)) before adding it", name)
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	_, err = bridgeCall("load", name, arr)
	return err
}
