package brand

import "github.com/neokapi/neokapi/core/check"

// Dimension is brand voice's category vocabulary — the value a brand finding
// sets on check.Finding.Category. Brand voice is one checkset over the
// framework's generic verification core (core/check).
type Dimension string

const (
	DimensionTone       Dimension = "tone"
	DimensionStyle      Dimension = "style"
	DimensionVocabulary Dimension = "vocabulary"
	DimensionClarity    Dimension = "clarity"
	DimensionBrand      Dimension = "brand_compliance"
)

// Severity re-exports the framework severity scale so brand findings share one
// set of levels and penalty weights with every other checker.
type Severity = check.Severity

const (
	SeverityNeutral  = check.SeverityNeutral
	SeverityMinor    = check.SeverityMinor
	SeverityMajor    = check.SeverityMajor
	SeverityCritical = check.SeverityCritical
)

// SeverityWeight returns the framework penalty weight for a severity level.
func SeverityWeight(s Severity) int { return check.SeverityWeight(s) }

// BrandVoiceFinding is the framework's unified check.Finding. A brand finding
// sets Category to a brand Dimension; the same struct flows through scoring,
// annotation, and bowrain governance as every other checker's findings.
type BrandVoiceFinding = check.Finding

// DimensionScore holds the score breakdown for a single brand dimension.
type DimensionScore struct {
	Dimension Dimension `json:"dimension"`
	Score     int       `json:"score"` // 0-100
	Penalty   int       `json:"penalty"`
	Issues    int       `json:"issues"`
}

// BrandComplianceScore is the brand-voice presentation of a check score: the
// roll-up plus the five brand dimensions. Generic checksets use check.Score;
// brand keeps this dimension-shaped view (and its ProfileID).
type BrandComplianceScore struct {
	Overall    int                 `json:"overall"` // 0-100
	Dimensions []DimensionScore    `json:"dimensions"`
	Findings   []BrandVoiceFinding `json:"findings"`
	WordCount  int                 `json:"word_count"`
	ProfileID  string              `json:"profile_id"`
}

// CalculateScore computes the Brand Compliance Score from findings, always
// presenting the five brand dimensions. Penalty aggregation uses the framework's
// severity weights (neutral=0, minor=1, major=5, critical=25); the roll-up is
// 100 − Σpenalty, clamped to [0,100].
func CalculateScore(findings []BrandVoiceFinding) BrandComplianceScore {
	penalties := make(map[Dimension]int)
	counts := make(map[Dimension]int)
	for _, f := range findings {
		penalties[Dimension(f.Category)] += check.SeverityWeight(f.Severity)
		counts[Dimension(f.Category)]++
	}

	allDims := []Dimension{DimensionTone, DimensionStyle, DimensionVocabulary, DimensionClarity, DimensionBrand}
	dimensions := make([]DimensionScore, 0, len(allDims))
	total := 0
	for _, dim := range allDims {
		penalty := penalties[dim]
		total += penalty
		score := max(100-penalty, 0)
		dimensions = append(dimensions, DimensionScore{
			Dimension: dim,
			Score:     score,
			Penalty:   penalty,
			Issues:    counts[dim],
		})
	}

	overall := max(100-total, 0)

	return BrandComplianceScore{
		Overall:    overall,
		Dimensions: dimensions,
		Findings:   findings,
	}
}
