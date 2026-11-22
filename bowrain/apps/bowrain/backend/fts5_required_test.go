//go:build !fts5

package backend

import (
	"fmt"
	"os"
)

// The desktop's local terms and content-memory stores use SQLite full-text
// search (FTS5), which go-sqlite3 compiles in only under the fts5 build tag.
// The app is always built with it. Without the tag every store a test opens
// fails in setup with "no such function: fts5", far from the cause, so the
// test binary stops here and says what to run instead.
func init() {
	fmt.Fprintln(os.Stderr, "bowrain desktop backend tests need SQLite FTS5: run go test -tags fts5 ./... (make bowrain-desktop-test adds it)")
	os.Exit(2)
}
