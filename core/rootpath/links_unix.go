//go:build unix

package rootpath

import (
	"io/fs"
	"syscall"
)

// hasOtherNames reports whether the file has more than one hard link.
func hasOtherNames(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}
