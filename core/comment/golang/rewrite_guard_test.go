package golang

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rewrite guarded by a fingerprint or by prose replaces only the comment that
// was read: the same bytes are rewritten wherever they now sit, and a comment
// whose bytes or prose differ is refused as changed.
func TestRewriteGuard(t *testing.T) {
	src := []byte(rewriteFixture)
	located, err := Provider{}.Locate("demo.go", src)
	require.NoError(t, err)
	i := indexOf(t, located, "type/Block")
	c := located.Comments[i]
	sum := sha256.Sum256(src[c.Start:c.End])
	fingerprint := hex.EncodeToString(sum[:])
	require.Equal(t, fingerprint, comment.Fingerprint(src, c), "Fingerprint is the SHA-256 of the comment's bytes")

	refusedAs := func(t *testing.T, err error) comment.RefusalReason {
		t.Helper()
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		return refusal.Reason
	}

	t.Run("a matching fingerprint rewrites the comment where it now sits", func(t *testing.T) {
		moved := bytes.Replace(src, []byte("import \"fmt\"\n"), []byte("import \"fmt\"\n\nvar _ = fmt.Sprint\n"), 1)
		r, err := comment.Rewrite(Provider{}, "demo.go", moved, nil, comment.Target{ID: "type/Block", Fingerprint: fingerprint, Lines: &c.Lines}, "Block holds exactly one unit.", comment.RenderOptions{})
		require.NoError(t, err)
		assert.True(t, r.Changed)
		assert.Equal(t, format.LineRange{First: c.Lines.First + 2, Last: c.Lines.Last + 2}, r.After.Lines)
	})

	t.Run("a fingerprint the comment does not have is refused as changed", func(t *testing.T) {
		changed := bytes.Replace(src, []byte("// Block holds one unit."), []byte("// Block holds a unit."), 1)
		_, err := comment.Rewrite(Provider{}, "demo.go", changed, nil, comment.Target{ID: "type/Block", Fingerprint: fingerprint}, "Block holds exactly one unit.", comment.RenderOptions{})
		assert.Equal(t, comment.RefusedChanged, refusedAs(t, err))
	})

	t.Run("prose guards a rewrite that carries no fingerprint", func(t *testing.T) {
		other := "Block holds a unit."
		_, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: "type/Block", Prose: &other}, "Block holds exactly one unit.", comment.RenderOptions{})
		assert.Equal(t, comment.RefusedChanged, refusedAs(t, err))

		read := "Block holds one unit.\r\n"
		r, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: "type/Block", Prose: &read}, "Block holds exactly one unit.", comment.RenderOptions{})
		require.NoError(t, err)
		assert.True(t, r.Changed)
	})

	t.Run("an unguarded rewrite read at other lines is refused as stale", func(t *testing.T) {
		other := format.LineRange{First: c.Lines.First + 1, Last: c.Lines.Last + 1}
		_, err := comment.Rewrite(Provider{}, "demo.go", src, nil, comment.Target{ID: "type/Block", Lines: &other}, "Block holds exactly one unit.", comment.RenderOptions{})
		assert.Equal(t, comment.RefusedStale, refusedAs(t, err))
	})

	t.Run("an id that names no comment points at the comment with the fingerprint", func(t *testing.T) {
		renamed := bytes.Replace(src, []byte("type Block struct"), []byte("type Unit struct"), 1)
		_, err := comment.Rewrite(Provider{}, "demo.go", renamed, nil, comment.Target{ID: "type/Block", Fingerprint: fingerprint}, "Unit holds one unit.", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedUnknown, refusal.Reason)
		assert.Contains(t, refusal.Detail, `"type/Unit"`)
	})
}
