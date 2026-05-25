package brand

import "github.com/neokapi/neokapi/core/model"

// Dimension represents a brand voice evaluation category.
type Dimension string

const (
	DimensionTone       Dimension = "tone"
	DimensionStyle      Dimension = "style"
	DimensionVocabulary Dimension = "vocabulary"
	DimensionClarity    Dimension = "clarity"
	DimensionBrand      Dimension = "brand_compliance"
)

// Severity represents the impact level of a brand voice finding.
type Severity string

const (
	SeverityNeutral  Severity = "neutral"
	SeverityMinor    Severity = "minor"
	SeverityMajor    Severity = "major"
	SeverityCritical Severity = "critical"
)

// SeverityWeight returns the MQM penalty weight for a severity level.
func SeverityWeight(s Severity) int {
	switch s {
	case SeverityNeutral:
		return 0
	case SeverityMinor:
		return 1
	case SeverityMajor:
		return 5
	case SeverityCritical:
		return 25
	default:
		return 0
	}
}

// BrandVoiceFinding represents a single brand voice compliance issue.
type BrandVoiceFinding struct {
	Dimension    Dimension      `json:"dimension"`
	Severity     Severity       `json:"severity"`
	Message      string         `json:"message"`
	Suggestion   string         `json:"suggestion,omitempty"`
	Position     model.RunRange `json:"position"`
	OriginalText string         `json:"original_text,omitempty"`
}

// DimensionScore holds the score breakdown for a single dimension.
type DimensionScore struct {
	Dimension Dimension `json:"dimension"`
	Score     int       `json:"score"` // 0-100
	Penalty   int       `json:"penalty"`
	Issues    int       `json:"issues"`
}

// BrandComplianceScore holds the overall brand voice compliance result.
type BrandComplianceScore struct {
	Overall    int                 `json:"overall"` // 0-100
	Dimensions []DimensionScore    `json:"dimensions"`
	Findings   []BrandVoiceFinding `json:"findings"`
	WordCount  int                 `json:"word_count"`
	ProfileID  string              `json:"profile_id"`
}

// CalculateScore computes the Brand Compliance Score from findings.
// Uses MQM-inspired penalty weighting: neutral=0, minor=1, major=5, critical=25.
// The overall score starts at 100 and is reduced by total penalties, clamped to 0.
func CalculateScore(findings []BrandVoiceFinding) BrandComplianceScore {
	dimPenalties := make(map[Dimension]int)
	dimCounts := make(map[Dimension]int)

	for _, f := range findings {
		dimPenalties[f.Dimension] += SeverityWeight(f.Severity)
		dimCounts[f.Dimension]++
	}

	allDims := []Dimension{DimensionTone, DimensionStyle, DimensionVocabulary, DimensionClarity, DimensionBrand}
	var dimensions []DimensionScore
	totalPenalty := 0

	for _, dim := range allDims {
		penalty := dimPenalties[dim]
		totalPenalty += penalty
		score := 100 - penalty
		if score < 0 {
			score = 0
		}
		dimensions = append(dimensions, DimensionScore{
			Dimension: dim,
			Score:     score,
			Penalty:   penalty,
			Issues:    dimCounts[dim],
		})
	}

	overall := 100 - totalPenalty
	if overall < 0 {
		overall = 0
	}

	return BrandComplianceScore{
		Overall:    overall,
		Dimensions: dimensions,
		Findings:   findings,
	}
}
