package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBlocksForCheck(t *testing.T) {
	app := &App{}
	app.InitRegistries()
	dir := t.TempDir()
	path := filepath.Join(dir, "en.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":"Hello","b":"World"}`), 0o644))

	// Auto-detect by extension.
	blocks, err := app.ReadBlocksForCheck(context.Background(), path, "", "en")
	require.NoError(t, err)
	require.Len(t, blocks, 2)

	// Explicit format override wins over the extension.
	txt := filepath.Join(dir, "data.unknownext")
	require.NoError(t, os.WriteFile(txt, []byte(`{"a":"Hello"}`), 0o644))
	blocks, err = app.ReadBlocksForCheck(context.Background(), txt, "json", "en")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())

	// A nil context is seeded (format readers select on ctx.Done()).
	blocks, err = app.ReadBlocksForCheck(nil, path, "", "en") //nolint:staticcheck // nil ctx is part of the contract
	require.NoError(t, err)
	require.Len(t, blocks, 2)
}

func TestReadBlocksForCheck_UsesDocumentCache(t *testing.T) {
	app := &App{}
	app.InitRegistries()
	root := t.TempDir()
	// A project layout so WithDocumentCache can open the cache dir.
	proj := &project.KapiProject{
		Version:  project.CurrentVersion,
		Defaults: project.Defaults{SourceLanguage: "en"},
		Content:  []project.ContentCollection{{Path: "en.json"}},
	}
	require.NoError(t, project.Save(filepath.Join(root, project.RecipeFileName), proj))
	path := filepath.Join(root, "en.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":"Hello"}`), 0o644))

	err := app.WithDocumentCache(root, func() error {
		require.NotNil(t, app.docCache, "the document cache should be open inside the session")
		// First read records the document; second read replays it.
		b1, err := app.ReadBlocksForCheck(context.Background(), path, "", "en")
		require.NoError(t, err)
		require.Len(t, b1, 1)
		b2, err := app.ReadBlocksForCheck(context.Background(), path, "", "en")
		require.NoError(t, err)
		require.Len(t, b2, 1)
		assert.Equal(t, b1[0].SourceText(), b2[0].SourceText())
		return nil
	})
	require.NoError(t, err)
	assert.Nil(t, app.docCache, "the cache must be closed after the session")
}

func TestOverlayTargets(t *testing.T) {
	src := []*model.Block{
		{ID: "1", Name: "a", Translatable: true},
		{ID: "2", Name: "b", Translatable: true},
	}
	src[0].SetSourceText("Hello")
	src[1].SetSourceText("World")

	tgt := []*model.Block{{ID: "9", Name: "a", Translatable: true}}
	tgt[0].SetSourceText("Bonjour")

	OverlayTargets(src, tgt, "fr")
	assert.Equal(t, "Bonjour", src[0].TargetText("fr"), "matched unit carries the target text")
	assert.Empty(t, src[1].TargetText("fr"), "unmatched unit keeps an empty target")
}

// TestOverlayTargets_Bilingual covers a bilingual target file — kapi's own .kbf
// interchange, where the source stays in place and the translation lives under
// targets.<locale>. OverlayTargets must lift the target-locale runs, not the
// block's source runs; lifting the source made every .kbf target read as
// identical to the source (the bug behind check --ship flagging every bilingual
// target "untranslated"). A monolingual target (translation in source position,
// e.g. fr-FR.json) still works via the source-runs fallback.
func TestOverlayTargets_Bilingual(t *testing.T) {
	src := []*model.Block{{ID: "1", Name: "a", Translatable: true}}
	src[0].SetSourceText("Home")

	// Bilingual target block: English still in the source slot, the translation
	// carried as the nb target — exactly what the .kbf reader produces.
	tgt := []*model.Block{{ID: "1", Name: "a", Translatable: true}}
	tgt[0].SetSourceText("Home")
	tgt[0].SetTargetText("nb", "Hjem")

	OverlayTargets(src, tgt, "nb")
	assert.Equal(t, "Hjem", src[0].TargetText("nb"),
		"the target-locale translation is lifted from a bilingual target, not its source runs")
}

func TestRunCheckToolAndFindingsFromBlock(t *testing.T) {
	b := &model.Block{ID: "1", Name: "a", Translatable: true}
	b.SetSourceText("Keep {name} intact")
	b.SetTargetText("fr", "Placeholder dropped") // {name} missing → finding

	tool := coretools.NewPlaceholderCheckTool(coretools.NewPlaceholderCheckConfig("fr"))
	RunCheckTool(context.Background(), tool, b)

	// Non-clearing read leaves the annotation in place.
	found := FindingsFromBlock(b, false)
	require.NotEmpty(t, found)
	assert.NotEmpty(t, FindingsFromBlock(b, false), "clear=false must not consume the annotation")

	// Clearing read consumes it, so a second checker starts from zero.
	found = FindingsFromBlock(b, true)
	require.NotEmpty(t, found)
	assert.Empty(t, FindingsFromBlock(b, true), "clear=true must remove the annotation")
	_, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)
	assert.False(t, ok)
}

// TestResolveBrandProfile_Ladder exercises the cobra-free resolution ladder:
// recipe binding (profile_file) wins, then the convention brand.yaml files,
// then found=false.
func TestResolveBrandProfile_Ladder(t *testing.T) {
	app := &App{}
	ctx := context.Background()
	profileYAML := []byte("id: house\nname: House Style\n")

	t.Run("profile_file binding", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), profileYAML, 0o644))
		proj := &project.KapiProject{
			Defaults: project.Defaults{BrandVoice: &project.BrandVoiceBinding{ProfileFile: "voice.yaml"}},
		}
		p, src, found, err := app.ResolveBrandProfile(ctx, proj, root, BrandResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "House Style", p.Name)
		assert.Equal(t, filepath.Join(root, "voice.yaml"), src)
	})

	t.Run("convention brand.yaml", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), profileYAML, 0o644))
		proj := &project.KapiProject{}
		p, src, found, err := app.ResolveBrandProfile(ctx, proj, root, BrandResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "House Style", p.Name)
		assert.Equal(t, filepath.Join(root, "brand.yaml"), src)
	})

	t.Run("convention .kapi/brand.yaml", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, project.StateDirName, "brand.yaml"), profileYAML, 0o644))
		proj := &project.KapiProject{}
		_, src, found, err := app.ResolveBrandProfile(ctx, proj, root, BrandResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, filepath.Join(root, project.StateDirName, "brand.yaml"), src)
	})

	t.Run("nothing bound", func(t *testing.T) {
		root := t.TempDir()
		_, _, found, err := app.ResolveBrandProfile(ctx, &project.KapiProject{}, root, BrandResolveOptions{})
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("store binding without a store errors, creating nothing", func(t *testing.T) {
		root := t.TempDir()
		proj := &project.KapiProject{
			Defaults: project.Defaults{BrandVoice: &project.BrandVoiceBinding{Profile: "house"}},
		}
		_, _, _, err := app.ResolveBrandProfile(ctx, proj, root, BrandResolveOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in local store")
		_, statErr := os.Stat(filepath.Join(root, "brand.db"))
		assert.True(t, os.IsNotExist(statErr), "a missing store must not be created by a lookup")
	})
}
