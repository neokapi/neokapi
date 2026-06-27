package projection

import "github.com/neokapi/neokapi/core/model"

// RoleTableRow is the projection-tree role for a table row. The content model
// has no first-class table-row Block role (rows are carried as GroupStart parts
// with Type "table-row", or inferred from a flat run of table-cell blocks); the
// projection tree makes the row an explicit structural node so every serializer
// renders rows the same way. RoleDocument is the synthetic root.
const (
	RoleDocument = "document"
	RoleTableRow = "table-row"
)

// RenderNode is one node in the normalized render AST (see package doc). A node
// is either structural (carries Children: document, table, table-row, list) or a
// leaf (carries Runs: paragraph, heading, table-cell, list-item, code, …).
// Roles are the canonical model.Role* vocabulary plus the two projection-local
// roles above; unknown roles are permitted and degrade to a generic block.
type RenderNode struct {
	// Role is the canonical semantic role (model.Role* or a projection role).
	Role string
	// Level is the heading level (1–6) or list nesting depth; 0 when n/a.
	Level int
	// Runs is the inline content of a leaf node (paragraph/heading/cell/list
	// item/caption/code). Structural nodes leave it nil.
	Runs []model.Run
	// Children are the structural children (rows of a table, items of a list,
	// cells of a row, blocks of the document).
	Children []*RenderNode

	// Props carries the source block's Properties verbatim (code.language,
	// table.header-kind, checkbox.checked, picture.subclass, …) so a tier-2
	// serializer can read format-specialized detail from the model rather than a
	// foreign skeleton. Nil when the block had none.
	Props map[string]string
	// Geometry is the optional spatial anchor (layout view / layout-target
	// serializers); nil for reflowable content.
	Geometry *model.GeometryAnnotation

	// ColSpan / RowSpan are the merged-cell extents, meaningful only for
	// table-cell / table-header nodes. 0 or 1 = a normal single cell.
	ColSpan int
	RowSpan int
	// Header marks a header cell (RoleTableHeader). HeaderKind (PropTableHeaderKind)
	// names the OTSL sub-kind when known.
	Header bool
	// Ordered marks an ordered (numbered) list; meaningful only on RoleList.
	Ordered bool

	// BlockID is the source block's ID, carried so the preview can anchor
	// run-indexed overlays and so inspect can cross-reference the content tree.
	BlockID string
}

// IsLeaf reports whether the node carries inline content rather than structural
// children. A node with neither (an empty paragraph) is still a leaf.
func (n *RenderNode) IsLeaf() bool { return len(n.Children) == 0 }

// HeaderKind returns the OTSL header sub-kind of a header cell (column/row/
// corner/section), or "" when unset or not a header.
func (n *RenderNode) HeaderKind() string {
	if n == nil || n.Props == nil {
		return ""
	}
	return n.Props[model.PropTableHeaderKind]
}

// Text returns the plain-text flattening of a leaf node's runs (text runs only;
// inline codes contribute nothing), a convenience for plaintext serialization
// and tests. Structural nodes return "".
func (n *RenderNode) Text() string {
	if n == nil {
		return ""
	}
	return model.RunsText(n.Runs)
}

// Walk visits n and every descendant in pre-order, invoking fn on each. A fn
// returning false prunes that node's subtree.
func (n *RenderNode) Walk(fn func(*RenderNode) bool) {
	if n == nil || !fn(n) {
		return
	}
	for _, c := range n.Children {
		c.Walk(fn)
	}
}
