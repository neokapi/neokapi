package sievepen_test

import (
	"path/filepath"
	"sync"
	"testing"

	sqltm "github.com/neokapi/neokapi/sievepen"
)

// TestNewSQLiteTM_ConcurrentFirstOpen: converge workers all open the project
// TM at once, including its very first creation+migration. Regression guard
// for the storage-layer races this exposed: the fresh-DB WAL switch returning
// an immediate "database is locked" (bypassing busy_timeout), and concurrent
// appliers double-recording the same migration version.
func TestNewSQLiteTM_ConcurrentFirstOpen(t *testing.T) {
	for round := 0; round < 5; round++ {
		path := filepath.Join(t.TempDir(), "tm.db")
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tm, err := sqltm.NewSQLiteTM(path)
				if err != nil {
					errs <- err
					return
				}
				tm.Close()
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: %v", round, err)
		}
	}
}
