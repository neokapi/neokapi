//go:build windows || js

package pluginhost

import "os"

// fileOwner reports no owner where the platform has no uid.
func fileOwner(os.FileInfo) (uid, gid int, ok bool) { return 0, 0, false }
