package formats

import (
	"sort"

	"github.com/neokapi/neokapi/core/formats/asciidoc"
	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
)

// blockFragmenters maps a target-format id to its per-block projection
// serializer (projection.RenderNode -> markup fragment). These are the
// "writer over a RenderNode" entry points the per-block projection view uses;
// the byte-exact document writers are unaffected.
var blockFragmenters = map[string]func(*projection.RenderNode) string{
	"html":     html.FragmentHTML,
	"markdown": markdown.FragmentMarkdown,
	"asciidoc": asciidoc.FragmentAsciidoc,
}

// RenderBlockFragment projects a single Block to the render AST
// (projection.ProjectBlock) and serializes it to a fragment in the named target
// format — the per-block "project" view powering `kapi inspect --project` and
// the convert-lab Blocks tab (each block rendered in each format inline, without
// running a whole-document convert per format). Returns ("", false) for an
// unsupported format.
func RenderBlockFragment(b *model.Block, format string) (string, bool) {
	fn, ok := blockFragmenters[format]
	if !ok {
		return "", false
	}
	return fn(projection.ProjectBlock(b)), true
}

// BlockFragmentFormats lists the target formats RenderBlockFragment supports,
// sorted for a stable UI/CLI listing.
func BlockFragmentFormats() []string {
	out := make([]string, 0, len(blockFragmenters))
	for k := range blockFragmenters {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
