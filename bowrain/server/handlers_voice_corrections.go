package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"

	"github.com/neokapi/neokapi/core/id"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// EventVoiceRulePromoted fires when a correction-derived rule is promoted
// into a voice profile (it carries the "voice." prefix so voice automations and
// notifications react to it). The closed loop becomes observable here.
const EventVoiceRulePromoted platev.EventType = "voice.rule_promoted"

// VoiceCorrectionRequest is the request body for creating a voice correction.
type VoiceCorrectionRequest struct {
	ProfileID     string                `json:"profile_id"`
	BlockID       string                `json:"block_id"`
	Dimension     coreprofile.Dimension `json:"dimension"`
	OriginalText  string                `json:"original_text"`
	CorrectedText string                `json:"corrected_text"`
	FindingID     string                `json:"finding_id,omitempty"`
}

// HandleCreateVoiceCorrection records a user correction to a voice finding.
func (s *Server) HandleCreateVoiceCorrection(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}

	if s.VoiceStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "voice not configured"})
	}

	var req VoiceCorrectionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.ProfileID == "" || req.OriginalText == "" || req.CorrectedText == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "profile_id, original_text, and corrected_text are required"})
	}

	userID, _ := c.Get("user_id").(string)

	correction := &coreprofile.Correction{
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

	if err := s.VoiceStore.StoreCorrection(c.Request().Context(), correction); err != nil {
		return serverErr(c, err)
	}

	// Progressive autonomy: if this correction pushes its term over the profile's
	// auto-promote threshold, promote the rule now (no human review). Best-effort —
	// a hiccup here never fails the correction that was already stored.
	wsID, _ := c.Get("workspace_id").(string)
	resp := map[string]any{"correction": correction}
	if profile, err := s.VoiceStore.GetProfile(c.Request().Context(), req.ProfileID); err == nil {
		if term, promoted := s.maybeAutoPromote(c, profile, wsID, userID, correction); promoted {
			resp["auto_promoted"] = term
		}
	}
	return c.JSON(http.StatusCreated, resp)
}

// HandleGetSuggestedRules returns vocabulary rules suggested from repeated corrections.
func (s *Server) HandleGetSuggestedRules(c echo.Context) error {
	if s.VoiceStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "voice not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	minCount := 3
	if mc := c.QueryParam("min_count"); mc != "" {
		if parsed, err := strconv.Atoi(mc); err == nil && parsed > 0 {
			minCount = parsed
		}
	}

	rules, err := s.VoiceStore.GetSuggestedRules(c.Request().Context(), wsID, minCount)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, rules)
}

// PromoteRuleRequest is the body for promoting a suggested rule into a profile.
type PromoteRuleRequest struct {
	Term            string                `json:"term"`
	Replacement     string                `json:"replacement"`
	Dimension       coreprofile.Dimension `json:"dimension,omitempty"`
	CorrectionCount int                   `json:"correction_count,omitempty"`
}

// HandlePromoteSuggestedRule promotes a reviewed, correction-derived rule into
// the workspace terms store as a forbidden term joined to the concept of its
// replacement, records the decision against the profile, and emits
// voice.rule_promoted. This closes the correction-learning loop: a correction a
// team made becomes a deterministic check on every future generation.
func (s *Server) HandlePromoteSuggestedRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if s.VoiceStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "voice not configured"})
	}

	var req PromoteRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Term == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "term is required"})
	}

	profileID := c.Param("id")
	wsSlug := c.Param("ws")
	wsID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)
	rule := coreprofile.SuggestedRule{
		Term:            req.Term,
		Replacement:     req.Replacement,
		Dimension:       req.Dimension,
		CorrectionCount: req.CorrectionCount,
	}

	profile, err := s.VoiceStore.GetProfile(c.Request().Context(), profileID)
	if err != nil || !profileInRequestWorkspace(c, profile) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "voice profile not found"})
	}
	changed, conceptID, kgEvents, err := s.promoteRuleToTerms(c.Request().Context(), wsSlug, wsID, rule)
	if err != nil {
		return serverErr(c, err)
	}

	if changed {
		// Record the decision so the candidate leaves the review list and the
		// promotion is traceable to the concept it landed in.
		_ = s.VoiceStore.RecordRuleDecision(c.Request().Context(), &coreprofile.RuleDecision{
			ProfileID:       profileID,
			Term:            req.Term,
			Replacement:     req.Replacement,
			Dimension:       req.Dimension,
			Status:          coreprofile.RuleDecisionPromoted,
			CorrectionCount: req.CorrectionCount,
			ConceptID:       conceptID,
			DecidedBy:       userID,
			DecidedAt:       time.Now().UTC(),
		})
		s.publishVoiceRuleEvent(EventVoiceRulePromoted, wsID, userID, profileID, req.Term, req.Replacement, 0)
	}
	// Announce the concept the promotion created or joined.
	s.publishKnowledgeEvents(c, kgEvents)

	return c.JSON(http.StatusOK, map[string]any{"promoted": changed, "concept_id": conceptID})
}

// DemoteRuleRequest removes a previously promoted brand rule.
type DemoteRuleRequest struct {
	Term string `json:"term"`
}

// HandleDemoteSuggestedRule removes a previously promoted term from the
// workspace terms store (the inverse of promote). Requires PermManageVoice.
//
// POST /:ws/voice-profiles/:id/demote-rule  { "term": "utilize" }
func (s *Server) HandleDemoteSuggestedRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageVoice); err != nil {
		return err
	}
	if s.VoiceStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "voice not configured"})
	}

	var req DemoteRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Term == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "term is required"})
	}

	profileID := c.Param("id")
	ctx := c.Request().Context()
	profile, err := s.VoiceStore.GetProfile(ctx, profileID)
	if err != nil || !profileInRequestWorkspace(c, profile) {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "voice profile not found"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	changed, err := s.demoteRuleFromTerms(ctx, c.Param("ws"), wsID, req.Term)
	if err != nil {
		return serverErr(c, err)
	}

	if changed {
		s.emitAudit(c, auditEvent{
			Type:         platev.EventType("brand.rule.demoted"),
			ResourceType: "voice_profile",
			ResourceID:   profileID,
			Data:         map[string]string{"term": req.Term},
		})
	}

	return c.JSON(http.StatusOK, map[string]any{"demoted": changed})
}
