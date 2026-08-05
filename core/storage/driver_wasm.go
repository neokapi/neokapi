//go:build wasm

package storage

import (
	"fmt"
	"strings"
)

// On wasm (GOOS=js / wasip1) there is no SQLite driver: modernc.org/sqlite
// depends on modernc.org/libc, which has no wasm support, and the cgo mattn
// driver needs a C compiler. The wasm build of kapi uses the in-memory
// content memory/terms stores, so no file-backed SQLite is opened. This stub keeps
// core/storage compiling on wasm (matching the pre-fallback behavior, where
// the cgo driver compiled to a no-op under CGO_ENABLED=0); calling
// storage.Open here fails at runtime by design.

// sqliteDriver names the (unregistered) driver; declared for build parity.
const sqliteDriver = "sqlite"

// driverUnavailable reports that this build has no SQLite driver at all. Open
// checks it first so a browser caller reading a store-backed asset gets the
// actual explanation instead of database/sql's `unknown driver "sqlite"
// (forgotten import?)`, which reads like a missing blank import in kapi rather
// than a deliberate property of the browser build.
func driverUnavailable() error {
	return fmt.Errorf("%w; the browser runs against the in-memory content memory and terms seeded from the lab fixtures", ErrNoSQLite)
}

// FTSWordTokenizer is unused on wasm (no SQLite); declared for build parity
// with the cgo / no-cgo drivers so memory and terms compile.
const FTSWordTokenizer = "unicode61"

// sqliteDSN mirrors the no-cgo DSN form, Options.ImmediateTx included; unused at
// runtime on wasm, where Open never gets past driverUnavailable.
func sqliteDSN(dbPath string, opts Options) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	params := []string{
		"_pragma=foreign_keys(1)",
		"_pragma=busy_timeout(5000)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=cache_size(-131072)",
		"_pragma=wal_autocheckpoint(10000)",
		"_pragma=temp_store(MEMORY)",
	}
	if opts.ImmediateTx {
		params = append(params, "_txlock=immediate")
	}
	return dbPath + sep + strings.Join(params, "&")
}
