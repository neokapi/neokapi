//go:build !windows && !js

package pluginhost

import (
	"os"
	"syscall"
)

// fileOwner is the uid and gid that own info's file.
func fileOwner(info os.FileInfo) (uid, gid int, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(st.Uid), int(st.Gid), true
}
