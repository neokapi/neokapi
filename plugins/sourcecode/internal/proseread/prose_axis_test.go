package proseread_test

import (
	"testing"

	"github.com/neokapi/neokapi/plugins/sourcecode/internal/proseread"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProseP0_ruby is Ruby's presence test on the Prose maturity axis
// (docs/internals/format-maturity.md §2.8): this build reads Ruby, so the
// language is present and scores from its rung tests rather than reporting as
// absent.
//
// It claims no rung. A comment arrives as block text with its `#` marker, one
// block per comment line, with no byte span, no line range and no directive
// classification, which is P0.
func TestProseP0_ruby(t *testing.T) {
	require.Contains(t, proseread.Grammars(), "ruby")

	got := texts(t, "testdata/kapi-desktop.rb", proseread.Options{Comments: true})
	assert.Contains(t, got, "# Homebrew Cask formula for Kapi.")
}
