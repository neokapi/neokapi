package mdx

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
)

// The MDX writer replays its own file's skeleton and cannot write ESM, JSX or
// expressions from the content model, so it declares itself skeleton-bound:
// not a conversion target. A document headed for MDX converts to Markdown.
func TestWriterIsSkeletonBound(t *testing.T) {
	w := NewWriter()
	assert.Equal(t, "mdx", w.Name())
	assert.False(t, w.Generative(), "mdx must not be offered as a kconv target")
	var _ format.SkeletonStoreConsumer = w
}
