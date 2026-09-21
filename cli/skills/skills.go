// Package skills carries the kapi agent skill into the binary, so `kapi init`
// can put a copy of it in a project without the user installing anything.
//
// The source tree under data/ is the one the make targets publish to the
// plugin marketplace and to the portable collection. Embedding it adds a third
// copy from the same source, and that copy has a property the published ones
// cannot have: it is the skill the running binary was built with, so the
// commands and flags it names are the commands and flags that binary has.
package skills

import (
	"embed"
	"io/fs"
)

//go:embed data
var data embed.FS

// Tree returns the skill directories as an agent host reads them: one
// directory per skill, named for the skill, each holding a SKILL.md and
// whatever files it references.
func Tree() fs.FS {
	sub, err := fs.Sub(data, "data")
	if err != nil {
		// data is embedded at build time, so a failure here is a broken build
		// rather than anything a caller did.
		panic("skills: embedded skill tree: " + err.Error())
	}
	return sub
}
