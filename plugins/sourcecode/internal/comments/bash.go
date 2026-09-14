package comments

import (
	"regexp"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsbash "github.com/tree-sitter/tree-sitter-bash/bindings/go"

	"github.com/neokapi/neokapi/core/comment"
)

// bashLanguages is Bash, and the POSIX shell scripts it reads. A `#` inside a
// string, a heredoc or a parameter expansion such as `${name#prefix}` is
// content.
func bashLanguages() []*Language {
	return []*Language{{
		Name:        "bash",
		DisplayName: "Bash",
		Extensions:  []string{".sh", ".bash"},
		Markers:     comment.Markers{Line: []string{"#"}},
		grammar:     tsbash.Language,
		syntax:      bashSyntax,
		once:        &sync.Once{},
		Canary: comment.Canary{
			Name:   "a shell comment with a doubled word, below a ShellCheck directive and above a string holding a comment marker",
			Source: []byte("#!/usr/bin/env bash\n# shellcheck disable=SC2086\n# Builds the the site.\nbuild() {\n  echo \"# not a comment\"\n}\n"),
			Block:  "func/build",
		},
	}}
}

var bashSyntax = &syntax{
	unit:       hashUnit,
	directives: bashDirectives,
	decl:       bashDecl,
	container:  func(kind, _ string) bool { return kind == "program" },
}

// bashDirectives are the comments a shell tool reads.
var bashDirectives = []directiveForm{
	{name: "shebang", match: func(t commentText) (string, bool) { return "shebang", t.kind == kindShebang }},
	regexpForm("shellcheck", "shellcheck", nil, regexp.MustCompile(`^shellcheck\s`)),
}

// bashDecl names a declaration at the top of a script: `func/build` for a
// function and `var/NAME` for an assignment.
func bashDecl(n *ts.Node, _ string, src []byte) (string, bool) {
	switch n.Kind() {
	case "function_definition":
		return "func/" + nodeName(n.ChildByFieldName("name"), src), true
	case "variable_assignment":
		return "var/" + nodeName(n.ChildByFieldName("name"), src), true
	case "declaration_command":
		for i := uint(0); i < n.NamedChildCount(); i++ {
			if c := n.NamedChild(i); c.Kind() == "variable_assignment" {
				return bashDecl(c, "", src)
			}
		}
	}
	return "", false
}
