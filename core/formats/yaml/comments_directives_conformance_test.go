package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/comment/commenttest"
)

// declaredYAMLFixture holds declared markers inside a group of comment lines,
// beside a value and as a whole comment, and prose that only mentions one.
const declaredYAMLFixture = "# Greets the reader.\n# okapi-skip: GreetingTest#testAll\n# Shown on the landing page.\ngreeting: Hello # okapi-unmapped: GreetingTest#testTrailing\n# okapi-skip: FarewellTest#testAll\nfarewell: Goodbye\n# See the okapi-skip: markers.\nother: value\n"

var declaredYAMLDirectives = comment.Directives{"okapi-skip:", "okapi-unmapped:"}

func declaredYAMLSuite(p comment.Provider) commenttest.Suite {
	s := yamlSuite(p)
	s.Directives = declaredYAMLDirectives
	s.Fixtures = append(s.Fixtures,
		commenttest.Fixture{Name: "declared.yaml", Source: declaredYAMLFixture},
		commenttest.Fixture{Name: "declared-crlf.yaml", Source: strings.ReplaceAll(declaredYAMLFixture, "\n", "\r\n")},
	)
	return s
}

func TestCommentProviderConformanceWithDeclaredDirectives(t *testing.T) {
	commenttest.Run(t, declaredYAMLSuite(CommentProvider{}))

	got, err := comment.Locate(CommentProvider{}, "declared.yaml", []byte(declaredYAMLFixture), declaredYAMLDirectives)
	require.NoError(t, err)
	forms := 0
	for _, e := range got.Excluded {
		if e.Reason == comment.ReasonDirective {
			forms++
		}
	}
	assert.Equal(t, 3, forms, "the suite ran over the declared markers")
}

// unmarkedYAML reads no comment line as whole, so no declared directive is
// found.
type unmarkedYAML struct{ CommentProvider }

func (unmarkedYAML) LineText([]byte) (int, string, bool) { return 0, "", false }

func TestCommentProviderConformanceWithDeclaredDirectivesCatchesAProviderThatReadsNoLine(t *testing.T) {
	failures := commenttest.Verify(declaredYAMLSuite(unmarkedYAML{}))
	require.NotEmpty(t, failures)
	assert.Contains(t, commenttest.Properties(failures), commenttest.PropDirective, "%v", commenttest.Err(failures))
	assert.Contains(t, commenttest.Properties(failures), commenttest.PropLineText, "%v", commenttest.Err(failures))
}
