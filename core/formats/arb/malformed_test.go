package arb_test

import (
	"testing"

	arb "github.com/neokapi/neokapi/core/formats/arb"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadInvalidJSON feeds malformed JSON and asserts that Read surfaces a
// clean error on its result channel rather than panicking.
func TestReadInvalidJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "not json",
			input: "definitely not json :: {[",
		},
		{
			// ARB must be a top-level object; an array is rejected.
			name:  "json array not object",
			input: `["a", "b"]`,
		},
		{
			name:  "truncated object",
			input: `{"appTitle": "Flutter`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := arb.NewReader()
			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			var foundError bool
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					if result.Error != nil {
						foundError = true
					}
				}
			})
			assert.True(t, foundError, "expected a clean error for malformed ARB input")
		})
	}
}

// TestReadBrokenICUPlaceholder feeds a syntactically valid ARB document whose
// message value contains an unbalanced ICU placeholder ("{name" with no closing
// brace). The reader must not panic: per icu.go's matchBrace, an unbalanced
// brace is treated as literal text so the value still round-trips, and the
// message is still extracted as a translatable block.
func TestReadBrokenICUPlaceholder(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := arb.NewReader()
	input := `{"@@locale": "en", "greeting": "Hello, {name"}`

	require.NotPanics(t, func() {
		err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	var blocks []*model.Block
	require.NotPanics(t, func() {
		blocks = testutil.CollectBlocks(t, reader.Read(ctx))
	})

	require.Len(t, blocks, 1)
	greeting := blocks[0]
	assert.Equal(t, "greeting", greeting.Name)
	// The unbalanced brace is kept verbatim in the (literal) text, so a faithful
	// render reproduces the original opaque value rather than dropping it.
	assert.Equal(t, "Hello, {name", model.RenderRunsWithData(greeting.SourceRuns()))
}

// TestReadNilDocument verifies Open rejects a nil document without panicking.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := arb.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}
