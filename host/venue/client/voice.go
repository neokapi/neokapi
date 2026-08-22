package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// This file holds the voice profile push surface: the idempotent upsert
// `kapi push` uses to carry the recipe-bound voice profile into the workspace
// brand hub. Like the knowledge graph, brand profiles live on the workspace
// content group (/api/v1/:ws/voice-profiles), so the method requires a
// workspace-scoped client (NewWorkspaceBowrainClient).

// ErrForbidden marks a request the server refused with HTTP 403 — for the
// brand upsert, a caller without the manage-brand permission. Callers detect
// it with errors.Is and degrade gracefully (a push skips the profile instead
// of failing).
var ErrForbidden = errors.New("permission denied")

// VoiceProfileUpsert mirrors the server's VoiceProfileRequest: the authored
// surface of a voice profile. Server-managed metadata (id, workspace, version,
// autonomy, timestamps) is never sent — the upsert cannot touch it.
type VoiceProfileUpsert struct {
	Name        string                                        `json:"name"`
	Description string                                        `json:"description,omitempty"`
	Tone        coreprofile.ToneProfile                       `json:"tone"`
	Style       coreprofile.StyleRules                        `json:"style"`
	Vocabulary  coreprofile.VocabularyRules                   `json:"vocabulary"`
	Examples    []coreprofile.VoiceExample                    `json:"examples"`
	Locales     map[model.LocaleID]coreprofile.LocaleOverride `json:"locales,omitempty"`
	Channels    map[string]coreprofile.ChannelOverride        `json:"channels,omitempty"`
	Personas    map[string]coreprofile.PersonaOverride        `json:"personas,omitempty"`
	// MinScore is the profile's own compliance bar (0–100); 0 means the default.
	// It is authored content — a recipe that raises the bar carries the raised
	// bar into the workspace rather than leaving the server on the default.
	MinScore int `json:"min_score,omitempty"`
}

// VoiceProfileUpsertFromProfile builds the upsert payload from a resolved
// local voice profile, carrying only the authored surface.
func VoiceProfileUpsertFromProfile(p *coreprofile.VoiceProfile) VoiceProfileUpsert {
	return VoiceProfileUpsert{
		Name:        p.Name,
		Description: p.Description,
		Tone:        p.Tone,
		Style:       p.Style,
		Vocabulary:  p.Vocabulary,
		Examples:    p.Examples,
		Locales:     p.Locales,
		Channels:    p.Channels,
		Personas:    p.Personas,
		MinScore:    p.MinScore,
	}
}

// VoiceProfileUpsertResult reports what the upsert did. Action is "created",
// "updated", or "unchanged"; Profile is the stored workspace profile after the
// action (its Version reflects any bump).
type VoiceProfileUpsertResult struct {
	Action  string                    `json:"action"`
	Profile *coreprofile.VoiceProfile `json:"profile"`
}

// ContextScanApprovedTerm is one candidate term a reviewer kept on a context scan.
// Locale falls back to the request's Locale, then to "en".
type ContextScanApprovedTerm struct {
	Term       string `json:"term"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

// ContextScanApproval is the reviewed outcome of a context scan: the edited draft
// profile and the terms that survived review, applied in one request.
type ContextScanApproval struct {
	// At is the point the approved artefacts would govern. Empty is the
	// project's default point. The server refuses a point whose axes the
	// workspace declares no content on, so this is a 409 rather than a
	// silently-ignored field.
	At      map[string]string         `json:"at,omitempty"`
	Profile VoiceProfileUpsert        `json:"profile"`
	Terms   []ContextScanApprovedTerm `json:"terms,omitempty"`
	Locale  string                    `json:"locale,omitempty"`
}

// ContextScanApprovalResult reports what the approval applied: the stored profile
// and which action produced it, plus the concepts created and the ones already
// present.
type ContextScanApprovalResult struct {
	Profile          *coreprofile.VoiceProfile `json:"profile"`
	ProfileAction    string                    `json:"profile_action"`
	ConceptsCreated  int                       `json:"concepts_created"`
	ConceptsExisting int                       `json:"concepts_existing"`
	ConceptIDs       []string                  `json:"concept_ids"`
}

// ApproveContextScan applies a reviewed context scan — profile and approved terms —
// in one request (POST /api/v1/:ws/context-scans/:id/approve). It is idempotent
// by content: the profile upserts by name, and a term is created only when the
// workspace has no concept carrying it in that locale, so a retry after a
// partial failure converges rather than duplicating. A 403 surfaces as
// ErrForbidden.
func (c *BowrainClient) ApproveContextScan(ctx context.Context, scanID string, req ContextScanApproval) (*ContextScanApprovalResult, error) {
	if c.workspace == "" {
		return nil, errors.New("brand scans are workspace-scoped (use NewWorkspaceBowrainClient)")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal brand-scan approval: %w", err)
	}

	path := "/context-scans/" + url.PathEscape(scanID) + "/approve"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.wsPrefix()+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", strings.TrimPrefix(path, "/"), err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var out ContextScanApprovalResult
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("decode brand-scan approval response: %w", err)
		}
		return &out, nil
	case http.StatusForbidden:
		return nil, fmt.Errorf("%s: %w", strings.TrimPrefix(path, "/"), ErrForbidden)
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError(strings.TrimPrefix(path, "/"), resp.StatusCode, respBody)
	}
}

// UpsertVoiceProfile creates or updates the workspace voice profile matching
// req by name (POST /api/v1/:ws/voice-profiles/upsert). The endpoint is
// idempotent: a re-push of identical content is a no-op ("unchanged"), and an
// update travels through the store's profile versioning (the previous state is
// archived, never clobbered). A 403 surfaces as ErrForbidden.
func (c *BowrainClient) UpsertVoiceProfile(ctx context.Context, req VoiceProfileUpsert) (*VoiceProfileUpsertResult, error) {
	if c.workspace == "" {
		return nil, errors.New("brand profiles are workspace-scoped (use NewWorkspaceBowrainClient)")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal voice profile: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.wsPrefix()+"/voice-profiles/upsert", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request voice-profiles/upsert: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var out VoiceProfileUpsertResult
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("decode voice-profiles/upsert response: %w", err)
		}
		return &out, nil
	case http.StatusForbidden:
		return nil, fmt.Errorf("voice-profiles/upsert: %w", ErrForbidden)
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("voice-profiles/upsert", resp.StatusCode, respBody)
	}
}

// ---------------------------------------------------------------------------
// Workspace brand-compliance rollup
// ---------------------------------------------------------------------------

// VoiceRollupEntry is one project's row in the workspace brand-compliance
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

// VoiceRollup is one page of the workspace brand-compliance rollup.
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

// GetVoiceRollup reads the workspace brand-compliance rollup
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
