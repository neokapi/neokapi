package golang

import (
	"strings"
)

// directiveForm is one kind of comment line a Go tool reads as an instruction.
// match receives the comment exactly as the parser reports it, markers
// included, and returns the name recorded on the exclusion.
type directiveForm struct {
	name  string
	match func(text string) (form string, ok bool)
}

// directiveForms is every form the provider sets aside. Each entry is tested on
// its own: the directive fixture holds at least one line that only that entry
// matches, so dropping any one of them lets prose through and fails the test.
//
// The first four are the rule go/ast applies when it strips directives from a
// comment's text, and the rule gofmt applies when it moves them to the end of a
// doc comment. Matching them exactly is what keeps this classification and the
// formatter's in agreement.
var directiveForms = []directiveForm{
	// A line directive resets the position the compiler reports: `//line
	// file.go:10`, or the delimited `/*line file.go:10:1*/` that may sit inside a
	// line of code.
	{name: "line", match: func(t string) (string, bool) {
		return "line", strings.HasPrefix(t, "//line ") || strings.HasPrefix(t, "/*line ")
	}},
	// gccgo's name for an external function.
	{name: "extern", match: func(t string) (string, bool) {
		return "extern", strings.HasPrefix(t, "//extern ")
	}},
	// cgo's export of a Go function to C.
	{name: "export", match: func(t string) (string, bool) {
		return "export", strings.HasPrefix(t, "//export ")
	}},
	// `//name:value`, with no space after the slashes: go:build, go:embed,
	// go:generate, go:linkname and the other compiler pragmas, and every linter
	// that adopted the convention (nolint:, lint:ignore, lint:file-ignore,
	// revive:, gosec:). The form recorded is the directive's first word.
	{name: "namespaced", match: func(t string) (string, bool) {
		body, ok := strings.CutPrefix(t, "//")
		if !ok || !isNamespacedDirective(body) {
			return "", false
		}
		if i := strings.IndexAny(body, " \t"); i >= 0 {
			body = body[:i]
		}
		return body, true
	}},
	// The build constraint syntax `//go:build` replaced. gofmt still pairs the
	// two, and a file may carry only the old one.
	{name: "+build", match: func(t string) (string, bool) {
		return "+build", strings.HasPrefix(t, "// +build")
	}},
	// golang.org/x/sys system call stubs, which mksyscall generates code from.
	// The stubs are written with a tab after the keyword, and mksyscall
	// normalises whitespace before matching, so either separator counts.
	{name: "sys", match: func(t string) (string, bool) {
		return "sys", strings.HasPrefix(t, "//sys ") || strings.HasPrefix(t, "//sys\t")
	}},
	{name: "sysnb", match: func(t string) (string, bool) {
		return "sysnb", strings.HasPrefix(t, "//sysnb ") || strings.HasPrefix(t, "//sysnb\t")
	}},
	// golangci-lint's suppression in the forms the namespaced rule misses: bare
	// `//nolint`, and `// nolint` with a space, which golangci-lint still
	// honours.
	{name: "nolint", match: func(t string) (string, bool) {
		body, ok := strings.CutPrefix(t, "//")
		if !ok {
			return "", false
		}
		rest, ok := strings.CutPrefix(strings.TrimLeft(body, " \t"), "nolint")
		if !ok || (rest != "" && rest[0] != ':' && rest[0] != ' ' && rest[0] != '\t') {
			return "", false
		}
		return "nolint", true
	}},
	// gosec reads `#nosec` anywhere in a comment, so a line carrying it is a
	// suppression wherever the marker sits.
	{name: "nosec", match: func(t string) (string, bool) {
		return "nosec", strings.Contains(t, "#nosec")
	}},
}

// isNamespacedDirective reports whether a `//` comment body (the slashes
// removed) has the shape `[a-z0-9]+:[a-z0-9]`. It is go/ast's own test.
func isNamespacedDirective(c string) bool {
	colon := strings.Index(c, ":")
	if colon <= 0 || colon+1 >= len(c) {
		return false
	}
	for i := 0; i <= colon+1; i++ {
		if i == colon {
			continue
		}
		b := c[i]
		if ('a' > b || b > 'z') && ('0' > b || b > '9') {
			return false
		}
	}
	return true
}

// classifyDirective returns the form of the first directive in forms that
// matches text.
func classifyDirective(text string, forms []directiveForm) (string, bool) {
	for _, f := range forms {
		if form, ok := f.match(text); ok {
			return form, true
		}
	}
	return "", false
}
