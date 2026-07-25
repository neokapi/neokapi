// Package bundlekind explains a rejected `kind` magic string.
//
// The three envelope kinds (.kbf.json documents, .memory.json content memories, .terms.json terms)
// were renamed when the vocabulary moved off the Okapi-inherited "translation
// memory / termbase" terms:
//
//	kapi-localization-format -> kapi-bundle
//	kapi-tm-format           -> kapi-memory
//	kapi-termbase-format     -> kapi-terms
//
// Readers deliberately do not accept the retired values. Silently reinterpreting
// a file written by an earlier release would make the envelope meaningless as a
// version marker, so a stale file is rejected — but the rejection has to read as
// "regenerate this file", not as an unexplained validation failure. That is what
// this package is for.
package bundlekind

import (
	"bytes"
	"fmt"
	"sort"
)

// retired maps each kind this project used to emit to the value that replaced it.
var retired = map[string]string{
	"kapi-localization-format": "kapi-bundle",
	"kapi-tm-format":           "kapi-memory",
	"kapi-termbase-format":     "kapi-terms",
}

// PredecessorsOf returns the retired kinds that were replaced by want.
//
// Format sniffers use this so a file written by an earlier release is still
// *detected* as the format it is. Without it a stale file matches no signature
// and fails as an unknown format, which hides the rename; routing it to the
// right reader lets that reader reject it through Unexpected, which explains
// what happened and names the command that rewrites it.
func PredecessorsOf(want string) []string {
	var out []string
	for old, replacement := range retired {
		if replacement == want {
			out = append(out, old)
		}
	}
	sort.Strings(out) // stable order; the map iteration order is not
	return out
}

// SniffKind reports whether data carries want as its kind, or any kind that was
// retired in favour of want.
func SniffKind(data []byte, want string) bool {
	if bytes.Contains(data, []byte(`"`+want+`"`)) {
		return true
	}
	for _, old := range PredecessorsOf(want) {
		if bytes.Contains(data, []byte(`"`+old+`"`)) {
			return true
		}
	}
	return false
}

// Unexpected returns the error for a document whose kind is not want.
//
// pkg names the reader ("kbf", "kmb", "ktb") and regen is the command that
// rewrites the file. When got is a retired kind, the message explains the rename
// instead of only reporting the mismatch.
func Unexpected(pkg, got, want, regen string) error {
	if replacement, ok := retired[got]; ok {
		return fmt.Errorf(
			"%s: kind %q was renamed to %q — this file was written by an earlier "+
				"release and is not read as-is; regenerate it with %s",
			pkg, got, replacement, regen)
	}
	if got == "" {
		return fmt.Errorf("%s: missing kind (want %q)", pkg, want)
	}
	return fmt.Errorf("%s: unexpected kind %q (want %q)", pkg, got, want)
}
