package jobs

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rejection that reaches the platform by push re-drafts its unit the way a
// rejection in the editor does: it clears the platform's mark that it has
// drafted the unit against the current source, so the next run drafts the unit
// once more. Only a rejection the push lands, that the ledger does not already
// hold, and that judges the translation the venue holds now clears the mark.
// These run the push worker and the translation worker against one PostgreSQL
// store.

const (
	redraftSource = "Delete account"
	redraftLocale = "fr"
	redraftUnit   = "b1"
)

type rejectionRedraft struct {
	t    *testing.T
	deps *WorkerDeps
	pid  string
	item string
}

// newRejectionRedraft pushes one source-only unit into a fresh project.
func newRejectionRedraft(t *testing.T) *rejectionRedraft {
	t.Helper()
	deps := newTestWorkerDeps(t)
	deps.Platform = &PlatformProviderConfig{Provider: "demo"}
	deps.ProviderStore = &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}}
	r := &rejectionRedraft{t: t, deps: deps, pid: "redraft-" + t.Name(), item: "en.json"}
	require.NoError(t, deps.ContentStore.CreateProject(t.Context(), &store.Project{
		ID:                    r.pid,
		Name:                  "Redraft",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{redraftLocale},
		Properties:            map[string]string{store.SourceGateProperty: string(model.SourceGateNone)},
	}))
	source := &model.Block{ID: redraftUnit, Name: redraftUnit, Translatable: true}
	source.SetSourceText(redraftSource)
	require.NoError(t, governedPush{
		projectID: r.pid, actor: "u-developer", item: r.item, blocks: []*model.Block{source},
	}.run(t, deps, "job-source"))
	return r
}

// draft runs a translation job for the locale and reports how many units it
// found owed.
func (r *rejectionRedraft) draft(id string) int {
	r.t.Helper()
	ctx := r.t.Context()
	job := &TranslationJob{
		ID: id, WorkspaceSlug: "acme", ProjectID: r.pid, ItemName: r.item,
		TargetLocale: redraftLocale, ProviderConfigID: "platform", Model: "demo", Status: StatusQueued,
	}
	require.NoError(r.t, r.deps.JobStore.CreateJob(ctx, job))
	claimed, epoch, err := r.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(r.t, err)
	require.True(r.t, claimed)
	require.NoError(r.t, executeTranslationWithDeps(ctx, r.deps, job, epoch))
	done, err := r.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(r.t, err)
	return done.TotalBlocks
}

// target is the translation the venue holds for the unit.
func (r *rejectionRedraft) target() string {
	r.t.Helper()
	rows, err := r.deps.ContentStore.GetBlocks(r.t.Context(), store.BlockQuery{
		ProjectID: r.pid, Stream: "main", ItemName: r.item, Limit: 10,
	})
	require.NoError(r.t, err)
	require.Len(r.t, rows, 1)
	return rows[0].Block.TargetText(redraftLocale)
}

// mark is the source the platform's latest draft of the unit was made against,
// or "" when no mark stands.
func (r *rejectionRedraft) mark() string {
	r.t.Helper()
	ds, ok := r.deps.ContentStore.(store.DecisionStore)
	require.True(r.t, ok)
	drafts, err := ds.ListDraftBases(r.t.Context(), r.pid, "main")
	require.NoError(r.t, err)
	for _, d := range drafts {
		if d.Unit == redraftUnit && d.Variant == redraftLocale {
			return d.SourceHash
		}
	}
	return ""
}

// verdictPush sends the unit's translation at one rung with a record carrying
// the given review state, from actor, under the given review permissions.
func (r *rejectionRedraft) verdictPush(jobID, actor string, permits map[string]bool, text, recorded string, rung model.TargetStatus, reviewState string, updated time.Duration) {
	r.t.Helper()
	r.deps.ReviewAuthority = pushAuthority{review: permits}
	at := time.Now().UTC().Add(updated).Format(time.RFC3339)
	record := venue.UnitDecision{
		ItemName: r.item, Unit: redraftUnit, Variant: redraftLocale,
		Status: string(rung), ReviewState: reviewState,
		TargetHash: state.TargetHash(recorded), ContentHash: state.SourceHash(redraftSource),
		DecidedBy: actor, DecidedAt: at, Updated: at,
	}
	require.NoError(r.t, governedPush{
		projectID: r.pid, actor: actor, item: r.item,
		blocks:    []*model.Block{reviewedBlock(redraftUnit, redraftSource, redraftLocale, text, rung)},
		decisions: []venue.UnitDecision{record},
	}.run(r.t, r.deps, jobID))
}

func TestPushedRejectionRedrafts(t *testing.T) {
	sourceHash := state.SourceHash(redraftSource)

	t.Run("a rejection the push lands clears the mark and the next run drafts once", func(t *testing.T) {
		r := newRejectionRedraft(t)
		require.Equal(t, 1, r.draft("job-draft-1"), "the first run drafts the unit")
		drafted := r.target()
		require.NotEmpty(t, drafted)
		require.Equal(t, sourceHash, r.mark(), "and marks the source it drafted against")
		require.Zero(t, r.draft("job-draft-2"), "nothing is owed before the rejection")

		r.verdictPush("job-reject", "u-translator", map[string]bool{}, drafted, drafted,
			model.TargetStatusDraft, venue.ReviewStateRejected, time.Hour)
		d, ok := heldDecision(t, r.deps, r.pid, redraftUnit, redraftLocale)
		require.True(t, ok)
		assert.Equal(t, venue.ReviewStateRejected, d.ReviewState, "the rejection is recorded")
		assert.True(t, jobGovernance(t, r.deps, "push-job-reject").Empty(), "and nothing was refused")
		assert.Empty(t, r.mark(), "the rejection the push landed clears the mark")

		assert.Equal(t, 1, r.draft("job-draft-3"), "the next run drafts the rejected unit")
		assert.Equal(t, sourceHash, r.mark(), "and marks the new draft")
		assert.Zero(t, r.draft("job-draft-4"), "once")

		// A producer that has not pulled sends the same record again, with the
		// translation it holds. It is not a new decision.
		r.verdictPush("job-reject-again", "u-translator", map[string]bool{}, drafted, drafted,
			model.TargetStatusDraft, venue.ReviewStateRejected, time.Hour)
		assert.Equal(t, sourceHash, r.mark(), "a rejection sent again clears nothing")
		assert.Zero(t, r.draft("job-draft-5"), "and buys no second draft")
	})

	t.Run("a rejection the push gate refuses leaves the mark", func(t *testing.T) {
		r := newRejectionRedraft(t)
		require.Equal(t, 1, r.draft("job-draft-1"))
		drafted := r.target()

		r.verdictPush("job-approve", "u-reviewer", map[string]bool{redraftLocale: true}, drafted, drafted,
			model.TargetStatusEstablished, venue.ReviewStateApproved, time.Hour)
		require.Equal(t, model.TargetStatusEstablished, storedTarget(t, r.deps, r.pid, r.item, redraftLocale))
		require.Equal(t, sourceHash, r.mark(), "an approval leaves the mark as it was")

		r.verdictPush("job-reject-refused", "u-translator", map[string]bool{}, drafted, drafted,
			model.TargetStatusDraft, venue.ReviewStateRejected, 2*time.Hour)
		assert.Equal(t, model.TargetStatusEstablished, storedTarget(t, r.deps, r.pid, r.item, redraftLocale),
			"without review permission the approval stands")
		d, ok := heldDecision(t, r.deps, r.pid, redraftUnit, redraftLocale)
		require.True(t, ok)
		assert.Equal(t, venue.ReviewStateApproved, d.ReviewState)
		assert.Len(t, jobGovernance(t, r.deps, "push-job-reject-refused").Refusals, 1, "the refusal is reported")
		assert.Equal(t, sourceHash, r.mark(), "a refused rejection leaves the mark")
		assert.Zero(t, r.draft("job-draft-2"), "and buys no draft")
	})

	t.Run("a rejection of a translation the venue no longer holds clears nothing", func(t *testing.T) {
		r := newRejectionRedraft(t)
		require.Equal(t, 1, r.draft("job-draft-1"))
		drafted := r.target()

		r.verdictPush("job-reject-stale", "u-translator", map[string]bool{}, drafted, "Effacer le compte",
			model.TargetStatusDraft, venue.ReviewStateRejected, time.Hour)
		assert.Equal(t, sourceHash, r.mark(), "the rejection judges a translation the venue has replaced")
		assert.Zero(t, r.draft("job-draft-2"), "so it buys no draft")
	})
}
