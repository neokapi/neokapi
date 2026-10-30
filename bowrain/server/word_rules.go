package server

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// workspaceWordRules returns the word rules the workspace's terms store imposes
// on content written in loc, or in every language when loc is empty. A server
// with no terms store has none.
func (s *Server) workspaceWordRules(ctx context.Context, wsSlug string, loc model.LocaleID) ([]coreprofile.TermRule, error) {
	if s.wsStores == nil || wsSlug == "" {
		return nil, nil
	}
	tb, err := s.wsStores.getTerms(ctx, wsSlug)
	if errors.Is(err, errNoPgDB) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return voicescope.WordRules(ctx, tb, loc)
}

// voiceGateFindings is what the deterministic voice gate raises on text under
// profile: the workspace's word rules and any the profile's file carries, then
// the profile's patterns. A request carries text and no runs, so the word
// rules are matched as plain text (coreprofile.MatchTermRulesInText) and each
// finding is anchored to one text run over the text.
func (s *Server) voiceGateFindings(ctx context.Context, wsSlug string, profile *coreprofile.VoiceProfile, loc model.LocaleID, text string) ([]coreprofile.VoiceFinding, error) {
	rules, err := s.workspaceWordRules(ctx, wsSlug, loc)
	if err != nil {
		return nil, err
	}
	sets := append(voicescope.WordRuleSets(rules), coreprofile.CarriedRuleSets(profile)...)
	runs := []model.Run{{Text: &model.TextRun{Text: text}}}
	findings := coreprofile.HitsToFindings(coreprofile.MatchTermRulesInText(sets, text), text, runs)
	return append(findings, coreprofile.PatternFindings(profile, text, runs)...), nil
}
