package brand

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// BrandStore defines the interface for brand voice profile storage.
type BrandStore interface {
	// Profile CRUD
	CreateProfile(ctx context.Context, profile *VoiceProfile) error
	GetProfile(ctx context.Context, id string) (*VoiceProfile, error)
	UpdateProfile(ctx context.Context, profile *VoiceProfile) error
	DeleteProfile(ctx context.Context, id string) error
	ListProfiles(ctx context.Context, workspaceID string) ([]*VoiceProfile, error)

	// Profile version history
	ListProfileVersions(ctx context.Context, profileID string) ([]*ProfileVersion, error)
	GetProfileVersion(ctx context.Context, profileID string, version int) (*ProfileVersion, error)
	GetProfileAtTag(ctx context.Context, profileID, tagName string) (*VoiceProfile, error)

	// Profile tags
	CreateProfileTag(ctx context.Context, tag *ProfileTag) error
	ListProfileTags(ctx context.Context, profileID string) ([]*ProfileTag, error)
	DeleteProfileTag(ctx context.Context, profileID, tagName string) error

	// Score storage
	StoreScore(ctx context.Context, score *StoredScore) error
	GetScores(ctx context.Context, projectID string, locale model.LocaleID) ([]*StoredScore, error)
	GetScoreTrends(ctx context.Context, projectID string, days int) ([]*ScoreTrend, error)
	GetScoresByStream(ctx context.Context, projectID, stream string) ([]*StoredScore, error)

	// Correction storage (feedback loop)
	StoreCorrection(ctx context.Context, correction *Correction) error
	GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*SuggestedRule, error)

	// Rule decisions (the review/approve/reject/promote governance over
	// correction-derived candidate rules). Candidates are derived live from the
	// correction stream; the decision is what persists.
	RecordRuleDecision(ctx context.Context, d *RuleDecision) error
	GetRuleDecision(ctx context.Context, profileID, term string) (*RuleDecision, error)
	ListRuleDecisions(ctx context.Context, profileID string) ([]*RuleDecision, error)

	Close() error
}

// RuleDecisionStatus is the governance state of a correction-derived candidate
// rule for a given profile.
type RuleDecisionStatus string

const (
	// RuleDecisionPending is the implicit state of a surfaced candidate with no
	// recorded human decision yet. It is never stored — it is the absence of a row.
	RuleDecisionPending RuleDecisionStatus = "pending"
	// RuleDecisionApproved marks a candidate a reviewer accepted but has not yet
	// promoted (e.g. queued behind a blast-radius review).
	RuleDecisionApproved RuleDecisionStatus = "approved"
	// RuleDecisionRejected marks a candidate a reviewer declined; it is suppressed
	// from future suggestions so the same term stops re-surfacing.
	RuleDecisionRejected RuleDecisionStatus = "rejected"
	// RuleDecisionPromoted marks a candidate applied to the profile at
	// PromotedVersion — the correction is now an enforced, versioned check.
	RuleDecisionPromoted RuleDecisionStatus = "promoted"
)

// RuleDecision is the durable record of what a team decided about a
// correction-derived candidate rule for a profile. Candidates themselves are
// derived live from the correction stream (GetSuggestedRules); the decision is
// what persists — so a rejected term stops re-surfacing and a promoted term is
// traceable to the exact profile version it landed in, and to whether a human or
// the autonomy threshold promoted it.
type RuleDecision struct {
	ProfileID       string             `json:"profile_id"`
	Term            string             `json:"term"`
	Replacement     string             `json:"replacement,omitempty"`
	Dimension       Dimension          `json:"dimension,omitempty"`
	Status          RuleDecisionStatus `json:"status"`
	CorrectionCount int                `json:"correction_count"` // observed at decision time
	PromotedVersion int                `json:"promoted_version,omitempty"`
	Auto            bool               `json:"auto,omitempty"` // promoted by autonomy, not a human
	// ConceptID is the knowledge-graph concept the promoted rule denotes (AD-021).
	// It is recorded at promotion time so the term→concept link survives even after
	// the rule is demoted (and the live profile no longer carries it); empty for
	// concept-less promotions on a standalone profile.
	ConceptID string    `json:"concept_id,omitempty"`
	DecidedBy string    `json:"decided_by,omitempty"`
	DecidedAt time.Time `json:"decided_at"`
}

// CandidateRule pairs a correction-derived suggestion with the team's decision
// about it — the shape the review UI and MCP tools consume. Status is
// RuleDecisionPending when no decision has been recorded.
type CandidateRule struct {
	SuggestedRule
	Status          RuleDecisionStatus `json:"status"`
	PromotedVersion int                `json:"promoted_version,omitempty"`
	Auto            bool               `json:"auto,omitempty"`
	DecidedBy       string             `json:"decided_by,omitempty"`
	DecidedAt       *time.Time         `json:"decided_at,omitempty"`
}

// StoredScore represents a persisted brand compliance score for a block.
type StoredScore struct {
	ID             string              `json:"id"`
	ProjectID      string              `json:"project_id"`
	Stream         string              `json:"stream"`
	BlockID        string              `json:"block_id"`
	ProfileID      string              `json:"profile_id"`
	ProfileVersion int                 `json:"profile_version"`
	Locale         model.LocaleID      `json:"locale"`
	Score          int                 `json:"score"`
	Dimensions     []DimensionScore    `json:"dimensions"`
	Findings       []BrandVoiceFinding `json:"findings"`
	CheckedAt      time.Time           `json:"checked_at"`
}

// Correction records a user correction to a brand voice finding.
type Correction struct {
	ID            string    `json:"id"`
	ProfileID     string    `json:"profile_id"`
	BlockID       string    `json:"block_id"`
	Dimension     Dimension `json:"dimension"`
	OriginalText  string    `json:"original_text"`
	CorrectedText string    `json:"corrected_text"`
	FindingID     string    `json:"finding_id,omitempty"`
	CorrectedBy   string    `json:"corrected_by"`
	CorrectedAt   time.Time `json:"corrected_at"`
}

// SuggestedRule represents a vocabulary rule derived from repeated corrections.
type SuggestedRule struct {
	Term            string    `json:"term"`
	Replacement     string    `json:"replacement"`
	CorrectionCount int       `json:"correction_count"`
	Dimension       Dimension `json:"dimension"`
	// ConceptID is the knowledge-graph concept the promoted rule should denote.
	// The platform sets it from a concept-backed correction; it is empty for
	// suggestions derived outside a knowledge graph.
	ConceptID string `json:"concept_id,omitempty"`
}

// ScoreTrend represents an aggregated score data point for trend analysis.
type ScoreTrend struct {
	Date     string  `json:"date"` // YYYY-MM-DD
	AvgScore float64 `json:"avg_score"`
	Count    int     `json:"count"`
}
