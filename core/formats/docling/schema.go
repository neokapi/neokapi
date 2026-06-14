package docling

// DoclingDocument JSON schema (the subset neokapi consumes). Mirrors the
// docling-core pydantic model (https://github.com/docling-project/docling-core):
// a flat content store (texts/tables/pictures/groups) plus a tree of $ref
// pointers rooted at `body`, walked in reading order. Unknown fields are ignored
// by encoding/json so the reader tolerates schema additions across Docling
// versions.

// ref is a "$ref" pointer such as "#/texts/0".
type ref struct {
	Ref string `json:"$ref"`
}

// docNode is a structural root (body / furniture): a self-ref plus an ordered
// child list.
type docNode struct {
	SelfRef  string `json:"self_ref"`
	Children []ref  `json:"children"`
}

// docGroup is a grouping node (list, ordered_list, inline, key-value region).
type docGroup struct {
	SelfRef  string `json:"self_ref"`
	Children []ref  `json:"children"`
	Name     string `json:"name"`
	Label    string `json:"label"`
}

// docText is a text item. label is the DocItemLabel (title, section_header,
// paragraph, list_item, caption, footnote, page_header, page_footer, code,
// formula, …); level applies to section_header.
type docText struct {
	SelfRef  string `json:"self_ref"`
	Label    string `json:"label"`
	Text     string `json:"text"`
	Orig     string `json:"orig"`
	Level    int    `json:"level"`
	Prov     []prov `json:"prov"`
	Children []ref  `json:"children"`
}

// docTable is a table item: provenance + OTSL-derived cell grid + captions.
type docTable struct {
	SelfRef  string    `json:"self_ref"`
	Label    string    `json:"label"`
	Prov     []prov    `json:"prov"`
	Captions []ref     `json:"captions"`
	Data     tableData `json:"data"`
}

// tableData is the resolved table grid.
type tableData struct {
	NumRows int         `json:"num_rows"`
	NumCols int         `json:"num_cols"`
	Cells   []tableCell `json:"table_cells"`
}

// tableCell is one grid cell. column_header is the current docling-core field;
// col_header is accepted for older exports.
type tableCell struct {
	Text            string `json:"text"`
	RowSpan         int    `json:"row_span"`
	ColSpan         int    `json:"col_span"`
	StartRow        int    `json:"start_row_offset_idx"`
	EndRow          int    `json:"end_row_offset_idx"`
	StartCol        int    `json:"start_col_offset_idx"`
	EndCol          int    `json:"end_col_offset_idx"`
	ColumnHeader    bool   `json:"column_header"`
	ColHeaderLegacy bool   `json:"col_header"`
	RowHeader       bool   `json:"row_header"`
	RowSection      bool   `json:"row_section"`
}

func (c tableCell) isHeader() bool {
	return c.ColumnHeader || c.ColHeaderLegacy || c.RowHeader
}

// prov is a provenance entry anchoring an item to a page region.
type prov struct {
	PageNo int   `json:"page_no"`
	BBox   *bbox `json:"bbox"`
}

// bbox is a Docling bounding box. coord_origin is "TOPLEFT" or "BOTTOMLEFT".
type bbox struct {
	L           float64 `json:"l"`
	T           float64 `json:"t"`
	R           float64 `json:"r"`
	B           float64 `json:"b"`
	CoordOrigin string  `json:"coord_origin"`
}

// doclingDoc is the top-level DoclingDocument.
type doclingDoc struct {
	SchemaName string       `json:"schema_name"`
	Version    string       `json:"version"`
	Name       string       `json:"name"`
	Body       *docNode     `json:"body"`
	Furniture  *docNode     `json:"furniture"`
	Groups     []docGroup   `json:"groups"`
	Texts      []docText    `json:"texts"`
	Tables     []docTable   `json:"tables"`
	Pictures   []docPicture `json:"pictures"`
}

// docPicture is a picture item: provenance + captions (the image binary is not
// ingested; its captions are translatable).
type docPicture struct {
	SelfRef  string `json:"self_ref"`
	Label    string `json:"label"`
	Prov     []prov `json:"prov"`
	Captions []ref  `json:"captions"`
	Children []ref  `json:"children"`
}

// docIndex resolves a self_ref string to its item.
type docIndex struct {
	texts    map[string]*docText
	tables   map[string]*docTable
	groups   map[string]*docGroup
	pictures map[string]*docPicture
}

// index builds a self_ref → item lookup over the document's flat arrays.
func (d *doclingDoc) index() *docIndex {
	idx := &docIndex{
		texts:    make(map[string]*docText, len(d.Texts)),
		tables:   make(map[string]*docTable, len(d.Tables)),
		groups:   make(map[string]*docGroup, len(d.Groups)),
		pictures: make(map[string]*docPicture, len(d.Pictures)),
	}
	for i := range d.Texts {
		idx.texts[d.Texts[i].SelfRef] = &d.Texts[i]
	}
	for i := range d.Tables {
		idx.tables[d.Tables[i].SelfRef] = &d.Tables[i]
	}
	for i := range d.Groups {
		idx.groups[d.Groups[i].SelfRef] = &d.Groups[i]
	}
	for i := range d.Pictures {
		idx.pictures[d.Pictures[i].SelfRef] = &d.Pictures[i]
	}
	return idx
}
