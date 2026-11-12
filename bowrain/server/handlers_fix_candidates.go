package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/reviewdeck"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// fixCandidatesResponse is what the fix sheet offers for one translation.
type fixCandidatesResponse struct {
	BlockID        string      `json:"block_id"`
	Locale         string      `json:"locale"`
	Reason         string      `json:"reason,omitempty"`
	SourceRevision string      `json:"source_revision"`
	TargetRevision string      `json:"target_revision"`
	CurrentRuns    []model.Run `json:"current_runs"`
	CurrentText    string      `json:"current_text"`
	// CurrentEditText is the draft in the placeholder form a set_content text
	// takes.
	CurrentEditText string `json:"current_edit_text"`
	reviewdeck.FixCandidatesResult
}

// HandleGetFixCandidates answers the ready-made corrections for one block's
// translation (reviewdeck.FixCandidates). They are the term fix from the
// workspace's word rules for the language and the content memory's
// alternatives. With ai=true they include a rewrite for the reason given,
// drafted by the workspace's AI provider under the governance a translation
// of the block is drafted under.
//
// GET /:ws/:id/blocks/:ref/:bid/fix-candidates?locale=nb&reason=wrong_term&ai=false
func (s *Server) HandleGetFixCandidates(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	loc := strings.TrimSpace(c.QueryParam("locale"))
	if loc == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}
	if err := s.requireLanguagePermission(c, platauth.PermReview, loc); err != nil {
		return err
	}
	reason, err := reviewdeck.ParseFixReason(c.QueryParam("reason"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	withAI := false
	if raw := c.QueryParam("ai"); raw != "" {
		if withAI, err = strconv.ParseBool(raw); err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ai is true or false"})
		}
	}
	if withAI {
		if err := billing.GuardSyncCredits(c, s.BillingStore, false, s.billingGuardEvent()); err != nil {
			return err
		}
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	ws := c.Param("ws")
	wsID, _ := c.Get("workspace_id").(string)
	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, c.Param("bid"))
	if err != nil || sb == nil || sb.Block == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "block not found"})
	}
	proj, err := s.ContentStore.GetProject(ctx, pid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	b := sb.Block
	target := model.LocaleID(loc)

	req := reviewdeck.FixCandidatesRequest{
		Block:           b,
		Source:          proj.DefaultSourceLanguage,
		Target:          target,
		Reason:          reason,
		MemoryProjectID: proj.ID,
	}
	var notes []string
	profile := s.fixVoiceProfile(ctx, proj, stream, target)
	if rules, rerr := s.workspaceWordRules(ctx, ws, target); rerr != nil {
		notes = append(notes, "the word rules could not be read: "+rerr.Error())
	} else {
		req.TermRules = append(voicescope.WordRuleSets(rules), coreprofile.CarriedRuleSets(profile)...)
	}
	if s.wsStores != nil && ws != "" {
		if tm, merr := s.wsStores.getMemory(ctx, ws); merr == nil {
			req.Memory = tm
		} else if !errors.Is(merr, errNoPgDB) {
			notes = append(notes, "the content memory could not be read: "+merr.Error())
		}
	}

	var usage aiprovider.TokenUsage
	if withAI {
		if issues, cerr := runChecksOnBlock(ctx, b, pointChecks{TargetLocale: target}); cerr == nil {
			for _, issue := range issues {
				req.Findings = append(req.Findings, fmt.Sprintf("[%s] %s", issue.Severity, issue.Message))
			}
		}
		req.AI = s.fixAIChain(ctx, proj, stream, sb.ItemName, wsID, ws, target, &usage)
	}

	res, err := reviewdeck.FixCandidates(ctx, req)
	if err != nil {
		return serverErr(c, err)
	}
	if withAI {
		s.meterFixAI(ctx, wsID, ws, proj.ID, loc, usage)
	}
	res.Notes = append(notes, res.Notes...)

	current := b.TargetRuns(target)
	if current == nil {
		current = []model.Run{}
	}
	return c.JSON(http.StatusOK, fixCandidatesResponse{
		BlockID:             b.ID,
		Locale:              loc,
		Reason:              string(reason),
		SourceRevision:      check.SourceRevision(b, proj.DefaultSourceLanguage),
		TargetRevision:      store.TargetRevision(sb, target),
		CurrentRuns:         current,
		CurrentText:         model.RenderRunsWithData(current),
		CurrentEditText:     model.RunsEditText(current),
		FixCandidatesResult: *res,
	})
}

// fixVoiceProfile is the voice profile bound at the block's point, the ladder
// the worker translates under, or nil where none is bound or it cannot be
// read.
func (s *Server) fixVoiceProfile(ctx context.Context, proj *store.Project, stream string, loc model.LocaleID) *coreprofile.VoiceProfile {
	if s.VoiceStore == nil {
		return nil
	}
	var wd voicescope.WorkspaceDefault
	if s.AuthStore != nil {
		wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
	}
	p, err := voicescope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, voicescope.Scope{
		WorkspaceID: proj.WorkspaceID, ProjectID: proj.ID, Stream: stream, Locale: loc,
	})
	if err != nil {
		return nil
	}
	return p
}

// fixAIChain builds the one-block translate an AI fix candidate is drafted
// with: the workspace's platform AI provider, and the translate config an
// editor translation of the block is built with (voice, terms, memory), with
// the fix instruction on top. Its tokens are added to usage.
func (s *Server) fixAIChain(ctx context.Context, proj *store.Project, stream, itemName, wsID, ws string, target model.LocaleID, usage *aiprovider.TokenUsage) reviewdeck.FixAIChain {
	return func(ctx2 context.Context, instruction string) ([]tool.Tool, func(), error) {
		prov, err := s.flowAIProvider(ctx2, wsID, "")
		if err != nil {
			return nil, nil, err
		}
		if prov == nil {
			return nil, nil, errors.New("no AI provider is configured for this workspace")
		}
		cfg := editorTranslateConfig(ctx, s.ContentStore, s.editorVoiceContext(), proj, proj.ID, stream, itemName, wsID, ws,
			TranslateRequest{TargetLocale: string(target), BatchSize: 1})
		cfg.Instruction = instruction
		cfg.SourceLocale = proj.DefaultSourceLanguage
		cfg.TargetLocale = target
		tl := tools.NewAITranslateTool(prov, cfg)
		return []tool.Tool{tl}, func() { *usage = usage.Add(tl.TotalUsage()) }, nil
	}
}

// meterFixAI records the tokens an AI fix candidate spent, as an editor
// translation records them: the abuse cap sees every call, and the platform
// key's tokens are deducted from the workspace's credits.
func (s *Server) meterFixAI(ctx context.Context, wsID, ws, projectID, loc string, usage aiprovider.TokenUsage) {
	if usage.TotalTokens() == 0 {
		return
	}
	if s.QuotaStore != nil {
		if err := s.QuotaStore.RecordUsage(ctx, jobs.AIUsageRecord{
			WorkspaceSlug: ws, WorkspaceID: wsID, ProjectID: projectID, Operation: "fix",
			PromptTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, TotalTokens: usage.TotalTokens(),
		}); err != nil {
			slog.WarnContext(ctx, "fix candidates: record AI usage failed", "workspace", ws, "error", err)
		}
	}
	refID := fmt.Sprintf("%s:fix:%s:%s", projectID, loc, id.New())
	s.BillingHooks.DeductTokens(ctx, wsID, usage.TotalTokens(), "ai_fix", refID)
}
