package profile

import (
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// BrandVoiceAnnotation carries brand voice compliance findings for a block.
type BrandVoiceAnnotation struct {
	ProfileID string              `json:"profile_id"`
	Score     int                 `json:"score"` // 0-100 overall
	Findings  []BrandVoiceFinding `json:"findings"`
	Position  model.RunRange      `json:"position"`
}

// AnnotationType returns the type identifier for this annotation.
func (a *BrandVoiceAnnotation) TypeName() string { return "brand-voice" }

// FindingSeverities implements check.SeverityLister so the source-readiness
// gate (core/check) folds brand-voice findings into its severity roll-up
// without a dependency cycle.
func (a *BrandVoiceAnnotation) FindingSeverities() []check.Severity {
	out := make([]check.Severity, len(a.Findings))
	for i, f := range a.Findings {
		out[i] = f.Severity
	}
	return out
}
