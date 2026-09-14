package comments

import (
	"bytes"
	"regexp"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// rustLanguages is Rust. `///` and `/** */` document the item after them, and
// `//!` and `/*! */` the module or item they sit in.
func rustLanguages() []*Language {
	return []*Language{{
		Name:        "rust",
		DisplayName: "Rust",
		Extensions:  []string{".rs"},
		// The doc markers come first, so a comment line read through the
		// markers loses its whole opener.
		Markers: comment.Markers{Line: []string{"///", "//!", "//"}, Block: []comment.BlockMarker{{Open: "/*", Close: "*/", Nested: true}}},
		grammar: tsrust.Language,
		syntax:  rustSyntax,
		once:    &sync.Once{},
		Canary: comment.Canary{
			Name:   "a Rust doc comment with a doubled word, below a licence tag and above a string holding a comment marker",
			Source: []byte("// SPDX-License-Identifier: Apache-2.0\n/// Parses the the input.\npub fn parse(text: &str) -> &str {\n    let _ = \"// not a comment\";\n    text\n}\n"),
			Block:  "func/parse",
		},
	}}
}

var rustSyntax = &syntax{
	unit:       rustUnit,
	directives: rustDirectives,
	decl:       rustDecl,
	container:  rustContainer,
	attached:   map[string]bool{"attribute_item": true},
}

// rustUnit classifies a Rust comment node. Its doc style follows rustc's
// lexer: `///` and `/**` document what follows, except `////`, `/***` and
// `/**/`, and `//!` and `/*!` document what encloses them. The grammar includes
// the line break in a line doc comment's node, and rustc reads a file with CRLF
// line endings as if it had LF ones, so a line comment ends before either.
func rustUnit(n *ts.Node, src []byte) (unit, bool) {
	start, end := int(n.StartByte()), int(n.EndByte())
	switch n.Kind() {
	case "line_comment":
		for end > start && (src[end-1] == '\n' || src[end-1] == '\r') {
			end--
		}
		text := src[start:end]
		switch {
		case bytes.HasPrefix(text, []byte("//!")):
			return unit{start: start, end: end, kind: kindInnerDocLine, open: 3}, true
		case bytes.HasPrefix(text, []byte("///")) && !bytes.HasPrefix(text, []byte("////")):
			return unit{start: start, end: end, kind: kindDocLine, open: 3}, true
		}
		return unit{start: start, end: end, kind: kindLine, open: 2}, true
	case "block_comment":
		text := src[start:end]
		switch {
		case bytes.HasPrefix(text, []byte("/*!")) && len(text) >= 5:
			return unit{start: start, end: end, kind: kindInnerDocBlock, open: 3, close: 2}, true
		case bytes.HasPrefix(text, []byte("/**")) && !bytes.HasPrefix(text, []byte("/***")) && len(text) >= 5:
			return unit{start: start, end: end, kind: kindDocBlock, open: 3, close: 2}, true
		}
		return unit{start: start, end: end, kind: kindBlock, open: 2, close: 2}, true
	}
	return unit{}, false
}

// rustDirectives are the comments a tool reads in Rust source: an SPDX licence
// tag, which licence scanners read, and the folding markers rust-analyzer reads.
var rustDirectives = []directiveForm{
	regexpForm("spdx", "SPDX-License-Identifier", []unitKind{kindLine, kindBlock}, regexp.MustCompile(`^SPDX-License-Identifier:`)),
	regexpForm("region", "region", []unitKind{kindLine}, regexp.MustCompile(`^(region|endregion)\b`)),
}

// rustContainer reports the nodes whose direct children are items or members:
// a file, a module, a trait, an impl or an extern block and its body, a
// struct's or a union's fields, and an enum's variants.
func rustContainer(kind, parent string) bool {
	switch kind {
	case "source_file", "mod_item", "trait_item", "impl_item", "foreign_mod_item", "struct_item", "union_item", "enum_item":
		return true
	case "declaration_list":
		return parent == "mod_item" || parent == "trait_item" || parent == "impl_item" || parent == "foreign_mod_item"
	case "field_declaration_list":
		return parent == "struct_item" || parent == "union_item"
	case "enum_variant_list":
		return parent == "enum_item"
	}
	return false
}

// rustDecl names an item: `func/parse`, `struct/Point`, `mod/util`, and
// `impl/Point` or `impl/fmt::Display for Point` in a file or a module, and a
// member's own name inside a trait, an impl, a struct, a union or an enum.
func rustDecl(n *ts.Node, parent string, src []byte) (string, bool) {
	member := parent == "field_declaration_list" || parent == "enum_variant_list"
	if parent == "declaration_list" {
		if body := n.Parent(); body != nil && body.Parent() != nil {
			switch body.Parent().Kind() {
			case "trait_item", "impl_item":
				member = true
			}
		}
	}
	named := func(prefix string) (string, bool) {
		name := n.ChildByFieldName("name")
		if name == nil {
			return "", false
		}
		if member {
			return nodeName(name, src), true
		}
		return prefix + nodeName(name, src), true
	}
	switch n.Kind() {
	case "function_item", "function_signature_item":
		return named("func/")
	case "struct_item":
		return named("struct/")
	case "enum_item":
		return named("enum/")
	case "union_item":
		return named("union/")
	case "trait_item":
		return named("trait/")
	case "mod_item":
		return named("mod/")
	case "const_item":
		return named("const/")
	case "static_item":
		return named("static/")
	case "type_item", "associated_type":
		return named("type/")
	case "macro_definition":
		return named("macro/")
	case "field_declaration", "enum_variant":
		if member {
			return named("")
		}
	case "impl_item":
		typ := n.ChildByFieldName("type")
		if typ == nil {
			return "", false
		}
		if trait := n.ChildByFieldName("trait"); trait != nil {
			return "impl/" + rustTypeName(trait, src) + " for " + rustTypeName(typ, src), true
		}
		return "impl/" + rustTypeName(typ, src), true
	}
	return "", false
}

// rustTypeName is a type as a path segment, without its type arguments.
func rustTypeName(n *ts.Node, src []byte) string {
	if n.Kind() == "generic_type" {
		if base := n.ChildByFieldName("type"); base != nil {
			n = base
		}
	}
	return selectorName(nodeName(n, src))
}
