//go:build darwin

package dataroot

import (
	"fmt"
	"syscall"
)

func init() { birthTimeOf = birthTime }

// birthTime is when the directory was created. A directory removed and created
// again can get the same inode number; its birth time tells the two apart.
func birthTime(_ string, st *syscall.Stat_t) string {
	return fmt.Sprintf("%d.%d", st.Birthtimespec.Sec, st.Birthtimespec.Nsec)
}
