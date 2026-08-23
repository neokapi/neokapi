// Package sourcecode holds the config for the `sourcecode` format, whose reader
// lives out of core.
//
// The reader is the kapi-sourcecode plugin: tree-sitter grammars are C, and
// building them into core would put cgo on every kapi binary and let a parser
// fault on a malformed file take the process with it. The plugin keeps both
// isolated, and imports this package so the config has ONE definition rather
// than one here and another in the plugin. That is the same split
// core/formats/pdf uses for kapi-pdfium.
//
// The format is READ-ONLY. There is no writer here and none in the plugin: a
// round-trip error in a document mangles a paragraph, while one in a program
// yields something that does not compile — or worse, something that does, with
// a changed string escape.
package sourcecode

import (
	"errors"
	"fmt"
)

// Config selects which prose a source file yields.
type Config struct {
	// NodePathPatterns names the calls whose arguments hold prose, e.g.
	// ["desc", "caveats"] for a Homebrew cask.
	//
	// It is the analogue of the yaml reader's KeyPathPatterns and the json
	// reader's extraction rules: a recipe already says which KEYS of a config
	// file hold sentences, and this says the same thing about a CALL. Empty
	// extracts every string the grammar exposes, each labelled with the call
	// that owns it — which is how a reader finds out what a file holds before
	// narrowing it.
	//
	// Matching is exact rather than glob: a call name is an identifier, not a
	// path, so there is no hierarchy for a wildcard to span.
	NodePathPatterns []string

	// Language names the grammar explicitly, e.g. "ruby".
	//
	// The reader can infer it from the file's extension, but it cannot always
	// SEE one: the host hands a plugin its input inline, and the ContentRef
	// carries no filename on that path. A single-format reader can default
	// (kapi-pdfium assumes "document.pdf"); one that dispatches across grammars
	// cannot guess a language without being wrong sooner or later. Declaring it
	// is the reliable answer, and inference is the convenience.
	Language string

	// Comments extracts comment nodes as well as strings.
	//
	// Off by default, and the default is a judgement rather than caution: a
	// comment is written for the next person reading the code, and holding it
	// to the register of published prose is a different decision from holding a
	// product string to it. A project that wants both says so.
	Comments bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "sourcecode" }

// Reset restores default values.
func (c *Config) Reset() {
	c.NodePathPatterns = nil
	c.Language = ""
	c.Comments = false
}

// Validate checks configuration validity.
//
// An empty or blank node path is refused rather than ignored: it can only come
// from a typo or a stray comma, and silently dropping it would narrow the
// extraction without saying so.
func (c *Config) Validate() error {
	for i, p := range c.NodePathPatterns {
		if p == "" {
			return fmt.Errorf("sourcecode: nodePathPatterns[%d] is empty", i)
		}
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "nodePathPatterns":
			switch v := val.(type) {
			case []any:
				paths := make([]string, len(v))
				for i, p := range v {
					s, ok := p.(string)
					if !ok {
						return errors.New("sourcecode: nodePathPatterns items must be strings")
					}
					paths[i] = s
				}
				c.NodePathPatterns = paths
			case []string:
				c.NodePathPatterns = v
			default:
				return errors.New("sourcecode: nodePathPatterns must be a string array")
			}
		case "language":
			s, ok := val.(string)
			if !ok {
				return errors.New("sourcecode: language must be a string")
			}
			c.Language = s
		case "comments":
			b, ok := val.(bool)
			if !ok {
				return errors.New("sourcecode: comments must be bool")
			}
			c.Comments = b
		default:
			return fmt.Errorf("sourcecode: unknown parameter: %s", key)
		}
	}
	return c.Validate()
}
