package backend

// The rules in force that a person or kapi put in force, with who did it and
// when: the lines `kapi rules list` reads from the operation log, each
// carrying its keeper. A rule held only in the terms store or the voice
// profile names no keeper and is not listed.

import (
	"context"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
)

// KeptRuleDTO is one rule in force and who kept it.
type KeptRuleDTO struct {
	// Kind is "name", "word", "tone" or "note".
	Kind string `json:"kind"`
	// Rule is the rule as a person reads it.
	Rule string `json:"rule"`
	// From is where the rule is held: terms, a pack, the workspace, the voice
	// profile. Empty for a rule the operation log alone holds.
	From string `json:"from,omitempty"`
	// Applies is where the rule holds.
	Applies string          `json:"applies"`
	KeptBy  ContextActorDTO `json:"kept_by"`
	// KeptAt is when the rule came into force, RFC 3339.
	KeptAt string `json:"kept_at"`
}

// KeptRules lists the rules in force in the project a tab holds that the
// operation log records a keeper for, in the order `kapi rules list` prints
// them. A folder without a kapi.yaml has none.
func (a *App) KeptRules(tabID string) ([]KeptRuleDTO, error) {
	op, err := a.decideTab(tabID)
	if err != nil {
		return nil, err
	}
	out := []KeptRuleDTO{}
	if op.unsaved.Load() {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	listing, err := a.hostEngine().ListRules(ctx, host.RulesRequest{Project: op.Path})
	if err != nil {
		return nil, err
	}
	for _, g := range listing.Groups {
		for _, l := range g.Rules {
			if dto, ok := keptRuleDTO(l); ok {
				out = append(out, dto)
			}
		}
	}
	return out, nil
}

// keptRuleDTO shapes one line that names its keeper.
func keptRuleDTO(l host.RuleLine) (KeptRuleDTO, bool) {
	who, at, ok := l.Kept()
	if !ok || l.Group == host.RulesGroupSuggested {
		return KeptRuleDTO{}, false
	}
	// The keeper's phrase is the last of From; the rest say where the rule is held.
	from := l.From
	if len(from) > 0 {
		from = from[:len(from)-1]
	}
	return KeptRuleDTO{
		Kind:    keptRuleKind(l),
		Rule:    l.Sentence,
		From:    strings.Join(from, ", "),
		Applies: l.Applies,
		KeptBy:  actorDTO(who),
		KeptAt:  at.UTC().Format(time.RFC3339),
	}, true
}

// keptRuleKind names a line's kind the way the screens mark it.
func keptRuleKind(l host.RuleLine) string {
	if l.SubjectKind() == contextop.SubjectNote {
		return "note"
	}
	switch l.Group {
	case host.RulesGroupNames:
		return "name"
	case host.RulesGroupTone:
		return "tone"
	}
	return "word"
}
