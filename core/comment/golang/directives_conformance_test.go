package golang

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
)

// declaredFixture holds declared markers where a Go comment line can open with
// one: inside a doc comment beside a blank line, as a whole comment, after code,
// and as a delimited comment on its own line. A marker inside a delimited
// comment over several lines, and prose that only mentions a marker, stay prose.
const declaredFixture = `package demo

// Parse reads the input.
// okapi-skip: ParseTest#testEmpty
//
// It returns an error for empty input.
func Parse() {}

// okapi-unmapped: FormatTest#testAll
// okapi-skip: FormatTest#testNone
func Format() {
	x := 1 // okapi-skip: FormatTest#testTrailing
	/* okapi-unmapped: FormatTest#testDelimited */
	_ = x
}

/*
okapi-skip: inside a delimited comment over several lines
*/
func Render() {}

// See the okapi-skip: markers; OKAPI-SKIP: in capitals is prose too.
func Sort() {}
`

var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

func declaredGoSuite(p comment.Provider) commenttest.Suite {
	s := goSuite(p)
	s.Directives = declaredDirectives
	s.Fixtures = append(s.Fixtures,
		commenttest.Fixture{Name: "declared.go", Source: declaredFixture},
		commenttest.Fixture{Name: "declared-crlf.go", Source: strings.ReplaceAll(declaredFixture, "\n", "\r\n")},
	)
	return s
}

// The conformance suite with directives declared: every line that opens with
// one is set aside as a directive, and every other comment is accounted for as
// it is with nothing declared.
func TestGoConformanceWithDeclaredDirectives(t *testing.T) {
	commenttest.Run(t, declaredGoSuite(Provider{}))

	got, err := comment.Locate(Provider{}, "declared.go", []byte(declaredFixture), declaredDirectives)
	require.NoError(t, err)
	forms := 0
	for _, e := range got.Excluded {
		if e.Reason == comment.ReasonDirective {
			forms++
		}
	}
	assert.Equal(t, 5, forms, "the suite ran over the declared markers")
}

// unmarkedGo reads no comment line as whole, so no declared directive is found.
type unmarkedGo struct{ Provider }

func (unmarkedGo) LineText([]byte) (int, string, bool) { return 0, "", false }

func TestGoConformanceWithDeclaredDirectivesCatchesAProviderThatReadsNoLine(t *testing.T) {
	failures := commenttest.Verify(declaredGoSuite(unmarkedGo{}))
	require.NotEmpty(t, failures)
	assert.Contains(t, commenttest.Properties(failures), commenttest.PropDirective, "%v", commenttest.Err(failures))
	assert.Contains(t, commenttest.Properties(failures), commenttest.PropLineText, "%v", commenttest.Err(failures))
}
