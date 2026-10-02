package golang

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderOwnProsePreservesMarkers(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(eol, func(t *testing.T) {
			source := []byte("package p\n\n// An example command.\n//\n// \tgo run .\n\nvar Value = 1\n")
			source = bytes.ReplaceAll(source, []byte("\n"), []byte(eol))
			located, err := (Provider{}).Locate("p.go", source)
			require.NoError(t, err)
			require.Len(t, located.Comments, 1)
			c := located.Comments[0]
			prose, err := (Provider{}).Prose(source, c)
			require.NoError(t, err)
			span, err := (Provider{}).Render("p.go", source, c, prose, comment.RenderOptions{})
			require.NoError(t, err)
			assert.Equal(t, source[c.Start:c.End], span)
		})
	}
}
