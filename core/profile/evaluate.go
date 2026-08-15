package profile

import "github.com/neokapi/neokapi/core/model"

// VoiceEvaluation captures the results of comparing voice profiles
// against content across two streams or profile versions.
type VoiceEvaluation struct {
	Stream          string `json:"stream"`
	BaselineStream  string `json:"baseline_stream"`
	StreamProfile   string `json:"stream_profile"`
	BaselineProfile string `json:"baseline_profile"`
	BlocksEvaluated int    `json:"blocks_evaluated"`

	StreamScore   AggregateScore `json:"stream_score"`
	BaselineScore AggregateScore `json:"baseline_score"`
	ScoreDelta    int            `json:"score_delta"` // stream - baseline (positive = improvement)

	BlastRadius         BlastRadius           `json:"blast_radius"`
	DimensionComparison []DimensionComparison `json:"dimension_comparison"`
	TopFindings         []EvaluationFinding   `json:"top_findings"`
}

// AggregateScore holds statistical aggregation of scores across blocks.
type AggregateScore struct {
	Overall      float64        `json:"overall"`
	Min          int            `json:"min"`
	Max          int            `json:"max"`
	Median       int            `json:"median"`
	Distribution map[string]int `json:"distribution"` // "0-25", "26-50", "51-75", "76-100"
}

// BlastRadius measures the impact of a voice change across content.
type BlastRadius struct {
	TotalBlocks        int                     `json:"total_blocks"`
	AffectedBlocks     int                     `json:"affected_blocks"`
	ImprovedBlocks     int                     `json:"improved_blocks"`
	DegradedBlocks     int                     `json:"degraded_blocks"`
	NewViolations      int                     `json:"new_violations"`
	ResolvedViolations int                     `json:"resolved_violations"`
	CriticalCount      int                     `json:"critical_count"`
	// PrescribedBlocks counts the affected blocks on which the candidate does
	// more than flag: at least one violation it newly raises carries a
	// replacement, so the guidance is "write this instead" rather than "look at
	// this". The two cost different amounts of work — a flag is answered by
	// annotating the text, a prescribed replacement by editing it — and a reader
	// deciding whether to adopt a rule needs to know which of the two the
	// AffectedBlocks number is buying.
	PrescribedBlocks int                     `json:"prescribed_blocks"`
	Collections      []CollectionBlastRadius `json:"collections"`
}

// CollectionBlastRadius breaks down impact for a single collection.
type CollectionBlastRadius struct {
	CollectionID   string  `json:"collection_id"`
	CollectionName string  `json:"collection_name"`
	AffectedBlocks int     `json:"affected_blocks"`
	AvgScoreDelta  float64 `json:"avg_score_delta"`
}

// DimensionComparison compares a single scoring dimension across two profiles.
type DimensionComparison struct {
	Dimension   string  `json:"dimension"`
	StreamAvg   float64 `json:"stream_avg"`
	BaselineAvg float64 `json:"baseline_avg"`
	Delta       float64 `json:"delta"`
}

// EvaluationFinding is a single voice profile finding with content context.
type EvaluationFinding struct {
	BlockID      string       `json:"block_id"`
	ItemName     string       `json:"item_name"`
	CollectionID string       `json:"collection_id"`
	SourceText   string       `json:"source_text"` // first 200 chars
	TargetText   string       `json:"target_text"` // first 200 chars (if locale specified)
	Finding      VoiceFinding `json:"finding"`
	IsNew        bool         `json:"is_new"` // true if finding doesn't exist under baseline profile
}

// EvaluateRequest holds parameters for a voice profile evaluation.
type EvaluateRequest struct {
	Stream             string         `json:"stream"`
	BaselineStream     string         `json:"baseline_stream"`
	ProfileTag         string         `json:"profile_tag,omitempty"`
	BaselineProfileTag string         `json:"baseline_profile_tag,omitempty"`
	Locale             model.LocaleID `json:"locale,omitempty"`
	Collection         string         `json:"collection,omitempty"`
	SampleSize         int            `json:"sample_size,omitempty"` // 0 = all blocks
}
