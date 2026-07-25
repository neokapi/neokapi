package server

import (
	"context"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceFirstHarness wires a Server with a real SQLite ContentStore, a
// ConvergenceRunStore, and a TaskStore — enough to exercise the source-first
// settle phase, the gate, and the hold-on-source outcome without Postgres or a
// live job worker. Job infra is a non-nil no-op fake so the produce path's
// nil-guard passes; the gate holds before any job is created in the hold cases.
func sourceFirstHarness(t *testing.T) (*Server, *sqlitestore.SQLiteStore, *bstore.ConvergenceRunStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "sf.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	runStore := bstore.NewConvergenceRunStore(cs.DB())
	taskStore := bstore.NewTaskStore(cs.DB())
	s := &Server{
		ContentStore:        cs,
		ConvergenceRunStore: runStore,
		TaskStore:           taskStore,
		JobStore:            noopJobStore{},
		JobQueue:            noopQueue{},
	}
	s.convergence = newConvergenceOrchestrator(s)
	return s, cs, runStore
}

// srcBlk describes one source block to seed.
type srcBlk struct {
	id, text string
	status   model.SourceStatus
}

// storeSourceItem writes all of an item's blocks in one call (an item store
// replaces the item's block set, so blocks that must coexist go together) and
// registers the item metadata so it counts in coverage/derivation.
func storeSourceItem(t *testing.T, cs *sqlitestore.SQLiteStore, projectID, item string, blks ...srcBlk) {
	t.Helper()
	require.NoError(t, cs.StoreItem(t.Context(), projectID, "main", &platstore.Item{
		ProjectID: projectID, Name: item, Format: "json",
	}))
	var blocks []*model.Block
	for _, sb := range blks {
		b := &model.Block{ID: sb.id, Translatable: true, SourceStatus: sb.status}
		b.SetSourceText(sb.text)
		blocks = append(blocks, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(t.Context(), projectID, "main", item, blocks))
}

// storeSourceBlock upserts one translatable source block as its own item.
func storeSourceBlock(t *testing.T, cs *sqlitestore.SQLiteStore, projectID, item, id, text string, status model.SourceStatus) {
	t.Helper()
	storeSourceItem(t, cs, projectID, item, srcBlk{id, text, status})
}

func mkProject(t *testing.T, cs *sqlitestore.SQLiteStore, id string, props map[string]string) {
	t.Helper()
	require.NoError(t, cs.CreateProject(t.Context(), &platstore.Project{
		ID: id, Name: id, DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages: []model.LocaleID{model.LocaleFrench},
		Properties:      props,
	}))
}

// TestSourceStatus_RoundTripsThroughStore proves the ContentStore now persists a
// block's SourceStatus (folded into properties) — the prerequisite for the gate.
func TestSourceStatus_RoundTripsThroughStore(t *testing.T) {
	_, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", nil)
	storeSourceBlock(t, cs, "p", "a.json", "b1", "Hello", model.SourceStatusChecked)

	got, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: "p", Stream: "main"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, model.SourceStatusChecked, got[0].Block.SourceStatus)
	// The reserved key must not leak back as an ordinary property.
	_, leaked := got[0].Block.Properties[platstore.PropSourceStatus]
	assert.False(t, leaked, "the folded status key must be stripped on read")
}

// TestSettleSource_StampsAndCounts: clean source is promoted to `checked` and
// clears the default gate; empty/whitespace source is demoted to `authored` and
// held. The blocked-on-source count reflects only the held block.
func TestSettleSource_StampsAndCounts(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", nil) // no source_gate → default `checked`
	storeSourceItem(t, cs, "p", "a.json",
		srcBlk{"clean", "A well-formed sentence.", model.SourceStatusNew},
		srcBlk{"empty", "   ", model.SourceStatusNew})

	res, err := s.convergence.settleSource(t.Context(), "p")
	require.NoError(t, err)
	assert.Equal(t, model.SourceGateChecked, res.Gate)
	assert.Equal(t, 2, res.Total)
	assert.Equal(t, 1, res.BlockedOnSource, "only the empty block is held")

	byID := settledStatusByID(t, cs, "p")
	assert.Equal(t, model.SourceStatusChecked, byID["clean"], "clean source is promoted to checked")
	// Empty source cannot be promoted; it stays at the New baseline, below the
	// gate — held either way.
	assert.False(t, model.SourceGateChecked.Admits(byID["empty"]), "empty source stays below the gate")
}

// TestSettleSource_GateNone_NoOp: the opt-out never settles or holds.
func TestSettleSource_GateNone_NoOp(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", map[string]string{"source_gate": "none"})
	storeSourceBlock(t, cs, "p", "a.json", "empty", "   ", model.SourceStatusNew)

	res, err := s.convergence.settleSource(t.Context(), "p")
	require.NoError(t, err)
	assert.Equal(t, model.SourceGateNone, res.Gate)
	assert.Equal(t, 0, res.BlockedOnSource)
	assert.Equal(t, 0, res.Total, "gate none skips settlement entirely")
}

// TestGateItemsBySource covers the fan-out decision at each gate level.
func TestGateItemsBySource(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", nil) // default checked
	// item "ready" has a checked block; item "held" has only authored blocks.
	storeSourceBlock(t, cs, "p", "ready.json", "r1", "ok", model.SourceStatusChecked)
	storeSourceBlock(t, cs, "p", "held.json", "h1", "nope", model.SourceStatusAuthored)

	prod, blocked, err := s.convergence.gateItemsBySource(t.Context(), "p", []string{"ready.json", "held.json"})
	require.NoError(t, err)
	assert.Equal(t, []string{"ready.json"}, prod, "only the ready item is producible")
	assert.Equal(t, 1, blocked, "the held item is counted")
}

// TestGateItemsBySource_None: opt-out makes every item producible.
func TestGateItemsBySource_None(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", map[string]string{"source_gate": "none"})
	storeSourceBlock(t, cs, "p", "held.json", "h1", "nope", model.SourceStatusAuthored)

	prod, blocked, err := s.convergence.gateItemsBySource(t.Context(), "p", []string{"held.json"})
	require.NoError(t, err)
	assert.Equal(t, []string{"held.json"}, prod)
	assert.Equal(t, 0, blocked)
}

// TestReGateOnSourceChange: a settled, approved block whose source content
// changes is reset below the gate on the next settle — re-gating ONLY that
// block, not its unchanged sibling (epic 019 acceptance #6).
func TestReGateOnSourceChange(t *testing.T) {
	s, cs, _ := sourceFirstHarness(t)
	mkProject(t, cs, "p", nil)
	storeSourceItem(t, cs, "p", "a.json",
		srcBlk{"keep", "Stable, well-formed source.", model.SourceStatusNew},
		srcBlk{"edit", "Original text.", model.SourceStatusNew})

	// First settle: both clean → checked, both admitted.
	res1, err := s.convergence.settleSource(t.Context(), "p")
	require.NoError(t, err)
	assert.Equal(t, 0, res1.BlockedOnSource)

	// Approve both (a human sign-off) so we can prove the reset drops approval.
	approveBlocks(t, cs, "p")

	// Edit ONE block's source in place — keeping its stored properties (the
	// settled-hash stamp) — so the content-hash change is what re-gates it, not a
	// wiped status. The sibling is untouched.
	editStoredBlockSource(t, cs, "p", "edit", "Rewritten, different text.")

	// Second settle: the edited block resets (its approval is stale) and is
	// re-checked to `checked`; the untouched block keeps its approval.
	res2, err := s.convergence.settleSource(t.Context(), "p")
	require.NoError(t, err)

	byID := settledStatusByID(t, cs, "p")
	assert.Equal(t, model.SourceStatusApproved, byID["keep"], "the untouched block keeps its approval")
	assert.Equal(t, model.SourceStatusChecked, byID["edit"], "the changed block re-gates to checked (approval dropped)")
	_ = res2
}

// settledStatusByID reads back the project's blocks and maps SourceID → the
// settled SourceStatus. Item stores remap the caller's block ID to an internal
// id, so tests address a block by the SourceID they seeded it with.
func settledStatusByID(t *testing.T, cs *sqlitestore.SQLiteStore, projectID string) map[string]model.SourceStatus {
	t.Helper()
	got, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	out := map[string]model.SourceStatus{}
	for _, sb := range got {
		key := sb.SourceID
		if key == "" {
			key = sb.Block.ID
		}
		out[key] = sb.Block.SourceStatus
	}
	return out
}

// approveBlocks stamps every stored block Approved and persists it (preserving
// its stored properties, incl. the settled-hash) — a stand-in for a human source
// sign-off.
func approveBlocks(t *testing.T, cs *sqlitestore.SQLiteStore, projectID string) {
	t.Helper()
	got, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	var blocks []*model.Block
	for _, sb := range got {
		sb.Block.SourceStatus = model.SourceStatusApproved
		blocks = append(blocks, sb.Block)
	}
	require.NoError(t, cs.StoreBlocks(t.Context(), projectID, "main", blocks))
}

// editStoredBlockSource rewrites the source text of the block seeded with the
// given SourceID, keeping its other stored fields (properties, status) so the
// change is a content-hash change — the input to the re-gate check.
func editStoredBlockSource(t *testing.T, cs *sqlitestore.SQLiteStore, projectID, sourceID, newText string) {
	t.Helper()
	got, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	for _, sb := range got {
		if sb.SourceID == sourceID || sb.Block.ID == sourceID {
			sb.Block.SetSourceText(newText)
			require.NoError(t, cs.StoreBlocks(t.Context(), projectID, "main", []*model.Block{sb.Block}))
			return
		}
	}
	t.Fatalf("block %q not found", sourceID)
}

// TestDrive_HoldsOnSource: a run over an entirely un-ready source HOLDS with
// stall_reason=source_not_ready, creates a source-review task, and creates NO
// translation jobs (the produce path returns the source-not-ready sentinel
// before any job is spawned). Exercises the real settle + gate + produce path.
func TestDrive_HoldsOnSource(t *testing.T) {
	s, cs, runStore := sourceFirstHarness(t)
	// Gate at `approved` (human sign-off); review fan-out is on by default, so
	// the reused create_source_review automation fires. A clean, non-empty source
	// settles to `checked` — which is below `approved`, so the whole locale is
	// held on source.
	mkProject(t, cs, "p", map[string]string{"source_gate": "approved"})
	storeSourceBlock(t, cs, "p", "a.json", "b1", "A well-formed sentence.", model.SourceStatusNew)

	run := &bstore.ConvergenceRun{ProjectID: "p", Trigger: "push", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(t.Context(), run))

	s.convergence.driveWith(t.Context(), run, convergence.LoopFuncs{
		Derive:  s.convergence.deriveFunc("p", nil),
		Produce: s.convergence.produceFunc("p", run.ID),
	})

	got, err := runStore.GetRun(t.Context(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunParked, got.State)
	assert.Equal(t, convergence.StallSourceNotReady, got.StallReason)
	assert.Equal(t, 1, got.BlockedOnSource)

	// A source-review task was created for the held source. Count directly (the
	// TaskStore.List readback scans timestamps in a driver-specific way); the
	// row's existence is what matters.
	var sourceReview int
	require.NoError(t, cs.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM tasks WHERE project_id=$1 AND type=$2`,
		"p", string(bstore.TaskSourceReview)).Scan(&sourceReview))
	assert.GreaterOrEqual(t, sourceReview, 1, "a source-review task must be created on hold")
}

// TestParkedStallReason_NeverSourceNotReady locks the finding-#1 fix: an
// ordinary park (pending remainder after a partial run) is labeled no_progress
// or checks_failing — NEVER source_not_ready. Only a PURE hold routes through
// the typed errStallSourceNotReady, so a partial run that translated ready items
// still reaches the translation review queue.
func TestParkedStallReason_NeverSourceNotReady(t *testing.T) {
	assert.Equal(t, convergence.StallNoProgress,
		parkedStallReason(convergence.PassState{Pending: []string{"fr"}}))
	assert.Equal(t, convergence.StallChecksFailing,
		parkedStallReason(convergence.PassState{Pending: []string{"fr"}, FailingChecks: 1}))
}

// noopJobStore satisfies jobs.JobStore for tests that never reach the job path
// (the source gate holds first). Only the embedded nil interface is present;
// calling any method would panic, which is the point — the hold path must not.
type noopJobStore struct{ jobs.JobStore }

// noopQueue satisfies jobs.Queue with the same never-called contract.
type noopQueue struct{ jobs.Queue }

// Ensure the fakes are used (compile-time), and silence unused in some builds.
var _ = context.Background
