package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// Phase 1 tools: check_vocabulary, list_profiles, get_voice_guide.

// registerPhase1Tools registers the basic voice tools.
func (s *MCPServer) registerPhase1Tools() {
	// check_vocabulary — validate text against brand terms.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "check_vocabulary",
		Description: "Check text against the vocabulary rules the governing profile holds. Returns forbidden and competitor term violations with suggested replacements. Prefer retrieving the guidance first (get_voice_guide) and writing to it. This reports what a rule caught after the fact.",
	}, s.handleCheckVocabulary)

	// list_profiles — list available voice profiles in a workspace.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_profiles",
		Description: "Context discovery: list the profiles this workspace holds: the named bundles of coordinates content is written under (audience, surface, register, market). Start here when you do not yet know which profile governs the content at hand.",
	}, s.handleListProfiles)

	// get_voice_guide — formatted voice guide for LLM consumption.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_voice_guide",
		Description: "Context retrieval: get the voice guidance that applies under a profile, formatted for a model to read. Includes tone, style rules, vocabulary constraints, and examples. Retrieve this BEFORE generating content, not after a check fails.",
	}, s.handleGetVoiceGuide)
}

// checkVocabularyInput is the input for the check_vocabulary tool.
type checkVocabularyInput struct {
	ProfileID string `json:"profile_id" jsonschema:"the voice profile ID to check against"`
	Text      string `json:"text" jsonschema:"the text to validate"`
	Locale    string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
}

// checkVocabularyOutput is the output for the check_vocabulary tool.
type checkVocabularyOutput struct {
	Findings []coreprofile.VoiceFinding  `json:"findings"`
	Score    coreprofile.ComplianceScore `json:"score"`
}

func (s *MCPServer) handleCheckVocabulary(ctx context.Context, req *mcp.CallToolRequest, input checkVocabularyInput) (*mcp.CallToolResult, checkVocabularyOutput, error) {
	profile, err := s.voiceStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, checkVocabularyOutput{}, fmt.Errorf("get profile: %w", err)
	}

	if input.Locale != "" {
		profile = coreprofile.ResolveProfile(profile, model.LocaleID(input.Locale), "", "")
	}

	runs := []model.Run{{Text: &model.TextRun{Text: input.Text}}}
	findings := coreprofile.Findings(profile, input.Text, runs)
	score := coreprofile.CalculateScore(findings)
	score.ProfileID = profile.ID

	return nil, checkVocabularyOutput{
		Findings: findings,
		Score:    score,
	}, nil
}

// listProfilesInput is the input for the list_profiles tool.
type listProfilesInput struct {
	WorkspaceID string `json:"workspace_id" jsonschema:"the workspace ID to list profiles for"`
}

// listProfilesOutput is the output for the list_profiles tool.
type listProfilesOutput struct {
	Profiles []profileSummary `json:"profiles"`
}

type profileSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Formality   string `json:"formality"`
	Version     int    `json:"version"`
}

func (s *MCPServer) handleListProfiles(ctx context.Context, req *mcp.CallToolRequest, input listProfilesInput) (*mcp.CallToolResult, listProfilesOutput, error) {
	profiles, err := s.voiceStore.ListProfiles(ctx, input.WorkspaceID)
	if err != nil {
		return nil, listProfilesOutput{}, fmt.Errorf("list profiles: %w", err)
	}

	var summaries []profileSummary
	for _, p := range profiles {
		summaries = append(summaries, profileSummary{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			Formality:   p.Tone.Formality,
			Version:     p.Version,
		})
	}

	return nil, listProfilesOutput{Profiles: summaries}, nil
}

// getVoiceGuideInput is the input for the get_voice_guide tool.
type getVoiceGuideInput struct {
	ProfileID string `json:"profile_id" jsonschema:"the voice profile ID"`
	Locale    string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	Channel   string `json:"channel,omitempty" jsonschema:"optional channel for channel-specific overrides"`
}

// getVoiceGuideOutput is the output for the get_voice_guide tool.
type getVoiceGuideOutput struct {
	Guide string `json:"guide"`
}

func (s *MCPServer) handleGetVoiceGuide(ctx context.Context, req *mcp.CallToolRequest, input getVoiceGuideInput) (*mcp.CallToolResult, getVoiceGuideOutput, error) {
	profile, err := s.voiceStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, getVoiceGuideOutput{}, fmt.Errorf("get profile: %w", err)
	}

	resolved := coreprofile.ResolveProfile(profile, model.LocaleID(input.Locale), input.Channel, "")
	guide := formatVoiceGuide(resolved)

	return nil, getVoiceGuideOutput{Guide: guide}, nil
}

// formatVoiceGuide produces a markdown-formatted voice guide optimized for LLM
// consumption. It delegates to coreprofile.RenderVoiceGuide, the single source of
// truth shared with the kapi CLI and the AI translate prompt.
func formatVoiceGuide(p *coreprofile.VoiceProfile) string {
	return coreprofile.RenderVoiceGuide(p)
}
