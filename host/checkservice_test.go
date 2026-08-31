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
	blocks, err := app.ReadBlocksForCheck(context.Background(), path, "", nil, "en")
	require.NoError(t, err)
	require.Len(t, blocks, 2)

	// Explicit format override wins over the extension.
	txt := filepath.Join(dir, "data.unknownext")
	require.NoError(t, os.WriteFile(txt, []byte(`{"a":"Hello"}`), 0o644))
	blocks, err = app.ReadBlocksForCheck(context.Background(), txt, "json", nil, "en")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())

	// A nil context is seeded (format readers select on ctx.Done()).
	blocks, err = app.ReadBlocksForCheck(nil, path, "", nil, "en") //nolint:staticcheck // nil ctx is part of the contract
	require.NoError(t, err)
	require.Len(t, blocks, 2)
}

func TestReadBlocksForCheck_UsesDocumentCache(t *testing.T) {
	app := &App{}
	app.InitRegistries()
	root := t.TempDir()
	// A project layout so WithDocumentCache can open the cache dir.
	proj := &project.KapiProject{
		Version:     project.CurrentVersion,
		Defaults:    project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{{Path: "en.json"}},
	}
	require.NoError(t, project.Save(filepath.Join(root, project.RecipeFileName), proj))
	path := filepath.Join(root, "en.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":"Hello"}`), 0o644))

	err := app.WithDocumentCache(root, func() error {
		require.NotNil(t, app.docCache, "the document cache should be open inside the session")
		// First read records the document; second read replays it.
		b1, err := app.ReadBlocksForCheck(context.Background(), path, "", nil, "en")
		require.NoError(t, err)
		require.Len(t, b1, 1)
		b2, err := app.ReadBlocksForCheck(context.Background(), path, "", nil, "en")
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

// TestOverlayTargets_TranslatedStructuralNames covers blocks whose keys are
// structural addresses built from heading slugs and which carry no invariant
// address — a format that composes none, or a document read before one was
// composed. Translating a heading re-addresses every block beneath it, so the
// key match reaches only the blocks above the first heading, and equal block
// counts are what license the rest to pair by position.
func TestOverlayTargets_TranslatedStructuralNames(t *testing.T) {
	src := []*model.Block{
		{ID: "1", Name: "p", Translatable: true},
		{ID: "2", Name: "h", Translatable: true},
		{ID: "3", Name: "what-it-reads/p", Translatable: true},
	}
	src[0].SetSourceText("Tidewatch compares the forecast.")
	src[1].SetSourceText("What it reads")
	src[2].SetSourceText("Three inputs, in this order:")

	tgt := []*model.Block{
		{ID: "1", Name: "p", Translatable: true},
		{ID: "2", Name: "h", Translatable: true},
		{ID: "3", Name: "hva-den-leser/p", Translatable: true},
	}
	tgt[0].SetSourceText("Tidewatch sammenligner prognosen.")
	tgt[1].SetSourceText("Hva den leser")
	tgt[2].SetSourceText("Tre kilder, i denne rekkefølgen:")

	OverlayTargets(src, tgt, "nb")
	assert.Equal(t, "Tidewatch sammenligner prognosen.", src[0].TargetText("nb"), "keyed match")
	assert.Equal(t, "Hva den leser", src[1].TargetText("nb"), "keyed match")
	assert.Equal(t, "Tre kilder, i denne rekkefølgen:", src[2].TargetText("nb"),
		"a block whose address changed with its heading pairs by position")
}

// TestOverlayTargets_InvariantAddressPairsDivergedDocuments reads a real
// markdown pair through the real reader: a source page and its translation,
// headings included, with one paragraph the source does not have. The counts
// differ, so the positional fallback cannot fire and every block under a
// translated heading has a key that exists in neither document. They pair on the
// invariant address, which is what makes the fix structural rather than a guard
// on a guess: only the paragraph with nothing to pair with is left untranslated,
// and it is left untranslated because it genuinely is.
func TestOverlayTargets_InvariantAddressPairsDivergedDocuments(t *testing.T) {
	const source = `# Tidewatch

An opening paragraph.

## What it reads

The first paragraph of the section.

## How it reports

A paragraph under the second heading.

A second paragraph under the second heading.
`

	// Translated, and one paragraph shorter under the second heading.
	const translated = `# Tidevakt

Et innledende avsnitt.

## Hva den leser

Det første avsnittet i seksjonen.

## Hvordan den rapporterer

Et avsnitt under den andre overskriften.
`

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "index.md")
	tgtPath := filepath.Join(dir, "index.nb.md")
	require.NoError(t, os.WriteFile(srcPath, []byte(source), 0o644))
	require.NoError(t, os.WriteFile(tgtPath, []byte(translated), 0o644))

	app := &App{}
	app.InitRegistries()
	ctx := context.Background()

	srcBlocks, err := app.ReadBlocksForCheck(ctx, srcPath, "", nil, "en")
	require.NoError(t, err)
	tgtBlocks, err := app.ReadBlocksForCheck(ctx, tgtPath, "", nil, "en")
	require.NoError(t, err)
	require.NotEqual(t, len(srcBlocks), len(tgtBlocks),
		"the counts must differ, or the positional fallback would carry the test")

	OverlayTargets(srcBlocks, tgtBlocks, "nb")

	got := map[string]string{}
	for _, b := range srcBlocks {
		got[b.SourceText()] = b.TargetText("nb")
	}
	assert.Equal(t, "Tidevakt", got["Tidewatch"])
	assert.Equal(t, "Et innledende avsnitt.", got["An opening paragraph."])
	assert.Equal(t, "Hva den leser", got["What it reads"])
	assert.Equal(t, "Det første avsnittet i seksjonen.", got["The first paragraph of the section."],
		"a block under a translated heading pairs on its invariant address")
	assert.Equal(t, "Hvordan den rapporterer", got["How it reports"])
	assert.Equal(t, "Et avsnitt under den andre overskriften.", got["A paragraph under the second heading."])
	assert.Empty(t, got["A second paragraph under the second heading."],
		"the one block the target genuinely does not have stays untranslated")
}

// TestOverlayTargets_DivergedDocumentKeepsKeyMatchOnly proves the positional
// fallback is guarded by equal block counts: a target that genuinely diverged
// from its source is not pairwise-aligned, and guessing there would report
// translations that are not translations of the blocks they were attached to.
func TestOverlayTargets_DivergedDocumentKeepsKeyMatchOnly(t *testing.T) {
	src := []*model.Block{
		{ID: "1", Name: "p", Translatable: true},
		{ID: "2", Name: "intro/p", Translatable: true},
	}
	src[0].SetSourceText("First")
	src[1].SetSourceText("Second")

	tgt := []*model.Block{{ID: "1", Name: "p", Translatable: true}}
	tgt[0].SetSourceText("Først")

	OverlayTargets(src, tgt, "nb")
	assert.Equal(t, "Først", src[0].TargetText("nb"))
	assert.Empty(t, src[1].TargetText("nb"), "unequal counts fall back to nothing, not to a guess")
}

// TestOverlayTargets_Bilingual covers a bilingual target file — kapi's own .kbf.json
// interchange, where the source stays in place and the translation lives under
// targets.<locale>. OverlayTargets must lift the target-locale runs, not the
// block's source runs; lifting the source made every .kbf.json target read as
// identical to the source (the bug behind check --ship flagging every bilingual
// target "untranslated"). A monolingual target (translation in source position,
// e.g. fr-FR.json) still works via the source-runs fallback.
func TestOverlayTargets_Bilingual(t *testing.T) {
	src := []*model.Block{{ID: "1", Name: "a", Translatable: true}}
	src[0].SetSourceText("Home")

	// Bilingual target block: English still in the source slot, the translation
	// carried as the nb target — exactly what the .kbf.json reader produces.
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
	require.NoError(t, RunCheckTool(context.Background(), tool, b))

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

// TestResolveVoiceProfile_Ladder exercises the cobra-free resolution ladder:
// recipe binding (profile_file) wins, then the convention voice.yaml files,
// then found=false.
func TestResolveVoiceProfile_Ladder(t *testing.T) {
	app := &App{}
	ctx := context.Background()
	profileYAML := []byte("id: house\nname: House Style\n")

	t.Run("profile_file binding", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), profileYAML, 0o644))
		proj := &project.KapiProject{
			Defaults: project.Defaults{Voice: &project.VoiceBinding{ProfileFile: "voice.yaml"}},
		}
		p, src, found, err := app.ResolveVoiceProfile(ctx, proj, root, VoiceResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "House Style", p.Name)
		assert.Equal(t, filepath.Join(root, "voice.yaml"), src)
	})

	t.Run("convention voice.yaml", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), profileYAML, 0o644))
		proj := &project.KapiProject{}
		p, src, found, err := app.ResolveVoiceProfile(ctx, proj, root, VoiceResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "House Style", p.Name)
		assert.Equal(t, filepath.Join(root, "voice.yaml"), src)
	})

	t.Run("convention .kapi/voice.yaml", func(t *testing.T) {
		root := t.TempDir()
		conv := filepath.Join(root, project.RelStatePath(VoiceConventionalName))
		require.NoError(t, os.MkdirAll(filepath.Dir(conv), 0o755))
		require.NoError(t, os.WriteFile(conv, profileYAML, 0o644))
		proj := &project.KapiProject{}
		_, src, found, err := app.ResolveVoiceProfile(ctx, proj, root, VoiceResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, conv, src)
	})

	// A profile's own directory answers before the project default does — the
	// recipe binds the point, the filesystem holds what that point overrides.
	t.Run("a matched profile is answered by its own directory", func(t *testing.T) {
		root := t.TempDir()
		conv := filepath.Join(root, project.RelStatePath(VoiceConventionalName))
		require.NoError(t, os.MkdirAll(filepath.Dir(conv), 0o755))
		require.NoError(t, os.WriteFile(conv, profileYAML, 0o644))

		profileVoice := filepath.Join(root,
			project.RelStatePath(project.ProfilesDirName, "bowrain", VoiceConventionalName))
		require.NoError(t, os.MkdirAll(filepath.Dir(profileVoice), 0o755))
		require.NoError(t, os.WriteFile(profileVoice, []byte("id: bowrain\nname: Bowrain Style\n"), 0o644))

		proj := &project.KapiProject{
			Profiles: map[string]project.Profile{
				"bowrain": {Channels: []project.Channel{{ID: "app"}}},
			},
			Collections: []project.Collection{{
				Name:    "app",
				Channel: "bowrain/app",
				Content: []project.ContentItem{{Path: "src/*.json"}},
			}},
		}
		proj.Defaults.Voice = &project.VoiceBinding{ProfileFile: ".kapi/voice.yaml"}

		p, src, found, err := app.ResolveVoiceProfile(ctx, proj, root, VoiceResolveOptions{Point: project.GovernancePoint{Collection: "app"}})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "Bowrain Style", p.Name, "the profile's directory outranks defaults.voice")
		assert.Equal(t, profileVoice, src)

		// A point no profile claims still gets the project default.
		p, _, found, err = app.ResolveVoiceProfile(ctx, proj, root, VoiceResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "House Style", p.Name)
	})

	// The state directory outranks the root: `.kapi/` is committed, so the
	// conventional home and the reviewed home are the same directory.
	t.Run("context directory outranks the root", func(t *testing.T) {
		root := t.TempDir()
		conv := filepath.Join(root, project.RelStatePath(VoiceConventionalName))
		require.NoError(t, os.MkdirAll(filepath.Dir(conv), 0o755))
		require.NoError(t, os.WriteFile(conv, profileYAML, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"),
			[]byte("id: root\nname: Root Style\n"), 0o644))

		p, src, found, err := app.ResolveVoiceProfile(ctx, &project.KapiProject{}, root, VoiceResolveOptions{})
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "House Style", p.Name)
		assert.Equal(t, conv, src)
	})

	t.Run("nothing bound", func(t *testing.T) {
		root := t.TempDir()
		_, _, found, err := app.ResolveVoiceProfile(ctx, &project.KapiProject{}, root, VoiceResolveOptions{})
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("store binding without a store errors, creating nothing", func(t *testing.T) {
		root := t.TempDir()
		proj := &project.KapiProject{
			Defaults: project.Defaults{Voice: &project.VoiceBinding{Profile: "house"}},
		}
		_, _, _, err := app.ResolveVoiceProfile(ctx, proj, root, VoiceResolveOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in local store")
		_, statErr := os.Stat(filepath.Join(root, "voice.db"))
		assert.True(t, os.IsNotExist(statErr), "a missing store must not be created by a lookup")
	})
}

// A target's provenance is what makes an approved answer judgeable rather than
// merely retrievable: the record absorber reads the governing context off the
// overlaid block, and a zero Origin reads the same as "produced under no
// governance". The overlay carries how the target was produced, not only what
// it says (#2278).
func TestOverlayTargets_CarriesTargetProvenance(t *testing.T) {
	src := []*model.Block{{ID: "1", Name: "a", Translatable: true}}
	src[0].SetSourceText("Sign in")

	tgt := []*model.Block{{ID: "9", Name: "a", Translatable: true}}
	tgt[0].SetTargetRuns("nb", []model.Run{{Text: &model.TextRun{Text: "Logg inn"}}})
	tgt[0].StampTargetProvenance("nb", model.TargetStatusTranslated, model.Origin{
		Kind:               "ai",
		Profile:            "northsea",
		ProfileVersion:     "3",
		ContextFingerprint: "cfp-abc123",
	})

	OverlayTargets(src, tgt, "nb")

	got := src[0].Target("nb")
	require.NotNil(t, got)
	assert.Equal(t, "Logg inn", src[0].TargetText("nb"))
	assert.Equal(t, "cfp-abc123", got.Origin.ContextFingerprint,
		"the fingerprint the absorber reads survives the overlay")
	assert.Equal(t, "northsea", got.Origin.Profile)
	assert.Equal(t, "ai", got.Origin.Kind)
	assert.Equal(t, model.TargetStatusTranslated, got.Status)
}

// A monolingual target file carries the translation as its own source, so there
// is no Origin to carry and the overlay must not invent one.
func TestOverlayTargets_MonolingualTargetHasNoProvenance(t *testing.T) {
	src := []*model.Block{{ID: "1", Name: "a", Translatable: true}}
	src[0].SetSourceText("Sign in")

	tgt := []*model.Block{{ID: "9", Name: "a", Translatable: true}}
	tgt[0].SetSourceText("Logg inn")

	OverlayTargets(src, tgt, "nb")

	got := src[0].Target("nb")
	require.NotNil(t, got)
	assert.Equal(t, "Logg inn", src[0].TargetText("nb"))
	assert.Equal(t, model.Origin{}, got.Origin)
}
