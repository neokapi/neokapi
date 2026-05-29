package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/id"
)

// EventBrandVoiceRulePromoted fires when a correction-derived rule is promoted
// into a brand profile (it carries the "brand." prefix so brand automations and
// notifications react to it). The closed loop becomes observable here.
const EventBrandVoiceRulePromoted platev.EventType = "brand.voice.rule_promoted"

// BrandCorrectionRequest is the request body for creating a brand voice correction.
type BrandCorrectionRequest struct {
	ProfileID     string              `json:"profile_id"`
	BlockID       string              `json:"block_id"`
	Dimension     corebrand.Dimension `json:"dimension"`
	OriginalText  string              `json:"original_text"`
	CorrectedText string              `json:"corrected_text"`
	FindingID     string              `json:"finding_id,omitempty"`
}

// HandleCreateBrandVoiceCorrection records a user correction to a brand voice finding.
func (s *Server) HandleCreateBrandVoiceCorrection(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}

	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	var req BrandCorrectionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.ProfileID == "" || req.OriginalText == "" || req.CorrectedText == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "profile_id, original_text, and corrected_text are required"})
	}

	userID, _ := c.Get("user_id").(string)

	correction := &corebrand.Correction{
		ID:            id.New(),
		ProfileID:     req.ProfileID,
		BlockID:       req.BlockID,
		Dimension:     req.Dimension,
		OriginalText:  req.OriginalText,
		CorrectedText: req.CorrectedText,
		FindingID:     req.FindingID,
		CorrectedBy:   userID,
		CorrectedAt:   time.Now().UTC(),
	}

	if err := s.BrandStore.StoreCorrection(c.Request().Context(), correction); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Progressive autonomy: if this correction pushes its term over the profile's
	// auto-promote threshold, promote the rule now (no human review). Best-effort —
	// a hiccup here never fails the correction that was already stored.
	wsID, _ := c.Get("workspace_id").(string)
	resp := map[string]any{"correction": correction}
	if profile, err := s.BrandStore.GetProfile(c.Request().Context(), req.ProfileID); err == nil {
		if term, promoted := s.maybeAutoPromote(c, profile, wsID, userID, correction); promoted {
			resp["auto_promoted"] = term
		}
	}
	return c.JSON(http.StatusCreated, resp)
}

// HandleGetSuggestedRules returns vocabulary rules suggested from repeated corrections.
func (s *Server) HandleGetSuggestedRules(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	minCount := 3
	if mc := c.QueryParam("min_count"); mc != "" {
		if parsed, err := strconv.Atoi(mc); err == nil && parsed > 0 {
			minCount = parsed
		}
	}

	rules, err := s.BrandStore.GetSuggestedRules(c.Request().Context(), wsID, minCount)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, rules)
}

// PromoteRuleRequest is the body for promoting a suggested rule into a profile.
type PromoteRuleRequest struct {
	Term            string              `json:"term"`
	Replacement     string              `json:"replacement"`
	Dimension       corebrand.Dimension `json:"dimension,omitempty"`
	CorrectionCount int                 `json:"correction_count,omitempty"`
}

// HandlePromoteSuggestedRule promotes a reviewed, correction-derived rule into
// the brand profile — appending it as an enforced forbidden term, bumping the
// profile version (the prior version is archived for audit/rollback), and
// emitting brand.voice.rule_promoted. This closes the correction-learning loop:
// a correction a team made becomes a deterministic check on every future
// generation.
func (s *Server) HandlePromoteSuggestedRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}

	var req PromoteRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Term == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "term is required"})
	}

	profileID := c.Param("id")
	rule := corebrand.SuggestedRule{
		Term:            req.Term,
		Replacement:     req.Replacement,
		Dimension:       req.Dimension,
		CorrectionCount: req.CorrectionCount,
	}
	profile, changed, err := corebrand.PromoteAndSave(c.Request().Context(), s.BrandStore, profileID, rule)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	userID, _ := c.Get("user_id").(string)
	wsID, _ := c.Get("workspace_id").(string)
	if changed {
		// Record the decision so the candidate leaves the review list and the
		// promotion is traceable to the profile version it landed in.
		_ = s.BrandStore.RecordRuleDecision(c.Request().Context(), &corebrand.RuleDecision{
			ProfileID:       profileID,
			Term:            req.Term,
			Replacement:     req.Replacement,
			Dimension:       req.Dimension,
			Status:          corebrand.RuleDecisionPromoted,
			CorrectionCount: req.CorrectionCount,
			PromotedVersion: profile.Version,
			DecidedBy:       userID,
			DecidedAt:       time.Now().UTC(),
		})
		s.publishBrandRuleEvent(EventBrandVoiceRulePromoted, wsID, userID, profileID, req.Term, req.Replacement, profile.Version)
	}

	return c.JSON(http.StatusOK, map[string]any{"profile": profile, "promoted": changed})
}
