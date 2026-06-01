//go:build js

package pluginhost

// oNoFollow is 0 on js/wasm: syscall.O_NOFOLLOW is undefined there and the wasm
// build (the docs CLI playground) has no filesystem symlink surface to harden.
const oNoFollow = 0
