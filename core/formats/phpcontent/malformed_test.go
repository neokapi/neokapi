package phpcontent_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/phpcontent"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformed feeds truncated, garbage, and structurally broken PHP at the
// hand-rolled lexer (parseHeredoc, parseSingleQuoted/parseDoubleQuoted,
// parseArrayIndex, and the raw slicing in skelTextStringPrefix/Suffix) and
// asserts it degrades gracefully: Open succeeds, Read never panics, and the
// channel does not surface a spurious PartResult.Error.
//
// Unlike the JSON-backed formats (arb, xcstrings) which reject malformed input
// with a channel error, phpcontent is a deliberately tolerant lexer — an
// unterminated string or heredoc is consumed to EOF and a bare "<<<" with no
// label falls back to plain code. The contract this test pins is therefore
// "no panic, no spurious error, graceful output" rather than "surfaces an
// error". The skeleton path is exercised separately because it switches the
// reader onto the raw-byte slicing branch (skelTextStringPrefix/Suffix), which
// is the riskiest untested surface for out-of-range slices.
func TestReadMalformed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unterminated single quote",
			input: `<?php $x = 'never closed`,
		},
		{
			name:  "unterminated double quote",
			input: `<?php $x = "never closed`,
		},
		{
			name:  "single quote trailing backslash",
			input: `<?php $x = 'tail\`,
		},
		{
			name:  "double quote trailing backslash",
			input: `<?php $x = "tail\`,
		},
		{
			name:  "lone triple-lt",
			input: `<?php $x = <<<`,
		},
		{
			name:  "triple-lt no label then newline",
			input: "<?php $x = <<<\n",
		},
		{
			name:  "unterminated heredoc body",
			input: "<?php $x = <<<EOT\nbody that is never closed\n",
		},
		{
			name:  "unterminated nowdoc body",
			input: "<?php $x = <<<'EOT'\nbody that is never closed\n",
		},
		{
			name:  "unterminated quoted heredoc label",
			input: `<?php $x = <<<"EOT`,
		},
		{
			name:  "unterminated nowdoc label",
			input: `<?php $x = <<<'EOT`,
		},
		{
			name:  "heredoc label only, no body",
			input: "<?php $x = <<<EOT",
		},
		{
			name:  "unbalanced array index quote",
			input: `<?php $arr['key`,
		},
		{
			name:  "lone open bracket",
			input: `<?php $arr[`,
		},
		{
			name:  "open bracket at EOF only",
			input: `[`,
		},
		{
			name:  "unterminated block comment",
			input: "<?php /* never closed comment",
		},
		{
			name:  "line comment at EOF",
			input: "<?php // dangling",
		},
		{
			name:  "trailing concat operator",
			input: `<?php $x = 'a' .`,
		},
		{
			name:  "raw control bytes",
			input: "<?php $x = \x00\x01\x02\xff'partial",
		},
		{
			name:  "invalid utf8 in string",
			input: "<?php $x = '\xff\xfe broken utf8';",
		},
		{
			name:  "garbage no php tag",
			input: "definitely not php :: {[<<<'",
		},
		{
			name:  "just triple-lt and quote",
			input: "<<<'",
		},
		{
			name:  "deeply nested unbalanced brackets",
			input: strings.Repeat("[", 64) + "'x",
		},
		{
			name:  "empty",
			input: "",
		},
		{
			name:  "single null byte",
			input: "\x00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			// Non-skeleton path: the tolerant lexer should consume the
			// input without panicking and without surfacing an error.
			t.Run("plain", func(t *testing.T) {
				reader := phpcontent.NewReader()
				require.NotPanics(t, func() {
					err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
					require.NoError(t, err)
				})
				defer reader.Close()

				var sawErr bool
				require.NotPanics(t, func() {
					for result := range reader.Read(ctx) {
						if result.Error != nil {
							sawErr = true
						}
					}
				})
				assert.False(t, sawErr,
					"phpcontent tolerates malformed input; no PartResult.Error expected")
			})

			// Skeleton path: routes through the raw-byte slicing branch
			// (skelTextStringPrefix/Suffix), the riskiest untested surface.
			t.Run("skeleton", func(t *testing.T) {
				reader := phpcontent.NewReader()
				store, err := format.NewSkeletonStore()
				require.NoError(t, err)
				defer store.Close()
				reader.SetSkeletonStore(store)

				require.NotPanics(t, func() {
					err := reader.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish))
					require.NoError(t, err)
				})
				defer reader.Close()

				var sawErr bool
				require.NotPanics(t, func() {
					for result := range reader.Read(ctx) {
						if result.Error != nil {
							sawErr = true
						}
					}
				})
				assert.False(t, sawErr,
					"skeleton path must not surface a spurious error on malformed input")
			})
		})
	}
}

// TestOpenRejectsNilReader verifies Open rejects a document whose Reader is nil
// (the second arm of the doc == nil || doc.Reader == nil guard in Open) without
// panicking. The nil-document case is covered by TestReaderNilDocument.
func TestOpenRejectsNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := phpcontent.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{Reader: nil})
		require.Error(t, err)
	})
}
