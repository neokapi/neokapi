package mcp

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// workspaceWordRules returns the word rules the workspace's terms store imposes
// on content written in loc, or in every language when loc is empty. A server
// with no terms resolver has none.
func (s *MCPServer) workspaceWordRules(ctx context.Context, workspaceID string, loc model.LocaleID) ([]coreprofile.TermRule, error) {
	if s.tbResolver == nil || workspaceID == "" {
		return nil, nil
	}
	tb, err := s.tbResolver.GetTB(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("open terms store: %w", err)
	}
	rules, err := voicescope.WordRules(ctx, tb, loc)
	if err != nil {
		return nil, fmt.Errorf("read word rules: %w", err)
	}
	return rules, nil
}

// wordRuleSets is every word rule text written under profile answers to: the
// workspace terms store's, then any the profile's voice file carries.
func (s *MCPServer) wordRuleSets(ctx context.Context, profile *coreprofile.VoiceProfile, loc model.LocaleID) ([]coreprofile.TermRuleSet, error) {
	rules, err := s.workspaceWordRules(ctx, profile.Scope, loc)
	if err != nil {
		return nil, err
	}
	return append(voicescope.WordRuleSets(rules), coreprofile.CarriedRuleSets(profile)...), nil
}

// voiceFindings is what the deterministic gate raises on text under profile:
// the word rules wordRuleSets gathers, then the profile's patterns.
func (s *MCPServer) voiceFindings(ctx context.Context, profile *coreprofile.VoiceProfile, loc model.LocaleID, text string) ([]coreprofile.VoiceFinding, error) {
	sets, err := s.wordRuleSets(ctx, profile, loc)
	if err != nil {
		return nil, err
	}
	runs := []model.Run{{Text: &model.TextRun{Text: text}}}
	findings := coreprofile.HitsToFindings(coreprofile.MatchTermRules(sets, text), text, runs)
	return append(findings, coreprofile.PatternFindings(profile, text, runs)...), nil
}
