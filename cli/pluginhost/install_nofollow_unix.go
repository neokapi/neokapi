//go:build unix

package pluginhost

import "syscall"

// oNoFollow makes OpenFile refuse to traverse a symlink at the target path
// (plugin-extraction hardening, #17/#69). syscall.O_NOFOLLOW exists on the unix
// targets; Windows and js/wasm fall back to 0 (see install_nofollow_other.go).
const oNoFollow = syscall.O_NOFOLLOW
