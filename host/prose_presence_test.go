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

// The presence tests of the formats that supply their files' comments. A format
// is present on the axis by registration, so these claim nothing there; each
// fails when the comment registry `kapi check` consults loses the format's
// provider.
func TestProseP0_yaml(t *testing.T)       { proseFormatPresence(t, "yaml") }
func TestProseP0_xml(t *testing.T)        { proseFormatPresence(t, "xml") }
func TestProseP0_html(t *testing.T)       { proseFormatPresence(t, "html") }
func TestProseP0_markdown(t *testing.T)   { proseFormatPresence(t, "markdown") }
func TestProseP0_androidxml(t *testing.T) { proseFormatPresence(t, "androidxml") }
func TestProseP0_resx(t *testing.T)       { proseFormatPresence(t, "resx") }
func TestProseP0_tmx(t *testing.T)        { proseFormatPresence(t, "tmx") }
func TestProseP0_doclang(t *testing.T)    { proseFormatPresence(t, "doclang") }
func TestProseP0_ts(t *testing.T)         { proseFormatPresence(t, "ts") }
func TestProseP0_xliff(t *testing.T)      { proseFormatPresence(t, "xliff") }
func TestProseP0_xliff2(t *testing.T)     { proseFormatPresence(t, "xliff2") }

func proseFormatPresence(t *testing.T, name string) {
	t.Helper()
	t.Run("kapi check reads the format's comments in this build", func(t *testing.T) {
		p, ok := commentProviders.ForFormat(name)
		require.True(t, ok, "the comment registry holds no provider for the %s format", name)
		assert.Equal(t, name, p.Language(), "the provider names the format its analyzer is recorded under")
	})

	t.Run("must fail: a registry with no provider for the format does not claim it", func(t *testing.T) {
		_, ok := comment.NewRegistry().ForFormat(name)
		assert.False(t, ok)
	})
}
