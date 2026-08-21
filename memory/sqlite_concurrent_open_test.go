package memory_test

import (
	"path/filepath"
	"sync"
	"testing"

	sqlmemory "github.com/neokapi/neokapi/memory"
)

// TestNewSQLiteMemory_ConcurrentFirstOpen: converge workers all open the project
// content memory at once, including its very first creation+migration. Regression guard
// for the storage-layer races this exposed: the fresh-DB WAL switch returning
// an immediate "database is locked" (bypassing busy_timeout), and concurrent
// appliers double-recording the same migration version.
func TestNewSQLiteMemory_ConcurrentFirstOpen(t *testing.T) {
	for round := range 5 {
		path := filepath.Join(t.TempDir(), "memory.db")
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for range 4 {
			wg.Go(func() {
				tm, err := sqlmemory.NewSQLiteStore(path)
				if err != nil {
					errs <- err
					return
				}
				tm.Close()
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: %v", round, err)
		}
	}
}
