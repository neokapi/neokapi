package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/brandscope"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
)

// brandScopeInput carries the optional organizational scope a scoring tool
// resolves its brand-voice profile from when profile_id is omitted. It maps to
// brandscope.Scope: the ladder is explicit profile_id → collection → stream →
// project → workspace default. An agent working inside a project passes
// project_id (and optionally stream/collection) and gets the right profile
// without knowing its ID.
type brandScopeInput struct {
	ProjectID    string `json:"project_id,omitempty" jsonschema:"optional project to resolve the bound brand voice profile from"`
	Stream       string `json:"stream,omitempty" jsonschema:"optional stream (branch) whose brand voice binding overrides the project"`
	CollectionID string `json:"collection_id,omitempty" jsonschema:"optional collection whose brand voice binding overrides the stream"`
}

// resolveProfile selects the effective brand-voice profile for a scoring call.
// An explicit profileID wins; otherwise the profile is resolved from the
// organizational hierarchy (collection → stream → project → workspace default).
// locale and channel overrides are applied to the selected profile. Returns an
// error when no profile is bound at any level, matching the prior behavior of
// an empty/unknown profile ID.
func (s *MCPServer) resolveProfile(ctx context.Context, profileID string, scope brandScopeInput, locale, channel string) (*corebrand.VoiceProfile, error) {
	projectID := scope.ProjectID
	if s.contentStore != nil {
		projectID = s.resolveProjectID(ctx, scope.ProjectID)
	}
	profile, err := brandscope.Resolve(ctx, s.contentStore, s.wsDefault, s.brandStore, brandscope.Scope{
		ExplicitProfileID: profileID,
		ProjectID:         projectID,
		Stream:            scope.Stream,
		CollectionID:      scope.CollectionID,
		Locale:            model.LocaleID(locale),
		Channel:           channel,
	})
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("no brand voice profile: pass profile_id or bind one to the project, stream, collection, or workspace")
	}
	return profile, nil
}

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
	ProfileID       string `json:"profile_id,omitempty" jsonschema:"the brand voice profile ID; omit to resolve from the project/stream/collection/workspace hierarchy"`
	Text            string `json:"text" jsonschema:"the text to score"`
	Locale          string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	brandScopeInput `json:",inline"`
}

// scoreBrandComplianceOutput is the output for the score_brand_compliance tool.
type scoreBrandComplianceOutput struct {
	Score corebrand.BrandComplianceScore `json:"score"`
}

func (s *MCPServer) handleScoreBrandCompliance(ctx context.Context, req *mcp.CallToolRequest, input scoreBrandComplianceInput) (*mcp.CallToolResult, scoreBrandComplianceOutput, error) {
	profile, err := s.resolveProfile(ctx, input.ProfileID, input.brandScopeInput, input.Locale, "")
	if err != nil {
		return nil, scoreBrandComplianceOutput{}, err
	}

	runs := []model.Run{{Text: &model.TextRun{Text: input.Text}}}
	findings := corebrand.HitsToFindings(corebrand.MatchVocabulary(profile, input.Text), input.Text, runs)
	score := corebrand.CalculateScore(findings)
	score.ProfileID = profile.ID
	score.WordCount = countWords(input.Text)

	return nil, scoreBrandComplianceOutput{Score: score}, nil
}

// suggestCorrectionsInput is the input for the suggest_corrections tool.
type suggestCorrectionsInput struct {
	ProfileID       string `json:"profile_id,omitempty" jsonschema:"the brand voice profile ID; omit to resolve from the project/stream/collection/workspace hierarchy"`
	Text            string `json:"text" jsonschema:"the original text to correct"`
	Locale          string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	brandScopeInput `json:",inline"`
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
	profile, err := s.resolveProfile(ctx, input.ProfileID, input.brandScopeInput, input.Locale, "")
	if err != nil {
		return nil, suggestCorrectionsOutput{}, err
	}

	findings := corebrand.HitsToFindings(corebrand.MatchVocabulary(profile, input.Text), input.Text, nil)
	var corrections []correction
	corrected := input.Text

	for _, f := range findings {
		// The structured replacement (the preferred term) is the concrete swap;
		// f.Suggestion is the human-readable "Use ... instead" message used as the
		// reason, so it must never be substituted into the corrected text.
		repl := f.Metadata["replacement"]
		if repl == "" {
			continue
		}
		corrections = append(corrections, correction{
			Original:    f.OriginalText,
			Replacement: repl,
			Reason:      f.Message,
		})
		corrected = strings.ReplaceAll(corrected, f.OriginalText, repl)
	}

	return nil, suggestCorrectionsOutput{
		Corrections: corrections,
		Corrected:   corrected,
	}, nil
}

// rewriteInVoiceInput is the input for the rewrite_in_voice tool.
type rewriteInVoiceInput struct {
	ProfileID       string `json:"profile_id,omitempty" jsonschema:"the brand voice profile ID; omit to resolve from the project/stream/collection/workspace hierarchy"`
	Text            string `json:"text" jsonschema:"the text to rewrite"`
	Locale          string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	Channel         string `json:"channel,omitempty" jsonschema:"optional channel for channel-specific overrides"`
	brandScopeInput `json:",inline"`
}

// rewriteInVoiceOutput is the output for the rewrite_in_voice tool.
type rewriteInVoiceOutput struct {
	Original  string   `json:"original"`
	Rewritten string   `json:"rewritten"`
	Changes   []string `json:"changes"`
	Guide     string   `json:"voice_guide"`
}

func (s *MCPServer) handleRewriteInVoice(ctx context.Context, req *mcp.CallToolRequest, input rewriteInVoiceInput) (*mcp.CallToolResult, rewriteInVoiceOutput, error) {
	resolved, err := s.resolveProfile(ctx, input.ProfileID, input.brandScopeInput, input.Locale, input.Channel)
	if err != nil {
		return nil, rewriteInVoiceOutput{}, err
	}

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
