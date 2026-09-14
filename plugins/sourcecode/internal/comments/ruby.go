package comments

import (
	"bytes"
	"regexp"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// rubyLanguages is Ruby. As RDoc and YARD read it, a `#` comment directly above
// a method, class, module, constant or DSL call documents it, and YARD's tags
// are placeholders. A `=begin` ... `=end` block is one comment.
func rubyLanguages() []*Language {
	return []*Language{{
		Name:        "ruby",
		DisplayName: "Ruby",
		Extensions:  []string{".rb", ".rake", ".gemspec", ".ru"},
		Markers:     comment.Markers{Line: []string{"#"}, Block: []comment.BlockMarker{{Open: "=begin", Close: "=end"}}},
		grammar:     tsruby.Language,
		syntax:      rubySyntax,
		once:        &sync.Once{},
		Canary: comment.Canary{
			Name:   "a Ruby comment with a doubled word, below a magic comment and above a string holding a comment marker",
			Source: []byte("# frozen_string_literal: true\n\n# Parses the the input.\ndef parse(text)\n  \"# not a comment #{text}\"\nend\n"),
			Block:  "func/parse",
		},
	}}
}

var rubySyntax = &syntax{
	unit:       rubyUnit,
	directives: rubyDirectives,
	decl:       rubyDecl,
	container:  rubyContainer,
	docTags:    true,
	yardTypes:  true,
}

// rubyUnit classifies a Ruby comment node. A `#` comment may document what
// follows it, a `#!` line that opens the file is its shebang, and `=begin`
// opens a block that a line starting with `=end` closes.
func rubyUnit(n *ts.Node, src []byte) (unit, bool) {
	if n.Kind() != "comment" {
		return unit{}, false
	}
	start, end := int(n.StartByte()), int(n.EndByte())
	for end > start && (src[end-1] == '\n' || src[end-1] == '\r') {
		end--
	}
	text := src[start:end]
	switch {
	case start == 0 && bytes.HasPrefix(text, []byte("#!")):
		return unit{start: start, end: end, kind: kindShebang, open: 2}, true
	case bytes.HasPrefix(text, []byte("=begin")):
		closer := 0
		if i := bytes.LastIndex(text, []byte("\n=end")); i >= 0 {
			closer = len(text) - i - 1
		}
		return unit{start: start, end: end, kind: kindBlock, open: len("=begin"), close: closer}, true
	}
	return unit{start: start, end: end, kind: kindDocLine, open: 1}, true
}

// rubyDirectives are the comments Ruby and its tools read: the shebang, the
// magic comments the interpreter reads, Sorbet's sigil, RuboCop's and Standard's
// switches, RDoc's directives and SimpleCov's exclusion.
var rubyDirectives = []directiveForm{
	{name: "shebang", match: func(t commentText) (string, bool) { return "shebang", t.kind == kindShebang }},
	regexpForm("magic", "magic comment", nil, regexp.MustCompile(`^(-\*-.*)?\b(frozen_string_literal|(en)?coding|warn_indent|shareable_constant_value)\s*:`)),
	regexpForm("sorbet", "typed", nil, regexp.MustCompile(`^typed:\s*(ignore|false|true|strict|strong)\b`)),
	regexpForm("rubocop", "rubocop", nil, regexp.MustCompile(`^rubocop:(disable|enable|todo)\b`)),
	regexpForm("standard", "standard", nil, regexp.MustCompile(`^standard:(disable|enable)\b`)),
	regexpForm("rdoc", "rdoc", nil, regexp.MustCompile(`^:(nodoc|stopdoc|startdoc|doc|notnew|yields|call-seq):`)),
	regexpForm("nocov", ":nocov:", nil, regexp.MustCompile(`^:nocov:`)),
}

// rubyContainer reports the nodes whose direct children are declarations: a
// file, a module, a class, a call and the block it takes, and the body of a
// module, class or block.
func rubyContainer(kind, parent string) bool {
	switch kind {
	case "program", "module", "class", "singleton_class", "call", "do_block", "block":
		return true
	case "body_statement", "block_body":
		switch parent {
		case "module", "class", "singleton_class", "do_block", "block":
			return true
		}
	}
	return false
}

// rubyDecl names a declaration: `module/Kapi` or `class/Parser` for a type at
// any depth, `func/parse` and `const/LIMIT` in a file, and a member's own name,
// such as `initialize`, `self.run` or `LIMIT`, inside a module or class. A call
// that takes a block, or that sits in a module, class or block body, is named
// for its method, as `cask` and `desc` are in a Homebrew cask.
func rubyDecl(n *ts.Node, parent string, src []byte) (string, bool) {
	inBody := parent == "body_statement" || parent == "block_body"
	member := false
	if inBody {
		if body := n.Parent(); body != nil && body.Parent() != nil {
			switch body.Parent().Kind() {
			case "class", "module", "singleton_class":
				member = true
			}
		}
	}
	switch n.Kind() {
	case "module", "class":
		if name := n.ChildByFieldName("name"); name != nil {
			return n.Kind() + "/" + nodeName(name, src), true
		}
	case "singleton_class":
		return "class/self", true
	case "method":
		if name := n.ChildByFieldName("name"); name != nil {
			if member {
				return nodeName(name, src), true
			}
			return "func/" + nodeName(name, src), true
		}
	case "singleton_method":
		if name := n.ChildByFieldName("name"); name != nil {
			if member {
				return "self." + nodeName(name, src), true
			}
			return "func/self." + nodeName(name, src), true
		}
	case "assignment":
		if left := n.ChildByFieldName("left"); left != nil && left.Kind() == "constant" {
			if member {
				return nodeName(left, src), true
			}
			return "const/" + nodeName(left, src), true
		}
	case "call":
		method := n.ChildByFieldName("method")
		if method != nil && (n.ChildByFieldName("block") != nil || inBody) {
			return nodeName(method, src), true
		}
	}
	return "", false
}
