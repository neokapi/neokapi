package comment

import (
	"path/filepath"
	"strings"
)

// Registry holds the two kinds of provider.
//
// A language provider reads a file that no format reader covers, and is found by
// the file's extension, so a caller consults it after format detection rather
// than instead of it. A format provider is supplied by a format whose reader
// already parses the file, and is found by the format's name. A language or a
// format with no provider in this build is absent, which a caller reports as a
// check that did not run rather than as a file with no comments.
type Registry struct {
	byExt    map[string]Provider
	byFormat map[string]Provider
}

// NewRegistry returns a registry holding language providers. A later provider
// for an extension replaces an earlier one.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{byExt: map[string]Provider{}, byFormat: map[string]Provider{}}
	for _, p := range providers {
		r.Register(p)
	}
	return r
}

// Register adds a language provider for each of its extensions.
func (r *Registry) Register(p Provider) {
	for _, ext := range p.Extensions() {
		r.byExt[strings.ToLower(ext)] = p
	}
}

// RegisterFormat adds the provider a format supplies for the files its reader
// parses.
func (r *Registry) RegisterFormat(format string, p Provider) {
	r.byFormat[format] = p
}

// For returns the language provider for path's extension.
func (r *Registry) For(path string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.byExt[strings.ToLower(filepath.Ext(path))]
	return p, ok
}

// ForFormat returns the provider the named format supplies.
func (r *Registry) ForFormat(format string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.byFormat[format]
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
	// FormatterCanary returns a file holding a comment the formatter rewrites,
	// which Disagreements must report.
	FormatterCanary() []byte
}

// Disagreement is one comment the formatter would write differently.
type Disagreement struct {
	// Comment indexes File.Comments.
	Comment int
	// Formatted is the comment group as the formatter writes it, with each
	// line's indentation removed.
	Formatted string
}
