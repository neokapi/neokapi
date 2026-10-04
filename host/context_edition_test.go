package host

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// The answer for the file of a translation the recipe keeps says what the
// file is and how to write it: an edition of its source, through apply_edits
// at the source with the edition named.
func TestContextAnswer_ATranslationFileSaysWhatItIs(t *testing.T) {
	item := project.ContentItem{Path: "docs/en/welcome.md", Target: "docs/{lang}/welcome.md"}
	_, recipe := changeProject(t, item, map[string]string{"docs/en/welcome.md": "# Welcome\n\nHello.\n"},
		func(p *project.KapiProject) { p.Defaults.TargetLanguages = []model.LocaleID{"nb"} })
	a := &App{}
	src, done := a.ContextSourcesAt(bindingsCmd(t, recipe), ContextPointRequest{Path: "docs/nb/welcome.md"})
	defer done()
	require.NotNil(t, src.EditionOf, "the file is the nb edition of its source")
	assert.Equal(t, ContextEditionOf{Source: "docs/en/welcome.md", Edition: "nb"}, *src.EditionOf)

	res, err := ResolveContextAt(context.Background(), src, ContextPointRequest{Path: "docs/nb/welcome.md"})
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, res.FormatText(&buf))
	assert.Contains(t, buf.String(), `This file is the nb edition of docs/en/welcome.md: write it with apply_edits (or `+"`kapi apply`"+`) at {"doc": "docs/en/welcome.md", "edition": "nb"}`)

	// The source document itself is no one's edition.
	srcDoc, done2 := a.ContextSourcesAt(bindingsCmd(t, recipe), ContextPointRequest{Path: "docs/en/welcome.md"})
	defer done2()
	assert.Nil(t, srcDoc.EditionOf)
}
