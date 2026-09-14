package comments

import (
	"bytes"
	"regexp"
	"strings"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// jsLanguages are TypeScript, TSX and JavaScript. They share one syntax: `//`
// and `/* */` comments, JSDoc documentation blocks, and the declarations of
// ECMAScript. TSX has a grammar of its own, because a `<T>` type assertion in a
// .ts file is JSX in a .tsx file, and each grammar is checked by its own canary.
func jsLanguages() []*Language {
	return []*Language{
		{
			Name:        "typescript",
			DisplayName: "TypeScript",
			Extensions:  []string{".ts", ".mts", ".cts"},
			grammar:     typescript.LanguageTypescript,
			syntax:      jsSyntax,
			once:        &sync.Once{},
			Canary: comment.Canary{
				Name:   "a TypeScript doc comment with a doubled word, below a directive and above a string holding a comment marker",
				Source: []byte("// eslint-disable-next-line no-console\n/** Parses the the input. */\nexport function parse(src: string): string {\n  return \"// not a comment\" + src;\n}\n"),
				Block:  "func/parse",
			},
		},
		{
			Name:        "tsx",
			DisplayName: "TSX",
			Extensions:  []string{".tsx"},
			grammar:     typescript.LanguageTSX,
			syntax:      jsSyntax,
			once:        &sync.Once{},
			Canary: comment.Canary{
				Name:   "a TSX doc comment with a doubled word, above JSX text holding a comment marker",
				Source: []byte("/** Renders the the greeting. */\nexport function Greeting() {\n  return <p>// not a comment {/* inline */}</p>;\n}\n"),
				Block:  "func/Greeting",
			},
		},
		{
			Name:        "javascript",
			DisplayName: "JavaScript",
			Extensions:  []string{".js", ".jsx", ".mjs", ".cjs"},
			grammar:     javascript.Language,
			syntax:      jsSyntax,
			once:        &sync.Once{},
			Canary: comment.Canary{
				Name:   "a JavaScript doc comment with a doubled word, below a shebang and above a template literal holding a comment marker",
				Source: []byte("#!/usr/bin/env node\n/** Reads the the config. */\nexport function read() {\n  return `// not a comment`;\n}\n"),
				Block:  "func/read",
			},
		},
	}
}

var jsSyntax = &syntax{
	unit:       jsUnit,
	directives: jsDirectives,
	decl:       jsDecl,
	container:  jsContainer,
	wrappers:   map[string]bool{"export_statement": true, "ambient_declaration": true, "expression_statement": true},
	docTags:    true,
}

// jsUnit classifies the comment nodes the ECMAScript grammars report: `//`,
// `/* */`, `/** */`, the HTML-like `<!-- -->` of legacy scripts, and the
// shebang.
func jsUnit(n *ts.Node, src []byte) (unit, bool) {
	start, end := int(n.StartByte()), int(n.EndByte())
	text := src[start:end]
	switch n.Kind() {
	case "comment":
		switch {
		case bytes.HasPrefix(text, []byte("//")):
			return unit{start: start, end: end, kind: kindLine, open: 2}, true
		case bytes.HasPrefix(text, []byte("/**")) && len(text) >= 5:
			return unit{start: start, end: end, kind: kindDocBlock, open: 3, close: 2}, true
		case bytes.HasPrefix(text, []byte("/*")):
			return unit{start: start, end: end, kind: kindBlock, open: 2, close: 2}, true
		}
	case "html_comment":
		u := unit{start: start, end: end, kind: kindBlock, open: 4}
		if bytes.HasSuffix(text, []byte("-->")) && len(text) >= 7 {
			u.close = 3
		}
		return u, true
	case "hash_bang_line":
		return unit{start: start, end: end, kind: kindShebang, open: 2}, true
	}
	return unit{}, false
}

var blockKinds = []unitKind{kindBlock, kindDocBlock}

// jsDirectives are the comments a JavaScript or TypeScript tool reads.
var jsDirectives = []directiveForm{
	{name: "shebang", match: func(t commentText) (string, bool) { return "shebang", t.kind == kindShebang }},
	// ESLint's inline configuration. The bare `eslint` and `global` forms are
	// read only in block comments, and only with a rule or a name after them.
	prefixForm("eslint-disable", "eslint-disable"),
	wordForm("eslint-enable", "eslint-enable"),
	wordForm("eslint-env", "eslint-env"),
	regexpForm("eslint-config", "eslint", blockKinds, regexp.MustCompile(`^eslint\s+[@\w/-]+\s*:`)),
	regexpForm("eslint-global", "global", blockKinds, regexp.MustCompile(`^globals?\s+[\w$]`)),
	prefixForm("oxlint", "oxlint-disable", "oxlint-enable"),
	prefixForm("biome", "biome-ignore"),
	regexpForm("tslint", "tslint", nil, regexp.MustCompile(`^tslint:`)),
	wordForm("typescript", "@ts-expect-error", "@ts-ignore", "@ts-nocheck", "@ts-check"),
	// A triple-slash directive: `/// <reference types="node" />`.
	{name: "triple-slash", match: func(t commentText) (string, bool) {
		rest, ok := strings.CutPrefix(t.raw, "///")
		if !ok || t.kind != kindLine {
			return "", false
		}
		rest = strings.TrimSpace(rest)
		for _, tag := range []string{"<reference", "<amd-module", "<amd-dependency"} {
			if strings.HasPrefix(rest, tag) {
				return "///" + tag + ">", true
			}
		}
		return "", false
	}},
	prefixForm("prettier", "prettier-ignore"),
	regexpForm("coverage", "coverage-ignore", nil, regexp.MustCompile(`^(istanbul|c8|v8) ignore\b`)),
	regexpForm("pure-annotation", "pure", blockKinds, regexp.MustCompile(`^[#@]__(PURE|NO_SIDE_EFFECTS)__$`)),
	wordForm("vite", "@vite-ignore"),
	regexpForm("bundler-magic", "bundler-magic", blockKinds, regexp.MustCompile(`^(webpack[A-Z]\w*|turbopackIgnore)\s*:`)),
	regexpForm("source-map", "sourceMappingURL", nil, regexp.MustCompile(`^[#@]\s?source(Mapping)?URL=`)),
	// A pragma a tool reads wherever it sits in the file's docblock: the test
	// environment, and the JSX factory.
	{name: "pragma", match: func(t commentText) (string, bool) {
		for _, l := range t.lines {
			w := leadingWord(strings.TrimSpace(l))
			switch w {
			case "@vitest-environment", "@jest-environment", "@jsx", "@jsxImportSource", "@jsxFrag", "@jsxRuntime":
				return w, true
			}
		}
		return "", false
	}},
}

// jsContainer reports the nodes whose direct children are structural, so a
// declaration among them extends a comment's subject.
func jsContainer(kind, parent string) bool {
	switch kind {
	case "program", "export_statement", "ambient_declaration", "internal_module", "module",
		"class_declaration", "abstract_class_declaration", "class", "interface_declaration",
		"type_alias_declaration", "enum_declaration", "class_body", "interface_body", "enum_body":
		return true
	case "expression_statement":
		return parent == "program" || parent == "statement_block"
	case "statement_block":
		return parent == "internal_module" || parent == "module" || parent == "ambient_declaration"
	case "object_type":
		return parent == "type_alias_declaration" || parent == "interface_declaration"
	}
	return false
}

// jsDecl names a declaration: `func/parse`, `class/Parser`, `interface/Options`,
// `type/Id`, `enum/Kind`, `const/value`, `namespace/Util` and `module/name` at
// the top of a file or a namespace, and the member's own name inside a class,
// interface, type literal or enum.
func jsDecl(n *ts.Node, parent string, src []byte) (string, bool) {
	name := func() string { return nodeName(n.ChildByFieldName("name"), src) }
	switch n.Kind() {
	case "function_declaration", "generator_function_declaration", "function_signature":
		return "func/" + name(), true
	case "class_declaration", "abstract_class_declaration":
		return "class/" + name(), true
	case "class":
		if n.ChildByFieldName("name") == nil {
			return "class/default", true
		}
		return "class/" + name(), true
	case "interface_declaration":
		return "interface/" + name(), true
	case "type_alias_declaration":
		return "type/" + name(), true
	case "enum_declaration":
		return "enum/" + name(), true
	case "internal_module":
		return "namespace/" + name(), true
	case "module":
		return "module/" + name(), true
	case "lexical_declaration", "variable_declaration":
		keyword := "var"
		if c := n.Child(0); c != nil {
			switch c.Kind() {
			case "const", "let", "using":
				keyword = c.Kind()
			}
		}
		for i := uint(0); i < n.NamedChildCount(); i++ {
			if d := n.NamedChild(i); d.Kind() == "variable_declarator" {
				return keyword + "/" + nodeName(d.ChildByFieldName("name"), src), true
			}
		}
		return keyword, true
	case "method_definition", "method_signature", "abstract_method_signature",
		"public_field_definition", "field_definition", "property_signature", "enum_assignment":
		return name(), true
	case "property_identifier":
		if parent == "enum_body" {
			return nodeName(n, src), true
		}
	case "index_signature":
		return "index", true
	case "call_signature":
		return "call", true
	case "construct_signature":
		return "new", true
	case "statement_block":
		if parent == "ambient_declaration" {
			return "global", true
		}
	case "export_statement":
		for i := uint(0); i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c.Kind() == "decorator" || c.Kind() == "comment" {
				continue
			}
			if segment, ok := jsDecl(c, parent, src); ok {
				return segment, true
			}
		}
		if n.ChildByFieldName("value") != nil {
			return "default", true
		}
	case "ambient_declaration":
		for i := uint(0); i < n.NamedChildCount(); i++ {
			if segment, ok := jsDecl(n.NamedChild(i), "ambient_declaration", src); ok {
				return segment, true
			}
		}
	case "expression_statement":
		if c := n.NamedChild(0); c != nil && c.Kind() == "internal_module" {
			return jsDecl(c, parent, src)
		}
	}
	return "", false
}

// nodeName is a declared name as one path segment: quotes removed, and a slash
// in a module name or a string key replaced so it cannot split the path.
func nodeName(n *ts.Node, src []byte) string {
	if n == nil {
		return "anonymous"
	}
	switch n.Kind() {
	case "object_pattern", "array_pattern":
		return "pattern"
	}
	name := strings.Trim(n.Utf8Text(src), "'\"`")
	name = strings.Join(strings.Fields(name), "")
	name = strings.ReplaceAll(name, "/", ".")
	if name == "" {
		return "anonymous"
	}
	return name
}
