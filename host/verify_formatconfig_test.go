package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The source document: three front-matter keys the recipe can declare as
// content, plus a heading and a paragraph the reader always emits.
const formatConfigDoc = `---
title: Tide tables
description: How to read them
sidebar_label: Tides
draft: false
---

# Reading the tide

A paragraph under the heading.
`

// TestUnitsFromProject_ReaderConfigReachesCoveragePath pins that a collection's
// reader config governs what the coverage/status path counts, so a measurement
// is taken over the units a convergence run produces rather than over the units
// a default reader would.
//
// Front matter is the sharp case: with `translateFrontMatter` the markdown
// reader emits the declared keys as units and without it they are configuration,
// so the two readings of one file differ by exactly the declared keys. When the
// config reached the flow and not the scope, `kapi up` reported "60/48 units" —
// a fraction over two denominators.
func TestUnitsFromProject_ReaderConfigReachesCoveragePath(t *testing.T) {
	frontMatter := map[string]any{
		"translateFrontMatter": true,
		"frontMatterKeys":      []any{"title", "description", "sidebar_label"},
	}

	tests := []struct {
		name       string
		defaults   map[string]any
		item       map[string]any
		formatFlag string
		wantBlocks int
	}{
		{
			name:       "no config reads the reader's defaults",
			wantBlocks: 2,
		},
		{
			name:       "item format.config reaches the read",
			item:       frontMatter,
			wantBlocks: 5,
		},
		{
			name:       "defaults.formats config reaches the read",
			defaults:   frontMatter,
			wantBlocks: 5,
		},
		{
			name:     "the item overrides defaults per key",
			defaults: frontMatter,
			item: map[string]any{
				"frontMatterKeys": []any{"title"},
			},
			wantBlocks: 3,
		},
		{
			name:       "the --format override displaces the binding",
			item:       frontMatter,
			formatFlag: "markdown",
			wantBlocks: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "tides.md"), []byte(formatConfigDoc), 0o644))

			item := project.ContentItem{
				Path:   "docs/*.md",
				Target: "i18n/{lang}/{path}.md",
			}
			if tc.item != nil {
				item.Format = &project.FormatSpec{Name: "markdown", Config: tc.item}
			}
			proj := &project.KapiProject{
				Version: project.CurrentVersion,
				Defaults: project.Defaults{
					SourceLanguage:  "en",
					TargetLanguages: []model.LocaleID{"nb"},
				},
				Collections: []project.Collection{{Name: "docs", Content: []project.ContentItem{item}}},
			}
			if tc.defaults != nil {
				proj.Defaults.Formats = map[string]project.FormatDefaults{"markdown": {Config: tc.defaults}}
			}

			app := &App{}
			app.InitRegistries()
			app.SourceLang = "en"
			app.FormatFlag = tc.formatFlag

			units, err := app.UnitsFromProject(proj, root, "")
			require.NoError(t, err)
			require.Len(t, units, 1)

			blocks, err := app.readSource(context.Background(), units[0])
			require.NoError(t, err)
			assert.Len(t, blocks, tc.wantBlocks)
		})
	}
}

// TestUnitFormatBinding_TargetInAnotherFormatIsDetected pins that the source's
// reader binding travels to the target only while the target is in the source's
// format. A target delivered in another format — a compiled catalog, which is
// write-only through the pipeline — must reach bilingualBlocks' file-presence
// fallback rather than be handed to a reader that cannot make sense of it.
func TestUnitFormatBinding_TargetInAnotherFormatIsDetected(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantFormat string
	}{
		{name: "same extension carries the binding", target: "i18n/nb/tides.md", wantFormat: "markdown"},
		{name: "another extension is detected", target: "i18n/nb/tides.mo", wantFormat: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proj := &project.KapiProject{
				Defaults: project.Defaults{
					Formats: map[string]project.FormatDefaults{
						"markdown": {Config: map[string]any{"translateFrontMatter": true}},
					},
				},
			}
			rf := project.ResolvedFile{Path: "/p/docs/tides.md", Format: "markdown", Item: &project.ContentItem{Path: "docs/*.md"}}

			srcFormat, srcCfg, tgtFormat, tgtCfg := unitFormatBinding(proj, rf, tc.target)
			assert.Equal(t, "markdown", srcFormat)
			assert.Equal(t, map[string]any{"translateFrontMatter": true}, srcCfg)
			assert.Equal(t, tc.wantFormat, tgtFormat)
			if tc.wantFormat == "" {
				assert.Nil(t, tgtCfg)
			} else {
				assert.Equal(t, srcCfg, tgtCfg)
			}
		})
	}
}
