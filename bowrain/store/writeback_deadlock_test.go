package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// sessionsWaitingOnLocks counts the database sessions waiting for a lock.
func sessionsWaitingOnLocks(t *testing.T, s *PostgresStore) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'`).Scan(&n); err != nil {
		return 0
	}
	return n
}

// A push and a write-back to the same stream both land. The push takes its row
// locks item by item and the write-back takes them in the order it read its
// batch, so each can hold a row the other waits on, and Postgres then fails one
// of them with "deadlock detected": the push, or the server run doing the
// write-back.
func TestWriteBackBlocks_AndAPushChangingTheSameRowsBothLand(t *testing.T) {
	paths := []struct {
		name  string
		apply func(s *PostgresStore, fn func(platstore.PushApplier) error) error
	}{
		{"ApplyPush", func(s *PostgresStore, fn func(platstore.PushApplier) error) error {
			return s.ApplyPush(context.Background(), fn)
		}},
		{"a transition binding the store", func(s *PostgresStore, fn func(platstore.PushApplier) error) error {
			return s.db.Transition(context.Background(), func(tx storage.Runner) error {
				return fn(s.Bind(tx))
			})
		}},
	}
	firstSteps := []struct {
		name  string
		write func(ctx context.Context, tx platstore.PushApplier, projectID string) error
	}{
		{"removing an item", func(ctx context.Context, tx platstore.PushApplier, projectID string) error {
			return tx.DeleteItem(ctx, projectID, "main", "a.json")
		}},
		{"pruning an item's blocks", func(ctx context.Context, tx platstore.PushApplier, projectID string) error {
			_, err := tx.PruneItemBlocks(ctx, projectID, "main", "a.json", nil)
			return err
		}},
		{"storing an item's new source", func(ctx context.Context, tx platstore.PushApplier, projectID string) error {
			return tx.StoreBlocksForItem(ctx, projectID, "main", "a.json",
				[]*model.Block{model.NewBlock("ka", "Alpha, revised")})
		}},
	}

	for _, path := range paths {
		for _, step := range firstSteps {
			t.Run(path.name+" "+step.name, func(t *testing.T) {
				s := newTestStore(t)
				p := createTestProject(t, s)
				seedWriteBackItem(t, s, p.ID, "a.json", model.NewBlock("ka", "Alpha"))
				seedWriteBackItem(t, s, p.ID, "b.json", model.NewBlock("kb", "Beta"))
				a := readItemByKey(t, s, p.ID, "a.json")["ka"]
				b := readItemByKey(t, s, p.ID, "b.json")["kb"]
				require.NotNil(t, a)
				require.NotNil(t, b)
				a.Block.SetTargetText("nb", "Alfa")
				b.Block.SetTargetText("nb", "Beta")

				// The push writes a.json and waits; the write-back takes b's row
				// and waits on a's; the push then removes b.json.
				wroteA := make(chan struct{})
				release := make(chan struct{})
				releaseOnce := sync.OnceFunc(func() { close(release) })
				t.Cleanup(releaseOnce)
				applied := make(chan error, 1)
				go func() {
					applied <- path.apply(s, func(tx platstore.PushApplier) error {
						err := step.write(context.Background(), tx, p.ID)
						close(wroteA)
						if err != nil {
							return err
						}
						<-release
						return tx.DeleteItem(context.Background(), p.ID, "main", "b.json")
					})
				}()
				<-wroteA

				type outcome struct {
					res platstore.WriteBackResult
					err error
				}
				written := make(chan outcome, 1)
				go func() {
					res, err := s.WriteBackBlocks(context.Background(), p.ID, "main", []*venue.StoredBlock{b, a})
					written <- outcome{res, err}
				}()
				require.Eventually(t, func() bool { return sessionsWaitingOnLocks(t, s) > 0 },
					10*time.Second, 20*time.Millisecond, "the write-back waits on the applying push")

				releaseOnce()
				require.NoError(t, <-applied, "the push applies")
				out := <-written
				require.NoError(t, out.err, "the write-back completes")
				assert.Zero(t, out.res.Written, "the push changed both rows before the write-back landed")
				assert.ElementsMatch(t, []string{a.Block.ID, b.Block.ID}, out.res.Skipped)
				assert.Zero(t, countRows(t, s, "blocks", `project_id=$1 AND item_name='b.json'`, p.ID), "the removal stands")
				assert.Zero(t, countRows(t, s, "translations", `project_id=$1`, p.ID), "no target lands on a changed row")
			})
		}
	}
}

// Removing an item directly and a write-back to its blocks both land. The
// removal deletes the item's targets and then its blocks; the write-back
// updates each block and then its target.
func TestDeleteItem_AndAWriteBackToItsBlocksBothLand(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// The removal takes the item's targets in block id order or in the order
	// they were stored, depending on its plan. The two agree when the block
	// stored first also has the lower id, so the item is seeded until it does.
	var item string
	var first, second *venue.StoredBlock
	for attempt := 0; item == ""; attempt++ {
		require.Less(t, attempt, 64, "no seeding put the blocks in id order")
		name := fmt.Sprintf("en-%d.json", attempt)
		one := model.NewBlock("k1", "One")
		one.SetTargetText("nb", "En")
		two := model.NewBlock("k2", "Two")
		two.SetTargetText("nb", "To")
		seedWriteBackItem(t, s, p.ID, name, one, two)
		byKey := readItemByKey(t, s, p.ID, name)
		require.NotNil(t, byKey["k1"])
		require.NotNil(t, byKey["k2"])
		var inOrder bool
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT $1::text < $2::text`,
			byKey["k1"].Block.ID, byKey["k2"].Block.ID).Scan(&inOrder))
		if inOrder {
			item, first, second = name, byKey["k1"], byKey["k2"]
		}
	}
	first.Block.SetTargetText("nb", "Én")
	second.Block.SetTargetText("nb", "Tå")

	// A third session holds the second block's target. The write-back takes the
	// second block's row and stops at that target; the removal takes the first
	// target and stops at the same one. Whichever gets it next then waits on a
	// row the other holds.
	holder, err := s.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Rollback() })
	rows, err := holder.QueryContext(ctx,
		`SELECT block_id FROM translations WHERE project_id=$1 AND block_id=$2 FOR UPDATE`, p.ID, second.Block.ID)
	require.NoError(t, err)
	held := 0
	for rows.Next() {
		held++
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, 1, held, "the second block's target is held")

	written := make(chan error, 1)
	go func() {
		_, err := s.WriteBackBlocks(context.Background(), p.ID, "main", []*venue.StoredBlock{second, first})
		written <- err
	}()
	require.Eventually(t, func() bool { return sessionsWaitingOnLocks(t, s) >= 1 },
		10*time.Second, 20*time.Millisecond, "the write-back waits on the held target")

	removed := make(chan error, 1)
	go func() { removed <- s.DeleteItem(context.Background(), p.ID, "main", item) }()
	require.Eventually(t, func() bool { return sessionsWaitingOnLocks(t, s) >= 2 },
		10*time.Second, 20*time.Millisecond, "the removal waits too")

	require.NoError(t, holder.Rollback())
	require.NoError(t, <-written, "the write-back completes")
	require.NoError(t, <-removed, "the removal completes")
	assert.Zero(t, countRows(t, s, "blocks", `project_id=$1 AND item_name=$2`, p.ID, item), "the removal stands")
	assert.Zero(t, countRows(t, s, "translations", `project_id=$1 AND block_id IN ($2, $3)`,
		p.ID, first.Block.ID, second.Block.ID))
}
