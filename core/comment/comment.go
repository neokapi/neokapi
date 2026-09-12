// Package comment is the comment layer: the prose a file carries in its
// comments, located precisely enough that the checks which read any other
// content can read it too.
//
// A comment reaches the layer from one of two providers. A format whose reader
// already parses a file hands over the comments it met on the way. A file that
// no reader covers goes through a language provider, which needs the language's
// grammar and nothing else. Everything after extraction is shared: a comment is
// a Comment whichever provider located it, and it becomes a block the same way.
//
// The layer is not a format. Nothing here emits parts into a reader's stream,
// so a file's round-trip and its parity are exactly what they were, and the
// only way to reach a comment is through what a provider located.
package comment

import (
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Style is the comment syntax a comment was written in.
type Style string

const (
	// StyleLine is a comment that runs to the end of its line: `//` in Go, `#`
	// in YAML. Consecutive line comments form one comment.
	StyleLine Style = "line"
	// StyleBlock is a delimited comment, `/* … */` in Go.
	StyleBlock Style = "block"
)

// Comment is one addressable comment: prose a check may read.
//
// Its span covers whole comment lines and never contains a directive, a
// generated file's comments, or anything else a toolchain reads, so whatever
// addresses a Comment addresses prose and nothing else.
type Comment struct {
	// Start and End are the half-open byte span [Start, End) of the comment in
	// the file, from its first marker to the last byte of its last line.
	Start int `json:"start"`
	End   int `json:"end"`
	// Lines is the range of lines the span covers.
	Lines format.LineRange `json:"lines"`
	Style Style            `json:"style"`

	// Subject is the structural path of what the comment sits on, such as
	// "func/Parse", "type/Block/ID" or "package". A comment inside a
	// declaration's body carries the declaration's path with "comment"
	// appended; one attached to nothing is "comment".
	Subject string `json:"subject"`

	// Doc reports that the comment documents a declaration, as opposed to
	// annotating the code around it.
	Doc bool `json:"doc,omitempty"`

	// Deprecated reports a paragraph opening with "Deprecated: ". It is prose,
	// but tools read the marker (a linter warns on every use of a deprecated
	// identifier), so anything that rewrites the comment has to keep it.
	Deprecated bool `json:"deprecated,omitempty"`

	// Runs is the comment's content with the comment markers removed. Prose is
	// text; code blocks, links and references to declarations are placeholders,
	// so no check reads them as sentences.
	Runs []model.Run `json:"runs"`
}

// Reason says why a located comment is not addressable.
type Reason string

const (
	// ReasonDirective is a line a tool reads as an instruction: a build
	// constraint, a compiler pragma, a linter suppression. Editing one changes
	// what the toolchain does.
	ReasonDirective Reason = "directive"
	// ReasonGenerated is a comment in a file its generator owns. The next
	// generation overwrites any edit to it.
	ReasonGenerated Reason = "generated"
	// ReasonCgoPreamble is the comment immediately above `import "C"`, which
	// cgo compiles as C.
	ReasonCgoPreamble Reason = "cgo-preamble"
	// ReasonExampleOutput is the output comment that closes a Go example
	// function, which `go test` compares against what the example prints.
	ReasonExampleOutput Reason = "example-output"
	// ReasonBlank is a comment line with nothing on it at the edge of a
	// comment, which carries no prose.
	ReasonBlank Reason = "blank"
)

// Excluded is one comment line a provider located and set aside.
type Excluded struct {
	Start  int              `json:"start"`
	End    int              `json:"end"`
	Lines  format.LineRange `json:"lines"`
	Reason Reason           `json:"reason"`
	// Form names the directive for ReasonDirective, such as "go:embed",
	// "nolint" or "+build". Empty for every other reason.
	Form string `json:"form,omitempty"`
}

// File is everything a provider located in one file: the comments that are
// addressable and the lines it set aside. Between them they account for every
// comment the file holds, each exactly once.
type File struct {
	// Language names the provider's language, such as "go".
	Language string     `json:"language"`
	Comments []Comment  `json:"comments"`
	Excluded []Excluded `json:"excluded,omitempty"`
}

// Provider locates the comments in a file of one language. It reads the bytes
// and writes nothing.
type Provider interface {
	// Language names the language, such as "go".
	Language() string
	// Extensions lists the file extensions the provider reads, with the dot.
	Extensions() []string
	// Locate returns the comments in src. name is the file's path; a provider
	// may use it where the language makes a rule depend on the file name, as Go
	// does for example functions in test files.
	Locate(name string, src []byte) (*File, error)
}
