package host

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// costRecipe declares the repository's documentation as one collection at one
// governance point, bound to the voice the context directory carries, the way
// the Tidewatch sample binds its own.
const costRecipe = `version: v1
id: prj_commitcheckcostaaaaaaaa
name: commitcheck-cost
defaults:
  source_language: en-GB
  target_languages: [nb]
  voice:
    profile: northsea
profiles:
  northsea:
    channels: [docs]
    voice:
      profile: northsea
collections:
  - name: docs
    channel: northsea/docs
    content:
      - path: docs/**/*.md
        target: i18n/{lang}/{path}.md
      - path: docs/**/*.mdx
        target: i18n/{lang}/{path}.mdx
`

// costProject is the repository's documentation in a project of its own,
// with its context read in, and every translatable block of it.
type costProject struct {
	recipe string
	// docs maps each document's project-relative path to its blocks.
	docs  map[string][]*model.Block
	order []string
}

// newCostProject copies web/docs (Markdown and MDX) into a scratch project and reads the context
// directory KAPI_COMMIT_COST_CONTEXT names into it: a `.kapi/`-shaped layout
// with a voice file and a terms bundle. The default is the Tidewatch sample's.
func newCostProject(b *testing.B) costProject {
	b.Helper()
	contextDir := os.Getenv("KAPI_COMMIT_COST_CONTEXT")
	if contextDir == "" {
		contextDir = filepath.Join("..", "samples", "tidewatch-docs", "context")
	}
	root := b.TempDir()
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(b, os.WriteFile(recipe, []byte(costRecipe), 0o644))
	for _, name := range []string{"voice.yaml", "terms.json"} {
		data, err := os.ReadFile(filepath.Join(contextDir, name))
		require.NoError(b, err)
		require.NoError(b, os.MkdirAll(filepath.Join(root, "context"), 0o755))
		require.NoError(b, os.WriteFile(filepath.Join(root, "context", name), data, 0o644))
	}
	src := filepath.Join("..", "web", "docs")
	require.NoError(b, filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || (filepath.Ext(path) != ".md" && filepath.Ext(path) != ".mdx") {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, "docs", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	}))

	a := &App{SourceLang: "en-GB"}
	a.InitRegistries()
	defer a.Shutdown()
	_, err := a.ImportProjectContext(b.Context(), recipe, ContextImportRequest{Dir: filepath.Join(root, "context")})
	require.NoError(b, err)

	p := costProject{recipe: recipe, docs: map[string][]*model.Block{}}
	require.NoError(b, filepath.WalkDir(filepath.Join(root, "docs"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		formatName := "markdown"
		if filepath.Ext(path) == ".mdx" {
			formatName = "mdx"
		}
		blocks, err := a.ReadBlocksForCheck(b.Context(), path, formatName, nil, "en-GB")
		if err != nil {
			return err
		}
		rel, _ := projectRelPath(root, path)
		for _, blk := range blocks {
			if blk.Translatable && len(blk.Source) > 0 {
				p.docs[rel] = append(p.docs[rel], blk)
			}
		}
		if len(p.docs[rel]) > 0 {
			p.order = append(p.order, rel)
		}
		return nil
	}))
	slices.Sort(p.order)
	return p
}

// sourceEdit is an edit that appends a word to a block's source.
func sourceEdit(doc string, blk *model.Block) change.EditionChange {
	after := append(slices.Clone(blk.Source), model.Run{Text: &model.TextRun{Text: " today"}})
	return change.EditionChange{Ref: change.Ref{Doc: doc, Block: blockKey(blk)}, Role: change.RoleAuthoritative,
		Before: blk.Source, After: after, Block: blk}
}

// translationEdit is an edit to a block's Norwegian translation, which starts
// as a copy of the source.
func translationEdit(doc string, blk *model.Block) change.EditionChange {
	before := slices.Clone(blk.Source)
	after := append(slices.Clone(blk.Source), model.Run{Text: &model.TextRun{Text: " i dag"}})
	b := *blk
	b.Targets = map[model.VariantKey]*model.Target{model.Variant("nb"): {Runs: after}}
	return change.EditionChange{Ref: change.Ref{Doc: doc, Block: blockKey(blk), Edition: model.EditionKey{Locale: "nb"}},
		Role: change.RoleDerived, Before: before, After: after, Block: &b}
}

// BenchmarkCommitCheck measures the commit check over the repository's own
// documentation, held to a sample's voice and terms. Each shape is one change
// set: one paragraph, every block of the largest document, every block of
// every document, and the Norwegian translation of every block of the largest
// document. A warm run keeps one App, as a long-lived surface does; a cold run
// starts each change set on a new App, as one `kapi apply` does.
//
//	go test -tags fts5 -run '^$' -bench BenchmarkCommitCheck -benchtime 5x ./host/
func BenchmarkCommitCheck(b *testing.B) {
	isolateBenchmark(b)
	p := newCostProject(b)
	largest := p.order[0]
	for _, doc := range p.order {
		if len(p.docs[doc]) > len(p.docs[largest]) {
			largest = doc
		}
	}
	var sweep []change.EditionChange
	for _, doc := range p.order {
		for _, blk := range p.docs[doc] {
			sweep = append(sweep, sourceEdit(doc, blk))
		}
	}
	var document, translations []change.EditionChange
	for _, blk := range p.docs[largest] {
		document = append(document, sourceEdit(largest, blk))
		translations = append(translations, translationEdit(largest, blk))
	}
	// The sweep is checked once first, to show the governance resolved: a
	// project that bound nothing would measure the hygiene analyzer alone.
	probe := &App{SourceLang: "en-GB"}
	probeCmd := NewEnvCommand(b.Context(), "apply")
	AddProjectFlag(probeCmd)
	require.NoError(b, probeCmd.Flags().Set("project", p.recipe))
	defer probe.Shutdown()
	outcomes, fingerprint, err := probe.CommitCheck(probeCmd).Check(b.Context(), sweep)
	require.NoError(b, err)
	require.NotEmpty(b, fingerprint, "the project's voice and terms govern the documentation")
	proj, err := project.Load(p.recipe)
	require.NoError(b, err)
	current, err := newContextFingerprints(probe, probeCmd, proj, filepath.Dir(p.recipe))
	require.NoError(b, err)
	governing, err := current.at(probe.GovernancePointFor("", largest), "en-GB")
	current.close()
	require.NoError(b, err)
	require.NotEmpty(b, governing.profileID, "a voice profile governs the documentation")
	byRule := map[string]int{}
	fails := 0
	for _, o := range outcomes {
		for _, f := range o.After {
			byRule[f.Rule]++
			if f.Fails {
				fails++
			}
		}
	}
	b.Logf("%d documents, %d blocks, largest %s with %d blocks; after the sweep, findings by rule %v, %d failing",
		len(p.order), len(sweep), largest, len(p.docs[largest]), byRule, fails)

	shapes := []struct {
		name    string
		changes []change.EditionChange
	}{
		{"paragraph", document[:1]},
		{"document", document},
		{"translations", translations},
		{"docs-sweep", sweep},
	}
	for _, shape := range shapes {
		for _, warm := range []bool{true, false} {
			name := shape.name + "/cold"
			if warm {
				name = shape.name + "/warm"
			}
			b.Run(name, func(b *testing.B) {
				var app *App
				newApp := func() {
					if app != nil {
						app.Shutdown()
					}
					app = &App{SourceLang: "en-GB"}
					app.InitRegistries()
				}
				newApp()
				defer func() { app.Shutdown() }()
				cmd := NewEnvCommand(b.Context(), "apply")
				AddProjectFlag(cmd)
				require.NoError(b, cmd.Flags().Set("project", p.recipe))
				if warm {
					_, _, err := app.CommitCheck(cmd).Check(b.Context(), shape.changes)
					require.NoError(b, err)
				}
				b.ResetTimer()
				for b.Loop() {
					if !warm {
						b.StopTimer()
						newApp()
						b.StartTimer()
					}
					_, _, err := app.CommitCheck(cmd).Check(b.Context(), shape.changes)
					require.NoError(b, err)
				}
				b.ReportMetric(float64(len(shape.changes)), "editions")
				b.ReportMetric(float64(b.Elapsed().Microseconds())/float64(b.N)/float64(len(shape.changes)), "µs/edition")
			})
		}
	}
}

// isolateBenchmark is isolateCheckExecution for a benchmark.
func isolateBenchmark(b *testing.B) {
	b.Helper()
	dir := b.TempDir()
	for k, v := range map[string]string{
		"KAPI_NO_PROJECT": "1", "KAPI_CONFIG_DIR": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"),
		"KAPI_PLUGINS_DIR_ONLY": "1", "KAPI_PLUGINS_DIR": filepath.Join(dir, "plugins"),
	} {
		b.Setenv(k, v)
	}
}
