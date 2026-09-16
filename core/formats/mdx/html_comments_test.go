package mdx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
)

// htmlCommentFixture is the shape a Docusaurus tree gives a document the recipe
// binds to MDX: a `.md` page holding HTML comments, one of them a prose note a
// person wrote, two of them the markers a script writes around generated
// content, beside an expression comment.
const htmlCommentFixture = "---\n" +
	"title: Install\n" +
	"---\n" +
	"\n" +
	"# Install\n" +
	"\n" +
	"<!--\n" +
	"Plan facts come from bowrain/billing/plans.go (PlanLimits, MonthlyCredits).\n" +
	"When plans.go changes, update this table in the same change.\n" +
	"-->\n" +
	"\n" +
	"## Downloads\n" +
	"\n" +
	"The links below always point at the release named in them.\n" +
	"\n" +
	"<!-- BEGIN:downloads-bowrain-desktop -->\n" +
	"Direct downloads for **Bowrain Desktop 1.2.0-rc13**:\n" +
	"<!-- END:downloads-bowrain-desktop -->\n" +
	"\n" +
	"<!-->\n" +
	"\n" +
	"<!--->\n" +
	"\n" +
	"{/* The page head. */}\n"

// An HTML comment in a document read as MDX is located, as it is in a Markdown
// document. The MDX reader already reads such a file, so the comment layer
// reads it too rather than refusing the whole document.
func TestLocateMDXCommentsLocatesAnHTMLComment(t *testing.T) {
	got, err := LocateComments([]byte(htmlCommentFixture))
	require.NoError(t, err, "a document the MDX reader reads has locatable comments")

	var located []string
	for _, c := range got.Comments {
		located = append(located, model.RunsText(c.Runs))
	}
	require.Len(t, located, 2, "the prose note and the expression comment: %v", located)
	assert.Contains(t, located[0], "Plan facts come from bowrain/billing/plans.go")
	assert.Contains(t, located[1], "The page head.")

	assert.Equal(t, "comment/install", got.Comments[0].Subject, "the note is named for the section it sits in")
	assert.Equal(t, comment.StyleBlock, got.Comments[0].Style)
}

// A comment carries the length of its own closing marker: `-->`, or the
// shorter marker of the empty forms `<!-->` and `<!--->`. Reading a comment's
// text with the wrong length eats its last bytes or keeps the marker.
func TestLocateMDXCommentsReadsAnEmptyHTMLCommentWhole(t *testing.T) {
	got, err := LocateComments([]byte(htmlCommentFixture))
	require.NoError(t, err)

	var empties int
	for _, e := range got.Excluded {
		if e.Reason == comment.ReasonBlank {
			empties++
		}
	}
	assert.Equal(t, 2, empties, "`<!-->` and `<!--->` hold nothing: %+v", got.Excluded)

	for _, c := range got.Comments {
		assert.NotContains(t, model.RunsText(c.Runs), ">", "a closing marker is never part of what a comment holds")
		assert.NotContains(t, model.RunsText(c.Runs), "<!--", "an opening marker is never part of what a comment holds")
	}
}

// The markers a script writes around generated content are what a tool reads,
// never prose, so they are set aside as the `region` directive rather than
// checked.
func TestLocateMDXCommentsSetsAsideAGeneratedRegionMarker(t *testing.T) {
	got, err := LocateComments([]byte(htmlCommentFixture))
	require.NoError(t, err)

	forms := map[string]comment.Reason{}
	for _, e := range got.Excluded {
		forms[e.Form] = e.Reason
	}
	assert.Equal(t, comment.ReasonDirective, forms["region"], "BEGIN and END are directives: %+v", got.Excluded)

	var regions int
	for _, e := range got.Excluded {
		if e.Form == "region" {
			regions++
		}
	}
	assert.Equal(t, 2, regions, "both the BEGIN and the END marker")
	for _, c := range got.Comments {
		assert.NotContains(t, model.RunsText(c.Runs), "BEGIN:downloads", "a marker is never checked as prose")
		assert.NotContains(t, model.RunsText(c.Runs), "END:downloads", "a marker is never checked as prose")
	}
}

// What MDX genuinely cannot place is still refused, and the reason names the
// construct.
func TestLocateMDXCommentsStillRefusesWhatMDXCannotPlace(t *testing.T) {
	for name, tc := range map[string]struct{ src, names string }{
		"an expression comment inside JSX":   {"<Tabs>\n{/* note */}\n</Tabs>\n", "the MDX scan"},
		"an expression comment in a line":    {"Hello {/* note */} world.\n", "the MDX scan"},
		"two comments in one expression":     {"{/* a */ /* b */}\n", "more than one comment"},
		"text after a comment on its line":   {"{/* a */} and more\n", "more than one comment"},
		"an expression that is never closed": {"{/* never closed\n", "the MDX scan"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LocateComments([]byte(tc.src))
			require.ErrorIs(t, err, ErrCommentsUnlocated)
			require.ErrorIs(t, err, comment.ErrUnlocated, "the host treats any provider's unlocated file the same way")
			assert.Contains(t, err.Error(), tc.names, "the reason names what MDX cannot place")
		})
	}
}
