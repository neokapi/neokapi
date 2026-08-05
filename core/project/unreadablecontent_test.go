package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1474 item 3. ResolveContent stat'ed every glob match behind
// `if err != nil || info.IsDir() { continue }`, so a file that MATCHED the
// pattern but could not be read was dropped exactly like a directory — the one
// case that is legitimate. This is the single content-resolution seam for ls,
// extract, merge --materialize, up and ExtractToBlockStore, so the file vanished
// from every count at once: coverage was computed over the smaller universe and
// the locale read 100% translated while that file shipped untranslated, and it
// never appeared in ExtractStats.Skipped either.

func resolveContentProject(t *testing.T, pattern string) (*project.ProjectContext, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "UnreadableContentTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
		},
		Content: []project.ContentCollection{
			{Name: "app", Path: pattern, Target: "out/{lang}.json"},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	return project.NewProjectContext(proj, recipe), dir
}

func contentRegistry(t *testing.T) *registry.FormatRegistry {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
}

// TestResolveContent_UnreadableMatchFailsNamingTheFile is the regression: a
// dangling symlink matches the pattern (something IS there under that name) and
// then cannot be stat'ed. Present-but-unreadable must not read as absent.
func TestResolveContent_UnreadableMatchFailsNamingTheFile(t *testing.T) {
	pctx, dir := resolveContentProject(t, "src/*.json")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"a":"b"}`), 0o644))
	broken := filepath.Join(dir, "src", "gone.json")
	require.NoError(t, os.Symlink(filepath.Join(dir, "src", "never-existed.json"), broken))

	files, err := pctx.ResolveContent(contentRegistry(t))
	require.Error(t, err,
		"a match that cannot be read must fail resolution, not silently shrink the universe")
	assert.Contains(t, err.Error(), "gone.json", "the message must name the file")
	assert.Contains(t, err.Error(), "silently drop out of every count")
	assert.Empty(t, files, "resolution failed, so no partial file list is returned")
}

// TestResolveContent_DirectoryMatchIsNotAnError is the discriminating control:
// ExpandGlob returns directories for `**` patterns and a directory holds no
// content of its own, so it must still be skipped quietly. Without this, "fail on
// any stat outcome that is not a plain file" would pass the test above.
func TestResolveContent_DirectoryMatchIsNotAnError(t *testing.T) {
	pctx, dir := resolveContentProject(t, "src/**")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "nested", "en.json"),
		[]byte(`{"a":"b"}`), 0o644))

	files, err := pctx.ResolveContent(contentRegistry(t))
	require.NoError(t, err, "a directory match is expected, not a fault")
	require.Len(t, files, 1, "the directory contributes its file, not itself")
	assert.Equal(t, filepath.Join("src", "nested", "en.json"), files[0].Relative)
}

// TestResolveContent_ReadableFilesStillResolve: the ordinary case is untouched.
func TestResolveContent_ReadableFilesStillResolve(t *testing.T) {
	pctx, dir := resolveContentProject(t, "src/*.json")
	for _, name := range []string{"en.json", "de.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", name), []byte(`{"a":"b"}`), 0o644))
	}
	files, err := pctx.ResolveContent(contentRegistry(t))
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

// #1474 item 5. ExtractToBlockStore counted a file as extracted (stats.Files++)
// and then wrote its drift stamp behind `if serr == nil`. A missing stamp is not
// recoverable state: DetectStoreDrift treats a file with no stamp as
// unconditionally Changed, and — unlike the whole-sidecar write below it, which
// self-heals on the next run — the same stat/hash would fail again every time, so
// the file would be re-extracted and re-translated forever while any "N drifted"
// summary permanently over-reported.
//
// PREMISE CHECK, recorded rather than implemented around: on today's code this
// swallow is LATENT, not live. `sourceStampFor` stats and hashes the very file
// `ReadSourceBlocks` has just opened and read, in the same loop iteration, with no
// cache between them and no byte budget on HashFile — so a stamp failure that a
// read survived cannot be constructed from outside the process (only a concurrent
// replace of the source between the two calls would do it). The fix is kept
// because the divergence is real, the strictness is free, and a future cached read
// path would make it live overnight — the same reasoning #1466 applied to its
// converge item, and the same latency #1472 found in NewSkeletonStore. What is NOT
// claimed is a demonstrated live failure, so there is no staged-failure test here
// pretending otherwise.
//
// TestDetectStoreDrift_MissingStampReadsAsChangedForever is therefore a CONTRACT
// test for the behaviour the fix depends on, not a revert-proof regression: it
// pins why a dropped stamp could never be shrugged off.
func TestDetectStoreDrift_MissingStampReadsAsChangedForever(t *testing.T) {
	pctx, dir := resolveContentProject(t, "src/*.json")
	for _, name := range []string{"en.json", "de.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", name),
			[]byte(`{"greeting":"Hello"}`), 0o644))
	}
	layout, err := project.LayoutFor(filepath.Join(dir, project.RecipeFileName))
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	files, err := pctx.ResolveContent(contentRegistry(t))
	require.NoError(t, err)
	require.Len(t, files, 2)

	db, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = project.ExtractToBlockStore(t.Context(), contentRegistry(t), pctx, db.Blocks(), db, files)
	require.NoError(t, err)

	// Nothing drifted right after a full extraction.
	require.Empty(t, db.DetectStoreDrift(t.Context(), files).Changed)

	// Now drop ONE file's stamp, exactly as the swallowed error used to. The file
	// itself is untouched, so nothing about it actually changed.
	stamps := db.LoadSourceStamps(t.Context())
	require.Len(t, stamps, 2)
	delete(stamps, files[0].Relative)
	require.NoError(t, db.SaveSourceStamps(t.Context(), stamps))

	drift := db.DetectStoreDrift(t.Context(), files)
	assert.Contains(t, drift.Changed, files[0].Relative,
		"a file with no stamp reads as drifted however unchanged it is — and it "+
			"cannot recover, because the stamp is only written on a successful extract")
	assert.NotContains(t, drift.Changed, files[1].Relative,
		"the file that kept its stamp is not drifted: the report distinguishes them, "+
			"so this is about the missing stamp and not a blanket 'everything drifted'")
}

// TestExtractToBlockStore_HealthySourceStampsAndDoesNotDrift is the control: a
// normal extraction stamps every file, and an immediately following drift check
// reports nothing changed. "Fail on any stamp trouble" must not break it.
func TestExtractToBlockStore_HealthySourceStampsAndDoesNotDrift(t *testing.T) {
	pctx, dir := resolveContentProject(t, "src/en.json")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello"}`), 0o644))

	layout, err := project.LayoutFor(filepath.Join(dir, project.RecipeFileName))
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	files, err := pctx.ResolveContent(contentRegistry(t))
	require.NoError(t, err)
	db, err := projectdb.Open(t.Context(), layout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	stats, err := project.ExtractToBlockStore(t.Context(), contentRegistry(t), pctx, db.Blocks(), db, files)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Files)

	drift := db.DetectStoreDrift(t.Context(), files)
	assert.Empty(t, drift.Changed,
		"a freshly stamped file must not read as drifted — that is the permanent "+
			"re-extraction loop the fix removes")
	assert.Empty(t, drift.Removed)
}
