package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	corebrand "github.com/neokapi/neokapi/core/brand"
)

// Phase 1 tools: check_vocabulary, list_profiles, get_voice_guide.

// registerPhase1Tools registers the basic brand voice tools.
func (s *MCPServer) registerPhase1Tools() {
	// check_vocabulary — validate text against brand terms.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "check_vocabulary",
		Description: "Validate text against a brand voice profile's vocabulary rules. Returns forbidden and competitor term violations with suggested replacements.",
	}, s.handleCheckVocabulary)

	// list_profiles — list available brand voice profiles in a workspace.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "list_profiles",
		Description: "List all brand voice profiles available in a workspace.",
	}, s.handleListProfiles)

	// get_voice_guide — formatted voice guide for LLM consumption.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "get_voice_guide",
		Description: "Get a formatted brand voice guide for a profile, optimized for LLM consumption. Includes tone, style rules, vocabulary constraints, and examples.",
	}, s.handleGetVoiceGuide)
}

// checkVocabularyInput is the input for the check_vocabulary tool.
type checkVocabularyInput struct {
	ProfileID string `json:"profile_id" jsonschema:"the brand voice profile ID to check against"`
	Text      string `json:"text" jsonschema:"the text to validate"`
	Locale    string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
}

// checkVocabularyOutput is the output for the check_vocabulary tool.
type checkVocabularyOutput struct {
	Findings []corebrand.BrandVoiceFinding  `json:"findings"`
	Score    corebrand.BrandComplianceScore `json:"score"`
}

func (s *MCPServer) handleCheckVocabulary(ctx context.Context, req *mcp.CallToolRequest, input checkVocabularyInput) (*mcp.CallToolResult, checkVocabularyOutput, error) {
	profile, err := s.brandStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, checkVocabularyOutput{}, fmt.Errorf("get profile: %w", err)
	}

	if input.Locale != "" {
		profile = corebrand.ResolveProfile(profile, input.Locale, "")
	}

	findings := checkVocab(input.Text, profile)
	score := corebrand.CalculateScore(findings)
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
	profiles, err := s.brandStore.ListProfiles(ctx, input.WorkspaceID)
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
	ProfileID string `json:"profile_id" jsonschema:"the brand voice profile ID"`
	Locale    string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	Channel   string `json:"channel,omitempty" jsonschema:"optional channel for channel-specific overrides"`
}

// getVoiceGuideOutput is the output for the get_voice_guide tool.
type getVoiceGuideOutput struct {
	Guide string `json:"guide"`
}

func (s *MCPServer) handleGetVoiceGuide(ctx context.Context, req *mcp.CallToolRequest, input getVoiceGuideInput) (*mcp.CallToolResult, getVoiceGuideOutput, error) {
	profile, err := s.brandStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, getVoiceGuideOutput{}, fmt.Errorf("get profile: %w", err)
	}

	resolved := corebrand.ResolveProfile(profile, input.Locale, input.Channel)
	guide := formatVoiceGuide(resolved)

	return nil, getVoiceGuideOutput{Guide: guide}, nil
}

// formatVoiceGuide produces a markdown-formatted voice guide optimized for LLM
// consumption. It delegates to corebrand.RenderVoiceGuide, the single source of
// truth shared with the kapi CLI and the AI translate prompt.
func formatVoiceGuide(p *corebrand.VoiceProfile) string {
	return corebrand.RenderVoiceGuide(p)
}

// checkVocab runs rule-based vocabulary checks against a brand voice profile.
// This mirrors the server's checkVocabulary function.
func checkVocab(text string, profile *corebrand.VoiceProfile) []corebrand.BrandVoiceFinding {
	var findings []corebrand.BrandVoiceFinding
	lowerText := strings.ToLower(text)

	for _, term := range profile.Vocabulary.ForbiddenTerms {
		if strings.Contains(lowerText, strings.ToLower(term.Term)) {
			sev := corebrand.SeverityMajor
			if term.Severity != "" {
				sev = corebrand.Severity(term.Severity)
			}
			findings = append(findings, corebrand.BrandVoiceFinding{
				Dimension:    corebrand.DimensionVocabulary,
				Severity:     sev,
				Message:      "Forbidden term: " + term.Term,
				Suggestion:   term.Replacement,
				OriginalText: term.Term,
			})
		}
	}

	for _, term := range profile.Vocabulary.CompetitorTerms {
		if strings.Contains(lowerText, strings.ToLower(term.Term)) {
			sev := corebrand.SeverityCritical
			if term.Severity != "" {
				sev = corebrand.Severity(term.Severity)
			}
			findings = append(findings, corebrand.BrandVoiceFinding{
				Dimension:    corebrand.DimensionVocabulary,
				Severity:     sev,
				Message:      "Competitor term: " + term.Term,
				Suggestion:   term.Replacement,
				OriginalText: term.Term,
			})
		}
	}

	return findings
}
