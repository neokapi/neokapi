//go:build !js

package host

// execTrustImplied is false on a native build: a recipe discovered on disk
// needs a person's answer, a recorded decision or KAPI_TRUST_EXEC.
const execTrustImplied = false
