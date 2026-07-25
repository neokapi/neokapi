//go:build !js

package format

// tempFilesystemAvailable reports whether the platform has a writable temp
// filesystem, i.e. whether os.CreateTemp is expected to work. Everywhere but
// js/wasm it is, so a temp-file failure there is an operational fault (a broken
// or read-only TMPDIR, a full filesystem, fd exhaustion) that the caller must
// surface — not a platform limitation to absorb in memory.
const tempFilesystemAvailable = true
