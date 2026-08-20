package store

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block_history row is stamped by the process that wrote it, on the same
// clock as everything it will be compared against.
//
// Point-in-time restore takes a cutoff from versions.created_at or
// change_log.logged_at — both bound by this process — and asks which history
// rows fall after it (ComputePointInTimeReverts). block_history.created_at was
// the one column in that comparison stamped with the database's NOW(), so the
// question spanned two clocks and a skew nobody controls. Run the server behind
// its database and a restore reverted nothing; run it ahead and it blanked
// targets that had content. It showed up as TestPhase4_RestoreToVersion failing
// only under parallel load, which is when the skew is widest.
//
// The invariant is checkable without any skew to reproduce: one transaction
// writes one timestamp, so a block's history row carries exactly the timestamp
// its own row does.
func TestBlockHistoryIsStampedOnTheWritersClock(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	fr := model.LocaleID("fr")

	before := time.Now().UTC()
	blk := model.NewBlock("greeting", "hi")
	blk.SetTargetText(fr, "salut")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "main", []*model.Block{blk}))
	after := time.Now().UTC()

	var historyAt, blockAt time.Time
	require.NoError(t, s.db.QueryRowContext(ctx,
		`SELECT h.created_at, b.updated_at
		   FROM block_history h JOIN blocks b ON b.id = h.block_id AND b.project_id = h.project_id
		  WHERE h.project_id = $1 AND h.block_id = $2 AND h.locale = $3`,
		p.ID, "greeting", "fr").Scan(&historyAt, &blockAt))

	// One transaction, one timestamp: the history row and the block row it
	// describes were written together and say so.
	assert.True(t, historyAt.Equal(blockAt),
		"history %s and block %s were written in one transaction and must carry one timestamp",
		historyAt, blockAt)

	// And that timestamp is this process's, not the database's — the only way
	// the comparison against a version or a cursor means anything.
	assert.False(t, historyAt.Before(before), "history %s predates the call that wrote it (%s)", historyAt, before)
	assert.False(t, historyAt.After(after), "history %s postdates the call that wrote it (%s)", historyAt, after)
}

// The restore itself, with the ordering stated rather than slept for: a version
// taken between two edits rolls the target back to the first.
func TestPointInTimeRevertsUseTheSameClockAsVersions(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	fr := model.LocaleID("fr")

	blk := model.NewBlock("greeting", "hi")
	blk.SetTargetText(fr, "v1")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "main", []*model.Block{blk}))

	ver, err := s.CreateVersion(ctx, p.ID, "main", "snap", "")
	require.NoError(t, err)

	blk.SetTargetText(fr, "v2")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "main", []*model.Block{blk}))

	cutoff, err := s.VersionTime(ctx, p.ID, ver.ID)
	require.NoError(t, err)

	// The whole mechanism in one assertion: the edit after the version is seen
	// as after it, and the state to restore is the one before it.
	reverts, err := s.ComputePointInTimeReverts(ctx, p.ID, "main", cutoff)
	require.NoError(t, err)
	require.Len(t, reverts, 1, "the edit made after the version is the one to roll back")
	assert.Equal(t, "v1", reverts[0].Text)
	assert.False(t, reverts[0].Clear, "the target had content at the version and is restored, not blanked")
}
