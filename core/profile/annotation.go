package profile

import (
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// VoiceAnnotation carries voice profile compliance findings for a block.
type VoiceAnnotation struct {
	ProfileID string         `json:"profile_id"`
	Score     int            `json:"score"` // 0-100 overall
	Findings  []VoiceFinding `json:"findings"`
	Position  model.Anchor   `json:"position"`
}

// AnnotationType returns the type identifier for this annotation.
func (a *VoiceAnnotation) TypeName() string { return "voice" }

// CheckFindings implements check.FindingLister so the source-readiness gate
// (core/check) reads voice findings without a dependency cycle.
func (a *VoiceAnnotation) CheckFindings() []check.Finding { return a.Findings }
