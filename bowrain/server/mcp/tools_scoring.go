package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// voiceScopeInput carries the optional organizational scope a scoring tool
// resolves its voice profile from when profile_id is omitted. It maps to
// voicescope.Scope: the ladder is explicit profile_id → collection → stream →
// project → workspace default. An agent working inside a project passes
// project_id (and optionally stream/collection) and gets the right profile
// without knowing its ID.
type voiceScopeInput struct {
	ProjectID    string `json:"project_id,omitempty" jsonschema:"optional project to resolve the bound voice profile from"`
	Stream       string `json:"stream,omitempty" jsonschema:"optional stream (branch) whose voice binding overrides the project"`
	CollectionID string `json:"collection_id,omitempty" jsonschema:"optional collection whose voice binding overrides the stream"`
	Persona      string `json:"persona,omitempty" jsonschema:"optional author persona to apply on top of the voice profile (within its guardrails); unknown persona falls back to the base profile"`
}

// resolveProfile selects the effective voice profile for a scoring call.
// An explicit profileID wins; otherwise the profile is resolved from the
// organizational hierarchy (collection → stream → project → workspace default).
// locale and channel overrides are applied to the selected profile, then the
// scope's author persona (if any) is layered on inside the brand's guardrails;
// an unknown persona simply leaves the base profile unchanged. Returns an error
// when no profile is bound at any level, matching the prior behavior of an
// empty/unknown profile ID.
func (s *MCPServer) resolveProfile(ctx context.Context, profileID string, scope voiceScopeInput, locale, channel string) (*coreprofile.VoiceProfile, error) {
	projectID := scope.ProjectID
	if s.contentStore != nil {
		projectID = s.resolveProjectID(ctx, scope.ProjectID)
	}
	profile, err := voicescope.Resolve(ctx, s.contentStore, s.wsDefault, s.voiceStore, voicescope.Scope{
		ExplicitProfileID: profileID,
		ProjectID:         projectID,
		Stream:            scope.Stream,
		CollectionID:      scope.CollectionID,
		Locale:            model.LocaleID(locale),
		Channel:           channel,
		Persona:           scope.Persona,
	})
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, errors.New("no voice profile: pass profile_id or bind one to the project, stream, collection, or workspace")
	}
	return profile, nil
}

// Phase 2 tools: score_voice_compliance, suggest_corrections, rewrite_in_voice.

// registerPhase2Tools registers the advanced scoring and rewriting tools.
func (s *MCPServer) registerPhase2Tools() {
	// score_voice_compliance — full vocabulary + AI check with scores.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "score_voice_compliance",
		Description: "Run a full voice compliance check on text, returning an overall score (0-100), per-dimension scores (tone, style, vocabulary, clarity, compliance), and detailed findings with severity levels.",
	}, s.handleScoreVoiceCompliance)

	// suggest_corrections — generate rewrites for findings.
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "suggest_corrections",
		Description: "Given voice compliance findings, suggest specific text corrections. Returns the original text with each finding mapped to a concrete replacement suggestion.",
	}, s.handleSuggestCorrections)

	// rewrite_in_voice — rule-based substitution plus the guide for the rest.
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "rewrite_in_voice",
		Description: "Rewrite text to match a voice profile by substituting its forbidden and competitor terms. " +
			"Returns the rewritten text, a summary of the substitutions, the rules that matched and were left in " +
			"place under skipped (no replacement, or an inflected form) with the term, list, severity and reason, " +
			"and the voice guide for the tone and style edits the caller makes by hand. " +
			"Verify the result with score_voice_compliance.",
	}, s.handleRewriteInVoice)
}

// scoreVoiceComplianceInput is the input for the score_voice_compliance tool.
type scoreVoiceComplianceInput struct {
	ProfileID       string `json:"profile_id,omitempty" jsonschema:"the voice profile ID; omit to resolve from the project/stream/collection/workspace hierarchy"`
	Text            string `json:"text" jsonschema:"the text to score"`
	Locale          string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	voiceScopeInput `json:",inline"`
}

// scoreVoiceComplianceOutput is the output for the score_voice_compliance tool.
type scoreVoiceComplianceOutput struct {
	Score coreprofile.ComplianceScore `json:"score"`
}

func (s *MCPServer) handleScoreVoiceCompliance(ctx context.Context, req *mcp.CallToolRequest, input scoreVoiceComplianceInput) (*mcp.CallToolResult, scoreVoiceComplianceOutput, error) {
	profile, err := s.resolveProfile(ctx, input.ProfileID, input.voiceScopeInput, input.Locale, "")
	if err != nil {
		return nil, scoreVoiceComplianceOutput{}, err
	}

	runs := []model.Run{{Text: &model.TextRun{Text: input.Text}}}
	findings := coreprofile.Findings(profile, input.Text, runs)
	score := coreprofile.CalculateScore(findings)
	score.ProfileID = profile.ID
	score.WordCount = model.CountWords(input.Text)

	return nil, scoreVoiceComplianceOutput{Score: score}, nil
}

// suggestCorrectionsInput is the input for the suggest_corrections tool.
type suggestCorrectionsInput struct {
	ProfileID       string `json:"profile_id,omitempty" jsonschema:"the voice profile ID; omit to resolve from the project/stream/collection/workspace hierarchy"`
	Text            string `json:"text" jsonschema:"the original text to correct"`
	Locale          string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	voiceScopeInput `json:",inline"`
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
	profile, err := s.resolveProfile(ctx, input.ProfileID, input.voiceScopeInput, input.Locale, "")
	if err != nil {
		return nil, suggestCorrectionsOutput{}, err
	}

	// Vocabulary only, deliberately: this tool rewrites text, and a term rule
	// carries the replacement that makes a swap mechanical. A prohibited pattern
	// describes a shape, not a substitution, so folding pattern findings in here
	// would emit corrections with nothing to correct to. Patterns are reported by
	// the scoring and check tools, which say what is wrong without rewriting it.
	findings := coreprofile.HitsToFindings(coreprofile.MatchVocabulary(profile, input.Text), input.Text, nil)
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
	ProfileID       string `json:"profile_id,omitempty" jsonschema:"the voice profile ID; omit to resolve from the project/stream/collection/workspace hierarchy"`
	Text            string `json:"text" jsonschema:"the text to rewrite"`
	Locale          string `json:"locale,omitempty" jsonschema:"optional locale for locale-specific overrides"`
	Channel         string `json:"channel,omitempty" jsonschema:"optional channel for channel-specific overrides"`
	voiceScopeInput `json:",inline"`
}

// rewriteInVoiceOutput is the output for the rewrite_in_voice tool. Skipped
// lists the vocabulary rules that matched and were left in place, with the
// reason, so an agent can tell an unchanged text with nothing to fix from one
// that still carries violations.
type rewriteInVoiceOutput struct {
	Original  string                    `json:"original"`
	Rewritten string                    `json:"rewritten"`
	Changes   []string                  `json:"changes"`
	Skipped   []coreprofile.RewriteSkip `json:"skipped"`
	Guide     string                    `json:"voice_guide"`
}

func (s *MCPServer) handleRewriteInVoice(ctx context.Context, req *mcp.CallToolRequest, input rewriteInVoiceInput) (*mcp.CallToolResult, rewriteInVoiceOutput, error) {
	resolved, err := s.resolveProfile(ctx, input.ProfileID, input.voiceScopeInput, input.Locale, input.Channel)
	if err != nil {
		return nil, rewriteInVoiceOutput{}, err
	}

	// The rule-based substitution the framework defines, so this tool and
	// the kapi voice_rewrite tool agree with the vocabulary check on what a
	// hit is and on what they report.
	result := coreprofile.RewriteVocabulary(resolved, input.Text)
	var changes []string
	for _, c := range result.Changes {
		changes = append(changes, fmt.Sprintf("Replaced %s term %q with %q", c.List, c.Term, c.Replacement))
	}

	guide := formatVoiceGuide(resolved)

	return nil, rewriteInVoiceOutput{
		Original:  input.Text,
		Rewritten: result.Text,
		Changes:   changes,
		Skipped:   result.Skipped,
		Guide:     guide,
	}, nil
}
