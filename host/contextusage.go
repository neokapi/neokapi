package host

import (
	"context"
	"regexp"
	"sort"
	"sync"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// contextUsage counts, for each suggestion a check of the whole project meets,
// how often the content writes the preferred form and how often it writes a
// form the suggestion avoids. The counts are recorded as usage signals when the
// check ends: they add to a suggestion's standing, and content moving to an
// avoided form counts against it (core/contextop settling).
type contextUsage struct {
	mu sync.Mutex
	// byTerm maps the form a rule avoids first to the newest suggestion
	// stating it, the one the resolution answers with.
	byTerm map[string]contextop.Record
	counts map[string]*usageCount
	match  map[string][2]*regexp.Regexp
}

// usageCount is one suggestion's uses in the content a check read.
type usageCount struct {
	record              contextop.Record
	preferred, rejected int
}

// newContextUsage indexes the suggestions a check of one project can meet.
func newContextUsage(records []contextop.Record, project workspace.ProjectKey) *contextUsage {
	u := &contextUsage{byTerm: map[string]contextop.Record{}, counts: map[string]*usageCount{}, match: map[string][2]*regexp.Regexp{}}
	for _, r := range records {
		rule, ok := r.Rule()
		if !ok || rule.Replacement == "" || r.Project != project || !r.Kind.Bears() || !r.Status.Advises() || r.Established {
			continue
		}
		if key := suggestionKey(rule.Term); key != "" {
			if _, held := u.byTerm[key]; !held {
				u.byTerm[key] = r
			}
		}
	}
	return u
}

// count adds one text's uses of the advisory rules in force where it sits.
func (u *contextUsage) count(advisory []profile.TermRule, text string) {
	if u == nil || len(u.byTerm) == 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, rule := range advisory {
		key := suggestionKey(rule.Term)
		r, ok := u.byTerm[key]
		if !ok {
			continue
		}
		m, ok := u.match[r.ID]
		if !ok {
			m = [2]*regexp.Regexp{
				formMatcher([]string{rule.Replacement}, rule.MatchesCase()),
				formMatcher(append([]string{rule.Term}, rule.Forms...), rule.MatchesCase()),
			}
			u.match[r.ID] = m
		}
		preferred, rejected := countUses(text, m[0]), countUses(text, m[1])
		if preferred == 0 && rejected == 0 {
			continue
		}
		c := u.counts[r.ID]
		if c == nil {
			c = &usageCount{record: r}
			u.counts[r.ID] = c
		}
		c.preferred += preferred
		c.rejected += rejected
	}
}

// countUses counts the uses of a form in a text.
func countUses(text string, re *regexp.Regexp) int {
	if re == nil {
		return 0
	}
	return len(re.FindAllStringIndex(text, -1))
}

// usageActor is who records usage counts: the check that counted them.
var usageActor = contextop.Actor{Kind: contextop.ActorTool, Name: "check"}

// recordContextUsage records the counts a whole-project check gathered as
// usage signals, and brings the stores in line with what they settle. A signal
// only adds to standing or counts against a rule, so it establishes nothing;
// against a rule settled on evidence it takes the rule back out of force.
//
// Every failure is swallowed: a count is advice, and not worth failing a check
// to record.
func (a *App) recordContextUsage(ctx context.Context, recipe string, u *contextUsage) {
	if u == nil || len(u.counts) == 0 || recipe == "" {
		return
	}
	s, err := a.contextOps(ctx, recipe)
	if err != nil {
		return
	}
	before, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return
	}
	ids := make([]string, 0, len(u.counts))
	for id := range u.counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := u.counts[id]
		if _, err := s.ledger.Append(ctx, contextop.Record{
			Actor:   usageActor,
			Kind:    contextop.KindSignal,
			Target:  id,
			Project: c.record.Project,
			Signal:  &contextop.Signal{Source: contextop.SignalUsage, Preferred: c.preferred, Rejected: c.rejected},
		}); err != nil {
			return
		}
	}
	_, _ = s.reconcile(ctx, before)
}
