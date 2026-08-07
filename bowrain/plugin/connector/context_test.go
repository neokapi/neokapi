package connector

import (
	"os"
	"path/filepath"
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newContextProject scaffolds a project with two named collections and one
// bare entry, and writes a file for each — the three cases an item can be in
// when the push asks which collection claims it.
func newContextProject(t *testing.T) *BowrainSourceConnector {
	t.Helper()
	root := t.TempDir()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage:  "en",
				TargetLanguages: []model.LocaleID{"fr"},
			},
			Profiles: map[string]coreproj.Profile{
				"kapi": {Channels: []coreproj.Channel{{ID: "docs"}}},
			},
			Collections: []coreproj.Collection{
				{
					Name:    "docs",
					Channel: "kapi/docs",
					Content: []coreproj.ContentItem{{Path: "docs/**/*.json", Format: &coreproj.FormatSpec{Name: "json"}}},
				},
				{
					Name:    "marketing",
					Content: []coreproj.ContentItem{{Path: "marketing/**/*.json", Format: &coreproj.FormatSpec{Name: "json"}}},
				},
				{Path: "loose/*.json", Format: &coreproj.FormatSpec{Name: "json"}},
			},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	for _, rel := range []string{"docs/a.json", "marketing/b.json", "loose/c.json", "stray/d.json"} {
		abs := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hello"}`), 0o644))
	}

	return NewLocalConnector(&host.App{}, proj, reg)
}

// TestBuildItemMeta_NamesTheClaimingCollection pins the linkage the context
// content type turns on: an item carries the name of the collection whose glob
// claims it, and an item no glob claims carries none.
//
// The empty case is the one worth stating out loud. A file outside every
// declared collection is not an error and not a failure to classify — it syncs
// ungrouped, with an empty collection, and the project's default point governs
// it. Anything else would make an ad-hoc file unpushable.
func TestBuildItemMeta_NamesTheClaimingCollection(t *testing.T) {
	conn := newContextProject(t)
	ctx := t.Context()

	changed := []itemBlock{
		{itemName: "docs/a.json", block: &model.Block{ID: "b1", Translatable: true}},
		{itemName: "marketing/b.json", block: &model.Block{ID: "b2", Translatable: true}},
		{itemName: "loose/c.json", block: &model.Block{ID: "b3", Translatable: true}},
		{itemName: "stray/d.json", block: &model.Block{ID: "b4", Translatable: true}},
	}
	meta := conn.buildItemMeta(ctx, changed)

	byName := map[string]string{}
	for _, m := range meta {
		byName[m.Name] = m.Collection
	}
	assert.Equal(t, "docs", byName["docs/a.json"])
	assert.Equal(t, "marketing", byName["marketing/b.json"])
	assert.Empty(t, byName["loose/c.json"], "a bare entry declares no collection to belong to")
	assert.Empty(t, byName["stray/d.json"], "a file outside every glob syncs ungrouped, not unpushed")
}

// TestApplyPulledContext_RecipeOwnedGovernanceIsNotApplied is the pull-side
// protection. The server reports a recipe-owned collection at a different point
// than the recipe declares; the pull records the observation and reports the
// divergence, and the governance a local run resolves is exactly what kapi.yaml
// says — before the pull and after it.
func TestApplyPulledContext_RecipeOwnedGovernanceIsNotApplied(t *testing.T) {
	conn := newContextProject(t)

	before, err := conn.project.Recipe.ResolveGovernance("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", before.Channel)

	res := conn.applyPulledContext([]*pb.SyncContextEntry{{
		Name:         "docs",
		Coordinates:  map[string]string{"product": "bowrain"},
		Channel:      "email",
		VoiceProfile: "Someone Else's Voice",
		Owner:        bowsync.ContextOwnerRecipe,
	}})

	assert.Equal(t, 1, res.Observed)
	assert.Equal(t, []string{"docs"}, res.Diverged, "a recipe-owned divergence is reported")
	assert.Empty(t, res.WorkspaceOwned)

	after, err := conn.project.Recipe.ResolveGovernance("docs")
	require.NoError(t, err)
	assert.Equal(t, before.Channel, after.Channel,
		"a pull must not move the channel a local run resolves")
	assert.Equal(t, "kapi/docs", conn.project.Recipe.Collections[0].Channel,
		"a pull must not rewrite the recipe's declared point")

	// What it did do is record the observation, which is what lets the
	// divergence be reported rather than silently absorbed.
	assert.Equal(t, bproject.ServerCollection{
		Coordinates:  map[string]string{"product": "bowrain"},
		Channel:      "email",
		VoiceProfile: "Someone Else's Voice",
		Owner:        bowsync.ContextOwnerRecipe,
	}, conn.cache.ServerContext["docs"])
}

// TestApplyPulledContext_AgreementIsNotADivergence keeps the report honest: a
// server holding a recipe-owned collection exactly as the recipe declares it
// must say nothing, or the warning becomes noise on every pull.
func TestApplyPulledContext_AgreementIsNotADivergence(t *testing.T) {
	conn := newContextProject(t)

	res := conn.applyPulledContext([]*pb.SyncContextEntry{{
		Name:        "docs",
		Coordinates: map[string]string{"product": "kapi", "channel": "docs"},
		Channel:     "docs",
		Owner:       bowsync.ContextOwnerRecipe,
	}})

	assert.Equal(t, 1, res.Observed)
	assert.Empty(t, res.Diverged)
}

// TestApplyPulledContext_WorkspaceOwnedIsRecorded covers the other side of the
// ownership lookup: a collection the workspace governs has no local governance
// to conflict with, so it is recorded and reported as the server's without any
// divergence claim.
func TestApplyPulledContext_WorkspaceOwnedIsRecorded(t *testing.T) {
	conn := newContextProject(t)

	res := conn.applyPulledContext([]*pb.SyncContextEntry{{
		Name:        "uploads",
		Coordinates: map[string]string{"product": "bowrain"},
		Owner:       bowsync.ContextOwnerWorkspace,
	}})

	assert.Equal(t, []string{"uploads"}, res.WorkspaceOwned)
	assert.Empty(t, res.Diverged)
	assert.Equal(t, bowsync.ContextOwnerWorkspace, conn.cache.ServerContext["uploads"].Owner)
}

// TestApplyPulledContext_UnownedEntryDefaultsToWorkspace pins the conservative
// default at the boundary: an entry from a server that predates the ownership
// field must not be read as recipe-owned, which would hand authority over it to
// a kapi.yaml that never mentioned it.
func TestApplyPulledContext_UnownedEntryDefaultsToWorkspace(t *testing.T) {
	conn := newContextProject(t)

	res := conn.applyPulledContext([]*pb.SyncContextEntry{{Name: "docs"}})

	assert.Equal(t, []string{"docs"}, res.WorkspaceOwned)
	assert.Equal(t, bowsync.ContextOwnerWorkspace, conn.cache.ServerContext["docs"].Owner)
}

// TestPushContextChanged_TracksTheRecipe pins the local half of the fast path:
// an unedited recipe is not worth a round trip, and an edited one must reach
// the server even when no content moved.
func TestPushContextChanged_TracksTheRecipe(t *testing.T) {
	conn := newContextProject(t)
	assert.False(t, conn.PushContextChanged(), "no declared context is not a change")

	entry := &pb.SyncContextEntry{Name: "docs", Owner: bowsync.ContextOwnerRecipe}
	conn.SetPushContext(newTestPushContext(entry))
	assert.True(t, conn.PushContextChanged(), "a context the cache has never seen is a change")

	conn.cache.ContextHash = bowsync.ContextHashOf([]*pb.SyncContextEntry{entry})
	assert.False(t, conn.PushContextChanged(), "the same context again is not a change")

	conn.SetPushContext(newTestPushContext(entry, &pb.SyncContextEntry{
		Name: "marketing", Owner: bowsync.ContextOwnerRecipe,
	}))
	assert.True(t, conn.PushContextChanged(), "a newly declared collection is a change")
}

// newTestPushContext builds the push context a caller would hand the connector.
func newTestPushContext(entries ...*pb.SyncContextEntry) *apiclient.PushContext {
	return apiclient.NewPushContext(entries)
}
