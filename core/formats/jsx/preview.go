package jsx

import (
	"fmt"
	"html"
	"strings"

	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
)

// Ensure Reader implements PreviewBuilder.
var _ format.PreviewBuilder = (*Reader)(nil)

// BuildPreview renders a KBF bundle as the components it was extracted from.
//
// A bundle is a catalog, but it is a catalog *of a React tree*: every block
// records the file, the component and the element it came from, and its runs
// carry the JSX structure — a variable, a nested element, a link. Without a
// preview of its own a bundle fell through to the generic block listing, which
// is a column of plain strings: the structure gone, every variable gone with
// it (a run with no text contributes nothing to a text listing), and no way to
// tell which component a string belongs to.
//
// So the preview groups the blocks by the component they were extracted from,
// and renders each through the same vocabulary the browser draws chips from —
// `core/kbf.RenderBlockHTML`, which is also what emits the `<kat-block>`
// wrapper every preview needs for selection and target switching.
func (r *Reader) BuildPreview(parts []*model.Part) string {
	// The same block renderer the block-level preview uses, so a block reads
	// identically whether the host asked for the document or for one block.
	blocks := NewPreviewBuilder()
	var body strings.Builder
	body.WriteString(previewStyle)
	body.WriteString(`<div class="kbf-doc">`)

	var current string
	for _, part := range parts {
		block, ok := part.Resource.(*model.Block)
		if !ok || block == nil {
			continue
		}
		ann, ok := model.AnnoAs[*KBFAnnotation](block, AnnotationType)
		if !ok || ann == nil {
			continue
		}

		// One heading per component, in document order — the order the
		// extractor walked the tree, which is the order the component reads in.
		if heading := componentHeading(ann); heading != current {
			if current != "" {
				body.WriteString(`</div>`)
			}
			current = heading
			fmt.Fprintf(&body, `<div class="kbf-component"><div class="kbf-component-name">%s</div>`,
				html.EscapeString(heading))
		}

		fmt.Fprintf(&body, `<div class="kbf-block"><span class="kbf-where" title=%q>%s</span>%s</div>`,
			html.EscapeString(blockOrigin(ann)),
			html.EscapeString(elementLabel(ann)),
			blocks.BuildBlockPreview(block))
	}
	if current != "" {
		body.WriteString(`</div>`)
	}
	body.WriteString(`</div>`)

	return editor.PreviewBoilerplateStart() + body.String() + editor.PreviewBoilerplateEnd()
}

// componentHeading names the component a block was extracted from, falling back
// to its file when the extractor recorded no component name.
func componentHeading(ann *KBFAnnotation) string {
	if ann.Properties.Component != "" {
		return ann.Properties.Component
	}
	if ann.Properties.File != "" {
		return ann.Properties.File
	}
	return ann.DocumentPath
}

// elementLabel is the JSX element or attribute the block sits in, written the
// way it appears in the source: `<Text>` for an element, `alt=` for an
// attribute. Empty when the extractor recorded neither.
func elementLabel(ann *KBFAnnotation) string {
	element := ann.Properties.Element
	if element == "" {
		return ""
	}
	if ann.Type == kbf.BlockTypeJSXAttribute {
		if attr := attributeName(ann.Properties.JSXPath); attr != "" {
			return attr + "="
		}
	}
	return "<" + element + ">"
}

// attributeName reads the attribute out of a jsxPath like `img[alt]`.
func attributeName(jsxPath string) string {
	open := strings.LastIndex(jsxPath, "[")
	closing := strings.LastIndex(jsxPath, "]")
	if open < 0 || closing < open {
		return ""
	}
	return jsxPath[open+1 : closing]
}

// blockOrigin is where the block came from, for the label's tooltip:
// `src/credits-warning.tsx:77`.
func blockOrigin(ann *KBFAnnotation) string {
	file := ann.Properties.File
	if file == "" {
		file = ann.DocumentPath
	}
	if ann.Properties.Line > 0 {
		return fmt.Sprintf("%s:%d", file, ann.Properties.Line)
	}
	return file
}

// The preview's own type: the component headings and the element gutter that
// make a catalog read as the tree it was extracted from. The chips inside a
// block are styled by the vocabulary's own markup (core/kbf), so nothing here
// restyles them.
const previewStyle = `<style>
  .kbf-doc { max-width: 60rem; }
  .kbf-component { margin: 0 0 18px; }
  .kbf-component-name {
    font: 600 12px/1.4 ui-monospace, SFMono-Regular, Menlo, monospace;
    color: #64748b; letter-spacing: 0.04em; text-transform: none;
    padding-bottom: 4px; margin-bottom: 6px; border-bottom: 1px solid #e2e8f0;
  }
  .kbf-block { display: flex; gap: 10px; align-items: baseline; padding: 3px 0; }
  .kbf-where {
    flex: 0 0 7.5rem; text-align: right; cursor: help;
    font: 400 11px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; color: #94a3b8;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .neokapi-var {
    display: inline-block; padding: 0 4px; border-radius: 3px;
    background: rgba(59,130,246,0.12); color: #1e40af;
    font: 500 0.9em ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .neokapi-node {
    display: inline-block; padding: 0 4px; border-radius: 3px;
    background: rgba(100,116,139,0.12); color: #334155;
    font: 500 0.9em ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  /* A plural is several readings of one block, and each form needs to read as
     its own line: the shared renderer labels them, and unstyled the label ran
     straight into the text ("plural:zeroYour cart is empty"). */
  .neokapi-plural, .neokapi-select { display: block; }
  .neokapi-plural-form, .neokapi-select-case {
    display: flex; gap: 8px; align-items: baseline;
  }
  .neokapi-plural-form-label, .neokapi-select-case-label {
    flex: 0 0 5.5rem; text-align: right; color: #94a3b8;
    font: 400 10px/1.8 ui-monospace, SFMono-Regular, Menlo, monospace;
  }
</style>`
