package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coreproj "github.com/neokapi/neokapi/core/project"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{
		Name:         "docs",
		Coordinates:  map[string]string{"product": "bowrain"},
		Channel:      "email",
		VoiceProfile: "Someone Else's Voice",
		Owner:        venue.ContextOwnerRecipe,
	}})

	assert.Equal(t, 1, res.Observed)
	assert.Equal(t, []string{"docs"}, res.Diverged, "a recipe-owned divergence is reported")
	assert.Equal(t, []string{"docs: point, channel, voice"}, res.DivergedDetail,
		"every differing part is named")
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
		Owner:        venue.ContextOwnerRecipe,
	}, conn.cache.ServerContext["docs"])
}

// TestApplyPulledContext_AgreementIsNotADivergence keeps the report honest: a
// server holding a recipe-owned collection exactly as the recipe declares it
// must say nothing, or the warning becomes noise on every pull.
func TestApplyPulledContext_AgreementIsNotADivergence(t *testing.T) {
	conn := newContextProject(t)

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{
		Name:        "docs",
		Coordinates: map[string]string{"product": "kapi", "channel": "docs"},
		Channel:     "docs",
		Owner:       venue.ContextOwnerRecipe,
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

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{
		Name:        "uploads",
		Coordinates: map[string]string{"product": "bowrain"},
		Owner:       venue.ContextOwnerWorkspace,
	}})

	assert.Equal(t, []string{"uploads"}, res.WorkspaceOwned)
	assert.Empty(t, res.Diverged)
	assert.Equal(t, venue.ContextOwnerWorkspace, conn.cache.ServerContext["uploads"].Owner)
}

// TestApplyPulledContext_UnownedEntryDefaultsToWorkspace pins the conservative
// default at the boundary: an entry from a server that predates the ownership
// field must not be read as recipe-owned, which would hand authority over it to
// a kapi.yaml that never mentioned it.
func TestApplyPulledContext_UnownedEntryDefaultsToWorkspace(t *testing.T) {
	conn := newContextProject(t)

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{Name: "docs"}})

	assert.Equal(t, []string{"docs"}, res.WorkspaceOwned)
	assert.Equal(t, venue.ContextOwnerWorkspace, conn.cache.ServerContext["docs"].Owner)
}

// The voice used to be excluded from the comparison, so a collection whose
// voice had been changed server-side pulled clean: not applied — which is
// correct, a file-bound voice is git's — and not reported either, which is not.
// The divergence a pull cannot resolve is exactly the one it must name.

// bindCollectionVoice writes a voice profile file and binds it to the docs
// collection, the way a project whose voice lives in git does.
func bindCollectionVoice(t *testing.T, conn *BowrainSourceConnector, name string) {
	t.Helper()
	root := conn.project.Root
	rel := filepath.Join(".kapi", "voice.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
	body, err := yaml.Marshal(&coreprofile.VoiceProfile{Name: name})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, rel), body, 0o644))
	conn.project.Recipe.KapiProject.Defaults.Voice = &coreproj.VoiceBinding{ProfileFile: rel}
}

// TestApplyPulledContext_VoiceDivergenceIsReported: the server governs a
// recipe-owned collection by a different voice than the recipe binds. The pull
// leaves the local binding alone and says so.
func TestApplyPulledContext_VoiceDivergenceIsReported(t *testing.T) {
	conn := newContextProject(t)
	bindCollectionVoice(t, conn, "Kapi Docs Voice")

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{
		Name:         "docs",
		Coordinates:  map[string]string{"product": "kapi", "channel": "docs"},
		Channel:      "docs",
		VoiceProfile: "Someone Else's Voice",
		Owner:        venue.ContextOwnerRecipe,
	}})

	assert.Equal(t, []string{"docs"}, res.Diverged,
		"a voice the recipe does not bind must be reported, not absorbed and not applied")
	assert.Equal(t, []string{"docs: voice"}, res.DivergedDetail,
		"the report must name the part that differs, or the reader diffs two sides they cannot see")

	// Report, never resolve: the profile a local run resolves is the recipe's.
	profile, _, _, found, err := conn.app.LoadCollectionVoice(t.Context(),
		&conn.project.Recipe.KapiProject, conn.project.Root, host.VoiceResolveOptions{Point: coreproj.GovernancePoint{Collection: "docs"}})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "Kapi Docs Voice", profile.Name, "a pull must not move the voice a local run resolves")
}

// TestApplyPulledContext_SameVoiceIsSilent keeps the report honest in the case
// the old exclusion existed for: a project whose voice is a FILE, agreeing with
// the server, must say nothing — or the warning is noise on every pull.
func TestApplyPulledContext_SameVoiceIsSilent(t *testing.T) {
	conn := newContextProject(t)
	bindCollectionVoice(t, conn, "Kapi Docs Voice")

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{
		Name:         "docs",
		Coordinates:  map[string]string{"product": "kapi", "channel": "docs"},
		Channel:      "docs",
		VoiceProfile: "Kapi Docs Voice",
		Owner:        venue.ContextOwnerRecipe,
	}})

	assert.Equal(t, 1, res.Observed)
	assert.Empty(t, res.Diverged, "a file-bound voice that matches the hub's name is agreement")
}

// TestApplyPulledContext_VoiceAppearingServerSideIsReported: the recipe binds
// no voice and the server governs the collection by one. The recipe is
// authoritative for a recipe-owned collection, so that is a divergence too.
func TestApplyPulledContext_VoiceAppearingServerSideIsReported(t *testing.T) {
	conn := newContextProject(t)

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{
		Name:         "docs",
		Coordinates:  map[string]string{"product": "kapi", "channel": "docs"},
		Channel:      "docs",
		VoiceProfile: "Workspace Default",
		Owner:        venue.ContextOwnerRecipe,
	}})

	assert.Equal(t, []string{"docs"}, res.Diverged)
}

// TestApplyPulledContext_WorkspaceOwnedVoiceIsNotADivergence: ownership decides
// first. A workspace-owned collection has no local governance to conflict with,
// whatever voice it carries.
func TestApplyPulledContext_WorkspaceOwnedVoiceIsNotADivergence(t *testing.T) {
	conn := newContextProject(t)
	bindCollectionVoice(t, conn, "Kapi Docs Voice")

	res := conn.applyPulledContext(t.Context(), []*pb.SyncContextEntry{{
		Name:         "docs",
		Coordinates:  map[string]string{"product": "kapi", "channel": "docs"},
		Channel:      "docs",
		VoiceProfile: "Someone Else's Voice",
		Owner:        venue.ContextOwnerWorkspace,
	}})

	assert.Empty(t, res.Diverged)
	assert.Equal(t, []string{"docs"}, res.WorkspaceOwned)
}

// TestPushContextChanged_TracksTheRecipe pins the local half of the fast path:
// an unedited recipe is not worth a round trip, and an edited one must reach
// the server even when no content moved.
func TestPushContextChanged_TracksTheRecipe(t *testing.T) {
	conn := newContextProject(t)
	assert.False(t, conn.PushContextChanged(), "no declared context is not a change")

	entry := &pb.SyncContextEntry{Name: "docs", Owner: venue.ContextOwnerRecipe}
	conn.SetPushContext(newTestPushContext(entry))
	assert.True(t, conn.PushContextChanged(), "a context the cache has never seen is a change")

	conn.refs.SetIdentity(conn.stream, ref.ComponentContext, venue.ContextHashOf([]*pb.SyncContextEntry{entry}))
	assert.False(t, conn.PushContextChanged(), "the same context again is not a change")

	conn.SetPushContext(newTestPushContext(entry, &pb.SyncContextEntry{
		Name: "marketing", Owner: venue.ContextOwnerRecipe,
	}))
	assert.True(t, conn.PushContextChanged(), "a newly declared collection is a change")
}

// newTestPushContext builds the push context a caller would hand the connector.
func newTestPushContext(entries ...*pb.SyncContextEntry) *apiclient.PushContext {
	return apiclient.NewPushContext(entries)
}
