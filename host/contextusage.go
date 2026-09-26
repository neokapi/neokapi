package host

import (
	"context"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"sync"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// contextUsage counts, for each suggestion and each established rule a check of
// the whole project meets, how often the content writes the preferred form and
// how often it writes a form the rule avoids. The counts are recorded as usage
// signals when the check ends. For a suggestion they add to its standing, and
// content moving to an avoided form counts against it (core/contextop
// settling). For an established rule they are what the digest reads as drift:
// content moving away from the rule after it came into force.
type contextUsage struct {
	mu sync.Mutex
	// byTerm maps the form a rule avoids first to the newest suggestion
	// stating it, the one the resolution answers with.
	byTerm map[string]contextop.Record
	// established are the project's rules in force, counted wherever their
	// scope covers the point a text sits at.
	established []contextop.Record
	counts      map[string]*usageCount
	match       map[string][2]*regexp.Regexp
}

// usageCount is one suggestion's uses in the content a check read.
type usageCount struct {
	record              contextop.Record
	preferred, rejected int
}

// newContextUsage indexes the suggestions and the established rules a check of
// one project can meet.
func newContextUsage(records []contextop.Record, project workspace.ProjectKey) *contextUsage {
	u := &contextUsage{byTerm: map[string]contextop.Record{}, counts: map[string]*usageCount{}, match: map[string][2]*regexp.Regexp{}}
	for _, r := range records {
		rule, ok := r.Rule()
		if !ok || rule.Replacement == "" || r.Project != project || !r.Kind.Bears() {
			continue
		}
		if r.Established && r.Status == contextop.StatusEstablished {
			u.established = append(u.established, r)
			continue
		}
		if !r.Status.Advises() || r.Established {
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

// empty reports a project with nothing to count.
func (u *contextUsage) empty() bool {
	return u == nil || (len(u.byTerm) == 0 && len(u.established) == 0)
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
		u.add(r, rule, text)
	}
}

// countEstablished adds one text's uses of the established rules whose scope
// covers the point at the coordinates given.
func (u *contextUsage) countEstablished(coordinates map[string]string, text string) {
	if u == nil || len(u.established) == 0 {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, r := range u.established {
		if !r.Scope.Covers(coordinates) {
			continue
		}
		rule, _ := r.Rule()
		u.add(r, rule, text)
	}
}

// add counts one rule's forms in one text. The caller holds the lock.
func (u *contextUsage) add(r contextop.Record, rule profile.TermRule, text string) {
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
		return
	}
	c := u.counts[r.ID]
	if c == nil {
		c = &usageCount{record: r}
		u.counts[r.ID] = c
	}
	c.preferred += preferred
	c.rejected += rejected
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
// A rule's counts are recorded only when they differ from the latest ones
// recorded for it. Standing and drift read the latest recorded counts, so a run
// over unchanged content adds nothing to the log, however often it runs.
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
	signals, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Kinds: []contextop.Kind{contextop.KindSignal}})
	if err != nil {
		return
	}
	// latest is the newest usage count recorded for each rule. Records come
	// newest first, so the first one seen per rule is it.
	latest := map[string]*contextop.Signal{}
	for _, r := range signals {
		if r.Signal == nil || r.Signal.Source != contextop.SignalUsage || !r.Status.Answers() {
			continue
		}
		if _, seen := latest[r.Target]; !seen {
			latest[r.Target] = r.Signal
		}
	}
	ids := make([]string, 0, len(u.counts))
	for id := range u.counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := u.counts[id]
		if prev := latest[id]; prev != nil && prev.Within == "" && prev.Preferred == c.preferred && prev.Rejected == c.rejected {
			continue
		}
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

// countProjectUsage reads the source content a `kapi up` run converged and
// records how it writes each suggestion's and each established rule's forms, as
// a whole-project check does. A project with neither reads nothing. Every failure is swallowed, for
// the reason recordContextUsage gives.
func (a *App) countProjectUsage(ctx context.Context, cmd Command, recipe string, files []project.ResolvedFile) {
	rules, err := a.newContextRules(cmd, recipe)
	if err != nil || rules == nil {
		return
	}
	usage := newContextUsage(rules.records, rules.key)
	if usage.empty() {
		return
	}
	formats, err := a.newCheckFormats(cmd)
	if err != nil {
		return
	}
	for _, rf := range files {
		point := project.GovernancePoint{Path: filepath.ToSlash(rf.Relative)}
		at, rerr := rules.at(point)
		if rerr != nil {
			continue
		}
		coordinates, cerr := rules.coordinatesAt(point)
		if cerr != nil || (len(at.Advisory) == 0 && len(usage.established) == 0) {
			continue
		}
		name, cfg := formats.forFile(a, rf.Path)
		blocks, berr := a.readBlocksAs(ctx, rf.Path, name, cfg, a.SourceLocale())
		if berr != nil {
			continue
		}
		for _, b := range blocks {
			usage.count(at.Advisory, b.SourceText())
			usage.countEstablished(coordinates, b.SourceText())
		}
	}
	a.recordContextUsage(ctx, recipe, usage)
}

// appliedTexts are the new wordings of the entries an edit applied.
func appliedTexts(entries []changeEntry, applied []string) []string {
	var out []string
	for _, e := range entries {
		if e.ID != "" && slices.Contains(applied, e.ID) {
			out = append(out, e.Text)
		}
	}
	return out
}

// applyActor is who records that an agent's edit followed a suggestion: the
// apply that wrote it.
var applyActor = contextop.Actor{Kind: contextop.ActorTool, Name: "apply"}

// noteAgentEdits records that an agent's applied edits write the form a
// suggestion prefers, one signal per suggestion and file. It adds to the
// suggestion's standing and establishes nothing: an agent following a
// suggestion is no person's signal. Every failure is swallowed, as
// recordContextUsage's are.
func (a *App) noteAgentEdits(ctx context.Context, recipe string, actor contextop.Actor, texts map[string][]string) {
	if actor.Kind != contextop.ActorAgent || len(texts) == 0 {
		return
	}
	s, err := a.contextOps(ctx, recipe)
	if err != nil {
		return
	}
	records, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return
	}
	usage := newContextUsage(records, s.key)
	if len(usage.byTerm) == 0 {
		return
	}
	files := make([]string, 0, len(texts))
	for file := range texts {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		rel := file
		if filepath.IsAbs(file) {
			rel = relSlash(s.root, file)
		}
		_, point := s.basisAt([]contextop.Evidence{{Path: rel}})
		for _, key := range slices.Sorted(maps.Keys(usage.byTerm)) {
			r := usage.byTerm[key]
			rule, _ := r.Rule()
			if !r.Scope.Covers(point.Coordinates) {
				continue
			}
			use := formMatcher([]string{rule.Replacement}, rule.MatchesCase())
			n := 0
			for _, text := range texts[file] {
				n += countUses(text, use)
			}
			if n == 0 {
				continue
			}
			if _, err := s.ledger.Append(ctx, contextop.Record{
				Actor:   applyActor,
				Kind:    contextop.KindSignal,
				Target:  r.ID,
				Project: r.Project,
				Signal: &contextop.Signal{
					Source: contextop.SignalApplied, Preferred: n, Within: rel, Session: actor.Session,
				},
			}); err != nil {
				return
			}
		}
	}
}
