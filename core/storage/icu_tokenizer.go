//go:build cgo

package storage

// This file statically links the FTS5 ICU tokenizer into the Go binary.
// The tokenizer C source (fts5_icu.c / fts5_icu.h from cwt/fts5-icu-tokenizer)
// is compiled with SQLITE_CORE so it calls SQLite functions directly instead
// of through the extension API indirection layer.
//
// The init function registers the tokenizer as an auto-extension, so it is
// available on every sqlite3_open() — no ConnectHook or dynamic loading
// needed.

// #cgo CFLAGS: -DSQLITE_CORE -DSQLITE_ENABLE_FTS5
// #cgo pkg-config: icu-uc icu-i18n
//
// #include "sqlite3.h"
//
// // The init function defined in fts5_icu.c. With SQLITE_CORE the
// // SQLITE_EXTENSION_INIT1/INIT2 macros are no-ops, so pApi is unused.
// extern int sqlite3_ftsicu_init(sqlite3 *db, char **pzErrMsg,
//                                const sqlite3_api_routines *pApi);
//
// static void register_icu_auto_extension(void) {
//     sqlite3_auto_extension(
//         (void(*)(void))sqlite3_ftsicu_init
//     );
// }
import "C"

func init() {
	C.register_icu_auto_extension()
}
