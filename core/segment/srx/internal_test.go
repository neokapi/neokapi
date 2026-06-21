package srx

import (
	"context"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

// TestRegexp2ReportsRuneOffsets documents and guards the load-bearing finding
// that github.com/dlclark/regexp2 reports Match/Group Index and Length as RUNE
// offsets (not bytes, not UTF-16 code units). The boundary offsets the engine
// passes to segment.Flattened.Spans are rune offsets into the masked text, so
// any unit mismatch here would silently misplace every boundary after a
// non-ASCII or supplementary-plane character.
func TestRegexp2ReportsRuneOffsets(t *testing.T) {
	// "Café. 😀 Déjà." — 'é' is one rune (and one UTF-16 unit), but 😀 is one
	// rune AND two UTF-16 code units, so a UTF-16-indexing engine would report
	// the second period one position later than the rune offset.
	s := "Café. 😀 Déjà."
	runes := []rune(s)

	var wantDots []int
	for i, r := range runes {
		if r == '.' {
			wantDots = append(wantDots, i)
		}
	}
	require.Len(t, wantDots, 2)

	re := regexp2.MustCompile(`\.`, regexp2.None)
	var got []int
	m, err := re.FindStringMatch(s)
	require.NoError(t, err)
	for m != nil {
		got = append(got, m.Index)
		m, err = re.FindNextMatch(m)
		require.NoError(t, err)
	}
	assert.Equal(t, wantDots, got, "regexp2 Index must be a rune offset")

	// Sanity: byte and UTF-16 counts differ from the rune count, so a passing
	// rune-offset assertion truly distinguishes the cases.
	assert.NotEqual(t, len(runes), len(s), "byte length should differ from rune length")
}

// TestBreaks_Kernel exercises the boundary kernel directly (rune offsets in,
// rune offsets out) without run projection.
func TestBreaks_Kernel(t *testing.T) {
	engAny, err := New(segment.BaseConfig{}, nil)
	require.NoError(t, err)
	eng := engAny.(*segmenter)

	text := []rune("One. Two. Three.")
	breaks, err := eng.Breaks(context.Background(), text, "en-US")
	require.NoError(t, err)
	// Boundaries land right after "One." (rune 4) and after "Two." (rune 9).
	assert.Equal(t, []int{4, 9}, breaks)

	// Empty input yields no breaks.
	empty, err := eng.Breaks(context.Background(), nil, "en-US")
	require.NoError(t, err)
	assert.Nil(t, empty)
}

// TestRulesCache verifies that rule selection is cached per locale.
func TestRulesCache(t *testing.T) {
	engAny, err := New(segment.BaseConfig{}, nil)
	require.NoError(t, err)
	eng := engAny.(*segmenter)

	r1, err := eng.rulesFor("en-US")
	require.NoError(t, err)
	r2, err := eng.rulesFor("en-US")
	require.NoError(t, err)
	// Same backing slice header => served from cache, not recompiled.
	require.NotEmpty(t, r1)
	assert.Equal(t, &r1[0], &r2[0])
}

// TestContextCancellation ensures a cancelled context short-circuits.
func TestContextCancellation(t *testing.T) {
	engAny, err := New(segment.BaseConfig{}, nil)
	require.NoError(t, err)
	eng := engAny.(*segmenter)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = eng.Segment(ctx, []model.Run{{Text: &model.TextRun{Text: "Hi. There."}}}, "en-US")
	assert.ErrorIs(t, err, context.Canceled)
}
