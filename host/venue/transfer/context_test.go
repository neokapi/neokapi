package transfer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newGovernedProject scaffolds a project whose context space binds one voice to
// two collections and leaves a third ungoverned, plus a bare entry that declares
// no collection at all.
func newGovernedProject(t *testing.T) (*host.App, *bproject.Project) {
	t.Helper()
	app := &host.App{}

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "acme-voice.yaml"), []byte(
		"name: Acme Voice\ndescription: How Acme sounds.\ntone:\n  formality: neutral\n"), 0o644))

	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
		},
		Profiles: map[string]coreproj.Profile{
			"kapi": {
				Channels: []coreproj.Channel{{ID: "docs"}, {ID: "email"}},
				Voice:    &coreproj.VoiceBinding{ProfileFile: "acme-voice.yaml"},
			},
			"bowrain": {Channels: []coreproj.Channel{{ID: "partners"}}},
		},
		Collections: []coreproj.Collection{
			{
				Name:    "docs",
				Channel: "kapi/docs",
				Content: []coreproj.ContentItem{{Path: "docs/**/*.json"}},
			},
			{
				Name:    "mail",
				Channel: "kapi/email",
				Content: []coreproj.ContentItem{{Path: "mail/**/*.json"}},
			},
			{
				Name:    "partner",
				Channel: "bowrain/partners",
				Content: []coreproj.ContentItem{{Path: "partner/**/*.json"}},
			},
			{Path: "loose/*.json"},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	return app, proj
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
	app, proj := newGovernedProject(t)

	pushCtx, brand, err := BuildPushContext(t.Context(), app, proj, false)
	require.NoError(t, err)
	require.NotNil(t, pushCtx)
	require.Len(t, pushCtx.Entries, 3, "the bare entry declares no collection")

	byName := entriesByName(pushCtx.Entries)
	docs := byName["docs"]
	require.NotNil(t, docs)
	assert.Equal(t, map[string]string{"product": "kapi", "channel": "docs"}, docs.Coordinates)
	assert.Equal(t, "docs", docs.Channel)
	assert.Equal(t, venue.ContextOwnerRecipe, docs.Owner,
		"a collection the recipe declares is git's, and says so on the wire")
	assert.Equal(t, "Acme Voice", docs.VoiceProfile)

	assert.Equal(t, "email", byName["mail"].Channel)
	assert.Equal(t, "Acme Voice", byName["mail"].VoiceProfile,
		"a profile governs every channel its product ships on")

	assert.Empty(t, byName["partner"].VoiceProfile,
		"a profile that binds no voice binds none for its collections")

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
	app, proj := newGovernedProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(proj.Root, "acme-voice.yaml"), []byte(
		"name: Acme Voice\ntone:\n  formality: neutral\nchannels:\n  docs:\n    tone:\n      formality: formal\n"), 0o644))

	pushCtx, _, err := BuildPushContext(t.Context(), app, proj, false)
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
	app, proj := newGovernedProject(t)

	pushCtx, brand, err := BuildPushContext(t.Context(), app, proj, true)
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
	app := &host.App{}

	proj, err := bproject.InitProject(t.TempDir(), &bproject.Recipe{
		Defaults:    coreproj.Defaults{SourceLanguage: "en"},
		Collections: []coreproj.Collection{{Path: "loose/*.json"}},
	})
	require.NoError(t, err)

	pushCtx, _, err := BuildPushContext(t.Context(), app, proj, false)
	require.NoError(t, err)
	require.NotNil(t, pushCtx)
	assert.Empty(t, pushCtx.Entries)
	assert.Equal(t, venue.ComputeContextHash(nil), pushCtx.Hash)
}

// TestBuildPushContext_ExpiredProfileCarriesNoCoordinates pins the push half of
// profile validity: a push declares the governance actually in force, so a
// collection whose profile has lapsed carries the coordinates it fell through
// to — not a point the recipe names but nothing applies. The control is the same
// recipe with the window open.
func TestBuildPushContext_ExpiredProfileCarriesNoCoordinates(t *testing.T) {
	tests := []struct {
		name            string
		validTo         string
		wantCoordinates map[string]string
		wantChannel     string
	}{
		{
			name:            "an expired profile stops placing its content",
			validTo:         "2020-01-01",
			wantCoordinates: nil,
			wantChannel:     "",
		},
		{
			name:            "an open window is untouched",
			validTo:         "",
			wantCoordinates: map[string]string{"product": "kapi", "channel": "docs"},
			wantChannel:     "docs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, proj := newGovernedProject(t)
			prof := proj.Recipe.Profiles["kapi"]
			prof.ValidTo = tt.validTo
			proj.Recipe.Profiles["kapi"] = prof

			pushCtx, _, err := BuildPushContext(t.Context(), app, proj, false)
			require.NoError(t, err)

			docs := entriesByName(pushCtx.Entries)["docs"]
			require.NotNil(t, docs)
			assert.Equal(t, tt.wantCoordinates, docs.Coordinates)
			assert.Equal(t, tt.wantChannel, docs.Channel)
		})
	}
}
