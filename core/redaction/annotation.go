package redaction

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
}

// AnnotationType identifies this annotation.
func (*SecretAnnotation) AnnotationType() string { return SecretAnnotationKey }

// Get returns the original value for a token.
func (a *SecretAnnotation) Get(token string) (RedactedValue, bool) {
	if a == nil {
		return RedactedValue{}, false
	}
	rv, ok := a.Values[token]
	return rv, ok
}
