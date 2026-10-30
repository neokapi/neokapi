package redaction

import "github.com/neokapi/neokapi/core/model"

// SecretAnnotationKey is the Block.Annotations key under which an in-process
// pipeline carries the token→original mapping between the redact and unredact
// tools. It is deliberately not a key any format writer serializes, so the
// secrets it holds never reach an output file; unredact deletes it once the
// originals are restored.
//
// The cross-process roundtrip (extract → external translation → merge) does
// NOT use this annotation — there the mapping goes to a [FileVault] sidecar
// so nothing sensitive ever rides on the serialized block.
const SecretAnnotationKey = "redaction.secret"

// SecretAnnotation carries redacted originals on a block while it flows
// through a single-process pipeline. Implements any.
type SecretAnnotation struct {
	// Values maps each placeholder token (the PlaceholderRun ID) to its
	// original value.
	Values map[string]RedactedValue
	// Source is the source's runs as redact read them, and Targets each
	// target edition redact rewrote, as it read it. Unredact puts back an
	// edition whose restored content equals what was read, with the status
	// and origin it had, so a redact and unredact around a tool that left it
	// alone change nothing about it.
	Source  []model.Run
	Targets map[model.LocaleID]model.Edition
}

// AnnotationType identifies this annotation.
func (*SecretAnnotation) TypeName() string { return SecretAnnotationKey }

// Get returns the original value for a token.
func (a *SecretAnnotation) Get(token string) (RedactedValue, bool) {
	if a == nil {
		return RedactedValue{}, false
	}
	rv, ok := a.Values[token]
	return rv, ok
}
