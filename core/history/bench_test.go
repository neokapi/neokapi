package history_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/storage"
)

// historyShape is a document's block history: blocks blocks, each changed in
// langs editions on each of writes passes.
type historyShape struct{ blocks, langs, writes int }

func (s historyShape) String() string {
	return fmt.Sprintf("blocks=%d/langs=%d/writes=%d", s.blocks, s.langs, s.writes)
}

// recordedHistory writes shape's history of one document, "d-1", and returns
// the store.
func recordedHistory(b *testing.B, shape historyShape) *history.Store {
	b.Helper()
	db, err := storage.OpenWith(filepath.Join(b.TempDir(), "context.db"), storage.ProjectOptions())
	require.NoError(b, err)
	b.Cleanup(func() { _ = db.Close() })
	s, err := history.Open(db)
	require.NoError(b, err)
	op := 0
	for w := range shape.writes {
		var rows []history.Row
		for l := range shape.langs {
			op++
			for i := range shape.blocks {
				rows = append(rows, history.Row{
					Op: fmt.Sprintf("op%06d", op), Address: fmt.Sprintf("a%06d", op), Doc: "d-1",
					Block: fmt.Sprintf("p#%05d", i), Edition: fmt.Sprintf("l%02d", l),
					Before: "r:x", After: fmt.Sprintf("r:%d", w), Basis: "r:b", At: at(w),
				})
			}
		}
		require.NoError(b, s.Put(context.Background(), rows))
	}
	return s
}

// BenchmarkLatest measures reading the most recent change to each edition of
// a document, for every edition and for one of them, beside the per-block
// lookups a read of a page of 20 blocks makes, as the history grows deeper
// and wider.
func BenchmarkLatest(b *testing.B) {
	ctx := context.Background()
	for _, shape := range []historyShape{{400, 1, 1}, {2000, 1, 5}, {2000, 10, 5}} {
		s := recordedHistory(b, shape)
		b.Run(shape.String()+"/every-edition", func(b *testing.B) {
			for b.Loop() {
				rows, err := s.Latest(ctx, "d-1")
				if err != nil || len(rows) != shape.blocks*shape.langs {
					b.Fatalf("read %d rows: %v", len(rows), err)
				}
			}
		})
		b.Run(shape.String()+"/one-edition", func(b *testing.B) {
			for b.Loop() {
				rows, err := s.Latest(ctx, "d-1", "l00")
				if err != nil || len(rows) != shape.blocks {
					b.Fatalf("read %d rows: %v", len(rows), err)
				}
			}
		})
		b.Run(shape.String()+"/page-of-20-by-block", func(b *testing.B) {
			for b.Loop() {
				for i := range 20 {
					if _, found, err := s.LastWrite(ctx, "d-1", fmt.Sprintf("p#%05d", i), "l00"); err != nil || !found {
						b.Fatalf("block %d: found %v: %v", i, found, err)
					}
				}
			}
		})
	}
}
