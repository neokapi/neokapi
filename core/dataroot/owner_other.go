//go:build !unix

package dataroot

import "io/fs"

// privateModeApplies is false where the permission bits do not govern access.
// On Windows the temporary directory is the user's own (%LOCALAPPDATA%\Temp)
// and access is an ACL, which this check does not read; a link is still
// refused.
const privateModeApplies = false

// ownedByMe accepts every file where ownership is not a uid to compare.
func ownedByMe(fs.FileInfo) bool { return true }
