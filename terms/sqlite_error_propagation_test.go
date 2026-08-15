package terms_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedConcepts inserts n concepts, each with an English and a French term.
func seedConcepts(t *testing.T, tb *terms.SQLiteStore, n int) {
	t.Helper()
	ctx := context.Background()
	for i := range n {
		require.NoError(t, tb.AddConcept(ctx, terms.Concept{
			ID:         fmt.Sprintf("c%04d", i),
			Domain:     "software",
			Definition: fmt.Sprintf("Concept number %d", i),
			Terms: []terms.Term{
				{Text: fmt.Sprintf("Term %d", i), Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: fmt.Sprintf("Terme %d", i), Locale: model.LocaleFrench, Status: model.TermPreferred},
			},
		}))
	}
}

const (
	// iterationSpan sizes the store so the two phases of Concepts are far
	// apart: collecting the IDs is one query over iterationSpan rows, while
	// materializing them is two queries per concept — some forty times the
	// work. cancelDelay lands between the two, with roughly a fivefold margin
	// on each side, so the cancellation is seen by the per-concept loop and not
	// by the opening query, which has always propagated its errors.
	iterationSpan = 4000
	cancelDelay   = 8 * time.Millisecond
)

func cancelDuringIteration(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(cancelDelay, cancel)
	t.Cleanup(func() {
		timer.Stop()
		cancel()
	})
	return ctx
}

// TestConceptsCancelledMidIteration pins the invariant that makes Concepts safe
// to write out: a cancelled context yields an error, never a short list with a
// nil error.
func TestConceptsCancelledMidIteration(t *testing.T) {
	tb, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tb.Close()
	seedConcepts(t, tb, iterationSpan)

	concepts, err := tb.Concepts(cancelDuringIteration(t))
	require.ErrorIs(t, err, context.Canceled, "Concepts must report the cancellation")
	assert.Empty(t, concepts, "no partial result alongside an error")
}

// TestExportTBXCancelledSurfacesError covers the consequence: Ctrl-C during
// `kapi terms export` must fail rather than write a well-formed TBX that is
// quietly missing concepts.
func TestExportTBXCancelledSurfacesError(t *testing.T) {
	tb, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tb.Close()
	seedConcepts(t, tb, iterationSpan)

	var buf bytes.Buffer
	err = terms.ExportTBX(cancelDuringIteration(t), tb, &buf, terms.TBXExportOptions{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, buf.Len(), "nothing written when the concept list is incomplete")
}

// corruptProperties writes invalid JSON into one concept's properties column,
// the shape a half-written or externally edited store presents.
func corruptProperties(t *testing.T, tb *terms.SQLiteStore, id string) {
	t.Helper()
	_, err := tb.DB().ExecContext(context.Background(),
		"UPDATE tb_concepts SET properties = ? WHERE id = ?", `{"broken":`, id)
	require.NoError(t, err)
}

// TestUnreadableConceptFailsEveryReadPath asserts that a concept the store
// cannot decode fails the read that would have included it — on the listing
// path and on both search paths — instead of dropping out of the result.
func TestUnreadableConceptFailsEveryReadPath(t *testing.T) {
	const badID = "c0002"

	read := map[string]func(context.Context, *terms.SQLiteStore) error{
		"Concepts": func(ctx context.Context, tb *terms.SQLiteStore) error {
			_, err := tb.Concepts(ctx)
			return err
		},
		"Search": func(ctx context.Context, tb *terms.SQLiteStore) error {
			_, _, err := tb.Search(ctx, "Term", model.LocaleEnglish, model.LocaleFrench, 0, 50)
			return err
		},
		"SearchForStream": func(ctx context.Context, tb *terms.SQLiteStore) error {
			_, _, err := tb.SearchForStream(ctx, "Term", model.LocaleEnglish, model.LocaleFrench, "", nil, 0, 50)
			return err
		},
	}

	for name, call := range read {
		t.Run(name, func(t *testing.T) {
			tb, err := terms.NewSQLiteStore(":memory:")
			require.NoError(t, err)
			defer tb.Close()
			seedConcepts(t, tb, 5)

			require.NoError(t, call(context.Background(), tb), "healthy store reads cleanly")

			corruptProperties(t, tb, badID)

			err = call(context.Background(), tb)
			require.Error(t, err, "an undecodable concept must not be skipped")
			assert.Contains(t, err.Error(), badID, "the error names the concept")
			assert.True(t, strings.Contains(err.Error(), "properties"),
				"the error says what could not be read, got: %v", err)
		})
	}
}

// failingWriter refuses every write, standing in for the destinations an export
// actually meets: a full disk, a revoked quota, a closed pipe.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

// TestExportToFailingWriterReportsError covers the other half of export safety:
// the store read cleanly, and the bytes still have to arrive. A csv.Writer
// buffers, so an export smaller than the buffer reaches the destination only at
// Flush — which makes Flush, not Write, the call that discovers the destination
// is broken. Both text exports must report that rather than returning nil over
// a file that is empty or cut off mid-record.
func TestExportToFailingWriterReportsError(t *testing.T) {
	sentinel := errors.New("no space left on device")

	export := map[string]func(context.Context, *terms.SQLiteStore, io.Writer) error{
		"CSV": func(ctx context.Context, tb *terms.SQLiteStore, w io.Writer) error {
			return terms.ExportCSV(ctx, tb, w, model.LocaleEnglish, model.LocaleFrench, true)
		},
		"TBX": func(ctx context.Context, tb *terms.SQLiteStore, w io.Writer) error {
			return terms.ExportTBX(ctx, tb, w, terms.TBXExportOptions{})
		},
	}

	for name, call := range export {
		t.Run(name, func(t *testing.T) {
			tb, err := terms.NewSQLiteStore(":memory:")
			require.NoError(t, err)
			defer tb.Close()
			// Few enough concepts that the whole export fits the csv.Writer's
			// buffer: the failure then surfaces only at Flush, which is the case
			// a Write-only error check misses.
			seedConcepts(t, tb, 3)

			var ok bytes.Buffer
			require.NoError(t, call(context.Background(), tb, &ok), "a working writer exports cleanly")
			require.NotZero(t, ok.Len(), "the export has content to lose")
			require.Less(t, ok.Len(), 4096, "the export must fit the buffer for this test to bite")

			err = call(context.Background(), tb, failingWriter{err: sentinel})
			require.Error(t, err, "an export that never reached the destination must not return nil")
			assert.ErrorIs(t, err, sentinel, "the error names what the destination said")
		})
	}
}

// TestConceptsIsAllOrError is the export-safety property stated directly:
// whatever Concepts returns with a nil error is the whole store.
func TestConceptsIsAllOrError(t *testing.T) {
	tb, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tb.Close()
	seedConcepts(t, tb, 20)

	corruptProperties(t, tb, "c0007")

	concepts, err := tb.Concepts(context.Background())
	require.Error(t, err)
	assert.Empty(t, concepts)

	count, err := tb.Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 20, count, "the rows are still there; only reading them fails")
}
