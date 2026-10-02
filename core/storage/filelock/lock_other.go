//go:build !unix && !windows

package filelock

import "os"

// Supported reports that this platform has no advisory lock to take. The
// browser build is the only one that lands here, and it runs one process, so
// there is no other writer to order.
const Supported = false

func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) error { return nil }
