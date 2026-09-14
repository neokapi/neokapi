// Package comments locates the comments in source files for kapi's comment
// layer (core/comment).
//
// A grammar reads the file, so a comment marker inside a string, a template
// literal, a regular expression or JSX text is content and never a comment.
// Each comment the grammar reports becomes part of an addressable comment or
// an exclusion, with a half-open byte span into the bytes it was given. A line
// comment's span runs from its marker to the end of its line, and a block
// comment's span holds both delimiters.
//
// The host reaches this package through the LocateComments RPC. The package
// never writes a file.
package comments

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"unsafe"

	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/neokapi/neokapi/core/comment"
)

// Language is one language whose comments the package locates.
type Language struct {
	// Name is the language as the comment layer reports it, such as "typescript".
	Name string
	// DisplayName is the human-readable name, such as "TypeScript".
	DisplayName string
	// Extensions lists the file extensions the language is read for.
	Extensions []string
	// Markers are the language's comment delimiters, which LineText reads.
	Markers comment.Markers
	// Canary is the file the host locates beside every real file.
	Canary comment.Canary

	grammar func() unsafe.Pointer
	syntax  *syntax
	once    *sync.Once
	lang    *ts.Language
}

// language returns the tree-sitter language, loading it once.
func (l *Language) language() *ts.Language {
	l.once.Do(func() { l.lang = ts.NewLanguage(l.grammar()) })
	return l.lang
}

// Languages lists every language the package reads, sorted by name.
func Languages() []Language {
	out := make([]Language, 0, len(languages))
	for _, l := range languages {
		out = append(out, *l)
	}
	slices.SortFunc(out, func(a, b Language) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Lookup returns the language with the given name.
func Lookup(name string) (Language, bool) {
	for _, l := range languages {
		if l.Name == name {
			return *l, true
		}
	}
	return Language{}, false
}

// languages is every language the package reads.
var languages = slices.Concat(jsLanguages(), pythonLanguages(), bashLanguages(), cssLanguages(), rustLanguages(), javaLanguages(), csharpLanguages(), cLanguages())

// Locate returns the comments in src, a file in the named language. name is
// the file's path, which the language may consult. A file the grammar cannot
// parse whole is ErrUnlocated, since a comment's position is only certain in a
// tree with no error in it, unless the language's syntax is tolerant: C and C++
// comments are located anyway, because the preprocessor makes a grammar read
// sound code as malformed while their comments stay where the lexer put them.
func Locate(language, name string, src []byte) (*comment.File, error) {
	var lang *Language
	for _, l := range languages {
		if l.Name == language {
			lang = l
		}
	}
	if lang == nil {
		return nil, fmt.Errorf("no comment grammar for language %q", language)
	}
	parser := ts.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang.language()); err != nil {
		return nil, fmt.Errorf("set the %s grammar: %w", lang.Name, err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("parse %s: no tree", name)
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() && !lang.syntax.tolerant {
		return nil, fmt.Errorf("%w: %s does not parse as %s (%s)", comment.ErrUnlocated, displayName(name), lang.DisplayName, firstError(root))
	}
	s := newScanner(lang, src, root)
	return s.file(), nil
}

// firstError describes where the first syntax error in a tree sits.
func firstError(root *ts.Node) string {
	c := root.Walk()
	defer c.Close()
	for {
		n := c.Node()
		if n.IsError() || n.IsMissing() {
			p := n.StartPosition()
			return fmt.Sprintf("a syntax error at line %d, column %d", p.Row+1, p.Column+1)
		}
		if n.HasError() && c.GotoFirstChild() {
			continue
		}
		for !c.GotoNextSibling() {
			if !c.GotoParent() {
				return "a syntax error"
			}
		}
	}
}

func displayName(name string) string {
	if name == "" {
		return "the file"
	}
	return name
}
