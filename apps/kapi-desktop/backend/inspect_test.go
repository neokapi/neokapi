package backend

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupInspectProject writes a project with one JSON source file (and optionally
// a translated target file), a convention voice.yaml (forbidden term "utilize"),
// and seeded project terms (term "dashboard" → "tableau de bord"). It
// opens the project and returns the tab id and the source file path.
func setupInspectProject(t *testing.T, app *App, sourceJSON, targetJSON string) (tabID, srcPath string) {
	t.Helper()
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "locales")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	srcPath = filepath.Join(srcDir, "en.json")
	require.NoError(t, os.WriteFile(srcPath, []byte(sourceJSON), 0o644))
	if targetJSON != "" {
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "fr.json"), []byte(targetJSON), 0o644))
	}

	// Convention voice profile: forbidden term "utilize" → "use".
	voiceYAML := `id: house
name: House Style
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "voice.yaml"), []byte(voiceYAML), 0o644))

	// Seed the project's terms so the term annotator has data to match. They live
	// in the project's one store, and this handle must be closed before the app
	// opens the project — two handles would be two pools on one file.
	db, err := projectdb.Open(context.Background(), project.Layout{
		Root: dir, StateDir: filepath.Join(dir, project.StateDirName),
	})
	require.NoError(t, err)
	require.NoError(t, db.Terms().AddConcept(context.Background(), terms.Concept{
		ID:     "c1",
		Domain: "ui",
		Terms: []terms.Term{
			{Text: "dashboard", Locale: model.LocaleID("en"), Status: model.TermPreferred},
			{Text: "tableau de bord", Locale: model.LocaleID("fr"), Status: model.TermPreferred},
		},
	}))
	require.NoError(t, db.Close())

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{model.LocaleID("fr")},
		},
		Collections: []project.Collection{
			{Path: "locales/en.json", Target: "locales/{lang}.json"},
		},
	}
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))

	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, srcPath
}

func findBlock(t *testing.T, tree *editor.ContentTree) *editor.ContentNode {
	t.Helper()
	var found *editor.ContentNode
	var walk func(n *editor.ContentNode)
	walk = func(n *editor.ContentNode) {
		if n.Kind == "block" && found == nil {
			found = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range tree.Root {
		walk(n)
	}
	require.NotNil(t, found, "expected at least one block in the tree")
	return found
}

func TestInspectFileReturnsContentTree(t *testing.T) {
	app := NewApp()
	tabID, src := setupInspectProject(t, app,
		`{"greeting":"Please utilize the dashboard"}`, "")

	out, err := app.InspectFile(tabID, src)
	require.NoError(t, err)
	require.NotEmpty(t, out)

	var tree editor.ContentTree
	require.NoError(t, json.Unmarshal([]byte(out), &tree))
	assert.Equal(t, "json", tree.Format)
	assert.GreaterOrEqual(t, tree.Stats.Blocks, 1)

	b := findBlock(t, &tree)
	assert.Equal(t, "Please utilize the dashboard", model.RunsText(b.Source))
	// Plain InspectFile must not annotate.
	assert.Empty(t, b.Overlays, "InspectFile should not produce overlays")
}

func TestInspectFileAnnotatedPopulatesOverlays(t *testing.T) {
	app := NewApp()
	tabID, src := setupInspectProject(t, app,
		`{"greeting":"Please utilize the the dashboard"}`, "")

	out, err := app.InspectFileAnnotated(tabID, src)
	require.NoError(t, err)

	var tree editor.ContentTree
	require.NoError(t, json.Unmarshal([]byte(out), &tree))
	b := findBlock(t, &tree)
	require.NotEmpty(t, b.Overlays, "annotated inspect should populate overlays")

	// Collect overlay span props by type/category.
	var (
		sawTerm, sawBrand, sawDoubledWord bool
	)
	for _, ov := range b.Overlays {
		for _, sp := range ov.Spans {
			switch ov.Type {
			case "term":
				if sp.Props["term"] == "dashboard" {
					sawTerm = true
					assert.Equal(t, "tableau de bord", sp.Props["target"], "term overlay carries preferred target")
					assert.Equal(t, "ui", sp.Props["domain"])
				}
			case "qa":
				switch sp.Props["category"] {
				case "voice-vocabulary":
					if sp.Props["term"] == "utilize" {
						sawBrand = true
						assert.Equal(t, "use", sp.Props["replacement"])
						assert.Equal(t, "forbidden", sp.Props["kind"])
					}
				case "doubled-word":
					sawDoubledWord = true
				}
			}
		}
	}
	assert.True(t, sawTerm, "expected a term overlay for the seeded terms term")
	assert.True(t, sawBrand, "expected a voice-vocabulary overlay for the forbidden term")
	assert.True(t, sawDoubledWord, "expected a doubled-word QA overlay (\"the the\")")
}

// kbfShapeFixture is a Kapi Bundle Format document whose blocks carry real
// placeholder runs — the shape the desktop preview actually annotates, and the
// shape the reported false positives came from. Each block is named for what it
// must (not) report.
const kbfShapeFixture = `{
  "schemaVersion": "1.0",
  "kind": "kapi-bundle",
  "project": { "id": "app", "sourceLocale": "en" },
  "documents": [
    {
      "id": "App.tsx",
      "documentType": "jsx",
      "path": "App.tsx",
      "blocks": [
        {
          "id": "sep-space", "translatable": true, "type": "jsx:element",
          "source": [
            { "text": "Hello " },
            { "ph": { "id": "1", "type": "jsx:var", "data": "{name}", "equiv": "name" } },
            { "text": " world" }
          ]
        },
        {
          "id": "sep-word", "translatable": true, "type": "jsx:element",
          "source": [
            { "text": "the " },
            { "ph": { "id": "1", "type": "jsx:var", "data": "{name}", "equiv": "name" } },
            { "text": " the end" }
          ]
        },
        {
          "id": "real-double-space", "translatable": true, "type": "jsx:element",
          "source": [
            { "ph": { "id": "1", "type": "jsx:var", "data": "{p.price}", "equiv": "p.price" } },
            { "text": " two  spaces" }
          ]
        },
        {
          "id": "real-doubled-word", "translatable": true, "type": "jsx:element",
          "source": [
            { "ph": { "id": "1", "type": "jsx:var", "data": "{p.price}", "equiv": "p.price" } },
            { "text": " the the end" }
          ]
        }
      ]
    }
  ]
}`

// TestInspectFileAnnotatedShapeOverlaysAreRunAware is the desktop end of #1441:
// the preview's shape overlays must judge the run-aware flattening, and the span
// they report must still land on the offending text after the projection back
// into RunsText coordinates — the coordinate space the tree's ranges live in.
func TestInspectFileAnnotatedShapeOverlaysAreRunAware(t *testing.T) {
	app := NewApp()
	tabID, src := setupInspectProject(t, app, `{"greeting":"Hello world"}`, "")

	kbfPath := filepath.Join(filepath.Dir(src), "App.kbf.json")
	require.NoError(t, os.WriteFile(kbfPath, []byte(kbfShapeFixture), 0o644))

	out, err := app.InspectFileAnnotated(tabID, kbfPath)
	require.NoError(t, err)

	var tree editor.ContentTree
	require.NoError(t, json.Unmarshal([]byte(out), &tree))

	// Collect, per block, the shape categories reported and the text each span
	// covers (read back through the run range).
	type hit struct{ category, covers string }
	hits := map[string][]hit{}
	var walk func(n *editor.ContentNode)
	walk = func(n *editor.ContentNode) {
		if n.Kind == "block" {
			plain := model.RunsText(n.Source)
			for _, ov := range n.Overlays {
				for _, sp := range ov.Spans {
					cat := sp.Props["category"]
					if cat != "double-spaces" && cat != "doubled-word" {
						continue
					}
					bs, be := sp.Range.ByteSpan(n.Source)
					require.LessOrEqual(t, be, len(plain))
					hits[n.ID] = append(hits[n.ID], hit{cat, plain[bs:be]})
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range tree.Root {
		walk(n)
	}

	assert.Empty(t, hits["sep-space"],
		"a space either side of a placeholder is not a double space")
	assert.Empty(t, hits["sep-word"],
		"the same word either side of a placeholder is not a doubled word")
	assert.Equal(t, []hit{{"double-spaces", "  "}}, hits["real-double-space"])
	assert.Equal(t, []hit{{"doubled-word", "the"}}, hits["real-doubled-word"])
}

func TestInspectFileIncludesProjectTargets(t *testing.T) {
	app := NewApp()
	tabID, src := setupInspectProject(t, app,
		`{"greeting":"Hello world"}`, `{"greeting":"Bonjour le monde"}`)

	out, err := app.InspectFile(tabID, src)
	require.NoError(t, err)

	var tree editor.ContentTree
	require.NoError(t, json.Unmarshal([]byte(out), &tree))
	b := findBlock(t, &tree)
	require.NotEmpty(t, b.Targets, "the sibling fr.json target should be overlaid onto the source block")
	frRuns, ok := b.Targets["fr"]
	require.True(t, ok, "expected an fr target variant, got %v", b.Targets)
	assert.Equal(t, "Bonjour le monde", model.RunsText(frRuns))
}

func TestInspectFileUnknownTab(t *testing.T) {
	app := NewApp()
	_, err := app.InspectFile("nope", "/tmp/whatever.json")
	require.Error(t, err)
}

// TestInspectTargetFileUsesTargetLocale guards the bug behind the kapi-desktop
// preview rendering an RTL target file as if it were the project's LTR
// English source: previewing a committed target file straight from the file
// tree (rather than via its source file's source↔target toggle) must tag the
// tree with that file's own locale, not the project's global source default.
// Get this wrong and the viewer's locale-driven `dir` derivation renders
// Arabic/Hebrew text left-to-right.
func TestInspectTargetFileUsesTargetLocale(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "locales")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	srcPath := filepath.Join(srcDir, "en.json")
	require.NoError(t, os.WriteFile(srcPath, []byte(`{"greeting":"Hello world"}`), 0o644))
	tgtPath := filepath.Join(srcDir, "ar.json")
	require.NoError(t, os.WriteFile(tgtPath, []byte(`{"greeting":"مرحبا بالعالم"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{model.LocaleID("ar")},
		},
		Collections: []project.Collection{
			{Path: "locales/en.json", Target: "locales/{lang}.json"},
		},
	}
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))

	app := NewApp()
	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	out, err := app.InspectFile(tab.ID, tgtPath)
	require.NoError(t, err)
	var tree editor.ContentTree
	require.NoError(t, json.Unmarshal([]byte(out), &tree))
	b := findBlock(t, &tree)
	assert.Equal(t, "ar", b.SourceLocale,
		"previewing ar.json directly must tag its blocks with the ar locale, not the project's en source default")
	assert.Equal(t, "مرحبا بالعالم", model.RunsText(b.Source))

	// The project's actual source file must still resolve to the source locale.
	srcOut, err := app.InspectFile(tab.ID, srcPath)
	require.NoError(t, err)
	var srcTree editor.ContentTree
	require.NoError(t, json.Unmarshal([]byte(srcOut), &srcTree))
	sb := findBlock(t, &srcTree)
	assert.Equal(t, "en", sb.SourceLocale)
}
