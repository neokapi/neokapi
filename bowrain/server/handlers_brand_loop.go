package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/id"
)

// The correction-learning loop's events. rule_promoted (manual) lives with the
// correction handlers; rejection and autonomy-driven promotion are their
// counterparts. All carry the "brand." prefix so brand automations react.
const (
	EventBrandVoiceRuleRejected     platev.EventType = "brand.voice.rule_rejected"
	EventBrandVoiceRuleAutoPromoted platev.EventType = "brand.voice.rule_auto_promoted"
)

// HandleListCandidates returns the correction-derived candidate rules for a
// profile, each annotated with the team's decision (pending/approved/rejected/
// promoted). Candidates are derived live from the workspace's correction stream
// and joined with this profile's recorded decisions. By default only candidates
// still needing a human (pending/approved) are returned; ?all=true includes the
// full history.
func (s *Server) HandleListCandidates(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}
	profileID := c.Param("id")
	wsID, _ := c.Get("workspace_id").(string)

	minCount := 3
	if mc := c.QueryParam("min_count"); mc != "" {
		if parsed, err := strconv.Atoi(mc); err == nil && parsed > 0 {
			minCount = parsed
		}
	}
	includeResolved := c.QueryParam("all") == "true"

	ctx := c.Request().Context()
	suggestions, err := s.BrandStore.GetSuggestedRules(ctx, wsID, minCount)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	decisions, err := s.BrandStore.ListRuleDecisions(ctx, profileID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	candidates := corebrand.MergeCandidates(suggestions, decisions, includeResolved)
	return c.JSON(http.StatusOK, candidates)
}

// HandleRejectSuggestedRule records a reviewer's decision to decline a
// correction-derived candidate. The rejection persists so the same term stops
// re-surfacing in the candidate list, and emits brand.voice.rule_rejected.
func (s *Server) HandleRejectSuggestedRule(c echo.Context) error {
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
	userID, _ := c.Get("user_id").(string)
	wsID, _ := c.Get("workspace_id").(string)
	decision := &corebrand.RuleDecision{
		ProfileID:       profileID,
		Term:            req.Term,
		Replacement:     req.Replacement,
		Dimension:       req.Dimension,
		Status:          corebrand.RuleDecisionRejected,
		CorrectionCount: req.CorrectionCount,
		DecidedBy:       userID,
		DecidedAt:       time.Now().UTC(),
	}
	if err := s.BrandStore.RecordRuleDecision(c.Request().Context(), decision); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.publishBrandRuleEvent(EventBrandVoiceRuleRejected, wsID, userID, profileID, req.Term, req.Replacement, 0)
	return c.JSON(http.StatusOK, decision)
}

// EvaluateRuleRequest is the body for a blast-radius preview: which rule, against
// which project content.
type EvaluateRuleRequest struct {
	Term        string `json:"term"`
	Replacement string `json:"replacement"`
	ProjectID   string `json:"project_id"`
	Stream      string `json:"stream,omitempty"`
}

// HandleEvaluateRulePromotion computes the blast radius of promoting a candidate
// rule: it runs the profile's vocabulary checks over the project's stored content
// with and without the rule, and reports how many blocks the change would newly
// flag, what it resolves, and the per-item breakdown — the number a reviewer sees
// before the rule lands. Nothing is persisted.
func (s *Server) HandleEvaluateRulePromotion(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil || s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice or content store not configured"})
	}
	var req EvaluateRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Term == "" || req.ProjectID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "term and project_id are required"})
	}

	ctx := c.Request().Context()
	baseline, err := s.BrandStore.GetProfile(ctx, c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	candidate := corebrand.CandidateWithRule(baseline, corebrand.SuggestedRule{Term: req.Term, Replacement: req.Replacement})

	stored, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: req.ProjectID, Stream: req.Stream})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	blocks := make([]corebrand.EvalBlock, 0, len(stored))
	for _, sb := range stored {
		if sb == nil || sb.Block == nil {
			continue
		}
		blocks = append(blocks, corebrand.EvalBlock{
			BlockID:        sb.Block.ID,
			CollectionID:   sb.ItemName,
			CollectionName: sb.ItemName,
			Text:           sb.Block.SourceText(),
		})
	}
	radius := corebrand.EvaluateBlastRadius(blocks, baseline, candidate)
	return c.JSON(http.StatusOK, radius)
}

// publishBrandRuleEvent emits a brand rule lifecycle event, if an event bus is
// configured. Shared by the reject/promote/auto-promote paths.
func (s *Server) publishBrandRuleEvent(t platev.EventType, wsID, userID, profileID, term, replacement string, version int) {
	if s.EventBus == nil {
		return
	}
	data := map[string]string{
		"profile_id":  profileID,
		"term":        term,
		"replacement": replacement,
	}
	if version > 0 {
		data["version"] = strconv.Itoa(version)
	}
	s.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      t,
		Source:    "brand",
		ProjectID: wsID,
		Actor:     userID,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
}

// driftConfigFromQuery reads drift-detection settings from query params, falling
// back to the analyzer's defaults (7-day recent window, 10-point drop).
func driftConfigFromQuery(c echo.Context) (corebrand.DriftConfig, int) {
	cfg := corebrand.DriftConfig{}
	days := 30
	if v := c.QueryParam("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	if v := c.QueryParam("recent_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RecentDays = n
		}
	}
	if v := c.QueryParam("min_score"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MinScore = n
		}
	}
	if v := c.QueryParam("drop_points"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DropPoints = n
		}
	}
	return cfg, days
}

// HandleGetBrandVoiceDrift returns the current brand-compliance drift analysis
// for a project — the recent vs. baseline average and whether compliance has
// drifted. Safe/read-only: it never fires an alert (use drift-check for that).
func (s *Server) HandleGetBrandVoiceDrift(c echo.Context) error {
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}
	projectID := c.Param("id")
	cfg, days := driftConfigFromQuery(c)
	trends, err := s.BrandStore.GetScoreTrends(c.Request().Context(), projectID, days)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, corebrand.AnalyzeDrift(trends, cfg))
}

// HandleRunBrandVoiceDriftCheck runs the drift analysis and, when compliance has
// drifted, publishes brand.voice.drift so automations and notifications react.
// This is the action a scheduled job (or a dashboard "check now") invokes; the
// GET variant is the safe read.
func (s *Server) HandleRunBrandVoiceDriftCheck(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	if s.BrandStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "brand voice not configured"})
	}
	projectID := c.Param("id")
	cfg, days := driftConfigFromQuery(c)
	trends, err := s.BrandStore.GetScoreTrends(c.Request().Context(), projectID, days)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	result := corebrand.AnalyzeDrift(trends, cfg)
	if result.Drifted && s.EventBus != nil {
		userID, _ := c.Get("user_id").(string)
		s.EventBus.Publish(platev.Event{
			ID:        id.New(),
			Type:      platev.EventBrandVoiceDrift,
			Source:    "brand",
			ProjectID: projectID,
			Actor:     userID,
			Data: map[string]string{
				"project_id":   projectID,
				"recent_avg":   strconv.FormatFloat(result.RecentAvg, 'f', 1, 64),
				"baseline_avg": strconv.FormatFloat(result.BaselineAvg, 'f', 1, 64),
				"drop":         strconv.FormatFloat(result.Drop, 'f', 1, 64),
				"reason":       result.Reason,
			},
			Timestamp: time.Now().UTC(),
		})
	}
	return c.JSON(http.StatusOK, result)
}

// maybeAutoPromote applies progressive autonomy: after a correction is stored, if
// the profile's autonomy threshold is met for that term and no decision has been
// recorded yet, the rule is promoted automatically (recorded as an auto decision
// and announced as brand.voice.rule_auto_promoted). Returns the promoted rule
// term when an auto-promotion happened. Best-effort: errors are swallowed so a
// correction is never lost to an autonomy hiccup.
func (s *Server) maybeAutoPromote(ctx echo.Context, profile *corebrand.VoiceProfile, wsID, userID string, correction *corebrand.Correction) (string, bool) {
	if profile == nil || profile.Autonomy.AutoPromoteAtCount <= 0 {
		return "", false
	}
	rctx := ctx.Request().Context()
	suggestions, err := s.BrandStore.GetSuggestedRules(rctx, wsID, profile.Autonomy.AutoPromoteAtCount)
	if err != nil {
		return "", false
	}
	for _, sug := range suggestions {
		if sug == nil || !strings.EqualFold(sug.Term, correction.OriginalText) {
			continue
		}
		if !profile.ShouldAutoPromote(*sug) {
			return "", false
		}
		// Skip if a human (or a prior auto-run) already decided this term.
		if existing, _ := s.BrandStore.GetRuleDecision(rctx, profile.ID, sug.Term); existing != nil {
			return "", false
		}

		// Link the rule into the brand knowledge graph (AD-021) before promoting,
		// so the auto-promoted TermRule denotes its concept. Best-effort: a
		// termbase hiccup must not cost the loop an auto-promotion it earned.
		rule := *sug
		conceptID, kgEvents, linkErr := s.linkRuleToConcept(rctx, ctx.Param("ws"), wsID, rule)
		if linkErr != nil {
			slog.Warn("brand: failed to fully link auto-promoted rule to knowledge graph",
				"profile_id", profile.ID, "term", sug.Term, "error", linkErr)
		}
		// Stamp the concept whenever the link produced one, even on a partial
		// failure, so the rule denotes the concept the published kgEvents announce
		// (no orphaned creation event). Empty when nothing was created.
		if conceptID != "" {
			rule.ConceptID = conceptID
		}

		updated, changed, err := corebrand.PromoteAndSave(rctx, s.BrandStore, profile.ID, rule)
		if err != nil || !changed {
			return "", false
		}
		_ = s.BrandStore.RecordRuleDecision(rctx, &corebrand.RuleDecision{
			ProfileID:       profile.ID,
			Term:            sug.Term,
			Replacement:     sug.Replacement,
			Dimension:       sug.Dimension,
			Status:          corebrand.RuleDecisionPromoted,
			CorrectionCount: sug.CorrectionCount,
			PromotedVersion: updated.Version,
			Auto:            true,
			ConceptID:       rule.ConceptID,
			DecidedAt:       time.Now().UTC(),
		})
		s.publishBrandRuleEvent(EventBrandVoiceRuleAutoPromoted, wsID, userID, profile.ID, sug.Term, sug.Replacement, updated.Version)
		s.publishKnowledgeEvents(ctx, kgEvents)
		return sug.Term, true
	}
	return "", false
}
