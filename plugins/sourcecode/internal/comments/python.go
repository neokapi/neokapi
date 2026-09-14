package comments

import (
	"bytes"
	"regexp"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// pythonLanguages is Python. Its comments run from `#` to the end of the line;
// a docstring is a string, and so is never a comment.
func pythonLanguages() []*Language {
	return []*Language{{
		Name:        "python",
		DisplayName: "Python",
		Extensions:  []string{".py"},
		Markers:     comment.Markers{Line: []string{"#"}},
		grammar:     tspython.Language,
		syntax:      pythonSyntax,
		once:        &sync.Once{},
		Canary: comment.Canary{
			Name:   "a Python comment with a doubled word, below a directive and above a string holding a comment marker",
			Source: []byte("#!/usr/bin/env python3\n# noqa: E501\n# Parses the the input.\ndef parse(text):\n    return \"# not a comment\" + text\n"),
			Block:  "func/parse",
		},
	}}
}

var pythonSyntax = &syntax{
	unit:       pythonUnit,
	directives: pythonDirectives,
	decl:       pythonDecl,
	container:  pythonContainer,
	wrappers:   map[string]bool{"decorated_definition": true},
}

// hashUnit classifies the comment nodes of a language whose comments run from
// `#` to the end of the line. A `#!` line that opens the file is its shebang.
func hashUnit(n *ts.Node, src []byte) (unit, bool) {
	if n.Kind() != "comment" {
		return unit{}, false
	}
	start, end := int(n.StartByte()), int(n.EndByte())
	if start == 0 && bytes.HasPrefix(src, []byte("#!")) {
		return unit{start: start, end: end, kind: kindShebang, open: 2}, true
	}
	return unit{start: start, end: end, kind: kindLine, open: 1}, true
}

// pythonUnit classifies a Python comment node. Python ends a line at a carriage
// return as well as a line feed, so a comment in a file with CRLF line endings
// stops before the carriage return the grammar includes.
func pythonUnit(n *ts.Node, src []byte) (unit, bool) {
	u, ok := hashUnit(n, src)
	for ok && u.end > u.start+u.open && src[u.end-1] == '\r' {
		u.end--
	}
	return u, ok
}

// codingDeclaration is PEP 263's pattern for the source encoding, read on the
// first two lines of a file.
var codingDeclaration = regexp.MustCompile(`^[ \t\f]*#.*?coding[:=][ \t]*[-_.a-zA-Z0-9]+`)

// pythonDirectives are the comments a Python tool reads.
var pythonDirectives = []directiveForm{
	{name: "shebang", match: func(t commentText) (string, bool) { return "shebang", t.kind == kindShebang }},
	{name: "coding", match: func(t commentText) (string, bool) {
		return "coding", t.line <= 2 && codingDeclaration.MatchString(t.raw)
	}},
	regexpForm("type", "type", nil, regexp.MustCompile(`^type:`)),
	// flake8 and coverage read their markers anywhere in the comment.
	{name: "noqa", match: func(t commentText) (string, bool) {
		return "noqa", noqaMarker.MatchString(t.raw)
	}},
	{name: "pragma", match: func(t commentText) (string, bool) {
		return "pragma", pragmaMarker.MatchString(t.raw)
	}},
	{name: "nosec", match: func(t commentText) (string, bool) {
		return "nosec", nosecMarker.MatchString(t.raw)
	}},
	regexpForm("pylint", "pylint", nil, regexp.MustCompile(`^pylint:`)),
	regexpForm("fmt", "fmt", nil, regexp.MustCompile(`^fmt:\s*(on|off|skip)\b`)),
	regexpForm("mypy", "mypy", nil, regexp.MustCompile(`^mypy:`)),
	regexpForm("pyright", "pyright", nil, regexp.MustCompile(`^pyright:`)),
	regexpForm("ruff", "ruff", nil, regexp.MustCompile(`^ruff:`)),
	regexpForm("isort", "isort", nil, regexp.MustCompile(`^isort:`)),
}

var (
	noqaMarker   = regexp.MustCompile(`(?i)#\s*noqa\b`)
	pragmaMarker = regexp.MustCompile(`#\s*pragma[:\s]`)
	nosecMarker  = regexp.MustCompile(`#\s*nosec\b`)
)

// pythonContainer reports the nodes whose direct children are structural: the
// module, a class and the body of a class.
func pythonContainer(kind, parent string) bool {
	switch kind {
	case "module", "class_definition", "decorated_definition":
		return true
	case "block":
		return parent == "class_definition"
	}
	return false
}

// pythonDecl names a declaration: `func/parse`, `class/Parser` and `var/LIMIT`
// at the top of a module, and a member's own name inside a class.
func pythonDecl(n *ts.Node, parent string, src []byte) (string, bool) {
	member := parent == "block"
	named := func(prefix string, name *ts.Node) (string, bool) {
		if member {
			return nodeName(name, src), true
		}
		return prefix + nodeName(name, src), true
	}
	switch n.Kind() {
	case "function_definition":
		return named("func/", n.ChildByFieldName("name"))
	case "class_definition":
		return named("class/", n.ChildByFieldName("name"))
	case "decorated_definition":
		if d := n.ChildByFieldName("definition"); d != nil {
			return pythonDecl(d, parent, src)
		}
	case "expression_statement":
		if a := n.NamedChild(0); a != nil && a.Kind() == "assignment" {
			if left := a.ChildByFieldName("left"); left != nil && left.Kind() == "identifier" {
				return named("var/", left)
			}
		}
	}
	return "", false
}
