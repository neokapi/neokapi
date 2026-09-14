package pluginhost

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

func TestCommentProviderLineTextReadsTheManifestMarkers(t *testing.T) {
	p := &daemonCommentProvider{lang: manifest.CommentLanguage{Markers: manifest.CommentMarkers{
		Line:   []string{"//"},
		Block:  []manifest.CommentBlockMarker{{Open: "/*", Close: "*/", Nested: true}},
		Splice: `\`,
	}}}

	n, text, whole := p.LineText([]byte("// a comment"))
	assert.True(t, whole)
	assert.Equal(t, 12, n)
	assert.Equal(t, " a comment", text)

	_, _, whole = p.LineText([]byte(`// runs on \`))
	assert.False(t, whole, "the manifest's splice carries the comment onto the next line")

	_, _, whole = p.LineText([]byte("/* a /* b */ still open"))
	assert.False(t, whole, "the manifest's nested block closes only once the comment opened inside it has")
}
