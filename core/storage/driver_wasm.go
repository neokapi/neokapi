//go:build wasm

package storage

import "strings"

// On wasm (GOOS=js / wasip1) there is no SQLite driver: modernc.org/sqlite
// depends on modernc.org/libc, which has no wasm support, and the cgo mattn
// driver needs a C compiler. The wasm build of kapi uses the in-memory
// TM/termbase stores, so no file-backed SQLite is opened. This stub keeps
// core/storage compiling on wasm (matching the pre-fallback behavior, where
// the cgo driver compiled to a no-op under CGO_ENABLED=0); calling
// storage.Open here fails at runtime by design.

// sqliteDriver names the (unregistered) driver; declared for build parity.
const sqliteDriver = "sqlite"

// FTSWordTokenizer is unused on wasm (no SQLite); declared for build parity
// with the cgo / no-cgo drivers so sievepen and termbase compile.
const FTSWordTokenizer = "unicode61"

// sqliteDSN mirrors the no-cgo DSN form; unused at runtime on wasm.
func sqliteDSN(dbPath string) string {
	if isMemoryDSN(dbPath) {
		return dbPath
	}
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	return dbPath + sep + "_pragma=busy_timeout(5000)"
}
