//go:build cgo

package storage

import "github.com/neokapi/neokapi/core/internal/icu"

// The FTS5 ICU tokenizer is linked statically into the binary and registered
// as an SQLite auto-extension, so it is available on every connection. Its C
// source lives in core/internal/icu, the one package that links ICU.
func init() {
	icu.RegisterFTS5Tokenizer()
}
