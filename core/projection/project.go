package projection

import "github.com/neokapi/neokapi/core/model"

// ProjectBlock projects a single Block to a render-AST leaf node — the
// block-first primitive of the projection layer. The result is a *fragment*: a
// table-cell projects to a table-cell node (valid only inside a row), a heading
// to a heading node, a list-item to a list-item node. A document serializer
// composes fragments inside the structural containers ProjectStream builds; a
// per-block consumer (kapi inspect --project, the convert-lab Blocks tab,
// per-block preview) renders the fragment directly within scaffolding it owns.
//
// Role resolution mirrors the writers' established chain: the canonical
// SemanticRole, falling back to the format-specific Block.Type. The full block
// is read (Properties, structure spans, geometry) so a tier-2 serializer can
// source format-specialized detail from the model rather than a foreign skeleton.
func ProjectBlock(b *model.Block) *RenderNode {
	role := b.SemanticRole()
	if role == "" {
		role = b.Type
	}
	n := &RenderNode{
		Role:    role,
		Runs:    b.SourceRuns(),
		BlockID: b.ID,
		Props:   nonEmptyProps(b.Properties),
	}
	if s, ok := b.Structure(); ok && s != nil {
		n.Level = s.Level
		n.ColSpan = s.ColSpan
		n.RowSpan = s.RowSpan
	}
	if role == model.RoleTableHeader {
		n.Header = true
	}
	if g, ok := b.Geometry(); ok && g != nil {
		n.Geometry = g
	}
	return n
}

// frame is one open container on the projection walk stack. node is where
// children attach; kind drives how matching End parts close it. A "layer" frame
// is transparent — its node is the enclosing container, so blocks inside an
// embedded-content layer attach to the layer's parent in the render tree.
type frame struct {
	node *RenderNode
	kind string // document | layer | table | table-row | list | group
}

// ProjectStream walks a Part stream (document order, as a reader emits it) once
// and assembles the normalized render AST rooted at a RoleDocument node. It is
// the generative-projection counterpart of editor.BuildContentTree: where that
// preserves the raw Layer/Group/Block anatomy for inspection, this normalizes it
// into a render model — tables as rows-of-cells, lists as items, headings with
// levels — that every serializer and the preview render identically.
//
// Table topology is taken from the canonical group shape (GroupStart Type
// "table" → "table-row" → cells, as the docling/asciidoc/csv/doclang readers and
// the normalized markdown reader emit). A run of bare table-cell blocks with no
// enclosing row group is tolerated via a best-effort flat-cell fallback (see
// flushFlatCells) so an un-normalized reader degrades to a single-row table
// rather than to standalone paragraphs.
//
// Skeleton replay is a separate path and never calls this; ProjectStream is the
// generative path only.
func ProjectStream(parts []*model.Part) *RenderNode {
	root := &RenderNode{Role: RoleDocument}
	stack := []frame{{node: root, kind: "document"}}

	cur := func() *frame { return &stack[len(stack)-1] }
	attach := func(n *RenderNode) { cur().node.Children = append(cur().node.Children, n) }
	push := func(n *RenderNode, kind string) { stack = append(stack, frame{node: n, kind: kind}) }
	// pendingCells buffers a run of bare table-cell blocks under the current
	// container so flushFlatCells can wrap them into a synthetic table once the
	// run ends (a non-cell part, or the container closing).
	var pendingCells []*RenderNode
	flush := func() {
		if len(pendingCells) == 0 {
			return
		}
		attach(flushFlatCells(pendingCells))
		pendingCells = nil
	}
	// popUntil closes frames down to (and including) the innermost frame of one
	// of the given kinds, flushing any pending flat cells first.
	popGroup := func() {
		flush()
		if len(stack) > 1 && cur().kind != "document" && cur().kind != "layer" {
			stack = stack[:len(stack)-1]
		}
	}

	for _, p := range parts {
		if p == nil || p.Resource == nil {
			continue
		}
		switch p.Type {
		case model.PartLayerStart:
			// Transparent: a layer groups structurally but does not render as a
			// node. Children attach to the enclosing container.
			push(cur().node, "layer")

		case model.PartLayerEnd:
			flush()
			if len(stack) > 1 && cur().kind == "layer" {
				stack = stack[:len(stack)-1]
			}

		case model.PartGroupStart:
			g, ok := p.Resource.(*model.GroupStart)
			if !ok {
				continue
			}
			flush()
			node, kind := groupNode(g)
			attach(node)
			push(node, kind)

		case model.PartGroupEnd:
			popGroup()

		case model.PartBlock:
			b, ok := p.Resource.(*model.Block)
			if !ok {
				continue
			}
			n := ProjectBlock(b)
			// A bare table cell whose enclosing frame is not a row means the
			// reader did not emit row groups; buffer it for flat-cell assembly.
			if isCellRole(n.Role) && cur().kind != "table-row" {
				pendingCells = append(pendingCells, n)
				continue
			}
			flush()
			attach(n)

		// Data and Media parts carry no generative render content (skeleton /
		// binary); the projection skips them. The preview surfaces media via the
		// content tree's Media leaves, not the render AST.
		case model.PartData, model.PartMedia:
			continue
		}
	}
	// Close out any trailing buffered cells.
	if len(pendingCells) > 0 {
		root.Children = append(root.Children, flushFlatCells(pendingCells))
	}
	return root
}

// groupNode maps a GroupStart to its render node and stack kind. Table and list
// groups become topology-bearing nodes; any other group type is preserved as a
// generic structural node under its own role name (section, picture, …).
func groupNode(g *model.GroupStart) (*RenderNode, string) {
	switch g.Type {
	case model.RoleTable:
		return &RenderNode{Role: model.RoleTable, BlockID: g.ID}, "table"
	case RoleTableRow:
		return &RenderNode{Role: RoleTableRow, BlockID: g.ID}, "table-row"
	case model.RoleList, "ordered-list", "unordered-list":
		return &RenderNode{Role: model.RoleList, BlockID: g.ID, Ordered: g.Type == "ordered-list"}, "list"
	default:
		return &RenderNode{Role: g.Type, BlockID: g.ID, Props: nonEmptyProps(g.Properties)}, "group"
	}
}

// flushFlatCells wraps a run of bare table-cell nodes (a reader that did not emit
// row groups) into a table. Without row-group delimiters the row topology is not
// recoverable from the block stream alone, so this is best-effort: cells split
// into rows on an explicit per-cell row hint when present, else collapse to a
// single row. Readers should emit table/table-row groups (the canonical shape)
// to get faithful multi-row tables; this keeps an un-normalized reader from
// degrading all the way to standalone paragraphs.
func flushFlatCells(cells []*RenderNode) *RenderNode {
	table := &RenderNode{Role: model.RoleTable}
	var row *RenderNode
	lastRow := ""
	for i, c := range cells {
		rk, hasHint := "", false
		if c.Props != nil {
			if v, ok := c.Props[propFlatRow]; ok {
				rk, hasHint = v, true
			}
		}
		if row == nil || (hasHint && rk != lastRow) {
			row = &RenderNode{Role: RoleTableRow}
			table.Children = append(table.Children, row)
			lastRow = rk
		}
		_ = i
		row.Children = append(row.Children, c)
	}
	return table
}

// propFlatRow is the optional per-cell row-index hint a reader may stamp on a
// flat (un-grouped) table cell so flat-cell assembly can recover rows.
const propFlatRow = "table.row"

func isCellRole(role string) bool {
	return role == model.RoleTableCell || role == model.RoleTableHeader
}

// nonEmptyProps returns props, or nil when empty, so a node omits an empty map.
func nonEmptyProps(props map[string]string) map[string]string {
	if len(props) == 0 {
		return nil
	}
	return props
}
