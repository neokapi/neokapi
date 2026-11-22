//go:build !unix

package rootpath

import "io/fs"

// hasOtherNames reports whether the file has more than one hard link. This
// platform does not report it through fs.FileInfo, so it reports false.
func hasOtherNames(fs.FileInfo) bool { return false }
