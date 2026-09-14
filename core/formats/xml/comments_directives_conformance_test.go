package xml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
)

// declaredFixture holds declared markers where an XML comment can open with
// one: as a whole comment, and in a comment after an element on its line. A
// marker inside a comment over several lines, and prose that only mentions a
// marker, stay prose.
const declaredFixture = `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <!-- okapi-skip: StringsTest#testEmpty -->
    <string name="greeting">Hello</string> <!-- okapi-unmapped: StringsTest#testTrailing -->
    <!--
        okapi-skip: inside a comment over several lines
    -->
    <string name="multi">Multi</string>
    <!-- See the okapi-skip: markers; OKAPI-SKIP: in capitals is prose too. -->
    <string name="sort">Sort</string>
</resources>
`

var declaredDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

func declaredXMLSuite(p comment.Provider) commenttest.Suite {
	return commenttest.Suite{
		Provider:   p,
		Scan:       xmlUnits,
		Directives: declaredDirectives,
		Fixtures: []commenttest.Fixture{
			{Name: "strings.xml", Source: stringsFixture, Literals: stringsLiterals},
			{Name: "declared.xml", Source: declaredFixture},
			{Name: "declared-crlf.xml", Source: strings.ReplaceAll(declaredFixture, "\n", "\r\n")},
		},
	}
}

// The conformance suite with directives declared: every comment that opens
// with one is set aside as a directive, and every other comment is accounted
// for as it is with nothing declared.
func TestXMLConformanceWithDeclaredDirectives(t *testing.T) {
	commenttest.Run(t, declaredXMLSuite(CommentProvider{}))

	got, err := comment.Locate(CommentProvider{}, "declared.xml", []byte(declaredFixture), declaredDirectives)
	require.NoError(t, err)
	forms := 0
	for _, e := range got.Excluded {
		if e.Reason == comment.ReasonDirective {
			forms++
		}
	}
	assert.Equal(t, 2, forms, "the suite ran over the declared markers")
	assert.Len(t, got.Comments, 2, "the comment over several lines and the one that mentions a marker stay prose")
}

// unmarkedXML reads no comment line as whole, so no declared directive is
// found.
type unmarkedXML struct{ CommentProvider }

func (unmarkedXML) LineText([]byte) (int, string, bool) { return 0, "", false }

func TestXMLConformanceWithDeclaredDirectivesCatchesAProviderThatReadsNoLine(t *testing.T) {
	failures := commenttest.Verify(declaredXMLSuite(unmarkedXML{}))
	require.NotEmpty(t, failures)
	assert.Contains(t, commenttest.Properties(failures), commenttest.PropDirective, "%v", commenttest.Err(failures))
	assert.Contains(t, commenttest.Properties(failures), commenttest.PropLineText, "%v", commenttest.Err(failures))
}
