//go:build !unix

package pluginhost

// oNoFollow is 0 on non-unix targets (Windows, js/wasm, plan9), where
// syscall.O_NOFOLLOW is undefined. Windows os.OpenFile has no O_NOFOLLOW
// equivalent, and the wasm build (the docs CLI playground) has no filesystem
// symlink surface to harden. See install_nofollow_unix.go for the unix
// implementation that hardens against symlink traversal during plugin extract.
const oNoFollow = 0
