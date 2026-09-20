package project

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"regexp"
	"strings"
)

// A project's identity is its `id:`: a value minted once, derived from nothing
// the project can change. `name:` is a label a user edits and the folder is a
// path a user moves, so anything keyed on either is re-keyed the moment one of
// them changes, and two clones of one repository read as two projects.
//
// The shape is IDPrefix followed by lowercase RFC 4648 base32 of random bytes.
// The prefix says what the value is wherever it is pasted, and base32 keeps the
// body case-insensitive and free of every character that needs quoting in YAML,
// a URL, a filename or a shell word.

// IDPrefix opens every project id.
const IDPrefix = "prj_"

// idEntropyBytes is how many random bytes NewID draws. 128 bits leaves the
// chance of two independently minted ids colliding far below the chance of the
// storage they name failing.
const idEntropyBytes = 16

// idMinBodyChars is the shortest body ValidateID accepts: 100 bits at the five
// bits a base32 character carries.
const idMinBodyChars = 20

// idMaxBodyChars bounds the body so a pasted paragraph is refused at load
// rather than carried into every key derived from the id.
const idMaxBodyChars = 64

var idPattern = regexp.MustCompile(fmt.Sprintf(
	`^%s[a-z2-7]{%d,%d}$`, regexp.QuoteMeta(IDPrefix), idMinBodyChars, idMaxBodyChars))

// idEncoding drops the padding, so an id is one word of the alphabet with no
// trailing `=` for a shell or a URL to escape.
var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewID mints a project id.
//
// Panic rationale: crypto/rand.Read failing means the OS has no entropy source.
// Minting an identity from weak randomness produces collisions that surface
// arbitrarily later as one project's context answering for another, so there is
// nothing safer to do than stop. This follows the standard library (crypto/rand
// .Prime panics on read failure) and core/id.New.
func NewID() string {
	buf := make([]byte, idEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return IDPrefix + strings.ToLower(idEncoding.EncodeToString(buf))
}

// ValidateID checks an id's shape, accepting the empty string: a recipe written
// by a kapi that had no ids carries none, and such a project keeps being
// identified by its name.
func ValidateID(id string) error {
	if id == "" {
		return nil
	}
	if !idPattern.MatchString(id) {
		return fmt.Errorf(
			"id: %q is not a project id (want %s followed by %d to %d characters from a-z2-7, as `kapi init` mints)",
			id, IDPrefix, idMinBodyChars, idMaxBodyChars)
	}
	return nil
}

// Identity is the key a project's recorded context is written under: the id
// when the recipe carries one, the name otherwise. A recipe with neither
// answers the empty string, which is the honest reading of a project that
// states no identity at all.
func (p *KapiProject) Identity() string {
	if p == nil {
		return ""
	}
	if p.ID != "" {
		return p.ID
	}
	return p.Name
}
