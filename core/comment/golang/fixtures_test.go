package golang

// The fixtures are Go source held in strings, so the formatter guard and the
// repository scan never mistake them for code, and a fixture can hold what a
// real file cannot (a stray directive, a CRLF block comment).

// spanFixture exercises every place a comment can sit.
const spanFixture = `// Package demo is a fixture.
//
// It has two paragraphs.
package demo

import (
	// fmt formats.
	"fmt"
)

// Answer is the answer.
const Answer = 42 // the trailing comment

// Grouped constants share a doc comment.
const (
	// A is the first.
	A = 1
	B = 2 // B is the second.
)

/*
Block is documented in a block comment.
*/
type Block struct {
	// ID identifies the block.
	ID string
	Nested struct {
		// Inner sits one level down.
		Inner int
	}
}

// Reader reads.
type Reader interface {
	// Read reads.
	Read() error
}

// Parse parses.
func Parse() {
	// A comment inside the body.
	fmt.Println("x") /* an inline block */
}

// Len reports the length.
func (b *Block) Len() int { return len(b.ID) }

// Get is generic.
func (s *Set[T]) Get() T { var zero T; return zero }

// A comment attached to nothing.

var x = 1
`

// directiveFixture holds nothing but directives. Each directive form in
// directiveForms has at least one line here that no other form matches, which
// is what lets the rung prove that dropping a form lets prose through.
const directiveFixture = `// +build linux

//go:build linux

package demo

import "embed"

//go:generate stringer -type=Kind
//go:embed data
var data embed.FS

//go:linkname runtimeNano runtime.nanotime
//go:noinline
//go:nosplit
func runtimeNano() int64

//export Hello
func Hello() {}

//extern puts
func puts()

//line generated.go:10
func afterLine() {}

var _ = /*line generated.go:20:1*/ 0

//sys	read(fd int, p []byte) (n int, err error)
//sysnb	getpid() (pid int)

//nolint
func a() {}

// nolint:errcheck
func b() {}

//nolint:errcheck,gosec // reason
func c() {}

//lint:ignore SA1019 reason
//lint:file-ignore U1000 reason

//revive:disable-next-line:exported
func d() {}

// #nosec G101
const secret = "x"

//gosec:disable G101
const other = "y"
`

// literalFixture holds comment markers in a string and a raw string, neither of
// which is a comment.
const literalFixture = "package demo\n\n// Greeting is prose.\nvar Greeting = \"// not a comment\"\n\nvar raw = `/* not a comment */` // beside a raw string\n"

// mixedFixture puts prose and directives in the same comment groups.
const mixedFixture = `package demo

import "embed"

// data holds the fixtures. The prose above a pragma is still prose.
//
//go:embed data
var data embed.FS

// before the directive
//
//nolint:gocritic // reason
//
// after the directive
func f() {}
`
