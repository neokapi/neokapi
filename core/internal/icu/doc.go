// Package icu is the one package in the framework that links ICU.
//
// Every package that declares `#cgo pkg-config: icu-uc icu-i18n` adds
// `-licui18n -licuuc` to the final link, and a binary that held two such
// packages passed both libraries twice; Apple's linker then warned
// "ignoring duplicate libraries" on every link. The ICU code of the
// framework, the FTS5 ICU tokenizer for SQLite and the UAX-29 sentence break
// iterator, lives here instead, and core/storage and core/segment/uax29 call
// it.
//
// The package builds only with cgo. Without cgo it is empty, and its callers
// build their own stubs.
package icu
