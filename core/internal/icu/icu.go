//go:build cgo

package icu

// The tokenizer C source (fts5_icu.c and fts5_icu.h, from
// cwt/fts5-icu-tokenizer) is compiled with SQLITE_CORE, so it calls SQLite
// functions directly instead of through the extension API. Those functions
// come from mattn/go-sqlite3, which this package imports so that every binary
// holding the tokenizer also holds SQLite.

// #cgo CFLAGS: -DSQLITE_CORE -DSQLITE_ENABLE_FTS5
// #cgo pkg-config: icu-uc icu-i18n
//
// #include <stdlib.h>
// #include "sqlite3.h"
// #include <unicode/utypes.h>
// #include <unicode/ubrk.h>
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
//
// // open_sentence_breaker opens a sentence BreakIterator over the given UTF-16
// // text for the given locale. A fresh iterator is opened per call, so callers
// // are safe under concurrent use (no shared *UBreakIterator). Returns NULL
// // and sets *status on failure.
// static UBreakIterator *open_sentence_breaker(const char *locale,
//                                              const UChar *text, int32_t len,
//                                              UErrorCode *status) {
//     return ubrk_open(UBRK_SENTENCE, locale, text, len, status);
// }
import "C"

import (
	"fmt"
	"unsafe"

	_ "github.com/mattn/go-sqlite3" // the SQLite the tokenizer calls into
)

// RegisterFTS5Tokenizer registers the ICU tokenizer as an SQLite
// auto-extension, so it is available on every connection opened afterwards,
// with no ConnectHook or dynamic loading.
func RegisterFTS5Tokenizer() {
	C.register_icu_auto_extension()
}

// SentenceBreaks runs ICU's sentence BreakIterator over UTF-16 text and
// returns every boundary after the first, in increasing order: the interior
// boundaries and then len(text). locale is an ICU locale name such as "en_US";
// an empty or unknown one uses ICU's root rules.
func SentenceBreaks(text []uint16, locale string) ([]int, error) {
	if len(text) == 0 {
		return nil, nil
	}
	cLocale := C.CString(locale)
	defer C.free(unsafe.Pointer(cLocale))

	// C.CBytes copies the text into C memory this function owns and frees;
	// ICU only reads it. UChar is a 16-bit unit in native byte order, which is
	// how the Go runtime laid out the slice.
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&text[0])), len(text)*2)
	cText := (*C.UChar)(C.CBytes(raw))
	defer C.free(unsafe.Pointer(cText))

	var status C.UErrorCode
	bi := C.open_sentence_breaker(cLocale, cText, C.int32_t(len(text)), &status)
	if int(status) > 0 || bi == nil {
		return nil, fmt.Errorf("ubrk_open failed (locale %q): %s", locale, C.GoString(C.u_errorName(status)))
	}
	defer C.ubrk_close(bi)

	breaks := make([]int, 0, 8)
	C.ubrk_first(bi)
	for {
		pos := C.ubrk_next(bi)
		if pos == C.UBRK_DONE {
			return breaks, nil
		}
		breaks = append(breaks, int(pos))
	}
}
