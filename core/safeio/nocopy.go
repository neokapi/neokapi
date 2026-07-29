package safeio

import "unsafe"

// NoCopyString returns a string that shares b's backing array instead of
// copying it.
//
// # The contract
//
// After this call, and for as long as the returned string is reachable,
// NOTHING may write to b or to any slice that aliases it.
//
// Go strings are immutable by definition, and the runtime depends on that: it
// interns them, hashes them once as map keys, and compares them by pointer
// where it can. Writing through the []byte afterwards mutates a value the type
// system has promised cannot change. The consequence is not a stale read — it
// is undefined behaviour, and its most likely visible form is a map lookup that
// stops finding a key it still contains.
//
// The obligation is on the caller because only the caller can see the whole
// lifetime of b. Every current caller satisfies it the same way: b is the fully
// read document buffer, the parse that follows only reads from it, and the
// string is handed to a map that outlives the read.
//
// # Why this exists rather than string(b)
//
// string(b) allocates and copies the entire document. These call sites stash
// the original bytes of every parsed file so the writer can splice only the
// values that changed and emit the rest byte-for-byte, so the conversion sits
// on the hot path of every read, once per file, at whole-document size. On a
// project with thousands of resource files that copy is the difference between
// a linear read and doubling peak memory for no benefit — the second copy is
// never written to, only read back.
//
// So the answer to "this looks unsafe, should it be string(b)?" is no, and this
// paragraph is why. If a caller genuinely needs a mutable buffer afterwards, it
// should copy at that point, deliberately, rather than making every reader pay.
//
// # Behaviour at the edges
//
// A nil or empty b yields "" without dereferencing anything: unsafe.String
// documents len == 0 as returning the empty string regardless of the pointer.
func NoCopyString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}
