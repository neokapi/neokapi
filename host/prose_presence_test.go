package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
)

// TestProseP0_go is Go's presence test on the Prose maturity axis
// (docs/internals/format-maturity.md §2.8). It asserts that this build contains
// a reader for Go comments: the registry `kapi check` consults holds a provider
// for .go files, and that provider names its language go.
//
// It claims no rung. It fails when no Go provider is registered, so removing
// the provider turns CI red; the Prose probe reads that failure as did-not-run
// and keeps this test's output as the reason.
//
// The "must fail" subtest shows the lookup can report absence. Nothing in here
// skips.
func TestProseP0_go(t *testing.T) {
	t.Run("kapi check reads Go comments in this build", func(t *testing.T) {
		p, ok := commentProviders.For("main.go")
		require.True(t, ok, "the comment registry holds no provider for .go files")
		assert.Equal(t, "go", p.Language())
	})

	t.Run("must fail: a registry with no Go provider does not claim .go files", func(t *testing.T) {
		_, ok := comment.NewRegistry().For("main.go")
		assert.False(t, ok, "an empty registry must report the language absent")
	})
}
