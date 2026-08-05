package openxml

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PresentationML has no w:pStyle. A slide's outline lives in the shape's
// placeholder type (<p:ph type="title"|"ctrTitle"|"subTitle"|…>), and until the
// reader read it, a slide title was indistinguishable from body text: a deck
// exported as a flat run of paragraphs with no slide boundaries and no hierarchy.

// pptxBlocksWithRoles reads a .pptx and returns (text, role, level) per block.
type roledBlock struct {
	text  string
	role  string
	level int
}

func pptxBlocks(t *testing.T, path string) []roledBlock {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := NewReader()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	var out []roledBlock
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		b, ok := pr.Part.Resource.(*model.Block)
		if !ok || b.SourceText() == "" {
			continue
		}
		out = append(out, roledBlock{b.SourceText(), b.SemanticRole(), b.HeadingLevel()})
	}
	return out
}

func TestSlideTitlePlaceholderBecomesHeading(t *testing.T) {
	blocks := pptxBlocks(t, "../../../apps/kapi-desktop/backend/sample/kapimart/marketing/en/onboarding-deck.pptx")
	require.NotEmpty(t, blocks)

	byText := make(map[string]roledBlock, len(blocks))
	for _, b := range blocks {
		byText[b.text] = b
	}

	// The three slide titles.
	for _, title := range []string{"Welcome to KapiMart", "Getting Started", "Next Steps"} {
		b, ok := byText[title]
		require.True(t, ok, "expected a block for %q", title)
		assert.Equal(t, model.RoleHeading, b.role, "%q is a slide title", title)
		assert.Equal(t, 1, b.level, "%q should be a level-1 heading", title)
	}

	// The subtitle sits one level below its title.
	sub, ok := byText["Partner Onboarding Guide"]
	require.True(t, ok)
	assert.Equal(t, model.RoleHeading, sub.role)
	assert.Equal(t, 2, sub.level)

	// Body text is untouched — this maps the outline, not everything on a slide.
	body, ok := byText["Complete your business profile and verify your identity"]
	require.True(t, ok)
	assert.NotEqual(t, model.RoleHeading, body.role,
		"body text must not be promoted to a heading")
}

// A placeholder type belongs to one shape. It must not leak into the next one,
// or every paragraph after a title becomes a heading too.
func TestPlaceholderTypeResetsAtShapeBoundary(t *testing.T) {
	blocks := pptxBlocks(t, "../../../apps/kapi-desktop/backend/sample/kapimart/marketing/en/onboarding-deck.pptx")

	headings := 0
	for _, b := range blocks {
		if b.role == model.RoleHeading {
			headings++
		}
	}
	// Three slide titles plus one subtitle — not every block on the deck.
	assert.Equal(t, 4, headings, "only placeholder titles should be headings")
	assert.Greater(t, len(blocks), headings, "the deck also has body text")
}
