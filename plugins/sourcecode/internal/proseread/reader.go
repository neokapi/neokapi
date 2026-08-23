// Package proseread turns a source file into the prose it contains.
//
// It is READ-ONLY by construction, and that is a decision rather than an
// omission. A round-trip error in a document produces a mangled paragraph; a
// round-trip error in a program produces one that does not compile, or — worse
// — one that does, with a changed string escape. kapi's write-back promise is
// built on byte-faithful round-trips proven over corpora, and source files
// would enter it at its weakest point. The plugin manifest declares
// `capabilities: ["read"]` and there is no writer to declare.
//
// What it extracts is decided by the SYNTAX TREE, not by pattern matching over
// bytes. That is the whole reason for a grammar: in
//
//	desc "Desktop workbench for a project's content context"
//	zap trash: ["~/Library/Caches/Kapi"]
//
// both arguments are string literals, and only the first is prose. The tree
// separates them because it knows the first is the argument of `desc` and the
// second an element of an array under `zap`. A regex cannot, which is why every
// grep-based checker eventually grows a hand-curated exemption list.
package proseread

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/formats/sourcecode"
	"github.com/neokapi/neokapi/core/model"
	ts "github.com/tree-sitter/go-tree-sitter"
	ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
)

// Options is the format's config, defined in core so a recipe naming
// `sourcecode` validates the same way whether or not this plugin is installed —
// and so the config has one definition rather than one on each side of the
// plugin boundary. See core/formats/sourcecode.
type Options = sourcecode.Config

// byName is the one place a language name maps to a grammar.
func byName(lang string) (*ts.Language, string, bool) {
	switch strings.ToLower(lang) {
	case "ruby":
		return ts.NewLanguage(ruby.Language()), "ruby", true
	default:
		return nil, "", false
	}
}

// byExt is the convenience: infer the language from the file's extension.
func byExt(path string) (*ts.Language, string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".rb":
		return byName("ruby")
	default:
		return nil, "", false
	}
}

// grammar resolves the language to parse with: the DECLARED one first, then the
// extension.
//
// Declaration wins because inference is not always available. The host hands a
// plugin its input inline and the ContentRef carries no filename on that path,
// so a reader dispatching across grammars has nothing to infer from — which is
// why the config takes `language` at all.
//
// Neither available is an error rather than a silent empty read: a collection
// declaring a file this reader cannot parse has a bug in the recipe, and
// reporting nothing would read as "checked, clean".
func grammar(path, declared string) (*ts.Language, string, error) {
	if declared != "" {
		lang, name, ok := byName(declared)
		if !ok {
			return nil, "", fmt.Errorf("no grammar for language %q", declared)
		}
		return lang, name, nil
	}
	if lang, name, ok := byExt(path); ok {
		return lang, name, nil
	}
	return nil, "", fmt.Errorf("no grammar for %q, and no language declared", filepath.Ext(path))
}

// proseKinds are the node kinds that can hold prose, per grammar. Everything
// else in the tree is structure: identifiers, operators, keywords, numbers.
var proseKinds = map[string]map[string]bool{
	"ruby": {
		"string_content": true,
		"heredoc_body":   true,
	},
}

var commentKinds = map[string]map[string]bool{
	"ruby": {"comment": true},
}

// Grammars names the languages this build understands, for the manifest and
// for `doctor` to report honestly.
func Grammars() []string { return []string{"ruby"} }

// ReadParts parses src and returns the layer + block sequence for the prose it
// holds. uri names the layer and chooses the grammar.
func ReadParts(src []byte, locale model.LocaleID, uri string, opts Options) ([]*model.Part, error) {
	lang, name, err := grammar(uri, opts.Language)
	if err != nil {
		return nil, err
	}

	parser := ts.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("set %s grammar: %w", name, err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("parse %s: no tree", uri)
	}
	defer tree.Close()

	root := &model.Layer{
		ID: "doc1", Name: uri, Format: "sourcecode", Locale: locale,
		Encoding: "UTF-8", MimeType: "text/plain",
	}
	parts := []*model.Part{{Type: model.PartLayerStart, Resource: root}}

	want := map[string]bool{}
	for _, p := range opts.NodePathPatterns {
		want[p] = true
	}

	kinds := map[string]bool{}
	for k := range proseKinds[name] {
		kinds[k] = true
	}
	if opts.Comments {
		for k := range commentKinds[name] {
			kinds[k] = true
		}
	}

	e := &extractor{src: src, kinds: kinds, want: want, heredocOwner: heredocOwners(tree.RootNode(), src)}
	e.walk(tree.RootNode(), "")

	for _, b := range e.blocks {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}
	parts = append(parts, &model.Part{Type: model.PartLayerEnd, Resource: root})
	return parts, nil
}

type extractor struct {
	src          []byte
	kinds        map[string]bool
	want         map[string]bool
	blocks       []*model.Block
	n            int
	heredocOwner []string
	heredocSeen  int
}

// callName is the node path segment a call contributes: `desc "..."` puts the
// string under `desc`.
func callName(n *ts.Node, src []byte) string {
	switch n.Kind() {
	case "call", "method_call", "command":
		if m := n.ChildByFieldName("method"); m != nil {
			return m.Utf8Text(src)
		}
	}
	return ""
}

// heredocOwners lists the call each heredoc belongs to, in document order.
//
// A heredoc body is NOT a child of the call that opens it. `caveats <<~EOS`
// puts a heredoc_beginning under the call and parks the body at the top level,
// so walking parents alone attributes the text to whatever block encloses it —
// `cask`, here, rather than the field a recipe would name. Bodies appear in the
// same order as their openers, which is what makes the pairing safe.
func heredocOwners(root *ts.Node, src []byte) []string {
	var owners []string
	var scan func(n *ts.Node, enclosing string)
	scan = func(n *ts.Node, enclosing string) {
		if m := callName(n, src); m != "" {
			enclosing = m
		}
		if n.Kind() == "heredoc_beginning" {
			owners = append(owners, enclosing)
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			scan(n.Child(i), enclosing)
		}
	}
	scan(root, "")
	return owners
}

func (e *extractor) walk(n *ts.Node, enclosing string) {
	if m := callName(n, e.src); m != "" {
		enclosing = m
	}

	kind := n.Kind()
	if e.kinds[kind] {
		path := enclosing
		if kind == "heredoc_body" {
			if e.heredocSeen < len(e.heredocOwner) {
				path = e.heredocOwner[e.heredocSeen]
			}
			e.heredocSeen++
		}
		e.emit(path, n.Utf8Text(e.src))
	}

	for i := uint(0); i < n.ChildCount(); i++ {
		e.walk(n.Child(i), enclosing)
	}
}

func (e *extractor) emit(path, text string) {
	if len(e.want) > 0 && !e.want[path] {
		return
	}
	// Whitespace-only nodes are structure the grammar happens to expose (the
	// blank line a heredoc opens with, indentation). A block with nothing in it
	// cannot be read, scored, or acted on, and reporting one as content is how a
	// check earns findings nobody trusts.
	if strings.TrimSpace(text) == "" {
		return
	}
	e.n++
	b := model.NewBlock(fmt.Sprintf("tu%d", e.n), strings.TrimSpace(text))
	if path != "" {
		b.Name = path
	}
	e.blocks = append(e.blocks, b)
}
