package golang

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The write canary passes for the Go provider and fails for a rewrite that
// skips containment and for a provider whose prose loses a byte.
func TestVerifyRewriter(t *testing.T) {
	t.Run("the Go provider passes its write canary", func(t *testing.T) {
		require.NoError(t, comment.VerifyRewriter(Provider{}, comment.Rewrite))
	})

	t.Run("must fail: a rewrite that skips containment", func(t *testing.T) {
		err := comment.VerifyRewriter(Provider{}, uncontainedRewrite)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be refused")
	})

	t.Run("must fail: a rewrite that ignores the fingerprint", func(t *testing.T) {
		err := comment.VerifyRewriter(Provider{}, func(p comment.Provider, name string, src []byte, declared comment.Directives, target comment.Target, text string, opts comment.RenderOptions) (*comment.Rewritten, error) {
			return comment.Rewrite(p, name, src, declared, comment.Target{ID: target.ID}, text, opts)
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refused as changed")
	})

	t.Run("must fail: a provider whose prose loses a byte", func(t *testing.T) {
		err := comment.VerifyRewriter(lossyProse{}, comment.Rewrite)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "own prose")
	})

	t.Run("must fail: a language that writes no comments", func(t *testing.T) {
		require.Error(t, comment.VerifyRewriter(readOnly{}, comment.Rewrite))
	})

	t.Run("must fail: a renderer that writes */ as it is", func(t *testing.T) {
		err := comment.VerifyRewriter(rawTerminator{}, comment.Rewrite)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not refused as holding its terminator")
	})

	t.Run("must fail: a provider whose delimited prose loses a byte", func(t *testing.T) {
		err := comment.VerifyRewriter(lossyBlockProse{}, comment.Rewrite)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delimited comment with its own prose")
	})
}

// terminatorStandIn takes the place of `*/` while rawTerminator renders.
const terminatorStandIn = "zzTERMINATORzz"

// rawTerminator renders like the Go provider, then writes each `*/` the text
// holds into the comment as it is.
type rawTerminator struct{ Provider }

func (p rawTerminator) Render(name string, src []byte, c comment.Comment, text string, opts comment.RenderOptions) ([]byte, error) {
	span, err := p.Provider.Render(name, src, c, strings.ReplaceAll(text, "*/", terminatorStandIn), opts)
	return bytes.ReplaceAll(span, []byte(terminatorStandIn), []byte("*/")), err
}

// lossyBlockProse reads a delimited comment's prose without its last full stop,
// and a line comment's as the Go provider does.
type lossyBlockProse struct{ Provider }

func (p lossyBlockProse) Prose(src []byte, c comment.Comment) (string, error) {
	prose, err := p.Provider.Prose(src, c)
	if c.Style == comment.StyleBlock {
		prose = strings.TrimSuffix(prose, ".")
	}
	return prose, err
}

// uncontainedRewrite renders the text and splices it into the file without
// holding the result to comment.Contain.
func uncontainedRewrite(p comment.Provider, name string, src []byte, declared comment.Directives, target comment.Target, text string, opts comment.RenderOptions) (*comment.Rewritten, error) {
	located, err := comment.Locate(p, name, src, declared)
	if err != nil {
		return nil, err
	}
	for i, b := range located.Blocks() {
		if b.ID != target.ID {
			continue
		}
		c := located.Comments[i]
		span, err := p.(comment.Rewriter).Render(name, src, c, text, opts)
		if err != nil {
			return nil, err
		}
		out := bytes.Join([][]byte{src[:c.Start], span, src[c.End:]}, nil)
		return &comment.Rewritten{Source: out, Index: i, ID: target.ID, Before: c, After: c, Changed: !bytes.Equal(out, src)}, nil
	}
	return nil, &comment.Refusal{Reason: comment.RefusedUnknown, Detail: target.ID}
}

// lossyProse reads a comment's prose without its last character.
type lossyProse struct{ Provider }

func (p lossyProse) Prose(src []byte, c comment.Comment) (string, error) {
	prose, err := p.Provider.Prose(src, c)
	return strings.TrimSuffix(prose, "."), err
}

// readOnly locates Go comments and implements no Rewriter.
type readOnly struct{}

func (readOnly) Language() string     { return Language }
func (readOnly) Extensions() []string { return []string{".go"} }
func (readOnly) Locate(name string, src []byte) (*comment.File, error) {
	return Provider{}.Locate(name, src)
}
func (readOnly) LineText(line []byte) (int, string, bool) { return Provider{}.LineText(line) }
func (readOnly) Canary() comment.Canary                   { return Provider{}.Canary() }
