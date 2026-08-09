package server

import (
	"context"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
)

// BrandProfileRequest is the request body for creating/updating a brand voice profile.
type BrandProfileRequest struct {
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

// BrandProfileUpsertResponse reports what HandleUpsertBrandProfile did.
// Action is "created", "updated", or "unchanged"; Profile is the stored
// workspace profile after the action.
type BrandProfileUpsertResponse struct {
	Action  string                    `json:"action"`
	Profile *coreprofile.VoiceProfile `json:"profile"`
}

// BrandCheckRequest is the request body for checking text against a brand profile.
type BrandCheckRequest struct {
	Text   string `json:"text"`
	Locale string `json:"locale,omitempty"`
}

// BrandCheckResponse is the response for a brand voice check.
type BrandCheckResponse struct {
	Score    coreprofile.ComplianceScore `json:"score"`
	Findings []coreprofile.VoiceFinding  `json:"findings"`
}

// StarterPackResponse describes an available starter pack template.
type StarterPackResponse struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateFromStarterRequest is the request body for creating a profile from a starter pack.
type CreateFromStarterRequest struct {
	Pack string `json:"pack"`
	Name string `json:"name,omitempty"`
}

// HandleListBrandProfiles lists all brand voice profiles in a workspace.
func (s *Server) HandleListBrandProfiles(c echo.Context) error {
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}
	wsID, _ := c.Get("workspace_id").(string)
	profiles, err := s.BrandStore.ListProfiles(c.Request().Context(), wsID)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, profiles)
}

// HandleCreateBrandProfile creates a new brand voice profile.
func (s *Server) HandleCreateBrandProfile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	var req BrandProfileRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if req.Name == "" {
		return apiErr(c, http.StatusBadRequest, "name is required")
	}

	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	now := time.Now().UTC()

	profile := &coreprofile.VoiceProfile{
		ID:          id.New(),
		Name:        req.Name,
		Description: req.Description,
		Tone:        req.Tone,
		Style:       req.Style,
		Vocabulary:  req.Vocabulary,
		Examples:    req.Examples,
		Locales:     req.Locales,
		Channels:    req.Channels,
		Personas:    req.Personas,
		Scope:       wsID,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   userID,
	}

	if err := s.BrandStore.CreateProfile(c.Request().Context(), profile); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusCreated, profile)
}

// HandleUpsertBrandProfile creates or updates the workspace brand profile
// matching the request by name — the idempotent endpoint behind `kapi push`.
// The linkage key is the profile name within the workspace (the store already
// enforces UNIQUE(workspace_id, name)), matched case-insensitively:
//
//   - no workspace profile with that name → create it (version 1);
//   - a match whose authored content equals the pushed content → no-op;
//   - a match that differs → apply the pushed content through the store's
//     UpdateProfile, which archives the current state as an immutable
//     ProfileVersion before bumping the version — server-side edits are never
//     lost, only superseded by a new, revertible version.
//
// Vocabulary rules the correction-learning loop promoted server-side (a
// promoted RuleDecision still present in the live profile) are folded back
// into the pushed vocabulary when the push does not carry the term, so a push
// from a stale local profile never reverts a promotion; demoting a rule stays
// a server-side, governed act.
func (s *Server) HandleUpsertBrandProfile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	var req BrandProfileRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if req.Name == "" {
		return apiErr(c, http.StatusBadRequest, "name is required")
	}

	ctx := c.Request().Context()
	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)

	result, err := s.upsertBrandProfile(ctx, wsID, userID, req, "superseded by kapi push")
	if err != nil {
		return serverErr(c, err)
	}
	switch result.Action {
	case brandUpsertCreated:
		return c.JSON(http.StatusCreated, BrandProfileUpsertResponse{Action: result.Action, Profile: result.Profile})
	case brandUpsertUpdated:
		s.emitAudit(c, auditEvent{
			Type:         platev.EventBrandProfileUpdated,
			ResourceType: "brand_profile",
			ResourceID:   result.Profile.ID,
			Data:         map[string]string{"name": result.Profile.Name, "via": "push"},
			Before:       map[string]string{"version": strconv.Itoa(result.BeforeVersion)},
			After:        map[string]string{"version": strconv.Itoa(result.Profile.Version)},
		})
	}
	return c.JSON(http.StatusOK, BrandProfileUpsertResponse{Action: result.Action, Profile: result.Profile})
}

// The three outcomes of a brand-profile upsert.
const (
	brandUpsertCreated   = "created"
	brandUpsertUpdated   = "updated"
	brandUpsertUnchanged = "unchanged"
)

// brandProfileUpsert is the outcome of upsertBrandProfile: the stored profile,
// which of the three actions produced it, and the version an update superseded
// (zero unless Action is "updated").
type brandProfileUpsert struct {
	Profile       *coreprofile.VoiceProfile
	Action        string
	BeforeVersion int
}

// upsertBrandProfile creates or updates the workspace profile matching req by
// name, case-insensitively, and reports which it did. An update archives the
// current state as an immutable version under versionNote before bumping;
// identical content is a no-op. Server-promoted vocabulary rules the request
// does not carry are folded back in, so an upsert from stale content never
// reverts a promotion.
//
// The caller emits any audit event: the same upsert is reached from a push and
// from a brand-scan approval, and those are not the same act.
func (s *Server) upsertBrandProfile(ctx context.Context, wsID, userID string, req BrandProfileRequest, versionNote string) (brandProfileUpsert, error) {
	profiles, err := s.BrandStore.ListProfiles(ctx, wsID)
	if err != nil {
		return brandProfileUpsert{}, err
	}
	var existing *coreprofile.VoiceProfile
	for _, p := range profiles {
		if strings.EqualFold(p.Name, req.Name) {
			existing = p
			break
		}
	}

	if existing == nil {
		now := time.Now().UTC()
		profile := &coreprofile.VoiceProfile{
			ID:          id.New(),
			Name:        req.Name,
			Description: req.Description,
			Tone:        req.Tone,
			Style:       req.Style,
			Vocabulary:  req.Vocabulary,
			Examples:    req.Examples,
			Locales:     req.Locales,
			Channels:    req.Channels,
			Personas:    req.Personas,
			Scope:       wsID,
			Version:     1,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   userID,
		}
		if err := s.BrandStore.CreateProfile(ctx, profile); err != nil {
			return brandProfileUpsert{}, err
		}
		return brandProfileUpsert{Profile: profile, Action: brandUpsertCreated}, nil
	}

	// The effective incoming vocabulary keeps the server's promoted rules.
	vocab := req.Vocabulary
	s.preservePromotedRules(ctx, existing, &vocab)

	incoming := brandContentOf(req.Name, req.Description, req.Tone, req.Style, vocab,
		req.Examples, req.Locales, req.Channels, req.Personas)
	current := brandContentOf(existing.Name, existing.Description, existing.Tone, existing.Style,
		existing.Vocabulary, existing.Examples, existing.Locales, existing.Channels, existing.Personas)
	if reflect.DeepEqual(incoming, current) {
		return brandProfileUpsert{Profile: existing, Action: brandUpsertUnchanged}, nil
	}

	beforeVersion := existing.Version
	existing.Name = req.Name
	existing.Description = req.Description
	existing.Tone = req.Tone
	existing.Style = req.Style
	existing.Vocabulary = vocab
	existing.Examples = req.Examples
	existing.Locales = req.Locales
	existing.Channels = req.Channels
	existing.Personas = req.Personas
	existing.VersionNote = versionNote
	existing.UpdatedAt = time.Now().UTC()

	// UpdateProfile archives the current state as an immutable ProfileVersion
	// and bumps existing.Version — the incoming change lands as a new version.
	if err := s.BrandStore.UpdateProfile(ctx, existing); err != nil {
		return brandProfileUpsert{}, err
	}
	return brandProfileUpsert{Profile: existing, Action: brandUpsertUpdated, BeforeVersion: beforeVersion}, nil
}

// brandProfileContent is the authored surface of a voice profile the push
// upsert compares — exactly the field set BrandProfileRequest carries.
// Server-managed metadata (ID, workspace, version, autonomy, timestamps) is
// excluded: a push never touches it.
type brandProfileContent struct {
	Name        string
	Description string
	Tone        coreprofile.ToneProfile
	Style       coreprofile.StyleRules
	Vocabulary  coreprofile.VocabularyRules
	Examples    []coreprofile.VoiceExample
	Locales     map[model.LocaleID]coreprofile.LocaleOverride
	Channels    map[string]coreprofile.ChannelOverride
	Personas    map[string]coreprofile.PersonaOverride
}

// brandContentOf builds a normalized comparable view: every empty top-level
// collection maps to nil so a YAML-decoded pushed profile (nil slices/maps)
// compares equal to a stored one that round-tripped through the database.
func brandContentOf(name, description string, tone coreprofile.ToneProfile, style coreprofile.StyleRules,
	vocab coreprofile.VocabularyRules, examples []coreprofile.VoiceExample,
	locales map[model.LocaleID]coreprofile.LocaleOverride, channels map[string]coreprofile.ChannelOverride,
	personas map[string]coreprofile.PersonaOverride,
) brandProfileContent {
	if len(tone.Personality) == 0 {
		tone.Personality = nil
	}
	if len(style.ProhibitedPatterns) == 0 {
		style.ProhibitedPatterns = nil
	}
	if len(style.RequiredPatterns) == 0 {
		style.RequiredPatterns = nil
	}
	if len(vocab.PreferredTerms) == 0 {
		vocab.PreferredTerms = nil
	}
	if len(vocab.ForbiddenTerms) == 0 {
		vocab.ForbiddenTerms = nil
	}
	if len(vocab.CompetitorTerms) == 0 {
		vocab.CompetitorTerms = nil
	}
	if len(vocab.Abbreviations) == 0 {
		vocab.Abbreviations = nil
	}
	if len(examples) == 0 {
		examples = nil
	}
	if len(locales) == 0 {
		locales = nil
	}
	if len(channels) == 0 {
		channels = nil
	}
	if len(personas) == 0 {
		personas = nil
	}
	return brandProfileContent{
		Name: name, Description: description, Tone: tone, Style: style,
		Vocabulary: vocab, Examples: examples, Locales: locales, Channels: channels, Personas: personas,
	}
}

// preservePromotedRules folds the profile's promoted-rule decisions into the
// pushed vocabulary so a push from a stale local profile never reverts what
// the correction-learning loop promoted server-side. A rule is preserved when
// its decision is promoted AND the live profile still carries it (a demoted
// rule is gone from the live profile and stays gone) AND the pushed vocabulary
// does not itself carry the term. Promotions land in ForbiddenTerms
// (profile.ApplySuggestedRule), so only that list needs folding. Returns the
// number of preserved rules.
func (s *Server) preservePromotedRules(ctx context.Context, existing *coreprofile.VoiceProfile, vocab *coreprofile.VocabularyRules) int {
	decisions, err := s.BrandStore.ListRuleDecisions(ctx, existing.ID)
	if err != nil || len(decisions) == 0 {
		return 0
	}
	promoted := map[string]bool{}
	for _, d := range decisions {
		if d.Status == coreprofile.RuleDecisionPromoted {
			promoted[strings.ToLower(d.Term)] = true
		}
	}
	if len(promoted) == 0 {
		return 0
	}
	pushed := map[string]bool{}
	for _, r := range vocab.ForbiddenTerms {
		pushed[strings.ToLower(r.Term)] = true
	}
	n := 0
	for _, rule := range existing.Vocabulary.ForbiddenTerms {
		key := strings.ToLower(rule.Term)
		if promoted[key] && !pushed[key] {
			vocab.ForbiddenTerms = append(vocab.ForbiddenTerms, rule)
			n++
		}
	}
	return n
}

// profileInRequestWorkspace reports whether the brand profile belongs to the
// workspace the request is scoped to. BrandStore.GetProfile/UpdateProfile/
// DeleteProfile take only a GLOBAL profile id and ignore VoiceProfile.Scope,
// so without this check an owner/admin of workspace A could read, update, or
// delete workspace B's brand profile by addressing it through their own
// workspace (/api/v1/<A>/brand-profiles/<B-profile-id>) — a cross-tenant IDOR.
// Callers respond 404 (anti-enumeration) on a mismatch, matching the
// project-level cross-tenant guard in ProjectAccessMiddleware. Fails closed when
// the workspace context is missing (wsID empty).
func profileInRequestWorkspace(c echo.Context, profile *coreprofile.VoiceProfile) bool {
	wsID, _ := c.Get("workspace_id").(string)
	return wsID != "" && profile != nil && profile.Scope == wsID
}

// HandleGetBrandProfile returns a single brand voice profile by ID.
func (s *Server) HandleGetBrandProfile(c echo.Context) error {
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	profile, err := s.BrandStore.GetProfile(c.Request().Context(), c.Param("id"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	if !profileInRequestWorkspace(c, profile) {
		return apiErr(c, http.StatusNotFound, "brand profile not found")
	}
	return c.JSON(http.StatusOK, profile)
}

// HandleUpdateBrandProfile updates an existing brand voice profile.
func (s *Server) HandleUpdateBrandProfile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	var req BrandProfileRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}

	ctx := c.Request().Context()
	profile, err := s.BrandStore.GetProfile(ctx, c.Param("id"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	if !profileInRequestWorkspace(c, profile) {
		return apiErr(c, http.StatusNotFound, "brand profile not found")
	}
	beforeVersion := strconv.Itoa(profile.Version)

	profile.Name = req.Name
	profile.Description = req.Description
	profile.Tone = req.Tone
	profile.Style = req.Style
	profile.Vocabulary = req.Vocabulary
	profile.Examples = req.Examples
	profile.Locales = req.Locales
	profile.Channels = req.Channels
	profile.Personas = req.Personas
	profile.Version++
	profile.UpdatedAt = time.Now().UTC()

	if err := s.BrandStore.UpdateProfile(ctx, profile); err != nil {
		return serverErr(c, err)
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventBrandProfileUpdated,
		ResourceType: "brand_profile",
		ResourceID:   profile.ID,
		Data:         map[string]string{"name": profile.Name},
		Before:       map[string]string{"version": beforeVersion},
		After:        map[string]string{"version": strconv.Itoa(profile.Version)},
	})
	return c.JSON(http.StatusOK, profile)
}

// HandleDeleteBrandProfile deletes a brand voice profile.
func (s *Server) HandleDeleteBrandProfile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	// Fetch first to assert workspace ownership: DeleteProfile takes a global id,
	// so an unscoped delete would let a caller destroy another tenant's profile.
	ctx := c.Request().Context()
	profile, err := s.BrandStore.GetProfile(ctx, c.Param("id"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	if !profileInRequestWorkspace(c, profile) {
		return apiErr(c, http.StatusNotFound, "brand profile not found")
	}
	if err := s.BrandStore.DeleteProfile(ctx, profile.ID); err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleCheckBrandVoice checks text against a brand voice profile and returns findings and score.
func (s *Server) HandleCheckBrandVoice(c echo.Context) error {
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	var req BrandCheckRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if req.Text == "" {
		return apiErr(c, http.StatusBadRequest, "text is required")
	}

	ctx := c.Request().Context()
	profile, err := s.BrandStore.GetProfile(ctx, c.Param("id"))
	if err != nil {
		return apiErr(c, http.StatusNotFound, err.Error())
	}
	if !profileInRequestWorkspace(c, profile) {
		return apiErr(c, http.StatusNotFound, "brand profile not found")
	}

	// Run vocabulary-based brand checks against the profile using the shared
	// matcher + mapper (profile.MatchVocabulary → profile.HitsToFindings): whole-word,
	// Unicode-aware matching (so "use" never matches inside "user") and concept_id
	// propagation, identical to the streaming pipeline tool and the MCP tool. A
	// single text run anchors each finding's position to the checked text.
	runs := []model.Run{{Text: &model.TextRun{Text: req.Text}}}
	findings := coreprofile.HitsToFindings(coreprofile.MatchVocabulary(profile, req.Text), req.Text, runs)
	score := coreprofile.CalculateScore(findings)
	score.ProfileID = profile.ID

	return c.JSON(http.StatusOK, BrandCheckResponse{
		Score:    score,
		Findings: findings,
	})
}

// HandleListStarterPacks lists available starter pack templates.
func (s *Server) HandleListStarterPacks(c echo.Context) error {
	names, err := packs.List()
	if err != nil {
		return serverErr(c, err)
	}

	result := make([]StarterPackResponse, 0, len(names))
	for _, name := range names {
		p, err := packs.Load(name)
		if err != nil {
			continue
		}
		result = append(result, StarterPackResponse{
			Name:        name,
			Description: p.Description,
		})
	}
	return c.JSON(http.StatusOK, result)
}

// HandleCreateFromStarter creates a brand voice profile from a starter pack template.
func (s *Server) HandleCreateFromStarter(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	var req CreateFromStarterRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if req.Pack == "" {
		return apiErr(c, http.StatusBadRequest, "pack name is required")
	}

	template, err := packs.Load(req.Pack)
	if err != nil {
		return apiErr(c, http.StatusNotFound, "starter pack not found: "+req.Pack)
	}

	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	now := time.Now().UTC()

	profile := template
	profile.ID = id.New()
	profile.Scope = wsID
	profile.Version = 1
	profile.CreatedAt = now
	profile.UpdatedAt = now
	profile.CreatedBy = userID
	if req.Name != "" {
		profile.Name = req.Name
	}

	if err := s.BrandStore.CreateProfile(c.Request().Context(), profile); err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusCreated, profile)
}

// HandleGetBrandVoiceScores returns brand compliance scores for a project.
func (s *Server) HandleGetBrandVoiceScores(c echo.Context) error {
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	projectID := c.Param("id")
	scores, err := s.BrandStore.GetScores(c.Request().Context(), projectID, "")
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, scores)
}

// HandleGetBrandVoiceScoresByLocale returns brand compliance scores filtered by locale.
func (s *Server) HandleGetBrandVoiceScoresByLocale(c echo.Context) error {
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	projectID := c.Param("id")
	locale := model.LocaleID(c.Param("locale"))
	scores, err := s.BrandStore.GetScores(c.Request().Context(), projectID, locale)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, scores)
}

// HandleGetBrandVoiceTrends returns brand compliance score trends for a project.
func (s *Server) HandleGetBrandVoiceTrends(c echo.Context) error {
	if s.BrandStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "brand voice not configured")
	}

	projectID := c.Param("id")
	days := 30
	if d := c.QueryParam("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	trends, err := s.BrandStore.GetScoreTrends(c.Request().Context(), projectID, days)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, trends)
}
