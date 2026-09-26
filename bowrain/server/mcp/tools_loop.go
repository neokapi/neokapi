package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/terms"
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
		Description: "List the candidate vocabulary rules a workspace's corrections have produced for a voice profile, each annotated with its decision status (pending, approved, rejected, promoted). Pending candidates are the ones awaiting review.",
	}, s.handleGetSuggestedRules)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "promote_rule",
		Description: "Promote a correction-derived candidate into the workspace terms store as a forbidden term, joined to the concept of its replacement, and record the decision against the voice profile, so the correction becomes a deterministic check on every future generation.",
	}, s.handlePromoteRule)

	if s.contentStore != nil {
		mcp.AddTool(s.server, &mcp.Tool{
			Name:        "evaluate_rule",
			Description: "Preview the blast radius of promoting a candidate rule: run the workspace terms store's word rules over a project's stored content with and without the rule, and report how many blocks it would newly flag, what it resolves, and the per-item breakdown, before the rule lands.",
		}, s.handleEvaluateRule)
	}
}

type getSuggestedRulesInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace whose corrections produced the candidates"`
	ProfileID   string `json:"profile_id" jsonschema:"the voice profile to annotate candidates against"`
	MinCount    int    `json:"min_count,omitempty" jsonschema:"minimum corrections behind a candidate (default 3)"`
	All         bool   `json:"all,omitempty" jsonschema:"include already rejected/promoted candidates (default false: only pending/approved)"`
}

type getSuggestedRulesOutput struct {
	Candidates []coreprofile.CandidateRule `json:"candidates"`
}

func (s *MCPServer) handleGetSuggestedRules(ctx context.Context, req *mcp.CallToolRequest, input getSuggestedRulesInput) (*mcp.CallToolResult, getSuggestedRulesOutput, error) {
	if err := s.authorizeWorkspace(ctx, req, input.WorkspaceID); err != nil {
		return nil, getSuggestedRulesOutput{}, err
	}
	minCount := input.MinCount
	if minCount <= 0 {
		minCount = 3
	}
	suggestions, err := s.voiceStore.GetSuggestedRules(ctx, input.WorkspaceID, minCount)
	if err != nil {
		return nil, getSuggestedRulesOutput{}, fmt.Errorf("get suggested rules: %w", err)
	}
	decisions, err := s.voiceStore.ListRuleDecisions(ctx, input.ProfileID)
	if err != nil {
		return nil, getSuggestedRulesOutput{}, fmt.Errorf("list rule decisions: %w", err)
	}
	return nil, getSuggestedRulesOutput{Candidates: coreprofile.MergeCandidates(suggestions, decisions, input.All)}, nil
}

type promoteRuleInput struct {
	ProfileID   string `json:"profile_id" jsonschema:"the voice profile whose corrections produced the rule; the rule lands in the terms store of its workspace"`
	Term        string `json:"term" jsonschema:"the term to forbid (what was repeatedly corrected away)"`
	Replacement string `json:"replacement,omitempty" jsonschema:"the preferred replacement (what it was corrected to)"`
	Locale      string `json:"locale,omitempty" jsonschema:"the language the term is written in; defaults to the source language of project_id"`
	ProjectID   string `json:"project_id,omitempty" jsonschema:"a project whose source language the term is written in, used when locale is omitted"`
}

type promoteRuleOutput struct {
	Promoted bool   `json:"promoted"`
	Message  string `json:"message"`
}

func (s *MCPServer) handlePromoteRule(ctx context.Context, req *mcp.CallToolRequest, input promoteRuleInput) (*mcp.CallToolResult, promoteRuleOutput, error) {
	if input.Term == "" {
		return nil, promoteRuleOutput{}, errors.New("term is required")
	}
	profile, err := s.voiceStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, promoteRuleOutput{}, fmt.Errorf("get profile: %w", err)
	}
	if profile == nil {
		return nil, promoteRuleOutput{}, fmt.Errorf("voice profile %q not found", input.ProfileID)
	}
	if err := s.authorizeWorkspace(ctx, req, profile.Scope); err != nil {
		return nil, promoteRuleOutput{}, err
	}
	loc, err := s.ruleLocale(ctx, req, input.Locale, input.ProjectID)
	if err != nil {
		return nil, promoteRuleOutput{}, err
	}
	if s.tbResolver == nil {
		return nil, promoteRuleOutput{}, errors.New("promote rule: no terms store is configured")
	}
	tb, err := s.tbResolver.GetTB(profile.Scope)
	if err != nil {
		return nil, promoteRuleOutput{}, fmt.Errorf("open terms store: %w", err)
	}
	rule := coreprofile.SuggestedRule{Term: input.Term, Replacement: input.Replacement}
	changed, err := terms.PromoteRule(ctx, tb, loc, rule)
	if err != nil {
		return nil, promoteRuleOutput{}, fmt.Errorf("promote rule: %w", err)
	}
	out := promoteRuleOutput{Promoted: changed}
	if changed {
		_ = s.voiceStore.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
			ProfileID:   input.ProfileID,
			Term:        input.Term,
			Replacement: input.Replacement,
			Dimension:   coreprofile.DimensionVocabulary,
			Status:      coreprofile.RuleDecisionPromoted,
		})
		out.Message = fmt.Sprintf("Promoted %q as a forbidden %s term in the workspace terms store", input.Term, loc)
	} else {
		out.Message = fmt.Sprintf("%q is already a forbidden %s term; no change", input.Term, loc)
	}
	return nil, out, nil
}

// ruleLocale is the language a promoted term is written in: locale when given,
// otherwise the source language of the named project.
func (s *MCPServer) ruleLocale(ctx context.Context, req *mcp.CallToolRequest, locale, projectID string) (model.LocaleID, error) {
	if locale != "" {
		return model.NormalizeLocale(model.LocaleID(locale)), nil
	}
	if projectID == "" || s.contentStore == nil {
		return "", errors.New("locale or project_id is required: the language the term is written in")
	}
	id, err := s.authorizeProject(ctx, req, projectID)
	if err != nil {
		return "", err
	}
	p, err := s.contentStore.GetProject(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	if p == nil || p.DefaultSourceLanguage == "" {
		return "", fmt.Errorf("project %q names no source language; pass locale", projectID)
	}
	return p.DefaultSourceLanguage, nil
}

type evaluateRuleInput struct {
	Term        string `json:"term" jsonschema:"the candidate term to evaluate"`
	Replacement string `json:"replacement,omitempty" jsonschema:"the preferred replacement"`
	ProjectID   string `json:"project_id" jsonschema:"the project whose content to evaluate against"`
	Stream      string `json:"stream,omitempty" jsonschema:"the stream (default main)"`
	Locale      string `json:"locale,omitempty" jsonschema:"the language the term is written in (default the project's source language)"`
}

func (s *MCPServer) handleEvaluateRule(ctx context.Context, req *mcp.CallToolRequest, input evaluateRuleInput) (*mcp.CallToolResult, coreprofile.BlastRadius, error) {
	if input.Term == "" || input.ProjectID == "" {
		return nil, coreprofile.BlastRadius{}, errors.New("term and project_id are required")
	}
	projectID, err := s.authorizeProject(ctx, req, input.ProjectID)
	if err != nil {
		return nil, coreprofile.BlastRadius{}, err
	}
	project, err := s.contentStore.GetProject(ctx, projectID)
	if err != nil {
		return nil, coreprofile.BlastRadius{}, fmt.Errorf("get project: %w", err)
	}
	if project == nil {
		return nil, coreprofile.BlastRadius{}, ErrProjectNotFound
	}
	loc := project.DefaultSourceLanguage
	if input.Locale != "" {
		loc = model.LocaleID(input.Locale)
	}
	rules, err := s.workspaceWordRules(ctx, project.WorkspaceID, loc)
	if err != nil {
		return nil, coreprofile.BlastRadius{}, err
	}
	baseline := voicescope.WordRuleSets(rules)
	candidate := coreprofile.CandidateWithRule(baseline, coreprofile.SuggestedRule{Term: input.Term, Replacement: input.Replacement})
	// Walked a batch at a time, projecting as it goes: the evaluation wants an
	// id, a name and the source text, and holding the stored blocks those came
	// from — runs, every target, properties, annotations — alongside the
	// projection meant paying for the corpus twice to look at part of it once.
	var blocks []coreprofile.EvalBlock
	err = store.EachBlockBatch(ctx, s.contentStore,
		store.BlockQuery{ProjectID: projectID, Stream: input.Stream},
		store.DefaultBlockBatch,
		func(batch []*venue.StoredBlock) error {
			for _, sb := range batch {
				if sb == nil || sb.Block == nil {
					continue
				}
				blocks = append(blocks, coreprofile.EvalBlock{
					BlockID:        sb.Block.ID,
					CollectionID:   sb.ItemName,
					CollectionName: sb.ItemName,
					Text:           sb.Block.SourceText(),
				})
			}
			return nil
		})
	if err != nil {
		return nil, coreprofile.BlastRadius{}, fmt.Errorf("get blocks: %w", err)
	}
	return nil, coreprofile.EvaluateBlastRadius(blocks, baseline, candidate), nil
}
