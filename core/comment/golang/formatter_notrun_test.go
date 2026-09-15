package golang

import (
	"fmt"
	"go/format"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// absentFormatter is the Go provider with a formatter that did not run.
type absentFormatter struct{ Provider }

func (absentFormatter) Format(name string, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("no formatter is installed for %s: %w", name, comment.ErrFormatterNotRun)
}

func (p absentFormatter) Disagreements(name string, src []byte, _ *comment.File) ([]comment.Disagreement, error) {
	_, err := p.Format(name, src)
	return nil, err
}

// bytesFormatter is the Go provider whose formatter only returns bytes, as a
// formatter a plugin language runs as a subprocess does, compared through
// comment.CompareFormatted.
type bytesFormatter struct{ Provider }

func (bytesFormatter) Format(name string, src []byte) ([]byte, error) {
	return format.Source(src)
}

func (p bytesFormatter) Disagreements(name string, src []byte, f *comment.File) ([]comment.Disagreement, error) {
	formatted, err := p.Format(name, src)
	if err != nil {
		return nil, err
	}
	return comment.CompareFormatted(p.Provider, name, src, formatted, f)
}

// A rewrite held to a formatter that did not run is neither written nor
// refused: Rewrite returns the error, which a caller reports as not run.
func TestRewriteWithAFormatterThatDidNotRun(t *testing.T) {
	src := []byte(rewriteFixture)
	_, err := comment.Rewrite(absentFormatter{}, "demo.go", src, nil, comment.Target{ID: "type/Block"}, "Block holds exactly one unit.", comment.RenderOptions{})
	require.ErrorIs(t, err, comment.ErrFormatterNotRun)
	_, refused := comment.AsRefusal(err)
	assert.False(t, refused, "a formatter that did not run refuses nothing: %v", err)

	r, err := comment.Rewrite(absentFormatter{}, "demo.go", src, nil, comment.Target{ID: "type/Block"}, "Block holds one unit.", comment.RenderOptions{})
	require.NoError(t, err, "a rewrite that changes nothing needs no formatter")
	assert.False(t, r.Changed)
}

// CompareFormatted pairs the comments of a file and of the formatter's output
// by position, and reports those whose lines differ.
func TestCompareFormatted(t *testing.T) {
	misindented := []byte("package p\n\n// F formats.\nfunc F() {\n  // Indented with spaces.\n\t_ = 1\n}\n")
	located, err := Provider{}.Locate("m.go", misindented)
	require.NoError(t, err)
	formatted, err := format.Source(misindented)
	require.NoError(t, err)

	got, err := comment.CompareFormatted(Provider{}, "m.go", misindented, formatted, located)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "func/F/comment", located.Blocks()[got[0].Comment].ID)
	assert.Equal(t, "// Indented with spaces.", got[0].Formatted)

	clean, err := comment.CompareFormatted(Provider{}, "m.go", formatted, formatted, located)
	require.NoError(t, err)
	assert.Empty(t, clean)

	t.Run("a rewrite held to a formatter that only returns bytes is refused when it disagrees", func(t *testing.T) {
		_, err := comment.Rewrite(bytesFormatter{}, "m.go", misindented, nil, comment.Target{ID: "func/F/comment"}, "Still indented with spaces.", comment.RenderOptions{})
		refusal, ok := comment.AsRefusal(err)
		require.True(t, ok, "%v", err)
		assert.Equal(t, comment.RefusedFormatter, refusal.Reason, refusal.Detail)
	})

	t.Run("must fail: output holding another number of comments cannot be paired", func(t *testing.T) {
		extra := append([]byte(nil), formatted...)
		extra = append(extra, "\n// A comment the formatter did not write.\nvar _ = 1\n"...)
		_, err := comment.CompareFormatted(Provider{}, "m.go", misindented, extra, located)
		require.ErrorIs(t, err, comment.ErrUnlocated)
	})
}
