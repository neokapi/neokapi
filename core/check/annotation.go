package check

import "github.com/neokapi/neokapi/core/model"

// FindingsAnnotation is the unified, producer-agnostic annotation that every
// checker writes onto a block. The same struct carries terminology,
// do-not-translate, placeholder, register, and brand-voice findings, so the CLI,
// the desktop app, and bowrain's governance read one shape.
type FindingsAnnotation struct {
	// Source identifies the checkset or profile that produced these findings.
	Source string `json:"source,omitempty"`
	// Score is the convenience roll-up (0-100) for this block.
	Score int `json:"score"`
	// Findings are the substantive output.
	Findings []Finding `json:"findings"`
	// Position is the block-level run-range the findings cover.
	Position model.RunRange `json:"position"`
}

// AnnotationType implements any.
func (a *FindingsAnnotation) TypeName() string { return AnnotationKey }

// AnnotationKey is the block annotation key and schema type for unified
// check findings.
const AnnotationKey = "quality.findings"
