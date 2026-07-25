//go:build js

package format

// tempFilesystemAvailable is false for the js/wasm browser build: there is no
// filesystem behind os.TempDir, so os.CreateTemp always fails. An in-memory
// skeleton backing is therefore the *design* there rather than a degradation —
// it keeps the in-browser round-trip byte-exact, and the documents the docs
// playground handles are small. See NewSkeletonStore.
const tempFilesystemAvailable = false
