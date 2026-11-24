//go:build !darwin && !linux

package pluginhost

import "os"

// changeTime is 0 where kapi does not read an inode change time; the mode,
// owner and modification time in the fingerprint still change with a fix.
func changeTime(os.FileInfo) int64 { return 0 }
