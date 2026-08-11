package editor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The preview document is shown two ways, and they want opposite things from
// overflow. A host that fixes the frame's height — the file preview panel, the
// editor's side-by-side pane — needs the document to scroll, or everything past
// the first screen is unreachable. The inline visual editor instead grows the
// frame to the content and scrolls its own page, and there a second scrollbar
// inside the frame is wrong.
//
// It used to be unconditionally hidden, which is the inline case's answer given
// to both, and the preview panel could not be scrolled at all.
func TestPreviewBoilerplateScrolls(t *testing.T) {
	head := PreviewBoilerplateStart()

	assert.Contains(t, head, "html, body { overflow: auto; }",
		"a preview in a fixed-height frame has to be able to scroll itself")
	assert.Contains(t, head, "html.kat-fit, html.kat-fit body { overflow: hidden; }",
		"a host that grows the frame to the content asks for the clip explicitly")

	// The opt-out has to be reachable, or the default is the only behaviour.
	require.Contains(t, PreviewBoilerplateEnd(), "kat-fit-height",
		"the host switches modes by message; without the handler nothing can opt out")
	assert.Contains(t, PreviewBoilerplateEnd(), `classList.toggle('kat-fit'`,
		"the message toggles the class the stylesheet keys on")
}

// Every reader that builds its own preview goes through the same boilerplate,
// so the scrolling contract holds for every format rather than for the generic
// fallback alone.
func TestGenericPreviewCarriesTheBoilerplate(t *testing.T) {
	html := BuildGenericPreview(nil)
	assert.True(t, strings.Contains(html, "overflow: auto") || strings.Contains(html, "kat-preview"),
		"the generic preview is either boilerplate-wrapped or marked for the kit to render")
}
