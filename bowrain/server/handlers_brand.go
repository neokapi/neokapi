package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/brand/packs"
	"github.com/neokapi/neokapi/core/id"
)

// BrandProfileRequest is the request body for creating/updating a brand voice profile.
type BrandProfileRequest struct {
	Name        string                               `json:"name"`
	Description string                               `json:"description,omitempty"`
	Tone        corebrand.ToneProfile                `json:"tone"`
	Style       corebrand.StyleRules                 `json:"style"`
	Vocabulary  corebrand.VocabularyRules            `json:"vocabulary"`
	Examples    []corebrand.VoiceExample             `json:"examples"`
	Locales     map[string]corebrand.LocaleOverride  `json:"locales,omitempty"`
	Channels    map[string]corebrand.ChannelOverride `json:"channels,omitempty"`
}

// BrandCheckRequest is the request body for checking text against a brand profile.
type BrandCheckRequest struct {
	Text   string `json:"text"`
	Locale string `json:"locale,omitempty"`
}

// BrandCheckResponse is the response for a brand voice check.
type BrandCheckResponse struct {
	Score    corebrand.BrandComplianceScore `json:"score"`
	Findings []corebrand.BrandVoiceFinding  `json:"findings"`
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
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	profiles, err := s.BrandStore.ListProfiles(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, profiles)
}

// HandleCreateBrandProfile creates a new brand voice profile.
func (s *Server) HandleCreateBrandProfile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	var req BrandProfileRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	now := time.Now().UTC()

	profile := &corebrand.VoiceProfile{
		ID:          id.New(),
		Name:        req.Name,
		Description: req.Description,
		Tone:        req.Tone,
		Style:       req.Style,
		Vocabulary:  req.Vocabulary,
		Examples:    req.Examples,
		Locales:     req.Locales,
		Channels:    req.Channels,
		WorkspaceID: wsID,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   userID,
	}

	if err := s.BrandStore.CreateProfile(c.Request().Context(), profile); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, profile)
}

// HandleGetBrandProfile returns a single brand voice profile by ID.
func (s *Server) HandleGetBrandProfile(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	profile, err := s.BrandStore.GetProfile(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, profile)
}

// HandleUpdateBrandProfile updates an existing brand voice profile.
func (s *Server) HandleUpdateBrandProfile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	var req BrandProfileRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()
	profile, err := s.BrandStore.GetProfile(ctx, c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
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
	profile.Version++
	profile.UpdatedAt = time.Now().UTC()

	if err := s.BrandStore.UpdateProfile(ctx, profile); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	if err := s.BrandStore.DeleteProfile(c.Request().Context(), c.Param("id")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleCheckBrandVoice checks text against a brand voice profile and returns findings and score.
func (s *Server) HandleCheckBrandVoice(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	var req BrandCheckRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Text == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "text is required"})
	}

	ctx := c.Request().Context()
	profile, err := s.BrandStore.GetProfile(ctx, c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	// Run vocabulary-based brand checks against the profile.
	findings := checkVocabulary(req.Text, profile)
	score := corebrand.CalculateScore(findings)
	score.ProfileID = profile.ID

	return c.JSON(http.StatusOK, BrandCheckResponse{
		Score:    score,
		Findings: findings,
	})
}

// checkVocabulary runs rule-based vocabulary checks against a brand voice profile.
func checkVocabulary(text string, profile *corebrand.VoiceProfile) []corebrand.BrandVoiceFinding {
	var findings []corebrand.BrandVoiceFinding

	lowerText := toLower(text)

	for _, term := range profile.Vocabulary.ForbiddenTerms {
		if containsTerm(lowerText, toLower(term.Term)) {
			sev := corebrand.SeverityMajor
			if term.Severity != "" {
				sev = corebrand.Severity(term.Severity)
			}
			findings = append(findings, corebrand.BrandVoiceFinding{
				Category:     string(corebrand.DimensionVocabulary),
				Severity:     sev,
				Message:      "Forbidden term: " + term.Term,
				Suggestion:   term.Replacement,
				OriginalText: term.Term,
			})
		}
	}

	for _, term := range profile.Vocabulary.CompetitorTerms {
		if containsTerm(lowerText, toLower(term.Term)) {
			sev := corebrand.SeverityCritical
			if term.Severity != "" {
				sev = corebrand.Severity(term.Severity)
			}
			findings = append(findings, corebrand.BrandVoiceFinding{
				Category:     string(corebrand.DimensionVocabulary),
				Severity:     sev,
				Message:      "Competitor term: " + term.Term,
				Suggestion:   term.Replacement,
				OriginalText: term.Term,
			})
		}
	}

	return findings
}

// toLower is a helper for case-insensitive matching.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		} else {
			b[i] = c
		}
	}
	return string(b)
}

// containsTerm checks if text contains a term (case-insensitive, already lowered).
func containsTerm(lowerText, lowerTerm string) bool {
	return len(lowerTerm) > 0 && len(lowerText) >= len(lowerTerm) &&
		indexOf(lowerText, lowerTerm) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// HandleListStarterPacks lists available starter pack templates.
func (s *Server) HandleListStarterPacks(c echo.Context) error {
	names, err := packs.List()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	var req CreateFromStarterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Pack == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "pack name is required"})
	}

	template, err := packs.Load(req.Pack)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "starter pack not found: " + req.Pack})
	}

	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	now := time.Now().UTC()

	profile := template
	profile.ID = id.New()
	profile.WorkspaceID = wsID
	profile.Version = 1
	profile.CreatedAt = now
	profile.UpdatedAt = now
	profile.CreatedBy = userID
	if req.Name != "" {
		profile.Name = req.Name
	}

	if err := s.BrandStore.CreateProfile(c.Request().Context(), profile); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, profile)
}

// HandleGetBrandVoiceScores returns brand compliance scores for a project.
func (s *Server) HandleGetBrandVoiceScores(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	projectID := c.Param("id")
	scores, err := s.BrandStore.GetScores(c.Request().Context(), projectID, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, scores)
}

// HandleGetBrandVoiceScoresByLocale returns brand compliance scores filtered by locale.
func (s *Server) HandleGetBrandVoiceScoresByLocale(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	projectID := c.Param("id")
	locale := c.Param("locale")
	scores, err := s.BrandStore.GetScores(c.Request().Context(), projectID, locale)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, scores)
}

// HandleGetBrandVoiceTrends returns brand compliance score trends for a project.
func (s *Server) HandleGetBrandVoiceTrends(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, trends)
}
