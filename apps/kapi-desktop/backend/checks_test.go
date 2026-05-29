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

// setupCheckProject writes a project with one JSON content file, a brand.yaml
// convention profile (forbidden term "utilize" → "use"), and opens it. Returns
// the tab ID and the absolute path of the source JSON file.
func setupCheckProject(t *testing.T, app *App, sourceJSON string) (tabID, srcPath string) {
	t.Helper()
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "locales")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))

	srcPath = filepath.Join(srcDir, "en.json")
	require.NoError(t, os.WriteFile(srcPath, []byte(sourceJSON), 0o644))

	// Convention brand profile at the project root.
	brandYAML := `id: house
name: House Style
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: major
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "brand.yaml"), []byte(brandYAML), 0o644))

	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Defaults: project.Defaults{SourceLanguage: "en"},
		Content: []project.ContentCollection{
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

func TestRunChecksFindsBrandVocab(t *testing.T) {
	app := NewApp()
	tabID, _ := setupCheckProject(t, app, `{"greeting":"Please utilize the dashboard"}`)

	res, err := app.RunChecks(tabID, "")
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
	assert.Equal(t, "use", vocab.Replacement)
	assert.True(t, vocab.Fixable, "a forbidden term with a replacement and a block id should be fixable")
	assert.NotEmpty(t, vocab.BlockID)
}

func TestApplyCheckFixRewritesSourceAndResolves(t *testing.T) {
	app := NewApp()
	tabID, srcPath := setupCheckProject(t, app, `{"greeting":"Please utilize the dashboard"}`)

	res, err := app.RunChecks(tabID, "")
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
	res2, err := app.RunChecks(tabID, "")
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
		Version:  project.CurrentVersion,
		Defaults: project.Defaults{SourceLanguage: "en"},
		Content:  []project.ContentCollection{{Path: "page.html"}},
	}
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))
	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// Read the block to learn its ID and confirm it is multi-run.
	blocks, err := app.readBlocksForChecks(context.Background(), path, "", "en")
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
