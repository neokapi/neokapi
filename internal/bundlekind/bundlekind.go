// Package bundlekind explains a rejected `kind` magic string.
//
// The three envelope kinds (.kbf documents, .kmb content memories, .ktb terms)
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

import "fmt"

// retired maps each kind this project used to emit to the value that replaced it.
var retired = map[string]string{
	"kapi-localization-format": "kapi-bundle",
	"kapi-tm-format":           "kapi-memory",
	"kapi-termbase-format":     "kapi-terms",
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
