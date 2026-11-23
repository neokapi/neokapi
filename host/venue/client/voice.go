package client

import (
	"context"
	"net/url"
	"strconv"
	"time"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// ---------------------------------------------------------------------------
// Workspace voice compliance rollup
// ---------------------------------------------------------------------------

// VoiceRollupEntry is one project's row in the workspace voice compliance
// rollup, mirroring the server's VoiceRollupEntry. Overall and LastScoredAt are
// nil when the project has never been scored; ProfileID/ProfileName come from
// the server's voice resolution ladder, which no client can reproduce.
type VoiceRollupEntry struct {
	ProjectID    string                       `json:"project_id"`
	ProjectName  string                       `json:"project_name"`
	ProfileID    string                       `json:"profile_id,omitempty"`
	ProfileName  string                       `json:"profile_name,omitempty"`
	Overall      *int                         `json:"overall"`
	Dimensions   []coreprofile.DimensionScore `json:"dimensions,omitempty"`
	Trend        string                       `json:"trend"` // "up" | "down" | "flat" | "" (no history)
	Drift        *coreprofile.DriftResult     `json:"drift,omitempty"`
	ScoredBlocks int                          `json:"scored_blocks"`
	LastScoredAt *time.Time                   `json:"last_scored_at"`
}

// VoiceRollup is one page of the workspace voice compliance rollup.
type VoiceRollup struct {
	Projects []VoiceRollupEntry `json:"projects"`
	Total    int                `json:"total"`
	Limit    int                `json:"limit"`
	Offset   int                `json:"offset"`
}

// VoiceRollupOptions selects the rollup page and the drift window. Zero fields
// are omitted, leaving the server's defaults in force.
type VoiceRollupOptions struct {
	Limit      int
	Offset     int
	RecentDays int
	MinScore   int
	DropPoints int
	Days       int
}

// values renders the options as query parameters, omitting the zero ones.
func (o VoiceRollupOptions) values() url.Values {
	q := url.Values{}
	for name, v := range map[string]int{
		"limit":       o.Limit,
		"offset":      o.Offset,
		"recent_days": o.RecentDays,
		"min_score":   o.MinScore,
		"drop_points": o.DropPoints,
		"days":        o.Days,
	} {
		if v > 0 {
			q.Set(name, strconv.Itoa(v))
		}
	}
	return q
}

// GetVoiceRollup reads the workspace voice compliance rollup
// (GET /api/v1/:ws/voice/rollup): one row per project with its effective
// bound profile, latest score, per-dimension breakdown, trend and drift.
//
// The aggregation is the server's — it reads stored per-project scores and
// resolves the profile through a ladder (collection → stream → project →
// workspace default) that only the server can walk. A client that recomposes
// the rollup from per-project reads gets a different answer.
func (c *BowrainClient) GetVoiceRollup(ctx context.Context, opts VoiceRollupOptions) (*VoiceRollup, error) {
	var out VoiceRollup
	if err := c.getWorkspaceJSON(ctx, "/voice/rollup", opts.values(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
