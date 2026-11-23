//go:build unix

package dataroot

import (
	"io/fs"
	"os"
	"syscall"
)

// privateModeApplies says the permission bits restrict access on this
// platform, so a directory with group or other bits set is refused.
const privateModeApplies = true

// ownedByMe reports whether this process's user owns the file.
func ownedByMe(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
