package markdown_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fmDoc = `---
sidebar_position: 3
title: Kapi
description: "Reads, translates, and ships content: faithfully."
keywords: [kapi, overview]
---

Body text.
`

// fmRoundtrip reads fmDoc with translateFrontMatter+frontMatterKeys, lets
// translate mutate the blocks, and writes back through the skeleton.
func fmRoundtrip(t *testing.T, translate func([]*model.Part)) (string, []*model.Part) {
	t.Helper()
	ctx := t.Context()

	reader := markdown.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"translateFrontMatter": true,
		"frontMatterKeys":      []any{"title", "description"},
	}))
	writer := markdown.NewWriter()
	writer.SetLocale("nb")

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(fmDoc, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	if translate != nil {
		translate(parts)
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	writer.Close()
	return buf.String(), parts
}

func frontMatterBlocks(parts []*model.Part) []*model.Block {
	var fm []*model.Block
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Type == "front-matter" {
			fm = append(fm, b)
		}
	}
	return fm
}

// TestFrontMatterKeys_FiltersExtraction: only the allowlisted keys become
// blocks; numbers, slugs, and tag lists stay skeleton — and untranslated
// content round-trips byte-identically, including the originally quoted
// value.
func TestFrontMatterKeys_FiltersExtraction(t *testing.T) {
	out, parts := fmRoundtrip(t, nil)
	assert.Equal(t, fmDoc, out, "untranslated round-trip must be byte-identical")

	fm := frontMatterBlocks(parts)
	require.Len(t, fm, 2, "only title and description extract")
	assert.Equal(t, "Kapi", fm[0].SourceText())
	assert.Equal(t, "Reads, translates, and ships content: faithfully.", fm[1].SourceText())
	assert.Equal(t, `"`, fm[1].Properties[markdown.BlockPropFrontMatterQuote])
}

// TestFrontMatter_TranslationQuoting: a plain scalar whose translation
// introduces ": " gains quoting; an originally quoted value stays quoted.
func TestFrontMatter_TranslationQuoting(t *testing.T) {
	out, _ := fmRoundtrip(t, func(parts []*model.Part) {
		for _, b := range frontMatterBlocks(parts) {
			switch b.Name {
			case "fm_title":
				b.SetTargetText("nb", "Kapi: oversikt")
			case "fm_description":
				b.SetTargetText("nb", "Leser, oversetter og leverer innhold: formattro.")
			}
		}
	})
	assert.Contains(t, out, "title: \"Kapi: oversikt\"\n", "plain scalar that gained ': ' must be quoted")
	assert.Contains(t, out, "description: \"Leser, oversetter og leverer innhold: formattro.\"\n", "originally quoted value stays quoted")
	assert.Contains(t, out, "sidebar_position: 3\n", "non-allowlisted keys untouched")
	assert.Contains(t, out, "keywords: [kapi, overview]\n", "list values untouched")
}
