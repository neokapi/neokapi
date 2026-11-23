//go:build linux

package dataroot

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func init() { birthTimeOf = birthTime }

// birthTime is when the directory at path was created, where the file system
// records it, and "" where it does not. A directory removed and created again
// can get the same inode number on Linux; its birth time tells the two apart.
func birthTime(path string, _ *syscall.Stat_t) string {
	var st unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, 0, unix.STATX_BTIME, &st); err != nil {
		return ""
	}
	if st.Mask&unix.STATX_BTIME == 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d", st.Btime.Sec, st.Btime.Nsec)
}
