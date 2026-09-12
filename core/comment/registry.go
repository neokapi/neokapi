package comment

import (
	"path/filepath"
	"strings"
)

// Registry maps file extensions to the language providers that read them.
//
// A file reaches a provider only when no format reader covers it, so the
// registry is consulted after format detection rather than instead of it. A
// language with no provider in this build is absent, which a caller reports as
// a check that did not run rather than as a file with no comments.
type Registry struct {
	byExt map[string]Provider
}

// NewRegistry returns a registry holding providers. A later provider for an
// extension replaces an earlier one.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{byExt: map[string]Provider{}}
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

// Register adds p for each of its extensions.
func (r *Registry) Register(p Provider) {
	for _, ext := range p.Extensions() {
		r.byExt[strings.ToLower(ext)] = p
	}
}

// For returns the provider for path's extension.
func (r *Registry) For(path string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.byExt[strings.ToLower(filepath.Ext(path))]
	return p, ok
}

// Formatter is implemented by a provider whose language has a canonical
// formatter. It compares a file with what the formatter would write and reports
// the comments whose lines differ. It never writes.
type Formatter interface {
	// FormatterName names the formatter, such as "gofmt".
	FormatterName() string
	// Disagreements reports the comments in f, located in src, that the
	// formatter would rewrite.
	Disagreements(name string, src []byte, f *File) ([]Disagreement, error)
}

// Disagreement is one comment the formatter would write differently.
type Disagreement struct {
	// Comment indexes File.Comments.
	Comment int
	// Formatted is the comment group as the formatter writes it, with each
	// line's indentation removed.
	Formatted string
}
