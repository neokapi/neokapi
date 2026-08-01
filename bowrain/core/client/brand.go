package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// This file holds the brand-profile push surface: the idempotent upsert
// `kapi push` uses to carry the recipe-bound voice profile into the workspace
// brand hub. Like the knowledge graph, brand profiles live on the workspace
// content group (/api/v1/:ws/brand-profiles), so the method requires a
// workspace-scoped client (NewWorkspaceBowrainClient).

// ErrForbidden marks a request the server refused with HTTP 403 — for the
// brand upsert, a caller without the manage-brand permission. Callers detect
// it with errors.Is and degrade gracefully (a push skips the profile instead
// of failing).
var ErrForbidden = errors.New("permission denied")

// BrandProfileUpsert mirrors the server's BrandProfileRequest: the authored
// surface of a voice profile. Server-managed metadata (id, workspace, version,
// autonomy, timestamps) is never sent — the upsert cannot touch it.
type BrandProfileUpsert struct {
	Name        string                                        `json:"name"`
	Description string                                        `json:"description,omitempty"`
	Tone        coreprofile.ToneProfile                       `json:"tone"`
	Style       coreprofile.StyleRules                        `json:"style"`
	Vocabulary  coreprofile.VocabularyRules                   `json:"vocabulary"`
	Examples    []coreprofile.VoiceExample                    `json:"examples"`
	Locales     map[model.LocaleID]coreprofile.LocaleOverride `json:"locales,omitempty"`
	Channels    map[string]coreprofile.ChannelOverride        `json:"channels,omitempty"`
	Personas    map[string]coreprofile.PersonaOverride        `json:"personas,omitempty"`
}

// BrandProfileUpsertFromProfile builds the upsert payload from a resolved
// local voice profile, carrying only the authored surface.
func BrandProfileUpsertFromProfile(p *coreprofile.VoiceProfile) BrandProfileUpsert {
	return BrandProfileUpsert{
		Name:        p.Name,
		Description: p.Description,
		Tone:        p.Tone,
		Style:       p.Style,
		Vocabulary:  p.Vocabulary,
		Examples:    p.Examples,
		Locales:     p.Locales,
		Channels:    p.Channels,
		Personas:    p.Personas,
	}
}

// BrandProfileUpsertResult reports what the upsert did. Action is "created",
// "updated", or "unchanged"; Profile is the stored workspace profile after the
// action (its Version reflects any bump).
type BrandProfileUpsertResult struct {
	Action  string                    `json:"action"`
	Profile *coreprofile.VoiceProfile `json:"profile"`
}

// UpsertBrandProfile creates or updates the workspace brand profile matching
// req by name (POST /api/v1/:ws/brand-profiles/upsert). The endpoint is
// idempotent: a re-push of identical content is a no-op ("unchanged"), and an
// update travels through the store's profile versioning (the previous state is
// archived, never clobbered). A 403 surfaces as ErrForbidden.
func (c *BowrainClient) UpsertBrandProfile(ctx context.Context, req BrandProfileUpsert) (*BrandProfileUpsertResult, error) {
	if c.workspace == "" {
		return nil, errors.New("brand profiles are workspace-scoped (use NewWorkspaceBowrainClient)")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal brand profile: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.wsPrefix()+"/brand-profiles/upsert", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request brand-profiles/upsert: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var out BrandProfileUpsertResult
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("decode brand-profiles/upsert response: %w", err)
		}
		return &out, nil
	case http.StatusForbidden:
		return nil, fmt.Errorf("brand-profiles/upsert: %w", ErrForbidden)
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, NewStatusError("brand-profiles/upsert", resp.StatusCode, respBody)
	}
}
