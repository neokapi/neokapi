package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
)

// Correction-learning loop tools: an AI assistant can see the candidate rules a
// team's corrections have produced, preview the impact of promoting one, and
// promote it — so the loop is drivable from any MCP client, not only the web UI.

// registerLoopTools registers the correction-learning loop tools. The
// blast-radius preview needs the content store; it is only registered when one
// is configured.
func (s *MCPServer) registerLoopTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_suggested_rules",
		Description: "List the candidate vocabulary rules a workspace's corrections have produced for a brand profile, each annotated with its decision status (pending, approved, rejected, promoted). Pending candidates are the ones awaiting review.",
	}, s.handleGetSuggestedRules)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "promote_rule",
		Description: "Promote a correction-derived candidate into a brand profile as an enforced forbidden term. Bumps and archives the profile version and records the decision, so the correction becomes a deterministic check on every future generation.",
	}, s.handlePromoteRule)

	if s.contentStore != nil {
		mcp.AddTool(s.server, &mcp.Tool{
			Name:        "evaluate_rule",
			Description: "Preview the blast radius of promoting a candidate rule: run the profile's vocabulary checks over a project's stored content with and without the rule, and report how many blocks it would newly flag, what it resolves, and the per-item breakdown — before the rule lands.",
		}, s.handleEvaluateRule)
	}
}

type getSuggestedRulesInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace whose corrections produced the candidates"`
	ProfileID   string `json:"profile_id" jsonschema:"the brand profile to annotate candidates against"`
	MinCount    int    `json:"min_count,omitempty" jsonschema:"minimum corrections behind a candidate (default 3)"`
	All         bool   `json:"all,omitempty" jsonschema:"include already rejected/promoted candidates (default false: only pending/approved)"`
}

type getSuggestedRulesOutput struct {
	Candidates []corebrand.CandidateRule `json:"candidates"`
}

func (s *MCPServer) handleGetSuggestedRules(ctx context.Context, _ *mcp.CallToolRequest, input getSuggestedRulesInput) (*mcp.CallToolResult, getSuggestedRulesOutput, error) {
	minCount := input.MinCount
	if minCount <= 0 {
		minCount = 3
	}
	suggestions, err := s.brandStore.GetSuggestedRules(ctx, input.WorkspaceID, minCount)
	if err != nil {
		return nil, getSuggestedRulesOutput{}, fmt.Errorf("get suggested rules: %w", err)
	}
	decisions, err := s.brandStore.ListRuleDecisions(ctx, input.ProfileID)
	if err != nil {
		return nil, getSuggestedRulesOutput{}, fmt.Errorf("list rule decisions: %w", err)
	}
	return nil, getSuggestedRulesOutput{Candidates: corebrand.MergeCandidates(suggestions, decisions, input.All)}, nil
}

type promoteRuleInput struct {
	ProfileID   string `json:"profile_id" jsonschema:"the brand profile to promote the rule into"`
	Term        string `json:"term" jsonschema:"the term to forbid (what was repeatedly corrected away)"`
	Replacement string `json:"replacement,omitempty" jsonschema:"the preferred replacement (what it was corrected to)"`
}

type promoteRuleOutput struct {
	Promoted bool   `json:"promoted"`
	Version  int    `json:"version"`
	Message  string `json:"message"`
}

func (s *MCPServer) handlePromoteRule(ctx context.Context, _ *mcp.CallToolRequest, input promoteRuleInput) (*mcp.CallToolResult, promoteRuleOutput, error) {
	if input.Term == "" {
		return nil, promoteRuleOutput{}, errors.New("term is required")
	}
	rule := corebrand.SuggestedRule{Term: input.Term, Replacement: input.Replacement}
	profile, changed, err := corebrand.PromoteAndSave(ctx, s.brandStore, input.ProfileID, rule)
	if err != nil {
		return nil, promoteRuleOutput{}, fmt.Errorf("promote rule: %w", err)
	}
	out := promoteRuleOutput{Promoted: changed, Version: profile.Version}
	if changed {
		_ = s.brandStore.RecordRuleDecision(ctx, &corebrand.RuleDecision{
			ProfileID:       input.ProfileID,
			Term:            input.Term,
			Replacement:     input.Replacement,
			Dimension:       corebrand.DimensionVocabulary,
			Status:          corebrand.RuleDecisionPromoted,
			PromotedVersion: profile.Version,
		})
		out.Message = fmt.Sprintf("Promoted %q as a forbidden term; profile is now version %d", input.Term, profile.Version)
	} else {
		out.Message = fmt.Sprintf("%q is already enforced; no change", input.Term)
	}
	return nil, out, nil
}

type evaluateRuleInput struct {
	ProfileID   string `json:"profile_id" jsonschema:"the baseline brand profile"`
	Term        string `json:"term" jsonschema:"the candidate term to evaluate"`
	Replacement string `json:"replacement,omitempty" jsonschema:"the preferred replacement"`
	ProjectID   string `json:"project_id" jsonschema:"the project whose content to evaluate against"`
	Stream      string `json:"stream,omitempty" jsonschema:"the stream (default main)"`
}

func (s *MCPServer) handleEvaluateRule(ctx context.Context, _ *mcp.CallToolRequest, input evaluateRuleInput) (*mcp.CallToolResult, corebrand.BlastRadius, error) {
	if input.Term == "" || input.ProjectID == "" {
		return nil, corebrand.BlastRadius{}, errors.New("term and project_id are required")
	}
	baseline, err := s.brandStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, corebrand.BlastRadius{}, fmt.Errorf("get profile: %w", err)
	}
	candidate := corebrand.CandidateWithRule(baseline, corebrand.SuggestedRule{Term: input.Term, Replacement: input.Replacement})
	stored, err := s.contentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: input.ProjectID, Stream: input.Stream})
	if err != nil {
		return nil, corebrand.BlastRadius{}, fmt.Errorf("get blocks: %w", err)
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
	return nil, corebrand.EvaluateBlastRadius(blocks, baseline, candidate), nil
}
