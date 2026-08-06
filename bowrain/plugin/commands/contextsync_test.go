package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newGovernedProject scaffolds a project whose context space binds one voice to
// two collections and leaves a third ungoverned, plus a bare entry that declares
// no collection at all.
func newGovernedProject(t *testing.T) *bproject.Project {
	t.Helper()
	prev := app
	app = &cli.App{}
	t.Cleanup(func() { app = prev })

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "voice.yaml"), []byte(
		"name: Acme Voice\ndescription: How Acme sounds.\ntone:\n  formality: neutral\n"), 0o644))

	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage:  "en",
				TargetLanguages: []model.LocaleID{"fr"},
			},
			Coordinates: coreproj.Coordinates{
				"product": {{ID: "kapi"}, {ID: "bowrain"}},
				"channel": {{ID: "docs"}, {ID: "email"}},
			},
			Profiles: []coreproj.ProfileBinding{{
				When:  map[string]string{"product": "kapi"},
				Voice: &coreproj.VoiceBinding{ProfileFile: "voice.yaml"},
			}},
			Content: []coreproj.ContentCollection{
				{
					Name:    "docs",
					Context: map[string]string{"product": "kapi", "channel": "docs"},
					Items:   []coreproj.ContentItem{{Path: "docs/**/*.json"}},
				},
				{
					Name:    "mail",
					Context: map[string]string{"product": "kapi", "channel": "email"},
					Items:   []coreproj.ContentItem{{Path: "mail/**/*.json"}},
				},
				{
					Name:    "partner",
					Context: map[string]string{"product": "bowrain"},
					Items:   []coreproj.ContentItem{{Path: "partner/**/*.json"}},
				},
				{Path: "loose/*.json"},
			},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	return proj
}

func entriesByName(entries []*pb.SyncContextEntry) map[string]*pb.SyncContextEntry {
	out := map[string]*pb.SyncContextEntry{}
	for _, e := range entries {
		out[e.Name] = e
	}
	return out
}

// TestBuildPushContext_CarriesDeclaredCollections pins what a push declares:
// one entry per NAMED collection, each at its declared point, with the channel
// the framework reads for itself resolved, and the bare entry left out because
// it declares no collection to reconcile.
func TestBuildPushContext_CarriesDeclaredCollections(t *testing.T) {
	proj := newGovernedProject(t)

	pushCtx, brand, err := BuildPushContext(t.Context(), proj, false)
	require.NoError(t, err)
	require.NotNil(t, pushCtx)
	require.Len(t, pushCtx.Entries, 3, "the bare entry declares no collection")

	byName := entriesByName(pushCtx.Entries)
	docs := byName["docs"]
	require.NotNil(t, docs)
	assert.Equal(t, map[string]string{"product": "kapi", "channel": "docs"}, docs.Coordinates)
	assert.Equal(t, "docs", docs.Channel)
	assert.Equal(t, bowsync.ContextOwnerRecipe, docs.Owner,
		"a collection the recipe declares is git's, and says so on the wire")
	assert.Equal(t, "Acme Voice", docs.VoiceProfile)

	assert.Equal(t, "email", byName["mail"].Channel)
	assert.Equal(t, "Acme Voice", byName["mail"].VoiceProfile,
		"the same profile governs every point its `when:` matches")

	assert.Empty(t, byName["partner"].VoiceProfile,
		"a point no profile matches binds no voice")

	require.NotNil(t, brand)
	assert.Equal(t, "Acme Voice", brand.Name)
	assert.Equal(t, "carried", brand.Action)
}

// TestBuildPushContext_CarriesTheVoiceAsAuthored pins the fold-in's correctness
// condition. The profile travels once per name, and it travels AS AUTHORED —
// with its channel overrides intact rather than resolved into one collection's
// channel, which would collapse the variants into whichever collection happened
// to be resolved last.
func TestBuildPushContext_CarriesTheVoiceAsAuthored(t *testing.T) {
	proj := newGovernedProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(proj.Root, "voice.yaml"), []byte(
		"name: Acme Voice\ntone:\n  formality: neutral\nchannels:\n  docs:\n    tone:\n      formality: formal\n"), 0o644))

	pushCtx, _, err := BuildPushContext(t.Context(), proj, false)
	require.NoError(t, err)

	byName := entriesByName(pushCtx.Entries)
	require.NotEmpty(t, byName["docs"].VoiceProfileJson, "the first entry carries the authored voice")
	assert.Empty(t, byName["mail"].VoiceProfileJson,
		"a second collection on the same voice carries only its name")

	var carried map[string]any
	require.NoError(t, json.Unmarshal(byName["docs"].VoiceProfileJson, &carried))
	tone, ok := carried["tone"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "neutral", tone["formality"],
		"the base tone must survive: resolving the docs channel into it would leave every other channel governed by the docs voice")
	assert.Contains(t, carried, "channels", "the overrides travel so the server can apply them itself")
}

// TestBuildPushContext_DryRunResolvesWithoutSending pins that --dry-run reports
// the governance it would carry: everything is resolved, nothing is claimed to
// have happened.
func TestBuildPushContext_DryRunResolvesWithoutSending(t *testing.T) {
	proj := newGovernedProject(t)

	pushCtx, brand, err := BuildPushContext(t.Context(), proj, true)
	require.NoError(t, err)
	require.Len(t, pushCtx.Entries, 3)
	require.NotNil(t, brand)
	assert.Equal(t, "would-push", brand.Action)
}

// TestBuildPushContext_NoCollectionsStillMakesAClaim pins the boundary that
// makes the undeclared report work. A recipe that declares no collections sends
// the empty fold, not nothing — otherwise a recipe that just dropped its last
// collection could never be told the server still holds it.
func TestBuildPushContext_NoCollectionsStillMakesAClaim(t *testing.T) {
	prev := app
	app = &cli.App{}
	t.Cleanup(func() { app = prev })

	proj, err := bproject.InitProject(t.TempDir(), &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{SourceLanguage: "en"},
			Content:  []coreproj.ContentCollection{{Path: "loose/*.json"}},
		},
	})
	require.NoError(t, err)

	pushCtx, _, err := BuildPushContext(t.Context(), proj, false)
	require.NoError(t, err)
	require.NotNil(t, pushCtx)
	assert.Empty(t, pushCtx.Entries)
	assert.Equal(t, bowsync.ComputeContextHash(nil), pushCtx.Hash)
}
