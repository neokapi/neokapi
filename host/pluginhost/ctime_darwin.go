//go:build darwin

package pluginhost

import (
	"os"
	"syscall"
)

// changeTime is the inode change time of info's file, in Unix nanoseconds.
func changeTime(info os.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Ctimespec.Nano()
	}
	return 0
}
