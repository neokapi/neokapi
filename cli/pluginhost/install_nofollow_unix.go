//go:build !js

package pluginhost

import "syscall"

// oNoFollow makes OpenFile refuse to traverse a symlink at the target path
// (plugin-extraction hardening, #17/#69). Real on every OS we ship binaries for.
const oNoFollow = syscall.O_NOFOLLOW
