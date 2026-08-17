package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
)

// #1937 / #2032 / #2033. A recipe says two things about a content item's
// format: which reader it is read with, and — through format.config — which of
// its leaves are content at all. The convergence path consulted neither: it
// detected the format from the file extension and configured the reader from
// `defaults.formats` alone, so an item's `extractionRules` / `keyPathPatterns`
// never restricted anything and the writer's serialization options never took
// effect.
//
// The damage is silent and it is corruption, not absence: a CMS-shaped document
// has its `_id`, `_type` and `slug` translated; a narration script has its scene
// ids and kinds translated; and where the run reads a document with a different
// reader than the store was filled with, the file-local block ids that address
// the store name different content on the two sides of one convergence, so a
// frontmatter title's translation lands on the first body paragraph.
//
// These tests converge a real project and read the file the run delivered.

// recipeFormatProject writes a project over the given sources and collections,
// with `recycle` as its only step so the run is a deterministic function of the
// seeded content memory — no provider, no network, no spend.
func recipeFormatProject(t *testing.T, sources map[string]string, colls []project.Collection, formats map[string]project.FormatDefaults) (*App, *EnvCommand, string, string) {
	t.Helper()
	dir := t.TempDir()
	// Dogfood isolation contract (CLAUDE.md).
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	for name, body := range sources {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "RecipeFormatTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Flow:            "recycle-only",
			SourceGate:      string(model.SourceGateNone),
			Materialize:     project.MaterializeOnConverge,
			Formats:         formats,
		},
		Collections: colls,
		Flows: map[string]*flow.StepsSpec{
			"recycle-only": {Steps: []flow.FlowStep{
				{Tool: "recycle", Config: map[string]any{"fillTarget": true, "fillTargetThreshold": 100}},
			}},
		},
		ShipGate: gate.Gate{"translated": {Pct: 100}},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))
	t.Chdir(dir)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return a, cmd, recipe, dir
}

// seedMemory puts source → nb pairs in the project content memory, which is all
// the leverage a `recycle`-only flow has.
func seedMemory(t *testing.T, a *App, recipe string, pairs map[string]string) {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	store := db.Memory()
	require.NotNil(t, store)
	for source, target := range pairs {
		require.NoError(t, store.Add(context.Background(), memory.Entry{
			ID: source,
			Variants: map[model.LocaleID][]model.Run{
				"en": {model.TextR(source)},
				"nb": {model.TextR(target)},
			},
			HintSrcLang: "en",
		}))
	}
}

// TestConverge_ItemExtractionRulesRestrictWhatIsWritten is #1937 on the JSON
// path: a CMS-shaped document declares which of its keys are content, and the
// run must translate those and leave the document's identity — its `_id`, its
// `_type`, its `slug` — exactly as it found it.
func TestConverge_ItemExtractionRulesRestrictWhatIsWritten(t *testing.T) {
	const page = `{
  "_id": "page.pilotage",
  "_type": "page",
  "slug": "pilotage",
  "title": "Pilotage that plans around the tide",
  "body": "<p>Every port has a window.</p>",
  "seo": {
    "description": "Tidewatch compares the forecast."
  }
}
`
	a, cmd, recipe, dir := recipeFormatProject(t,
		map[string]string{"src/page.json": page},
		[]project.Collection{{
			Name: "pages",
			Content: []project.ContentItem{{
				Path: "src/page.json",
				Format: &project.FormatSpec{Name: "json", Config: map[string]any{
					"extractAllPairs":      false,
					"useKeyAsName":         true,
					"escapeForwardSlashes": false,
					"extractionRules":      "(title|body|description)$",
				}},
				Target: "out/{lang}.json",
			}},
		}}, nil)

	// The memory answers every leaf, the identifiers included. That is the
	// realistic state — a venue asked to translate a block translates it, and
	// "pilotage" is an ordinary English word — so the only thing that can keep
	// `slug` out of the output is the recipe's extraction rules being honoured.
	seedMemory(t, a, recipe, map[string]string{
		"Pilotage that plans around the tide": "Losing som planlegger etter tidevannet",
		"<p>Every port has a window.</p>":     "<p>Hver havn har et vindu.</p>",
		"Tidewatch compares the forecast.":    "Tidewatch sammenligner varselet.",
		"page.pilotage":                       "side.losing",
		"page":                                "side",
		"pilotage":                            "losing",
	})

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged,
		"three declared content leaves, three seeded answers — the run must converge")

	body, err := os.ReadFile(filepath.Join(dir, "out", "nb.json"))
	require.NoError(t, err)
	got := string(body)

	// The document's identity survives verbatim. Each of these is a machine
	// identifier: `_id` and `_type` address the record, `slug` is its URL.
	for _, identifier := range []string{
		`"_id": "page.pilotage"`,
		`"_type": "page"`,
		`"slug": "pilotage"`,
	} {
		assert.Contains(t, got, identifier,
			"the recipe's extractionRules exclude this key, so the run must leave it alone")
	}

	// The declared content leaves carry their translations.
	for _, translated := range []string{
		"Losing som planlegger etter tidevannet",
		"Hver havn har et vindu.",
		"Tidewatch sammenligner varselet.",
	} {
		assert.Contains(t, got, translated, "a declared content leaf went untranslated")
	}

	// The write side of the same declaration: escapeForwardSlashes: false.
	assert.NotContains(t, got, `<\/p>`,
		"the item's writer options must take effect, not just its extraction rules")
	assert.Contains(t, got, "</p>")
}

// TestConverge_ItemKeyPathPatternsRestrictWhatIsWritten is #2032's mechanism on
// the YAML path: a narration script's ids and kinds are structure — the harness
// matches an overlay to its master by id — and only the spoken keys the recipe
// names are translatable.
func TestConverge_ItemKeyPathPatternsRestrictWhatIsWritten(t *testing.T) {
	const demo = `id: s0-northsea
title: Govern the content you already have
kind: use-case
narration:
  - id: discover
    kind: terminal
    text: A company repository.
`
	a, cmd, recipe, dir := recipeFormatProject(t,
		map[string]string{"demos/s0/demo.yaml": demo},
		[]project.Collection{{
			Name: "demos",
			Base: "demos",
			Content: []project.ContentItem{{
				Path: "*/demo.yaml",
				Format: &project.FormatSpec{Name: "yaml", Config: map[string]any{
					"keyPathPatterns": []any{"title", "narration.*.text"},
				}},
				Target: "{dir}/demo.{lang}.yaml",
			}},
		}}, nil)

	// Answers for the structural leaves too, spelled as the committed corruption
	// spelled them: only the recipe's key-path patterns can keep them out.
	seedMemory(t, a, recipe, map[string]string{
		"Govern the content you already have": "Styr innholdet du allerede har",
		"A company repository.":               "Et bedriftsrepository.",
		"s0-northsea":                         "s0-nordsjoen",
		"use-case":                            "brukstilfelle",
		"discover":                            "oppdag",
		"terminal":                            "skrivebord",
	})

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged, "two declared spoken keys, two seeded answers")

	body, err := os.ReadFile(filepath.Join(dir, "demos", "s0", "demo.nb.yaml"))
	require.NoError(t, err)
	got := string(body)

	assert.Contains(t, got, "id: s0-northsea", "a demo id is structure, never prose")
	assert.Contains(t, got, "id: discover", "a scene id is what the overlay is matched by")
	assert.Contains(t, got, "kind: use-case", "a scene kind is an enum the renderer dispatches on")
	assert.Contains(t, got, "kind: terminal")

	assert.Contains(t, got, "Styr innholdet du allerede har")
	assert.Contains(t, got, "Et bedriftsrepository.")
}

// TestConverge_ReadsTheFormatTheItemDeclares is #2033. The recipe binds `.md`
// docs to the MDX reader, which is the truthful reader for a Docusaurus page.
// Detecting the format instead gives the markdown reader, which reads an
// `import …` line as a paragraph — one extra block before the body, so every
// file-local block id after it names different content than the store holds.
//
// The observable damage is the frontmatter title's translation landing on the
// first body paragraph, which is what this asserts: the run may translate the
// title, and must leave a body with no answer of its own in English.
func TestConverge_ReadsTheFormatTheItemDeclares(t *testing.T) {
	const page = `---
sidebar_position: 6
title: Formats
---

import { BlockPreview } from "@site/src/components/curated";

# Formats

A format in neokapi is a paired reader and writer for a document type.
`
	fmCfg := map[string]any{
		"translateFrontMatter": true,
		"frontMatterKeys":      []any{"title", "description", "sidebar_label"},
	}
	a, cmd, recipe, dir := recipeFormatProject(t,
		map[string]string{"docs/formats.md": page},
		[]project.Collection{{
			Name: "docs",
			Content: []project.ContentItem{{
				Path:   "docs/**/*.md",
				Format: &project.FormatSpec{Name: "mdx"},
				Target: "i18n/{lang}/{path}.md",
			}},
		}},
		map[string]project.FormatDefaults{
			"markdown": {Config: fmCfg},
			"mdx":      {Config: fmCfg},
		})

	// Only the title has a committed answer — the shape the issue found on six
	// framework pages. The H1 shares the title's text, so it is the one body
	// block an overlay exists for, and it is the block a mis-numbered read puts
	// that overlay one slot away from.
	seedMemory(t, a, recipe, map[string]string{"Formats": "Formater"})

	_ = converge(t, a, cmd, recipe)

	body, err := os.ReadFile(filepath.Join(dir, "i18n", "nb", "formats.md"))
	require.NoError(t, err)
	got := string(body)

	assert.Contains(t, got, "A format in neokapi is a paired reader and writer for a document type.",
		"the body paragraph has no answer of its own and must survive as source; "+
			"the run wrote the frontmatter title over it")
	assert.Contains(t, got, `import { BlockPreview } from "@site/src/components/curated";`,
		"an ESM import is not prose — the MDX reader the recipe names never offers it")
	assert.Contains(t, got, "title: Formater", "the declared frontmatter key is translatable")
}
