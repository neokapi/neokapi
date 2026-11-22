//go:build js

package rootpath

// Supported reports whether the platform resolves links and opens files
// through os.Root. The browser build (js/wasm) runs over an in-memory file
// system that has neither, so its checks are lexical there.
const Supported = false
