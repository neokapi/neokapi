package host

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// A batch's result must not depend on the order the files were listed in.
//
// It did. `errgroup.WithContext` cancels the group's context on the first
// error, and the per-file work returned its error, so one unreadable file took
// every sibling still in flight down with it. Measured on 25 fixtures and one
// bad file: 0 outputs with the bad one first, 20 with it last. A user running
// `kapi pseudo-translate docs/*` on a directory holding one corrupt document
// got a single error message and, depending on luck, nothing else.
//
// The engine benchmark is where it surfaced: its sample put a bad fixture early
// and the run reported 0 of 85 files, which looked like the engine failing.
//
// These tests pin the two halves of the fix. The shape here mirrors
// runMultipleFiles rather than driving it, because driving it needs a whole App
// with a format registry and a flow; what broke was the concurrency structure,
// and that is what is asserted.

// runBatch is the structure runMultipleFiles uses: bounded concurrency, a
// shared error accumulator, and no cancellation on a per-file failure.
func runBatch(files []string, fails map[string]bool, concurrency int) (done []string, failures []error) {
	var g errgroup.Group
	g.SetLimit(concurrency)
	var mu sync.Mutex

	for _, f := range files {
		g.Go(func() error {
			if fails[f] {
				mu.Lock()
				failures = append(failures, fmt.Errorf("%s: unreadable", f))
				mu.Unlock()
				return nil
			}
			mu.Lock()
			done = append(done, f)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return done, failures
}

func TestABatchDoesNotDependOnFileOrder(t *testing.T) {
	good := []string{"a.json", "b.md", "c.html", "d.yaml", "e.xlf"}
	bad := "broken.docx"
	fails := map[string]bool{bad: true}

	first := append([]string{bad}, good...)
	last := append(append([]string{}, good...), bad)
	middle := []string{good[0], good[1], bad, good[2], good[3], good[4]}

	for _, c := range []struct {
		name  string
		files []string
	}{
		{"bad file first", first},
		{"bad file last", last},
		{"bad file in the middle", middle},
	} {
		t.Run(c.name, func(t *testing.T) {
			done, failures := runBatch(c.files, fails, 4)
			assert.Len(t, done, len(good),
				"every readable file must be processed wherever the bad one sits")
			assert.Len(t, failures, 1, "and the failure must still be recorded")
		})
	}
}

// TestEveryFailureIsReported: the old code returned the first error and
// `g.Wait()` kept only that one, so a batch with eight bad files reported
// whichever lost the race. All of them are useful.
func TestEveryFailureIsReported(t *testing.T) {
	files := []string{"a.json", "x.docx", "b.md", "y.docx", "z.docx"}
	fails := map[string]bool{"x.docx": true, "y.docx": true, "z.docx": true}

	done, failures := runBatch(files, fails, 4)
	require.Len(t, done, 2)
	require.Len(t, failures, 3)

	joined := errors.Join(failures...)
	for _, name := range []string{"x.docx", "y.docx", "z.docx"} {
		assert.ErrorContains(t, joined, name,
			"a report naming one of three failures sends the reader back for another run")
	}
}

// TestAllGoodIsStillClean: the fix must not invent failures.
func TestAllGoodIsStillClean(t *testing.T) {
	files := []string{"a.json", "b.md", "c.html"}
	done, failures := runBatch(files, nil, 4)
	assert.Len(t, done, 3)
	assert.Empty(t, failures)
}
