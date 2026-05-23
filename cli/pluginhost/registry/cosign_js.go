//go:build js

// Sigstore verification depends on sigstore-go / in-toto, which don't build
// for GOOS=js (they use unix syscalls). Plugins are never installed in the
// browser build anyway — there's no subprocess to dispatch to — so this stub
// keeps the package compiling for wasm while refusing verification.
package registry

import "context"

// SignatureVerificationError mirrors the non-js type so callers can still
// type-switch / errors.As against it.
type SignatureVerificationError struct {
	Reason string
	Err    error
}

func (e *SignatureVerificationError) Error() string {
	if e.Err == nil {
		return "signature verification failed: " + e.Reason
	}
	return "signature verification failed: " + e.Reason + ": " + e.Err.Error()
}

func (e *SignatureVerificationError) Unwrap() error { return e.Err }

// CosignVerifyOptions is a placeholder so the zero value used by callers
// (registry.CosignVerifyOptions{}) still type-checks under wasm.
type CosignVerifyOptions struct{}

// VerifyBundle always fails under wasm: plugin signature verification isn't
// available in the browser.
func VerifyBundle(_ context.Context, _, _, _, _ string, _ CosignVerifyOptions) error {
	return &SignatureVerificationError{Reason: "signature verification is not available in the wasm build"}
}
