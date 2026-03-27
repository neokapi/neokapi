package brand

import (
	"context"
	"time"
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
	GetScores(ctx context.Context, projectID, locale string) ([]*StoredScore, error)
	GetScoreTrends(ctx context.Context, projectID string, days int) ([]*ScoreTrend, error)
	GetScoresByStream(ctx context.Context, projectID, stream string) ([]*StoredScore, error)

	// Correction storage (feedback loop)
	StoreCorrection(ctx context.Context, correction *Correction) error
	GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*SuggestedRule, error)

	Close() error
}

// StoredScore represents a persisted brand compliance score for a block.
type StoredScore struct {
	ID             string              `json:"id"`
	ProjectID      string              `json:"project_id"`
	Stream         string              `json:"stream"`
	BlockID        string              `json:"block_id"`
	ProfileID      string              `json:"profile_id"`
	ProfileVersion int                 `json:"profile_version"`
	Locale         string              `json:"locale"`
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
}

// ScoreTrend represents an aggregated score data point for trend analysis.
type ScoreTrend struct {
	Date     string  `json:"date"` // YYYY-MM-DD
	AvgScore float64 `json:"avg_score"`
	Count    int     `json:"count"`
}
