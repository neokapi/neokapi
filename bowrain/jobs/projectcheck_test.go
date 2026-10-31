package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	fwterms "github.com/neokapi/neokapi/terms"
)

// TestProjectCheck_EndToEnd runs a project-wide check through the durable
// worker path over the PostgreSQL stores. Every item of the stream is checked
// at the source language under the project's voice and the workspace's
// terms. Each item's tally is written, and the job reports its progress in
// items and completes.
func TestProjectCheck_EndToEnd(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)

	const (
		projectID = "proj-check"
		wsSlug    = "acme-check"
		wsID      = "ws-check"
	)
	bs, err := voicepg.NewPostgresVoiceStore(db)
	require.NoError(t, err)
	profile := &coreprofile.VoiceProfile{
		Scope: wsID,
		Name:  "Acme Voice",
		Style: coreprofile.StyleRules{
			ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bLocalizely\b`, Description: "Name no competitor"}},
		},
	}
	require.NoError(t, bs.CreateProfile(ctx, profile))
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID: projectID, Name: "Check Test", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"}, WorkspaceID: wsID,
		Properties: map[string]string{coreprofile.PropertyProfileID: profile.ID},
	}))

	tb, err := sqlterms.NewPostgresStoreFromDB(db, wsSlug)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, fwterms.Concept{
		ID: "c-affordable",
		Terms: []fwterms.Term{
			{Text: "affordable", Locale: "en", Status: model.TermPreferred},
			{Text: "cheap", Locale: "en", Status: model.TermForbidden},
		},
	}))

	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &store.Item{Name: "docs/a.md", Format: "markdown", ItemType: "file"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "docs/a.md", []*model.Block{
		model.NewBlock("a-1", "The cheap plan is ready."),
		model.NewBlock("a-2", "Faster than Localizely."),
		model.NewBlock("a-3", "Nothing to say here."),
	}))
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &store.Item{Name: "docs/b.md", Format: "markdown", ItemType: "file"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "docs/b.md", []*model.Block{
		model.NewBlock("b-1", "An affordable plan."),
	}))

	deps := &WorkerDeps{
		JobStore:     js,
		ContentStore: cs,
		VoiceStore:   bs,
		TermsResolver: TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
			return sqlterms.NewPostgresStoreFromDB(db, slug)
		}),
	}
	job := NewProjectCheckJob(wsID, wsSlug, projectID, "", "member-a")
	assert.Equal(t, "project-check", job.Kind())
	require.NoError(t, js.CreateJob(ctx, job))
	require.NoError(t, processJobWithDeps(ctx, deps, job.ID))

	got, err := js.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status, got.Error)
	assert.Equal(t, 2, got.DoneBlocks, "progress counts items")
	assert.Equal(t, 2, got.TotalBlocks)
	assert.Equal(t, 100, got.Progress)

	items, err := cs.ListItems(ctx, projectID, "main")
	require.NoError(t, err)
	idOf := map[string]string{}
	for _, it := range items {
		idOf[it.Name] = it.ID
	}
	tallies, err := cs.ListItemCheckTallies(ctx, projectID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 2)
	byItem := map[string]store.ItemCheckTally{}
	for _, tl := range tallies {
		byItem[tl.ItemID] = tl
		assert.False(t, tl.CheckedAt.IsZero())
	}
	a := byItem[idOf["docs/a.md"]]
	assert.Equal(t, 2, a.Findings, "the forbidden term and the competitor's name")
	assert.Equal(t, 1, a.Fixable, "the term rule names a replacement; the pattern none")
	b := byItem[idOf["docs/b.md"]]
	assert.Zero(t, b.Findings)
	assert.Zero(t, b.Fixable)

	// An item deleted since takes its tally with it at the next check.
	require.NoError(t, cs.DeleteItem(ctx, projectID, "main", "docs/b.md"))
	again := NewProjectCheckJob(wsID, wsSlug, projectID, "main", "member-a")
	require.NoError(t, js.CreateJob(ctx, again))
	require.NoError(t, processJobWithDeps(ctx, deps, again.ID))
	tallies, err = cs.ListItemCheckTallies(ctx, projectID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Equal(t, idOf["docs/a.md"], tallies[0].ItemID)
}

// TestProjectCheck_EmptyStreamIsDone: a check of a stream with no items
// completes with its progress at the end, not at the start.
func TestProjectCheck_EmptyStreamIsDone(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)
	require.NoError(t, cs.CreateProject(ctx, &store.Project{ID: "proj-empty", Name: "Empty", DefaultSourceLanguage: "en", WorkspaceID: "ws-empty"}))

	job := NewProjectCheckJob("ws-empty", "empty", "proj-empty", "main", "member-a")
	require.NoError(t, js.CreateJob(ctx, job))
	require.NoError(t, processJobWithDeps(ctx, &WorkerDeps{JobStore: js, ContentStore: cs}, job.ID))
	got, err := js.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status, got.Error)
	assert.Equal(t, 100, got.Progress)
}
