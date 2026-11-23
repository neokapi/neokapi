//go:build unix && !linux && !darwin

package project

import "syscall"

// birthTime is "" on a platform where kapi reads no birth time; the device and
// inode alone identify a file there.
func birthTime(string, *syscall.Stat_t) string { return "" }
