package ocr

import "github.com/neokapi/neokapi/core/model"

// layoutLabels is PP-DocLayoutV3's class list (the label_list order from the
// model's inference.yml). The detection output's class id indexes this slice.
var layoutLabels = []string{
	"abstract", "algorithm", "aside_text", "chart", "content",
	"display_formula", "doc_title", "figure_title", "footer", "footer_image",
	"footnote", "formula_number", "header", "header_image", "image",
	"inline_formula", "number", "paragraph_title", "reference", "reference_content",
	"seal", "table", "text", "vertical_text", "vision_footnote",
}

// layoutRoleByLabel maps a PP-DocLayout class label to a neokapi content role
// (model.Role*). Unmapped labels fall back to paragraph.
var layoutRoleByLabel = map[string]string{
	"doc_title":         model.RoleTitle,
	"paragraph_title":   model.RoleHeading,
	"abstract":          model.RoleParagraph,
	"content":           model.RoleParagraph,
	"text":              model.RoleParagraph,
	"vertical_text":     model.RoleParagraph,
	"aside_text":        model.RoleParagraph,
	"reference":         model.RoleParagraph,
	"reference_content": model.RoleParagraph,
	"number":            model.RoleParagraph,
	"table":             model.RoleTable,
	"figure_title":      model.RoleCaption,
	"chart":             model.RolePicture,
	"image":             model.RolePicture,
	"header_image":      model.RolePicture,
	"footer_image":      model.RolePicture,
	"seal":              model.RolePicture,
	"algorithm":         model.RoleCode,
	"display_formula":   model.RoleFormula,
	"inline_formula":    model.RoleFormula,
	"formula_number":    model.RoleFormula,
	"footnote":          model.RoleFootnote,
	"vision_footnote":   model.RoleFootnote,
	"header":            model.RolePageHeader,
	"footer":            model.RolePageFooter,
}

// layoutRole returns the content role for a PP-DocLayout class id. Out-of-range
// ids and unmapped labels map to paragraph.
func layoutRole(classID int) string {
	if classID < 0 || classID >= len(layoutLabels) {
		return model.RoleParagraph
	}
	if r, ok := layoutRoleByLabel[layoutLabels[classID]]; ok {
		return r
	}
	return model.RoleParagraph
}
