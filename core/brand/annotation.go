package brand

import "github.com/neokapi/neokapi/core/model"

// BrandVoiceAnnotation carries brand voice compliance findings for a block.
type BrandVoiceAnnotation struct {
	ProfileID string              `json:"profile_id"`
	Score     int                 `json:"score"` // 0-100 overall
	Findings  []BrandVoiceFinding `json:"findings"`
	Position  model.RunRange      `json:"position"`
}

// AnnotationType returns the type identifier for this annotation.
func (a *BrandVoiceAnnotation) TypeName() string { return "brand-voice" }
