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

// The benchmark measures a scratch project under the Tidewatch sample's
// context unless these name a project in place: a recipe whose root holds
// web/docs, and the data root holding its context. `make bench-commit-check`
// sets both for this repository's own recipe.
const (
	envCommitCostRecipe  = "KAPI_COMMIT_COST_RECIPE"
	envCommitCostDataDir = "KAPI_COMMIT_COST_DATA_DIR"
)

// costRecipe declares the repository's documentation as one collection at one
// governance point, bound to the voice the Tidewatch sample carries.
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

// costProject is the repository's documentation in a project, with its
// context read in, and every translatable block of it.
type costProject struct {
	recipe string
	// source is the language the documentation is read in.
	source string
	// docs maps each document's project-relative path to its blocks.
	docs  map[string][]*model.Block
	order []string
}

// newCostProject is the project the benchmark measures: the documentation in
// place under the recipe KAPI_COMMIT_COST_RECIPE names, or, by default, every
// Markdown and MDX file of web/docs copied into a scratch project with the
// Tidewatch sample's context read in.
func newCostProject(b *testing.B) costProject {
	b.Helper()
	if recipe := os.Getenv(envCommitCostRecipe); recipe != "" {
		dataDir := os.Getenv(envCommitCostDataDir)
		require.NotEmpty(b, dataDir, "%s names the data root holding the context of %s", envCommitCostDataDir, recipe)
		b.Setenv(EnvDataDir, dataDir)
		proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
		require.NoError(b, err)
		root := filepath.Dir(recipe)
		return readCostProject(b, recipe, filepath.Join(root, "web", "docs"), ResolveSourceLocale("", proj.Defaults.SourceLanguage))
	}

	contextDir := filepath.Join("..", "samples", "tidewatch-docs", "context")
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

	a := &App{}
	a.InitRegistries()
	defer a.Shutdown()
	_, err := a.ImportProjectContext(b.Context(), recipe, ContextImportRequest{Dir: filepath.Join(root, "context")})
	require.NoError(b, err)
	return readCostProject(b, recipe, filepath.Join(root, "docs"), "en-GB")
}

// readCostProject reads every Markdown and MDX file under dir, in the project
// of recipe, in the source language.
func readCostProject(b *testing.B, recipe, dir, source string) costProject {
	b.Helper()
	a := &App{}
	a.InitRegistries()
	defer a.Shutdown()
	root := filepath.Dir(recipe)
	p := costProject{recipe: recipe, source: source, docs: map[string][]*model.Block{}}
	require.NoError(b, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || (filepath.Ext(path) != ".md" && filepath.Ext(path) != ".mdx") {
			return err
		}
		formatName := "markdown"
		if filepath.Ext(path) == ".mdx" {
			formatName = "mdx"
		}
		blocks, err := a.ReadBlocksForCheck(b.Context(), path, formatName, nil, source)
		if err != nil {
			return err
		}
		rel, _ := projectRelPath(root, path)
		for _, blk := range blocks {
			if blk.Translatable && len(blk.SourceRuns()) > 0 {
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
	after := append(slices.Clone(blk.SourceRuns()), model.Run{Text: &model.TextRun{Text: " today"}})
	return change.EditionChange{Ref: change.Ref{Doc: doc, Block: blockKey(blk)}, Role: change.RoleAuthoritative,
		Before: blk.SourceRuns(), After: after, Block: blk}
}

// translationEdit is an edit to a block's Norwegian translation, which starts
// as a copy of the source.
func translationEdit(doc string, blk *model.Block) change.EditionChange {
	before := slices.Clone(blk.SourceRuns())
	after := append(slices.Clone(blk.SourceRuns()), model.Run{Text: &model.TextRun{Text: " i dag"}})
	b := blk.CopyEditionSet()
	for k := range blk.EachTargetEdition {
		b.RemoveEdition(k)
	}
	b.SetEdition(model.Variant("nb"), model.Edition{Runs: after})
	return change.EditionChange{Ref: change.Ref{Doc: doc, Block: blockKey(blk), Edition: model.EditionKey{Locale: "nb"}},
		Role: change.RoleDerived, Before: before, After: after, Block: b}
}

// BenchmarkCommitCheck measures the commit check over the repository's own
// documentation. Each shape is one change set: one paragraph, every block of
// the largest document, every block of every document, and the Norwegian
// translation of every block of the largest document. A warm run keeps one
// App, as a long-lived surface does; a cold run starts each change set on a
// new App, as one `kapi apply` does.
//
//	go test -tags fts5 -run '^$' -bench BenchmarkCommitCheck -benchtime 5x ./host/
//
// `make bench-commit-check` runs it under the sample's governance and under
// this repository's own.
func BenchmarkCommitCheck(b *testing.B) {
	isolateCheckExecution(b)
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
	probe := &App{}
	probeCmd := NewEnvCommand(b.Context(), "apply")
	AddProjectFlag(probeCmd)
	require.NoError(b, probeCmd.Flags().Set("project", p.recipe))
	defer probe.Shutdown()
	outcomes, fingerprint, err := probe.CommitCheck(probeCmd).Check(b.Context(), sweep)
	require.NoError(b, err)
	require.NotEmpty(b, fingerprint, "the project's voice and terms govern the documentation")
	proj, err := project.LoadWithOptions(p.recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(b, err)
	current, err := newContextFingerprints(probe, probeCmd, proj, filepath.Dir(p.recipe))
	require.NoError(b, err)
	current.source = p.source
	governing, err := current.at(probe.GovernancePointFor("", largest), p.source)
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
	b.Logf("%d documents, %d blocks, largest %s with %d blocks; voice %s; after the sweep, findings by rule %v, %d failing",
		len(p.order), len(sweep), largest, len(p.docs[largest]), governing.profileID, byRule, fails)

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
					app = &App{}
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
