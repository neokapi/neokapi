package regex_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/regex"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadUncompilablePattern feeds an uncompilable regex pattern (an
// unterminated group "(") and asserts that the reader surfaces a single clean
// error on its result channel rather than panicking. readContent compiles each
// rule up front and routes a compile failure through emitError, so the error
// must arrive on the channel as PartResult.Error.
func TestReadUncompilablePattern(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := regex.NewReader()

	cfg := reader.Config().(*regex.Config)
	cfg.Rules = []regex.Rule{
		{
			// "(" is not a valid Go regexp: missing closing ")".
			Pattern:     "(",
			SourceGroup: 1,
		},
	}

	require.NotPanics(t, func() {
		err := reader.Open(ctx, testutil.RawDocFromString(`"key" = "value";`, model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	var errCount int
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				errCount++
			}
		}
	})
	assert.Equal(t, 1, errCount, "expected exactly one clean error for an uncompilable pattern")
}

// TestReadMalformedInput feeds empty, truncated, and garbage inputs and asserts
// that the reader never panics. With no rules configured the reader treats the
// whole input as opaque Data, so these inputs must drain cleanly without an
// error on the channel.
func TestReadMalformedInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty",
			input: "",
		},
		{
			name:  "truncated strings entry",
			input: `"key" = "Hello`,
		},
		{
			name:  "binary garbage",
			input: "\x00\x01\x02\xff\xfe not text at all \x00",
		},
		{
			name:  "lone backslashes",
			input: `\ \\ \\\ key = `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := regex.NewReader()

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			var sawError bool
			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					if result.Error != nil {
						sawError = true
					}
				}
			})
			assert.False(t, sawError, "no rules configured: malformed input should drain as Data without error")
		})
	}
}

// TestReadMalformedInputWithRules feeds the same broken/garbage inputs through a
// configured Mac .strings rule. Non-matching or partially matching input must
// still drain without panicking; truncated entries simply yield no block.
func TestReadMalformedInputWithRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "truncated strings entry",
			input: `"key" = "Hello`,
		},
		{
			name:  "binary garbage",
			input: "\x00\x01\x02\xff\xfe not text at all \x00",
		},
		{
			name:  "lone backslashes",
			input: `\ \\ \\\ key = `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := regex.NewReader()

			cfg := reader.Config().(*regex.Config)
			cfg.Rules = []regex.Rule{
				{
					Pattern:     `"([^"]*?)"\s*=\s*"((?:[^"\\]|\\.)*)"\s*;`,
					SourceGroup: 2,
					IDGroup:     1,
				},
			}

			require.NotPanics(t, func() {
				err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					require.NoError(t, result.Error)
				}
			})
		})
	}
}

// TestReadNilReader verifies Open rejects a RawDocument whose Reader is nil
// without panicking, mirroring the nil-document guard in reader.go's Open.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := regex.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{URI: "test://input"})
		require.Error(t, err)
	})
}
