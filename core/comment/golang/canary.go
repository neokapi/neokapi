package golang

import "github.com/neokapi/neokapi/core/comment"

// Canary implements comment.Provider. The doubled word sits in a doc comment
// that shares its group with a directive, so a provider that drops the prose of
// such a group misses it.
func (Provider) Canary() comment.Canary {
	return comment.Canary{
		Name:   "a Go doc comment with a doubled word, beside a directive",
		Source: []byte("package canary\n\n// Parse reads the the input.\n//\n//go:noinline\nfunc Parse() {}\n"),
		Block:  "func/Parse",
	}
}

// RewriteCanary implements comment.Rewriter. It rewrites the doc comment of
// Canary, which shares its group with a directive, and its refused text would
// make the comment's second line a linter suppression.
func (p Provider) RewriteCanary() comment.RewriteCanary {
	c := p.Canary()
	return comment.RewriteCanary{
		Name:    "canary.go",
		Source:  c.Source,
		Block:   c.Block,
		Refused: "Parse reads the input.\nnolint",
	}
}

// FormatterCanary implements comment.Formatter: a comment indented with
// spaces, which gofmt reindents with a tab.
func (Provider) FormatterCanary() []byte {
	return []byte("package canary\n\nfunc F() {\n  // Indented with spaces.\n\t_ = 1\n}\n")
}
