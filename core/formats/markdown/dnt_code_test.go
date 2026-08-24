package markdown

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/translatability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A code span holds a command, a path or an identifier. Translating it is how
// `kapi check --ship` reached the docs site as `ķàþî çĥéçķ --šĥîþ`: the
// backticks survived and the command did not.
//
// The rule is not local to this reader. core/translatability.Classify says No
// for <code>, from the W3C table the JSX transform uses, and the backtick is
// that element's markdown spelling.

func blocksOf(t *testing.T, src string) []*model.Block {
	t.Helper()
	r := NewReader()
	doc := &model.RawDocument{
		URI:          "case.md",
		Reader:       io.NopCloser(bytes.NewReader([]byte(src))),
		SourceLocale: model.LocaleID("en"),
	}
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, doc))

	var blocks []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

// runText returns the text of every run, split by whether the translator is
// meant to touch it.
func runText(runs []model.Run) (translatable, protected string) {
	var t, p strings.Builder
	for _, r := range runs {
		if r.Text == nil {
			continue
		}
		if r.Text.NoTranslate {
			p.WriteString(r.Text.Text)
			continue
		}
		t.WriteString(r.Text.Text)
	}
	return t.String(), p.String()
}

func TestCodeSpanContentIsNotTranslatable(t *testing.T) {
	blocks := blocksOf(t, "Run `kapi check --ship` in CI.\n")
	require.Len(t, blocks, 1)

	translatable, protected := runText(blocks[0].Source)
	assert.Equal(t, "kapi check --ship", protected,
		"the command belongs to the tool, not the translator")
	assert.Contains(t, translatable, "Run ")
	assert.Contains(t, translatable, " in CI.")
	assert.NotContains(t, translatable, "kapi check")
}

// The text stays in the block, so content memory, search and the term checks
// still see it. That is the whole reason this is a flag on the run rather than
// a placeholder: a placeholder contributes nothing to SourceText.
func TestProtectedCodeStaysVisibleAsText(t *testing.T) {
	blocks := blocksOf(t, "Run `kapi check --ship` in CI.\n")
	require.Len(t, blocks, 1)

	assert.Equal(t, "Run kapi check --ship in CI.", blocks[0].SourceText(),
		"the command is still part of the block's text")
}

func TestEmphasisStaysTranslatable(t *testing.T) {
	blocks := blocksOf(t, "Run **the check** and _the gate_ now.\n")
	require.Len(t, blocks, 1)

	translatable, protected := runText(blocks[0].Source)
	assert.Empty(t, protected, "bold and italic are prose, whatever the markers")
	assert.Contains(t, translatable, "the check")
	assert.Contains(t, translatable, "the gate")
}

// The scope is the shared table's, not a list this package invented.
func TestScopeMatchesTheSharedTable(t *testing.T) {
	assert.Equal(t, translatability.No, translatability.Classify("code"),
		"the backtick span is the markdown spelling of <code>")
	for _, e := range []string{"kbd", "samp", "var"} {
		assert.Equal(t, translatability.No, translatability.Classify(e))
	}
	for _, e := range []string{"b", "strong", "em", "i"} {
		assert.NotEqual(t, translatability.No, translatability.Classify(e),
			"%s is emphasis, and emphasis is prose", e)
	}
}
