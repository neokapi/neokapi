package backend

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

// setupCheckProject writes a project with one JSON content file, a voice.yaml
// convention profile (forbidden term "utilize" → "use"), and opens it. Returns
// the tab ID and the absolute path of the source JSON file.
func setupCheckProject(t *testing.T, app *App, sourceJSON string) (tabID, srcPath string) {
	t.Helper()
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "locales")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	srcPath = filepath.Join(srcDir, "en.json")
	require.NoError(t, os.WriteFile(srcPath, []byte(sourceJSON), 0o644))

	// Convention voice profile at the project root.
	voiceYAML := `id: house
name: House Style
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "voice.yaml"), []byte(voiceYAML), 0o644))

	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Defaults: project.Defaults{SourceLanguage: "en"},
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

func TestRunChecksFindsVoiceVocab(t *testing.T) {
	app := NewApp()
	tabID, _ := setupCheckProject(t, app, `{"greeting":"Please utilize the dashboard"}`)

	res, err := app.RunChecks(tabID, ProjectFilter{})
	require.NoError(t, err)
	require.NotNil(t, res)

	// A forbidden-term (major) finding lowers the score but is not critical,
	// so the gate passes.
	assert.True(t, res.Pass, "a major finding should not fail the gate")
	assert.Less(t, res.Score, 100, "the forbidden term should lower the roll-up score")

	require.Len(t, res.Files, 1)
	findings := res.Files[0].Findings
	require.NotEmpty(t, findings)

	var vocab *DesktopFinding
	for i := range findings {
		if findings[i].OriginalText == "utilize" {
			vocab = &findings[i]
			break
		}
	}
	require.NotNil(t, vocab, "expected a finding on the forbidden term %q", "utilize")
	assert.Equal(t, "source", vocab.Field)
	assert.Equal(t, "en", vocab.Locale, "a source-side finding carries the project's source locale")
	assert.Equal(t, "use", vocab.Replacement)
	assert.True(t, vocab.Fixable, "a forbidden term with a replacement and a block id should be fixable")
	assert.NotEmpty(t, vocab.BlockID)
}

func TestRunChecksTargetFindingCarriesTargetLocale(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "locales")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "en.json"),
		[]byte(`{"greeting":"Hello {name}"}`), 0o644))
	// The target drops the placeholder — a placeholder-integrity finding.
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "de-DE.json"),
		[]byte(`{"greeting":"Hallo"}`), 0o644))

	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Defaults: project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{
			{Path: "locales/en.json", Target: "locales/{lang}.json"},
		},
	}
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))

	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	res, err := app.RunChecks(tab.ID, ProjectFilter{Languages: []string{"de-DE"}})
	require.NoError(t, err)
	require.Len(t, res.Files, 1)

	var placeholder *DesktopFinding
	for i := range res.Files[0].Findings {
		if res.Files[0].Findings[i].Field == "target" {
			placeholder = &res.Files[0].Findings[i]
			break
		}
	}
	require.NotNil(t, placeholder, "the de-DE translation dropped {name}")
	assert.Equal(t, "de-DE", placeholder.Locale, "a target-side finding carries the checked target locale")
}

func TestApplyCheckFixRewritesSourceAndResolves(t *testing.T) {
	app := NewApp()
	tabID, srcPath := setupCheckProject(t, app, `{"greeting":"Please utilize the dashboard"}`)

	res, err := app.RunChecks(tabID, ProjectFilter{})
	require.NoError(t, err)
	require.Len(t, res.Files, 1)

	var vocab *DesktopFinding
	for i := range res.Files[0].Findings {
		if res.Files[0].Findings[i].Fixable {
			vocab = &res.Files[0].Findings[i]
			break
		}
	}
	require.NotNil(t, vocab, "expected a fixable finding")

	// Apply the one-click fix.
	require.NoError(t, app.ApplyCheckFix(
		tabID, res.Files[0].Path, vocab.BlockID, vocab.Field, vocab.OriginalText, vocab.Replacement,
	))

	// The file on disk now uses the preferred term.
	data, rerr := os.ReadFile(srcPath)
	require.NoError(t, rerr)
	assert.Contains(t, string(data), "use the dashboard")
	assert.NotContains(t, string(data), "utilize")

	// Re-running checks resolves the finding.
	res2, err := app.RunChecks(tabID, ProjectFilter{})
	require.NoError(t, err)
	require.Len(t, res2.Files, 1)
	for _, f := range res2.Files[0].Findings {
		assert.NotEqual(t, "utilize", f.OriginalText, "the forbidden term should be gone after the fix")
	}
	assert.Equal(t, 100, res2.Score, "score should be perfect once the only finding is fixed")
}

func TestApplyCheckFixRefusesMarkupContent(t *testing.T) {
	app := NewApp()
	// Block whose source carries inline markup: a plain substring replace
	// could corrupt the paired code, so the fix must refuse.
	tabID := setupMarkupBlock(t, app)

	err := app.ApplyCheckFix(tabID.tabID, tabID.path, tabID.blockID, "source", "utilize", "use")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manual fix needed")
}

func TestApplyCheckFixValidatesArgs(t *testing.T) {
	app := NewApp()
	tabID, src := setupCheckProject(t, app, `{"greeting":"hi"}`)

	// Missing block id.
	require.Error(t, app.ApplyCheckFix(tabID, src, "", "source", "a", "b"))
	// Bad field.
	require.Error(t, app.ApplyCheckFix(tabID, src, "x", "middle", "a", "b"))
	// Empty original/replacement.
	require.Error(t, app.ApplyCheckFix(tabID, src, "x", "source", "", "b"))
	// Unknown tab.
	require.Error(t, app.ApplyCheckFix("nope", src, "x", "source", "a", "b"))
}

type markupFixture struct {
	tabID   string
	path    string
	blockID string
}

// setupMarkupBlock writes an HTML file with a single paragraph containing an
// inline <b> tag (so the block has multiple runs), opens the project, and
// returns the block id the fix should refuse to touch.
func setupMarkupBlock(t *testing.T, app *App) markupFixture {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	require.NoError(t, os.WriteFile(path, []byte(`<html><body><p>Please <b>utilize</b> it</p></body></html>`), 0o644))

	proj := &project.KapiProject{
		Version:     project.CurrentVersion,
		Defaults:    project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{{Path: "page.html"}},
	}
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))
	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// Read the block to learn its ID and confirm it is multi-run.
	blocks, err := app.readBlocksForChecks(context.Background(), path, "", nil, "en")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	var multi *model.Block
	for _, b := range blocks {
		if len(b.SourceRuns()) > 1 {
			multi = b
			break
		}
	}
	require.NotNil(t, multi, "expected a block with inline markup (multiple runs)")
	return markupFixture{tabID: tab.ID, path: path, blockID: multi.ID}
}

// TestResolveTargetPathMatchesRunner asserts that the checks panel resolves
// translated-file paths through the same canonical resolver the runner uses
// (project.ResolveTargetPath), so checks probe exactly the paths the runner
// writes — including item bases, multi-segment `**` globs, and directory
// targets that the old hand-rolled expansion mishandled.
func TestResolveTargetPathMatchesRunner(t *testing.T) {
	root := t.TempDir()
	projPath := filepath.Join(root, "proj.kapi")

	cases := []struct {
		name     string
		itemPath string
		base     string
		target   string
		relative string // source path relative to the project root
		lang     string
		want     string // expected path relative to the project root
	}{
		{
			name:     "lang template",
			itemPath: "locales/en.json",
			target:   "locales/{lang}.json",
			relative: "locales/en.json",
			lang:     "fr",
			want:     filepath.Join("locales", "fr.json"),
		},
		{
			name:     "base dir with directory target mirrors subtree",
			itemPath: "src/**/*.md",
			base:     "src",
			target:   "out/{lang}/",
			relative: filepath.Join("src", "docs", "guide.md"),
			lang:     "fr",
			want:     filepath.Join("out", "fr", "docs", "guide.md"),
		},
		{
			name:     "multi-segment glob with star target",
			itemPath: "content/**/*.md",
			target:   "translated/{lang}/*.md",
			relative: filepath.Join("content", "blog", "post.md"),
			lang:     "de",
			want:     filepath.Join("translated", "de", "post.md"),
		},
		{
			name:     "multi-segment glob with relpath token",
			itemPath: "content/**/*.md",
			target:   "translated/{lang}/{relpath}",
			relative: filepath.Join("content", "blog", "post.md"),
			lang:     "de",
			want:     filepath.Join("translated", "de", "blog", "post.md"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &project.ContentItem{Path: tc.itemPath, Base: tc.base, Target: tc.target}
			op := &openProject{
				ID:   "tab",
				Path: projPath,
				Project: &project.KapiProject{
					Version:  project.CurrentVersion,
					Defaults: project.Defaults{SourceLanguage: "en"},
					Collections: []project.Collection{
						{Path: tc.itemPath, Target: tc.target},
					},
				},
			}
			app := &App{projects: map[string]*openProject{"tab": op}}

			rf := project.ResolvedFile{
				Path:     filepath.Join(root, tc.relative),
				Relative: tc.relative,
				Item:     item,
			}

			got := app.resolveTargetPath(rf, op, tc.lang)
			assert.Equal(t, filepath.Join(root, tc.want), got)

			// Parity with the canonical resolver — the same
			// project.ResolveTargetPath the flow runner now reaches through
			// the shared cli orchestrator (cli.App.RunFlowAllLocales), so
			// checks-side and runner-side resolution cannot diverge.
			relSlash := filepath.ToSlash(tc.relative)
			canonical := filepath.Join(root, project.ResolveTargetPath(tc.itemPath, tc.base, tc.target, relSlash, tc.lang))
			assert.Equal(t, canonical, got, "checks-side resolution must equal project.ResolveTargetPath")
		})
	}
}

// TestResolveTargetPathNoTemplate covers the empty cases: no item or no target
// template means no path to probe.
func TestResolveTargetPathNoTemplate(t *testing.T) {
	app := &App{}
	op := &openProject{Path: filepath.Join(t.TempDir(), "proj.kapi")}
	assert.Empty(t, app.resolveTargetPath(project.ResolvedFile{Relative: "a.json"}, op, "fr"))
	assert.Empty(t, app.resolveTargetPath(project.ResolvedFile{Relative: "a.json", Item: &project.ContentItem{Path: "a.json"}}, op, "fr"))
}
