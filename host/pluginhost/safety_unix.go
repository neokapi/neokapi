//go:build !windows && !js

package pluginhost

import (
	"os"
	"syscall"
)

// fileOwner is the uid that owns info's file.
func fileOwner(info os.FileInfo) (int, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
