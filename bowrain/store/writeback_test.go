package store

import (
	"context"
	"sync"
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readItemByKey reads an item's blocks keyed by the durable key they were
// stored under.
func readItemByKey(t *testing.T, s *PostgresStore, projectID, itemName string) map[string]*venue.StoredBlock {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: itemName, Limit: 100,
	})
	require.NoError(t, err)
	byKey := make(map[string]*venue.StoredBlock, len(rows))
	for _, sb := range rows {
		byKey[sb.SourceID] = sb
	}
	return byKey
}

func seedWriteBackItem(t *testing.T, s *PostgresStore, projectID, itemName string, blocks ...*model.Block) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.StoreItem(ctx, projectID, "main", &platstore.Item{Name: itemName, Format: "json"}))
	require.NoError(t, s.StoreBlocksForItem(ctx, projectID, "main", itemName, blocks))
}

// A write-back lands only on the row it was read from, and only while that row
// holds the content the caller read. A row removed since the read is not
// stored again, and a row whose source changed keeps the newer source.
func TestWriteBackBlocks_LandsOnlyOnTheRowItWasRead(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	seedWriteBackItem(t, s, p.ID, "en.json",
		model.NewBlock("same", "Unchanged"),
		model.NewBlock("gone", "Removed"),
		model.NewBlock("moved", "Before"))
	read := readItemByKey(t, s, p.ID, "en.json")
	require.Len(t, read, 3)

	require.NoError(t, s.DeleteBlock(ctx, p.ID, "main", read["gone"].Block.ID))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json",
		[]*model.Block{model.NewBlock("moved", "After")}))

	reads := make([]*venue.StoredBlock, 0, len(read))
	for _, key := range []string{"same", "gone", "moved"} {
		read[key].Block.SetTargetText("nb", "Oversatt "+key)
		reads = append(reads, read[key])
	}
	res, err := s.WriteBackBlocks(ctx, p.ID, "main", reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Written)
	assert.ElementsMatch(t, []string{read["gone"].Block.ID, read["moved"].Block.ID}, res.Skipped)

	after := readItemByKey(t, s, p.ID, "en.json")
	require.Len(t, after, 2, "the removed block is not stored again")
	assert.Equal(t, "Oversatt same", after["same"].Block.TargetText("nb"))
	assert.Equal(t, "After", after["moved"].Block.SourceText(), "the newer source stands")
	assert.Empty(t, after["moved"].Block.TargetText("nb"), "a target of the old source does not land on the new one")
	assert.Zero(t, countRows(t, s, "blocks", `project_id=$1 AND item_name=''`, p.ID))
	assert.Zero(t, countRows(t, s, "translations", `project_id=$1 AND block_id=$2`, p.ID, read["gone"].Block.ID))
	assert.Zero(t, countRows(t, s, "change_log", `project_id=$1 AND block_id=$2 AND change_type LIKE 'target_%'`,
		p.ID, read["moved"].Block.ID), "nothing is logged for a block that did not land")
}

// A caller that changes the source it read, such as an accepted source
// proposal, still writes it: the row holds the content the caller read.
func TestWriteBackBlocks_ASourceEditOfTheReadContentLands(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	seedWriteBackItem(t, s, p.ID, "en.json", model.NewBlock("edit", "Before"))
	sb := readItemByKey(t, s, p.ID, "en.json")["edit"]
	require.NotNil(t, sb)

	sb.Block.SetSourceText("After")
	res, err := s.WriteBackBlocks(ctx, p.ID, "main", []*venue.StoredBlock{sb})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Written)
	assert.Empty(t, res.Skipped)

	assert.Equal(t, "After", readItemByKey(t, s, p.ID, "en.json")["edit"].Block.SourceText())
	assert.Equal(t, 1, countRows(t, s, "change_log",
		`project_id=$1 AND block_id=$2 AND change_type='source_modified'`, p.ID, sb.Block.ID))
}

// The removal a push applies can commit while a write-back is under way: the
// write-back's own read sees the row, and its write waits on the push's lock.
// When the push commits, the write lands nowhere rather than inserting the row
// again with no item.
func TestWriteBackBlocks_ARemovalCommittingMidWriteIsNotUndone(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	seedWriteBackItem(t, s, p.ID, "en.json", model.NewBlock("k", "Hello"))
	sb := readItemByKey(t, s, p.ID, "en.json")["k"]
	require.NotNil(t, sb)
	sb.Block.SetTargetText("nb", "Hei")

	deleted := make(chan struct{})
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	applied := make(chan error, 1)
	go func() {
		applied <- s.ApplyPush(context.Background(), func(tx platstore.PushApplier) error {
			if err := tx.DeleteItem(context.Background(), p.ID, "main", "en.json"); err != nil {
				close(deleted)
				return err
			}
			close(deleted)
			<-release
			return nil
		})
	}()
	<-deleted

	type outcome struct {
		res platstore.WriteBackResult
		err error
	}
	written := make(chan outcome, 1)
	go func() {
		res, err := s.WriteBackBlocks(context.Background(), p.ID, "main", []*venue.StoredBlock{sb})
		written <- outcome{res, err}
	}()

	waiting := func() bool {
		var n int
		err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&n)
		return err == nil && n > 0
	}
	require.Eventually(t, waiting, 10*time.Second, 20*time.Millisecond,
		"the write-back waits on the uncommitted removal")

	releaseOnce()
	require.NoError(t, <-applied)
	out := <-written
	require.NoError(t, out.err)
	assert.Zero(t, out.res.Written)
	assert.Equal(t, []string{sb.Block.ID}, out.res.Skipped)
	assert.Zero(t, countRows(t, s, "blocks", `project_id=$1`, p.ID), "the removal stands")
	assert.Zero(t, countRows(t, s, "translations", `project_id=$1`, p.ID))
}
