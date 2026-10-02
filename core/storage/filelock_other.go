//go:build !unix && !windows

package storage

import "os"

// fileLockSupported reports that this platform has no advisory lock to take.
// The browser build is the only one that lands here, and it has no other
// process to order writers against.
const fileLockSupported = false

func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) error { return nil }
