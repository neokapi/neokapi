package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnitsFromProject_ExpandsPathToken guards that verify/coverage resolve a
// content item's target template with the SAME token set as the write path
// (project.ResolveTargetPath), not just {lang}. A recipe like
// `target: i18n-{lang}/{path}.kbf.json` over a `**` glob writes i18n-fr/sub/a.kbf.json via
// `kapi up`; when verify only expanded {lang} it looked for the literal
// i18n-fr/{path}.kbf.json and reported every target "missing" — the bug behind
// check --ship / coverage failing on any glob-source + templated-target project.
func TestUnitsFromProject_ExpandsPathToken(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "i18n", "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "i18n", "sub", "a.kbf.json"),
		[]byte(`{"kind":"kapi-bundle"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
		},
		Content: []project.ContentCollection{
			{Name: "c", Path: "i18n/**/*.kbf.json", Target: "i18n-{lang}/{path}.kbf.json"},
		},
	}

	app := &App{}
	app.InitRegistries()
	app.SourceLang = "en"

	units, err := app.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.Len(t, units, 1)
	// {path} = source relative to the glob's fixed prefix (i18n/), without ext.
	want := filepath.Join(root, "i18n-fr", "sub", "a.kbf.json")
	assert.Equal(t, want, units[0].TargetPath,
		"{path} must expand like the write path (ResolveTargetPath), not stay literal")
}

// TestCoverageParity_FileScanVsBlockStore asserts the two block sources of
// the shared coverage engine agree: the CLI's file-scan derivation
// (ComputeShipCoverage over working-tree reads) and the desktop status
// panel's block-store derivation (convergence.TallyBlockStore over
// `.kapi/cache/blocks.db`) produce identical LocaleCoverage rows — and the
// same absolute translated counts the desktop renders — for the same
// underlying data.
func TestCoverageParity_FileScanVsBlockStore(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "en.json"),
		[]byte(`{"a":"Hello","b":"World","c":"Bye"}`), 0o644))
	// fr covers 2 of the 3 units.
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "fr.json"),
		[]byte(`{"a":"Bonjour","b":"Monde"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
		},
		Content: []project.ContentCollection{
			{Name: "docs", Path: "src/en.json", Target: "src/{lang}.json"},
		},
		ShipGate: gate.Gate{"translated": {Pct: 100}},
	}
	projPath := filepath.Join(root, "kapi.yaml")
	require.NoError(t, project.Save(projPath, proj))

	app := &App{}
	app.InitRegistries()
	app.SourceLang = "en"
	ctx := context.Background()

	// --- file-scan side: the CLI's `kapi status` / verify --ship metric.
	units, err := app.UnitsFromProject(proj, root, "")
	require.NoError(t, err)
	require.NotEmpty(t, units)
	fileRows, err := app.ComputeShipCoverage(ctx, proj, root, units, nil)
	require.NoError(t, err)

	// --- block-store side: extract, commit targets/fr overlays for exactly
	// the units the fr file translates, then tally — the desktop status path.
	db, err := app.ProjectDB(ctx, root)
	require.NoError(t, err)
	store := db.Blocks()

	pctx := project.NewProjectContext(proj, projPath)
	resolved, err := pctx.ResolveContent(app.FormatReg)
	require.NoError(t, err)
	stats, err := project.ExtractToBlockStore(ctx, app.FormatReg, pctx, store, db, resolved)
	require.NoError(t, err)
	require.Equal(t, 3, stats.Blocks)

	translated := map[string]bool{"Hello": true, "World": true}
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	tr := true
	var overlayIDs []string
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: "docs", Translatable: &tr}) {
		require.NoError(t, berr)
		if translated[model.RunsText(b.Source)] {
			id := b.ID
			if id == "" {
				id = b.Hash
			}
			overlayIDs = append(overlayIDs, id)
		}
	}
	require.Len(t, overlayIDs, 2)
	for _, id := range overlayIDs {
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind: "targets/fr", BlockHash: id, Payload: []byte(`{}`),
		}))
	}
	require.NoError(t, sess.Commit())
	require.NoError(t, sess.Close())

	readSess, err := store.Begin(ctx)
	require.NoError(t, err)
	defer readSess.Close()
	scopes := []convergence.BlockStoreScope{{
		Collection: "docs",
		Label:      project.CollectionLabel("docs"),
		Locales:    []string{"fr"},
	}}
	tally, totals, err := convergence.TallyBlockStore(readSess, scopes)
	require.NoError(t, err)
	rules, err := proj.BuildShipGates()
	require.NoError(t, err)
	storeRows := tally.Rollup(rules)

	// The rollups agree row for row: same totals, same per-rung percentages,
	// same gating verdicts.
	assert.Equal(t, fileRows, storeRows, "file-scan and block-store coverage must be identical")

	// And the desktop's absolute numbers (CollectionStatus.BlockCount /
	// Coverage[locale]) match what the CLI reports.
	require.Len(t, storeRows, 1)
	assert.Equal(t, "fr", storeRows[0].Locale)
	assert.Equal(t, "docs", storeRows[0].Collection)
	assert.Equal(t, 3, storeRows[0].Total)
	assert.Equal(t, 67, storeRows[0].Pct[string(model.TargetStatusTranslated)])
	assert.True(t, storeRows[0].Gated)
	assert.False(t, storeRows[0].Shippable, "2/3 translated must not clear a translated:100 gate")

	assert.Equal(t, map[string]int{"docs": 3}, totals)
	cov, ok := tally.Coverage(convergence.Scope{Collection: "docs", Locale: "fr"})
	require.True(t, ok)
	assert.Equal(t, 2, cov.AtLeastCount(gate.TargetLadder(), string(model.TargetStatusTranslated)))
}
