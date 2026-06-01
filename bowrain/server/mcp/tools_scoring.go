package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
)

// Phase 2 tools: score_brand_compliance, suggest_corrections, rewrite_in_voice.

// registerPhase2Tools registers the advanced scoring and rewriting tools.
func (s *MCPServer) registerPhase2Tools() {
	// score_brand_compliance — full vocabulary + AI check with scores.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "score_brand_compliance",
		Description: "Run a full brand compliance check on text, returning an overall score (0-100), per-dimension scores (tone, style, vocabulary, clarity, brand_compliance), and detailed findings with severity levels.",
	}, s.handleScoreBrandCompliance)

	// suggest_corrections — generate rewrites for findings.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "suggest_corrections",
		Description: "Given brand compliance findings, suggest specific text corrections. Returns the original text with each finding mapped to a concrete replacement suggestion.",
	}, s.handleSuggestCorrections)

	// rewrite_in_voice — full rewrite with diff.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "rewrite_in_voice",
		Description: "Rewrite text to match a brand voice profile. Returns the rewritten text and a summary of changes made. Uses vocabulary rules and style guidelines to transform the input.",
	}, s.handleRewriteInVoice)
}

// scoreBrandComplianceInput is the input for the score_brand_compliance tool.
type scoreBrandComplianceInput struct {
	ProfileID string `json:"profile_id" jsonschema:"the brand voice profile ID"`
	Text      string `json:"text" jsonschema:"the text to score"`
	Locale    string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
}

// scoreBrandComplianceOutput is the output for the score_brand_compliance tool.
type scoreBrandComplianceOutput struct {
	Score corebrand.BrandComplianceScore `json:"score"`
}

func (s *MCPServer) handleScoreBrandCompliance(ctx context.Context, req *mcp.CallToolRequest, input scoreBrandComplianceInput) (*mcp.CallToolResult, scoreBrandComplianceOutput, error) {
	profile, err := s.brandStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, scoreBrandComplianceOutput{}, fmt.Errorf("get profile: %w", err)
	}

	if input.Locale != "" {
		profile = corebrand.ResolveProfile(profile, model.LocaleID(input.Locale), "")
	}

	findings := checkVocab(input.Text, profile)
	score := corebrand.CalculateScore(findings)
	score.ProfileID = profile.ID
	score.WordCount = countWords(input.Text)

	return nil, scoreBrandComplianceOutput{Score: score}, nil
}

// suggestCorrectionsInput is the input for the suggest_corrections tool.
type suggestCorrectionsInput struct {
	ProfileID string `json:"profile_id" jsonschema:"the brand voice profile ID"`
	Text      string `json:"text" jsonschema:"the original text to correct"`
	Locale    string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
}

type correction struct {
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Reason      string `json:"reason"`
}

// suggestCorrectionsOutput is the output for the suggest_corrections tool.
type suggestCorrectionsOutput struct {
	Corrections []correction `json:"corrections"`
	Corrected   string       `json:"corrected_text"`
}

func (s *MCPServer) handleSuggestCorrections(ctx context.Context, req *mcp.CallToolRequest, input suggestCorrectionsInput) (*mcp.CallToolResult, suggestCorrectionsOutput, error) {
	profile, err := s.brandStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, suggestCorrectionsOutput{}, fmt.Errorf("get profile: %w", err)
	}

	if input.Locale != "" {
		profile = corebrand.ResolveProfile(profile, model.LocaleID(input.Locale), "")
	}

	findings := checkVocab(input.Text, profile)
	var corrections []correction
	corrected := input.Text

	for _, f := range findings {
		if f.Suggestion != "" {
			corrections = append(corrections, correction{
				Original:    f.OriginalText,
				Replacement: f.Suggestion,
				Reason:      f.Message,
			})
			corrected = strings.ReplaceAll(corrected, f.OriginalText, f.Suggestion)
		}
	}

	return nil, suggestCorrectionsOutput{
		Corrections: corrections,
		Corrected:   corrected,
	}, nil
}

// rewriteInVoiceInput is the input for the rewrite_in_voice tool.
type rewriteInVoiceInput struct {
	ProfileID string `json:"profile_id" jsonschema:"the brand voice profile ID"`
	Text      string `json:"text" jsonschema:"the text to rewrite"`
	Locale    string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	Channel   string `json:"channel,omitempty" jsonschema:"optional channel for channel-specific overrides"`
}

// rewriteInVoiceOutput is the output for the rewrite_in_voice tool.
type rewriteInVoiceOutput struct {
	Original  string   `json:"original"`
	Rewritten string   `json:"rewritten"`
	Changes   []string `json:"changes"`
	Guide     string   `json:"voice_guide"`
}

func (s *MCPServer) handleRewriteInVoice(ctx context.Context, req *mcp.CallToolRequest, input rewriteInVoiceInput) (*mcp.CallToolResult, rewriteInVoiceOutput, error) {
	profile, err := s.brandStore.GetProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, rewriteInVoiceOutput{}, fmt.Errorf("get profile: %w", err)
	}

	resolved := corebrand.ResolveProfile(profile, model.LocaleID(input.Locale), input.Channel)

	// Apply vocabulary-based rewrites.
	rewritten := input.Text
	var changes []string

	for _, term := range resolved.Vocabulary.ForbiddenTerms {
		if term.Replacement != "" && strings.Contains(strings.ToLower(rewritten), strings.ToLower(term.Term)) {
			rewritten = strings.ReplaceAll(rewritten, term.Term, term.Replacement)
			changes = append(changes, fmt.Sprintf("Replaced forbidden term %q with %q", term.Term, term.Replacement))
		}
	}
	for _, term := range resolved.Vocabulary.CompetitorTerms {
		if term.Replacement != "" && strings.Contains(strings.ToLower(rewritten), strings.ToLower(term.Term)) {
			rewritten = strings.ReplaceAll(rewritten, term.Term, term.Replacement)
			changes = append(changes, fmt.Sprintf("Replaced competitor term %q with %q", term.Term, term.Replacement))
		}
	}

	guide := formatVoiceGuide(resolved)

	return nil, rewriteInVoiceOutput{
		Original:  input.Text,
		Rewritten: rewritten,
		Changes:   changes,
		Guide:     guide,
	}, nil
}

// countWords counts the number of whitespace-delimited words in text.
func countWords(text string) int {
	return len(strings.Fields(text))
}
