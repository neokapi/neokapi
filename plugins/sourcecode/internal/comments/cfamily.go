package comments

import (
	"bytes"
	"regexp"
	"strings"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsc "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tscpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// cMarkers are the comment delimiters of C and C++. The Doxygen markers come
// first, so a comment line read through the markers loses its whole opener. A
// backslash that ends a line comment's line splices the next line onto it.
var cMarkers = comment.Markers{Line: []string{"///", "//!", "//"}, Block: []comment.BlockMarker{{Open: "/*", Close: "*/"}}, Splice: `\`}

// cLanguages is C and C++. Doxygen's `///`, `//!`, `/** */` and `/*! */`
// directly before a declaration document it.
//
// A grammar that does not expand macros reads much sound C and C++ as
// malformed, and can fold a comment into a directive's text. So cComments reads
// each file's comments from its characters too, and the file is located only
// when the tree reports a comment at exactly each span that scan reads and at
// no other. A file whose tree agrees is located even when the tree holds syntax
// errors.
func cLanguages() []*Language {
	return []*Language{
		{
			Name:        "c",
			DisplayName: "C",
			Extensions:  []string{".c", ".h"},
			Markers:     cMarkers,
			grammar:     tsc.Language,
			syntax:      cFamily(cDialect{}),
			once:        &sync.Once{},
			Canary: comment.Canary{
				Name:   "a Doxygen comment with a doubled word, below a licence tag and above a string holding a comment marker",
				Source: []byte("// SPDX-License-Identifier: Apache-2.0\n/** Parses the the input. */\nint parse(const char *text);\nstatic const char *marker = \"// not a comment\";\n"),
				Block:  "func/parse",
			},
		},
		{
			Name:        "cpp",
			DisplayName: "C++",
			Extensions:  []string{".cpp", ".cc", ".cxx", ".c++", ".hpp", ".hh", ".hxx", ".h++", ".ipp", ".tpp"},
			Markers:     cMarkers,
			grammar:     tscpp.Language,
			syntax:      cFamily(cDialect{rawStrings: true}),
			once:        &sync.Once{},
			Canary: comment.Canary{
				Name:   "a Doxygen comment with a doubled word, below a clang-format directive and above a raw string holding a comment marker",
				Source: []byte("// clang-format off\n/// Parses the the input.\nclass Parser {};\nauto marker = R\"(// not a comment)\";\n"),
				Block:  "class/Parser",
			},
		},
	}
}

// cFamily is the syntax C and C++ share, with comments read from a file's
// characters in dialect d.
func cFamily(d cDialect) *syntax {
	return &syntax{
		unit:        cUnit,
		directives:  cDirectives,
		decl:        cDecl,
		container:   cContainer,
		wrappers:    map[string]bool{"template_declaration": true},
		attached:    map[string]bool{"attribute_declaration": true},
		docTags:     true,
		docCommands: true,
		lexical:     func(src []byte) ([][2]int, error) { return cComments(src, d) },
	}
}

func cUnit(n *ts.Node, src []byte) (unit, bool) {
	if n.Kind() != "comment" {
		return unit{}, false
	}
	start, end := int(n.StartByte()), int(n.EndByte())
	if bytes.HasPrefix(src[start:end], []byte("//")) {
		for end > start && (src[end-1] == '\n' || src[end-1] == '\r') {
			end--
		}
	}
	return cCommentUnit(src, start, end), true
}

// cCommentUnit classifies a C or C++ comment by its Doxygen marker. `///` and
// `//!` document what follows, except `////`; `/**` and `/*!` do too, except
// `/***` and `/**/`. A marker ending in `<`, such as `///<`, documents the
// member before it on its line, and reads as a comment after code.
func cCommentUnit(src []byte, start, end int) unit {
	text := src[start:end]
	if bytes.HasPrefix(text, []byte("//")) {
		switch {
		case bytes.HasPrefix(text, []byte("///<")), bytes.HasPrefix(text, []byte("//!<")):
			return unit{start: start, end: end, kind: kindLine, open: 4}
		case bytes.HasPrefix(text, []byte("//!")), bytes.HasPrefix(text, []byte("///")) && !bytes.HasPrefix(text, []byte("////")):
			return unit{start: start, end: end, kind: kindDocLine, open: 3}
		}
		return unit{start: start, end: end, kind: kindLine, open: 2}
	}
	switch {
	case end-start >= 6 && (bytes.HasPrefix(text, []byte("/**<")) || bytes.HasPrefix(text, []byte("/*!<"))):
		return unit{start: start, end: end, kind: kindBlock, open: 4, close: 2}
	case end-start >= 5 && (bytes.HasPrefix(text, []byte("/*!")) || bytes.HasPrefix(text, []byte("/**")) && !bytes.HasPrefix(text, []byte("/***"))):
		return unit{start: start, end: end, kind: kindDocBlock, open: 3, close: 2}
	}
	return unit{start: start, end: end, kind: kindBlock, open: 2, close: 2}
}

// cDirectives are the comments a C or C++ tool reads: an SPDX licence tag,
// clang-tidy's NOLINT forms, clang-format's switches, cppcheck's and
// include-what-you-use's pragmas, lcov's and gcovr's exclusions, Sonar's
// suppression, and an editor's mode line. A comment that only labels what a
// closing line closes, such as `#endif // DEBUG` or `} // namespace kapi`, is a
// label rather than prose.
var cDirectives = []directiveForm{
	regexpForm("spdx", "SPDX-License-Identifier", nil, regexp.MustCompile(`^SPDX-License-Identifier:`)),
	{name: "nolint", match: func(t commentText) (string, bool) { return "NOLINT", nolintMarker.MatchString(t.raw) }},
	regexpForm("clang-format", "clang-format", nil, regexp.MustCompile(`^clang-format\s+(on|off)\b`)),
	regexpForm("cppcheck", "cppcheck-suppress", nil, regexp.MustCompile(`^cppcheck-suppress\b`)),
	regexpForm("iwyu", "IWYU pragma", nil, regexp.MustCompile(`^IWYU\s+pragma:`)),
	regexpForm("coverage", "LCOV_EXCL", nil, regexp.MustCompile(`^(LCOV|GCOVR)_EXCL_(LINE|START|STOP|BR_LINE|BR_START|BR_STOP)\b`)),
	{name: "nosonar", match: func(t commentText) (string, bool) { return "NOSONAR", nosonarMarker.MatchString(t.raw) }},
	regexpForm("mode-line", "mode line", nil, regexp.MustCompile(`^-\*-.*-\*-$`)),
	{name: "label", match: func(t commentText) (string, bool) {
		switch {
		case closingDirective.MatchString(t.before):
			return "label", true
		case (t.before == "}" || t.before == "};") && closingLabel.MatchString(t.first()):
			return "label", true
		}
		return "", false
	}},
}

var (
	// nolintMarker is clang-tidy's suppression, read anywhere in a comment.
	nolintMarker = regexp.MustCompile(`\bNOLINT(NEXTLINE|BEGIN|END)?\b`)
	// closingDirective is a directive that closes or turns a conditional.
	closingDirective = regexp.MustCompile(`^#\s*(endif|else)\b`)
	// closingLabel names the scope a closing brace ends.
	closingLabel = regexp.MustCompile(`^(end\s+(of\s+)?)?((anonymous|unnamed)\s+)?(namespace\b|extern\s+"C")`)
)

// cContainer reports the nodes whose direct children are declarations: a file,
// a namespace, a linkage specification, a type's body, a template, and the
// branches of a conditional directive, which add nothing to a subject.
func cContainer(kind, parent string) bool {
	switch kind {
	case "translation_unit", "linkage_specification", "namespace_definition", "struct_specifier", "union_specifier",
		"enum_specifier", "class_specifier", "type_definition", "template_declaration",
		"preproc_if", "preproc_ifdef", "preproc_else", "preproc_elif", "preproc_elifdef":
		return true
	case "declaration_list":
		return parent == "linkage_specification" || parent == "namespace_definition"
	case "field_declaration_list":
		return parent == "struct_specifier" || parent == "union_specifier" || parent == "class_specifier"
	case "enumerator_list":
		return parent == "enum_specifier"
	}
	return false
}

// cDecl names a declaration: `func/parse` for a function, `var/count` for a
// variable, `type/point_t` for a typedef or alias, `struct/point`,
// `class/Parser`, `enum/kind` or `union/value` for a type with a body,
// `macro/LIMIT` for a macro and `namespace/kapi` for a namespace, and a member's
// own name inside a type.
func cDecl(n *ts.Node, parent string, src []byte) (string, bool) {
	member := parent == "field_declaration_list" || parent == "enumerator_list"
	switch n.Kind() {
	case "function_definition", "declaration", "field_declaration":
		name, function := declaratorName(n.ChildByFieldName("declarator"), src)
		if name == "" {
			// A declaration of a type alone, such as `struct point { int x; };`.
			if typ := n.ChildByFieldName("type"); typ != nil && n.Kind() != "function_definition" {
				return cDecl(typ, parent, src)
			}
			return "", false
		}
		switch {
		case member:
			return name, true
		case function:
			return "func/" + name, true
		}
		return "var/" + name, true
	case "type_definition":
		if name, _ := declaratorName(n.ChildByFieldName("declarator"), src); name != "" {
			return "type/" + name, true
		}
	case "alias_declaration", "concept_definition":
		if name := n.ChildByFieldName("name"); name != nil {
			return "type/" + nodeName(name, src), true
		}
	case "struct_specifier", "union_specifier", "enum_specifier", "class_specifier":
		name := n.ChildByFieldName("name")
		if parent == "type_definition" || name == nil || n.ChildByFieldName("body") == nil {
			return "", false
		}
		return strings.TrimSuffix(n.Kind(), "_specifier") + "/" + nodeName(name, src), true
	case "enumerator":
		if name := n.ChildByFieldName("name"); name != nil {
			return nodeName(name, src), true
		}
	case "preproc_def", "preproc_function_def":
		if name := n.ChildByFieldName("name"); name != nil {
			return "macro/" + nodeName(name, src), true
		}
	case "namespace_definition":
		if name := n.ChildByFieldName("name"); name != nil {
			return "namespace/" + nodeName(name, src), true
		}
		return "namespace", true
	case "template_declaration":
		for i := uint(0); i < n.NamedChildCount(); i++ {
			if name, ok := cDecl(n.NamedChild(i), parent, src); ok {
				return name, true
			}
		}
	}
	return "", false
}

// declaratorName follows a declarator to the name it declares, and reports
// whether it declares a function.
func declaratorName(d *ts.Node, src []byte) (string, bool) {
	function := false
	for d != nil {
		switch d.Kind() {
		case "identifier", "field_identifier", "type_identifier", "qualified_identifier", "destructor_name", "operator_name":
			return nodeName(d, src), function
		case "function_declarator":
			function = true
		}
		next := d.ChildByFieldName("declarator")
		if next == nil && d.NamedChildCount() > 0 && (d.Kind() == "reference_declarator" || d.Kind() == "parenthesized_declarator") {
			next = d.NamedChild(0)
		}
		d = next
	}
	return "", function
}
