package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/review"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/state"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// The platform is authoritative for review governance: a push moves content, it
// does not decide. These tests are the proof, and they run the whole worker
// against a real store, because every part of the answer is persistence
// rather than arithmetic: the rung on a decoded block, the ledger row, the
// authorship the policy reads.

// pushAuthority is a review.Authority with the answers stated rather than
// resolved: which languages the pusher may review, and what the workspace
// policy is.
type pushAuthority struct {
	review   map[string]bool
	mode     platauth.SoDMode
	failWith error
}

func (a pushAuthority) GetSoDMode(context.Context, string) (platauth.SoDMode, error) {
	if a.mode == "" {
		return platauth.SoDOff, nil
	}
	return a.mode, nil
}

func (a pushAuthority) AllowsLanguage(_ context.Context, q review.Query) (bool, error) {
	if a.failWith != nil {
		return false, a.failWith
	}
	return a.review[q.Locale], nil
}

// governedPush is one push, as the worker receives it: blocks, decision
// records, and the authenticated pusher who sent them.
type governedPush struct {
	projectID string
	actor     string
	blocks    []*model.Block
	item      string
	decisions []venue.UnitDecision
}

// run puts the push through the worker and returns the job's outcome.
func (p governedPush) run(t *testing.T, deps *WorkerDeps, jobID string) error {
	t.Helper()
	ctx := t.Context()

	syncBlocks := make([]*pb.SyncBlock, 0, len(p.blocks))
	for _, b := range p.blocks {
		syncBlocks = append(syncBlocks, venue.BlockToProto(b, p.item))
	}
	chunk := &pb.SyncChunk{ContentType: "blocks", RecordCount: int32(len(syncBlocks)), Blocks: syncBlocks}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)
	sum := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(sum[:])
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	items, _ := json.Marshal([]map[string]string{{"name": p.item, "format": "json"}})
	decisions, _ := json.Marshal(p.decisions)
	manifest := map[string]any{
		"project_id": p.projectID,
		"stream":     "main",
		"actor_id":   p.actor,
		"chunks": []map[string]any{{
			"index": 0, "content_type": "blocks", "hash": chunkHash,
			"record_count": len(syncBlocks), "byte_size": len(chunkData),
		}},
		"items":     json.RawMessage(items),
		"decisions": json.RawMessage(decisions),
	}
	manifestData, _ := json.Marshal(manifest)
	ref, err := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})
	require.NoError(t, err)

	job := &TranslationJob{
		ID: jobID, ProjectID: p.projectID, ItemName: SyncPushItemName,
		Model: ref.Key, PushID: "push-" + jobID, Status: StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))
	return ProcessSyncPushJobForTest(ctx, deps, job.ID)
}

// storedTarget is the rung the venue holds for one block's locale.
func storedTarget(t *testing.T, deps *WorkerDeps, projectID, item, locale string) model.TargetStatus {
	t.Helper()
	rows, err := deps.ContentStore.GetBlocks(t.Context(), store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: item, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	target := rows[0].Block.Target(model.LocaleID(locale))
	require.NotNil(t, target, "the content must land whatever the gate said about its rung")
	return target.Status
}

// heldDecision is the ledger row for one unit, or false when the venue holds
// none.
func heldDecision(t *testing.T, deps *WorkerDeps, projectID, unit, variant string) (venue.UnitDecision, bool) {
	t.Helper()
	ds, ok := deps.ContentStore.(store.DecisionStore)
	require.True(t, ok)
	rows, err := ds.ListUnitDecisions(t.Context(), projectID, "main")
	require.NoError(t, err)
	for _, d := range rows {
		if d.Unit == unit && d.Variant == variant {
			return d, true
		}
	}
	return venue.UnitDecision{}, false
}

// reviewedBlock is one translated, approved unit as a push carries it.
func reviewedBlock(id, source, locale, target string, status model.TargetStatus) *model.Block {
	b := &model.Block{ID: id, Name: id, Translatable: true}
	b.SetSourceText(source)
	b.SetTargetText(model.LocaleID(locale), target)
	b.Target(model.LocaleID(locale)).Status = status
	return b
}

func TestPushReviewGovernance(t *testing.T) {
	const item = "en.json"
	const locale = "fr"

	// The unit's durable identity is the store's, so the decision records are
	// built after the first push has told us what it is.
	setup := func(t *testing.T, auth review.Authority) (*WorkerDeps, string) {
		t.Helper()
		deps := newTestWorkerDeps(t)
		deps.ReviewAuthority = auth
		projectID := "gov-" + t.Name()
		require.NoError(t, deps.ContentStore.CreateProject(t.Context(),
			&store.Project{ID: projectID, Name: "Governed"}))
		return deps, projectID
	}

	t.Run("a reviewer's approval is stored, and attributed to them", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{review: map[string]bool{locale: true}})
		push := governedPush{
			projectID: pid, actor: "u-reviewer", item: item,
			blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
			decisions: []venue.UnitDecision{{
				ItemName: item, Unit: "b1", Variant: locale,
				Status: string(model.TargetStatusReviewed), ReviewState: venue.ReviewStateApproved,
				DecidedBy: "someone-else@example.com", Updated: "2026-09-03T10:00:00Z",
			}},
		}
		require.NoError(t, push.run(t, deps, "job-approve"))

		assert.Equal(t, model.TargetStatusReviewed, storedTarget(t, deps, pid, item, locale))
		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok, "an approval the pusher may make is recorded")
		assert.Equal(t, venue.ReviewStateApproved, d.ReviewState)
		assert.Equal(t, "u-reviewer", d.DecidedBy,
			"the decider is the authenticated pusher, never the string the client sent")
	})

	t.Run("without review permission the rung is refused and the content lands", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{review: map[string]bool{}})
		push := governedPush{
			projectID: pid, actor: "u-translator", item: item,
			blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
			decisions: []venue.UnitDecision{{
				ItemName: item, Unit: "b1", Variant: locale,
				Status: string(model.TargetStatusReviewed), ReviewState: venue.ReviewStateApproved,
				DecidedBy: "u-translator", Updated: "2026-09-03T10:00:00Z",
			}},
		}
		require.NoError(t, push.run(t, deps, "job-refused"), "a refused rung never fails the push")

		assert.Equal(t, model.TargetStatusTranslated, storedTarget(t, deps, pid, item, locale),
			"the translation lands; the rung above translated does not")
		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok, "the basis is kept: the venue knows the translation exists")
		assert.Empty(t, d.ReviewState, "no verdict is recorded for a pusher who may not review")
		assert.Empty(t, d.DecidedBy)
		assert.Equal(t, string(model.TargetStatusTranslated), d.Status)
	})

	t.Run("a sign-off without permission is refused as a sign-off", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{review: map[string]bool{}})
		push := governedPush{
			projectID: pid, actor: "u-translator", item: item,
			blocks: []*model.Block{reviewedBlock("b1", "Hello", "de", "Guten Tag", model.TargetStatusSignedOff)},
		}
		require.NoError(t, push.run(t, deps, "job-signoff"))
		assert.Equal(t, model.TargetStatusTranslated, storedTarget(t, deps, pid, item, "de"))

		report := jobGovernance(t, deps, "push-job-signoff")
		require.Len(t, report.Refusals, 1)
		assert.Equal(t, venue.VerdictSignOff, report.Refusals[0].Kind)
		assert.Equal(t, "de", report.Refusals[0].Locale)
		assert.Equal(t, venue.RefusedNoReviewPermission, report.Refusals[0].Reason)
		assert.Equal(t, 1, report.Refusals[0].Count)
	})

	t.Run("separation of duties refuses the pusher's own writing", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{
			review: map[string]bool{locale: true}, mode: platauth.SoDBlock,
		})
		// The pusher wrote this translation by hand: the venue holds a target
		// it attributes to them.
		seedHandWritten(t, deps, pid, item, "u-author", locale, "Bonjour")

		push := governedPush{
			projectID: pid, actor: "u-author", item: item,
			blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
		}
		require.NoError(t, push.run(t, deps, "job-sod"))
		assert.Equal(t, model.TargetStatusTranslated, storedTarget(t, deps, pid, item, locale))

		report := jobGovernance(t, deps, "push-job-sod")
		require.Len(t, report.Refusals, 1)
		assert.Equal(t, venue.RefusedSeparationOfDuties, report.Refusals[0].Reason)
	})

	t.Run("a warning policy records the conflict and accepts the rung", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{
			review: map[string]bool{locale: true}, mode: platauth.SoDWarn,
		})
		seedHandWritten(t, deps, pid, item, "u-author", locale, "Bonjour")

		push := governedPush{
			projectID: pid, actor: "u-author", item: item,
			blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
		}
		require.NoError(t, push.run(t, deps, "job-sod-warn"))
		assert.Equal(t, model.TargetStatusReviewed, storedTarget(t, deps, pid, item, locale))
		assert.True(t, jobGovernance(t, deps, "push-job-sod-warn").Empty(),
			"warn allows the verdict, so there is nothing to report as refused")
	})

	t.Run("re-pushing a verdict the venue already holds is not a decision", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{review: map[string]bool{locale: true}})
		approved := venue.UnitDecision{
			ItemName: item, Unit: "b1", Variant: locale,
			Status: string(model.TargetStatusReviewed), ReviewState: venue.ReviewStateApproved,
			DecidedBy: "u-reviewer", Updated: "2026-09-03T10:00:00Z",
		}
		first := governedPush{
			projectID: pid, actor: "u-reviewer", item: item,
			blocks:    []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
			decisions: []venue.UnitDecision{approved},
		}
		require.NoError(t, first.run(t, deps, "job-first"))

		// The same record, sent by somebody who holds no review permission at
		// all. It is not a new decision, so it is not refused.
		deps.ReviewAuthority = pushAuthority{review: map[string]bool{}}
		again := governedPush{
			projectID: pid, actor: "u-nobody", item: item,
			blocks:    []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
			decisions: []venue.UnitDecision{approved},
		}
		require.NoError(t, again.run(t, deps, "job-again"))

		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok)
		assert.Equal(t, venue.ReviewStateApproved, d.ReviewState, "the standing approval is untouched")
		assert.Equal(t, "u-reviewer", d.DecidedBy, "and it still names the person who made it")

		report := jobGovernance(t, deps, "push-job-again")
		for _, r := range report.Refusals {
			assert.NotEqual(t, venue.VerdictApproval, r.Kind,
				"re-sending what the venue holds must not be reported as a refused decision: %+v", r)
		}
	})

	t.Run("a rejection needs no review permission and lands at draft", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{review: map[string]bool{}})
		b := reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusDraft)
		push := governedPush{
			projectID: pid, actor: "u-translator", item: item,
			blocks: []*model.Block{b},
			decisions: []venue.UnitDecision{{
				ItemName: item, Unit: "b1", Variant: locale,
				Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
				DecidedBy: "u-translator", Updated: "2026-09-03T10:00:00Z",
			}},
		}
		require.NoError(t, push.run(t, deps, "job-reject"))

		assert.Equal(t, model.TargetStatusDraft, storedTarget(t, deps, pid, item, locale))
		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok)
		assert.Equal(t, venue.ReviewStateRejected, d.ReviewState,
			"withdrawing work is the translate permission's, as it is on the web")
		assert.True(t, jobGovernance(t, deps, "push-job-reject").Empty())
	})

	t.Run("an unanswerable permission fails the push rather than demoting", func(t *testing.T) {
		deps, pid := setup(t, pushAuthority{failWith: errors.New("auth store unreachable")})
		push := governedPush{
			projectID: pid, actor: "u-reviewer", item: item,
			blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
		}
		err := push.run(t, deps, "job-unanswerable")
		require.Error(t, err, "a question that could not be asked is not an answer of no")

		rows, qerr := deps.ContentStore.GetBlocks(t.Context(), store.BlockQuery{
			ProjectID: pid, Stream: "main", ItemName: item, Limit: 10,
		})
		require.NoError(t, qerr)
		assert.Empty(t, rows, "the transition rolled back, so the producer still holds the work")
	})

	t.Run("a deployment that cannot check permissions refuses a push carrying a verdict", func(t *testing.T) {
		deps, pid := setup(t, nil)
		deps.ReviewAuthority = nil
		withVerdict := governedPush{
			projectID: pid, actor: "u-reviewer", item: item,
			blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
		}
		require.Error(t, withVerdict.run(t, deps, "job-nogate"))

		// A push claiming nothing is unaffected: it needs no gate.
		plain := governedPush{
			projectID: pid, actor: "u-translator", item: item,
			blocks: []*model.Block{reviewedBlock("b2", "Bye", locale, "Au revoir", model.TargetStatusTranslated)},
		}
		require.NoError(t, plain.run(t, deps, "job-plain"))
	})
}

// A push that lowers a target the venue holds at signed-off, keeping the
// translation and the source the sign-off blessed, is withdrawing that
// sign-off. The web asks review permission for the language before it lets an
// un-review or a rejection do the same, and the worker asks the same question.
// A refused withdrawal keeps the venue's rung and ledger record, and the record
// travels back so the producer can hold the same; an edited translation is not
// a withdrawal and lands at translated, as an edit on the web does.
func TestPushReviewGovernance_SignOffWithdrawal(t *testing.T) {
	const item = "en.json"
	const locale = "fr"
	const source, text = "Hello", "Bonjour"

	// signedOff is the ledger record a sign-off leaves, as a push carries it;
	// withdrawn is the same unit's record after a local un-review: the basis,
	// written later.
	signedOff := venue.UnitDecision{
		ItemName: item, Unit: "b1", Variant: locale,
		Status: string(model.TargetStatusSignedOff), ReviewState: venue.ReviewStateSignedOff,
		TargetHash: state.TargetHash(text), ContentHash: state.SourceHash(source),
		DecidedBy: "u-reviewer", DecidedAt: "2026-09-03T10:00:00Z", Updated: "2026-09-03T10:00:00Z",
	}
	withdrawn := signedOff.AsBasis(model.TargetStatusTranslated)
	withdrawn.Updated = "2026-09-04T10:00:00Z"

	// venueHolding is a venue holding the unit at one rung, decided by a
	// reviewer, with the review authority the test under it wants.
	venueHolding := func(t *testing.T, rung model.TargetStatus, held venue.UnitDecision, auth review.Authority) (*WorkerDeps, string) {
		t.Helper()
		deps := newTestWorkerDeps(t)
		deps.ReviewAuthority = pushAuthority{review: map[string]bool{locale: true}}
		pid := "gov-withdraw-" + t.Name()
		require.NoError(t, deps.ContentStore.CreateProject(t.Context(),
			&store.Project{ID: pid, Name: "Signed off"}))
		first := governedPush{
			projectID: pid, actor: "u-reviewer", item: item,
			blocks:    []*model.Block{reviewedBlock("b1", source, locale, text, rung)},
			decisions: []venue.UnitDecision{held},
		}
		require.NoError(t, first.run(t, deps, "job-hold"))
		require.Equal(t, rung, storedTarget(t, deps, pid, item, locale))
		deps.ReviewAuthority = auth
		return deps, pid
	}

	// withdrawal is the push a local un-review produces: the same translation
	// at translated, and the basis where the sign-off was.
	withdrawal := func(pid, actor string) governedPush {
		return governedPush{
			projectID: pid, actor: actor, item: item,
			blocks:    []*model.Block{reviewedBlock("b1", source, locale, text, model.TargetStatusTranslated)},
			decisions: []venue.UnitDecision{withdrawn},
		}
	}

	t.Run("without review permission the sign-off stands and is reported", func(t *testing.T) {
		deps, pid := venueHolding(t, model.TargetStatusSignedOff, signedOff, pushAuthority{review: map[string]bool{}})
		require.NoError(t, withdrawal(pid, "u-translator").run(t, deps, "job-withdraw"),
			"a refused withdrawal never fails the push")

		assert.Equal(t, model.TargetStatusSignedOff, storedTarget(t, deps, pid, item, locale),
			"the venue's rung stands")
		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok)
		assert.Equal(t, venue.ReviewStateSignedOff, d.ReviewState, "and so does its ledger record")
		assert.Equal(t, "u-reviewer", d.DecidedBy)

		report := jobGovernance(t, deps, "push-job-withdraw")
		require.Len(t, report.Refusals, 1)
		assert.Equal(t, venue.DecisionRefusal{
			Locale: locale, Kind: venue.VerdictDemotion, Reason: venue.RefusedSignOffWithdrawal, Count: 1,
		}, report.Refusals[0], "stated twice, on the block and in its record, and counted once")
		require.Len(t, report.Units, 1)
		unit := report.Units[0]
		assert.Equal(t, "b1", unit.Unit)
		assert.Equal(t, venue.RefusedSignOffWithdrawal, unit.Reason)
		require.NotNil(t, unit.Held, "the record the venue kept travels back for the producer to hold")
		assert.Equal(t, venue.ReviewStateSignedOff, unit.Held.ReviewState)
		assert.Equal(t, "u-reviewer", unit.Held.DecidedBy)
		assert.Equal(t, signedOff.TargetHash, unit.Held.TargetHash)
	})

	t.Run("with review permission the withdrawal lands and is audited", func(t *testing.T) {
		deps, pid := venueHolding(t, model.TargetStatusSignedOff, signedOff, pushAuthority{review: map[string]bool{locale: true}})
		bus := &recordingBus{}
		deps.EventBus = bus
		require.NoError(t, withdrawal(pid, "u-reviewer-2").run(t, deps, "job-withdraw-ok"))

		assert.Equal(t, model.TargetStatusTranslated, storedTarget(t, deps, pid, item, locale))
		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok)
		assert.Empty(t, d.ReviewState, "the ledger holds the basis the push carried")
		assert.Equal(t, string(model.TargetStatusTranslated), d.Status)
		assert.True(t, jobGovernance(t, deps, "push-job-withdraw-ok").Empty())

		var decided []platev.Event
		for _, ev := range bus.published() {
			if ev.Type == platev.EventReviewDecided {
				decided = append(decided, ev)
			}
		}
		require.Len(t, decided, 1, "one entry for one rung change, however many times the push stated it")
		assert.Equal(t, "u-reviewer-2", decided[0].Actor)
		assert.Equal(t, "unreviewed", decided[0].Data["decision"])
		assert.Equal(t, "push", decided[0].Data["via"])
		assert.Equal(t, string(model.TargetStatusSignedOff), decided[0].Before["status"])
		assert.Equal(t, string(model.TargetStatusTranslated), decided[0].After["status"])
	})

	t.Run("a rejection of a signed-off target is held to the same permission", func(t *testing.T) {
		rejected := withdrawn
		rejected.Status = string(model.TargetStatusDraft)
		rejected.ReviewState = venue.ReviewStateRejected
		for _, tc := range []struct {
			name    string
			permits map[string]bool
			want    model.TargetStatus
		}{
			{"refused", map[string]bool{}, model.TargetStatusSignedOff},
			{"accepted", map[string]bool{locale: true}, model.TargetStatusDraft},
		} {
			t.Run(tc.name, func(t *testing.T) {
				deps, pid := venueHolding(t, model.TargetStatusSignedOff, signedOff, pushAuthority{review: tc.permits})
				push := governedPush{
					projectID: pid, actor: "u-translator", item: item,
					blocks:    []*model.Block{reviewedBlock("b1", source, locale, text, model.TargetStatusDraft)},
					decisions: []venue.UnitDecision{rejected},
				}
				require.NoError(t, push.run(t, deps, "job-reject"))
				assert.Equal(t, tc.want, storedTarget(t, deps, pid, item, locale))
				d, ok := heldDecision(t, deps, pid, "b1", locale)
				require.True(t, ok)
				if tc.want == model.TargetStatusDraft {
					assert.Equal(t, venue.ReviewStateRejected, d.ReviewState)
					assert.True(t, jobGovernance(t, deps, "push-job-reject").Empty())
					return
				}
				assert.Equal(t, venue.ReviewStateSignedOff, d.ReviewState)
				assert.Len(t, jobGovernance(t, deps, "push-job-reject").Refusals, 1)
			})
		}
	})

	t.Run("a reviewed target is lowered either way", func(t *testing.T) {
		approved := signedOff
		approved.Status = string(model.TargetStatusReviewed)
		approved.ReviewState = venue.ReviewStateApproved
		deps, pid := venueHolding(t, model.TargetStatusReviewed, approved, pushAuthority{review: map[string]bool{}})
		require.NoError(t, withdrawal(pid, "u-translator").run(t, deps, "job-unreview"))

		assert.Equal(t, model.TargetStatusTranslated, storedTarget(t, deps, pid, item, locale),
			"taking back an approval is ordinary translation work, as it is on the web")
		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok)
		assert.Empty(t, d.ReviewState)
		assert.True(t, jobGovernance(t, deps, "push-job-unreview").Empty())
	})

	t.Run("an edited translation is not a withdrawal and lands at translated", func(t *testing.T) {
		deps, pid := venueHolding(t, model.TargetStatusSignedOff, signedOff, pushAuthority{review: map[string]bool{}})
		edited := withdrawn
		edited.TargetHash = state.TargetHash("Salut")
		push := governedPush{
			projectID: pid, actor: "u-translator", item: item,
			blocks:    []*model.Block{reviewedBlock("b1", source, locale, "Salut", model.TargetStatusTranslated)},
			decisions: []venue.UnitDecision{edited},
		}
		require.NoError(t, push.run(t, deps, "job-edit"))

		assert.Equal(t, model.TargetStatusTranslated, storedTarget(t, deps, pid, item, locale),
			"a sign-off judged one translation; rewriting it invalidates the sign-off, as an edit on the web does")
		d, ok := heldDecision(t, deps, pid, "b1", locale)
		require.True(t, ok)
		assert.Empty(t, d.ReviewState)
		assert.Equal(t, edited.TargetHash, d.TargetHash)
		assert.True(t, jobGovernance(t, deps, "push-job-edit").Empty())
	})

	t.Run("a basis record alone does not overwrite a standing sign-off", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			permits map[string]bool
			want    string
		}{
			{"without permission", map[string]bool{}, venue.ReviewStateSignedOff},
			{"with permission", map[string]bool{locale: true}, ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				deps, pid := venueHolding(t, model.TargetStatusSignedOff, signedOff, pushAuthority{review: tc.permits})
				// The block as it was pulled, still at signed-off; only the
				// record says otherwise.
				push := governedPush{
					projectID: pid, actor: "u-translator", item: item,
					blocks:    []*model.Block{reviewedBlock("b1", source, locale, text, model.TargetStatusSignedOff)},
					decisions: []venue.UnitDecision{withdrawn},
				}
				require.NoError(t, push.run(t, deps, "job-basis"))
				d, ok := heldDecision(t, deps, pid, "b1", locale)
				require.True(t, ok)
				assert.Equal(t, tc.want, d.ReviewState)
				if tc.want == "" {
					assert.True(t, jobGovernance(t, deps, "push-job-basis").Empty())
					return
				}
				assert.Equal(t, model.TargetStatusSignedOff, storedTarget(t, deps, pid, item, locale))
				report := jobGovernance(t, deps, "push-job-basis")
				require.Len(t, report.Refusals, 1)
				assert.Equal(t, venue.VerdictDemotion, report.Refusals[0].Kind)
				require.Len(t, report.Units, 1)
				assert.NotNil(t, report.Units[0].Held)
			})
		}
	})

	t.Run("a permission that cannot be resolved fails the push rather than deciding", func(t *testing.T) {
		deps, pid := venueHolding(t, model.TargetStatusSignedOff, signedOff, pushAuthority{failWith: errors.New("auth store unreachable")})
		require.Error(t, withdrawal(pid, "u-reviewer").run(t, deps, "job-unanswerable"))
		assert.Equal(t, model.TargetStatusSignedOff, storedTarget(t, deps, pid, item, locale),
			"the transition rolled back")

		deps.ReviewAuthority = nil
		require.Error(t, withdrawal(pid, "u-reviewer").run(t, deps, "job-nogate"),
			"a deployment with no way to ask refuses a push that withdraws a sign-off")
		assert.Equal(t, model.TargetStatusSignedOff, storedTarget(t, deps, pid, item, locale))
	})
}

// jobGovernance reads back what the worker recorded about a push's refusals.
func jobGovernance(t *testing.T, deps *WorkerDeps, pushID string) venue.PushGovernance {
	t.Helper()
	raw, err := deps.JobStore.PushGovernance(t.Context(), pushID)
	require.NoError(t, err)
	var report venue.PushGovernance
	if raw != "" {
		require.NoError(t, json.Unmarshal([]byte(raw), &report))
	}
	return report
}

// seedHandWritten puts a translation in the venue attributed to one person, so
// the separation-of-duties policy has an author to recognise.
func seedHandWritten(t *testing.T, deps *WorkerDeps, projectID, item, author, locale, text string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, deps.ContentStore.StoreItem(ctx, projectID, "main", &store.Item{
		Name: item, Format: "json", ItemType: "file",
	}))
	b := &model.Block{ID: "b1", Name: "b1", Translatable: true}
	b.SetSourceText("Hello")
	require.NoError(t, deps.ContentStore.StoreBlocksForItem(ctx, projectID, "main", item, []*model.Block{b}))

	rows, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: item, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	rows[0].Block.SetTargetText(model.LocaleID(locale), text)
	wctx := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: author})
	require.NoError(t, deps.ContentStore.StoreBlocks(wctx, projectID, "main", []*model.Block{rows[0].Block}))
}

// A rung a push moved is a review decision, so it leaves the same audit entry a
// decision made on the platform leaves: who, which block, which language, and
// the rungs it moved between. The marker says it arrived by push, because that
// is the one thing the web entry cannot be confused with.
//
// One entry per target. A push states a promotion twice, on the block and in
// the decision record, and the audit trail records the change rather than the
// statements of it.
func TestPushReviewGovernance_AuditsAcceptedRungs(t *testing.T) {
	const item = "en.json"
	const locale = "fr"

	cases := []struct {
		name     string
		rung     model.TargetStatus
		state    string
		decision string
	}{
		{"approval", model.TargetStatusReviewed, venue.ReviewStateApproved, "approved"},
		{"sign-off", model.TargetStatusSignedOff, venue.ReviewStateSignedOff, "signed-off"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestWorkerDeps(t)
			deps.ReviewAuthority = pushAuthority{review: map[string]bool{locale: true}}
			bus := &recordingBus{}
			deps.EventBus = bus

			pid := "gov-audit-" + tc.name
			require.NoError(t, deps.ContentStore.CreateProject(t.Context(),
				&store.Project{ID: pid, Name: "Audited"}))

			push := governedPush{
				projectID: pid, actor: "u-reviewer", item: item,
				blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", tc.rung)},
				decisions: []venue.UnitDecision{{
					ItemName: item, Unit: "b1", Variant: locale,
					Status: string(tc.rung), ReviewState: tc.state,
					Updated: "2026-09-03T10:00:00Z",
				}},
			}
			require.NoError(t, push.run(t, deps, "job-audit-"+tc.name))

			var decided []platev.Event
			for _, ev := range bus.published() {
				if ev.Type == platev.EventReviewDecided {
					decided = append(decided, ev)
				}
			}
			require.Len(t, decided, 1, "one entry for one rung change, however many times the push stated it")
			ev := decided[0]
			assert.Equal(t, "u-reviewer", ev.Actor)
			assert.Equal(t, pid, ev.ProjectID)
			assert.Equal(t, "block", ev.ResourceType)
			assert.NotEmpty(t, ev.ResourceID)
			assert.Equal(t, locale, ev.Data["locale"])
			assert.Equal(t, tc.decision, ev.Data["decision"],
				"the audit label is the one the review surfaces use for the rung")
			assert.Equal(t, "push", ev.Data["via"])
			assert.Equal(t, string(tc.rung), ev.After["status"])
		})
	}
}

// A refused rung is nobody's decision, so it leaves no review entry. The
// workspace policy catching a decider on their own work is a different record,
// and it is filed once for the push with the count.
func TestPushReviewGovernance_AuditsNoRefusedRung(t *testing.T) {
	const item = "en.json"
	const locale = "fr"

	deps := newTestWorkerDeps(t)
	deps.ReviewAuthority = pushAuthority{review: map[string]bool{locale: true}, mode: platauth.SoDBlock}
	bus := &recordingBus{}
	deps.EventBus = bus

	pid := "gov-audit-refused"
	require.NoError(t, deps.ContentStore.CreateProject(t.Context(),
		&store.Project{ID: pid, Name: "Audited"}))
	seedHandWritten(t, deps, pid, item, "u-author", locale, "Bonjour")

	push := governedPush{
		projectID: pid, actor: "u-author", item: item,
		blocks: []*model.Block{reviewedBlock("b1", "Hello", locale, "Bonjour", model.TargetStatusReviewed)},
	}
	require.NoError(t, push.run(t, deps, "job-audit-refused"))

	reviews, violations := 0, 0
	for _, ev := range bus.published() {
		switch ev.Type {
		case platev.EventReviewDecided:
			reviews++
		case platev.EventType("sod.violation"):
			violations++
			assert.Equal(t, "u-author", ev.Data["actor"])
			assert.Equal(t, "push", ev.Data["via"])
			assert.Equal(t, "1", ev.Data["targets"])
		}
	}
	assert.Zero(t, reviews, "a refused rung moved nothing, so there is nothing to record as decided")
	assert.Equal(t, 1, violations, "one record for the push, with the count")
}
