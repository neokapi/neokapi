package tmx

// Regression test for the safeio byte-budget guard on the whole-document
// read (AD-005): a document larger than the budget must fail with the typed
// safeio.ErrByteBudget error instead of being read unbounded into memory.
// The test is in-package so it can shrink docBudget to a few KiB rather than
// streaming the default 1 GiB.

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

func TestReaderRejectsOverBudgetDocument(t *testing.T) {
	orig := docBudget
	docBudget = safeio.DefaultBudget().WithMaxBytes(4 << 10) // 4 KiB
	t.Cleanup(func() { docBudget = orig })

	// A stream that keeps producing bytes well past the budget. If the guard
	// were missing, io.ReadAll would consume all 1 MiB (and, with the real
	// default budget, an unbounded stream would consume all memory).
	over := strings.NewReader(`<?xml version="1.0"?><tmx version="1.4">` +
		strings.Repeat("<!-- padding -->", 64<<10)) // ~1 MiB

	reader := NewReader()
	require.NoError(t, reader.Open(context.Background(), &model.RawDocument{
		URI:          "test://over-budget.tmx",
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(over),
	}))
	t.Cleanup(func() { _ = reader.Close() })

	var readErr error
	for res := range reader.Read(context.Background()) {
		if res.Error != nil {
			readErr = res.Error
			break
		}
	}
	require.Error(t, readErr, "over-budget document must surface a read error")
	require.ErrorIs(t, readErr, safeio.ErrByteBudget)

	var limitErr *safeio.LimitError
	require.ErrorAs(t, readErr, &limitErr)
	require.Equal(t, int64(4<<10), limitErr.Limit)
}
