//go:build darwin

package project

import (
	"fmt"
	"syscall"
)

// birthTime is when the file at path was created. A file removed and created
// again can get the same inode number; its birth time tells the two apart.
func birthTime(_ string, st *syscall.Stat_t) string {
	return fmt.Sprintf("%d.%d", st.Birthtimespec.Sec, st.Birthtimespec.Nsec)
}
