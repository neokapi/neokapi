package contextop

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// WidenedKind is the kind this package widens rules under, in the workspace's
// rule store. A rule widened by anything else carries a kind of its own and is
// not read here.
const WidenedKind = "contextop.rule"

// RuleStore is where rules widened to the whole workspace live.
// *workspace.Workspace satisfies it.
type RuleStore interface {
	WidenRule(ctx context.Context, rule workspace.Rule) error
	WidenedRules(ctx context.Context, kind string) ([]workspace.Rule, error)
	NarrowRule(ctx context.Context, id string) error
}

// WidenedRule is one rule in force across the workspace, as the workspace holds
// it.
type WidenedRule struct {
	// Operation is the id of the operation whose rule this is, which is also
	// the rule's id in the workspace. Narrowing it takes the rule back out.
	Operation string `json:"operation"`
	// Subject is the rule itself.
	Subject Subject `json:"subject"`
	// Scope is how far it reaches and at which coordinates.
	Scope Scope `json:"scope"`
	// Project is where the evidence was seen, kept so provenance survives
	// widening.
	Project workspace.ProjectKey `json:"project,omitempty"`
	// At is when it was widened.
	At time.Time `json:"at"`
}

// Widen puts an established rule in force across the workspace.
//
// A project-scoped rule lives in the project's own terms store or voice
// profile, where every reader already looks. A widened one has no project to
// live in, so it lives beside the workspace registry and is read back beneath
// whatever the project itself says.
func Widen(ctx context.Context, store RuleStore, r Record) error {
	if _, ok := r.Rule(); !ok {
		return fmt.Errorf("contextop: operation %s states no rule to widen", r.ID)
	}
	body, err := json.Marshal(WidenedRule{
		Operation: r.ID,
		Subject:   r.Subject,
		Scope:     r.Scope,
		Project:   r.Project,
		At:        r.At,
	})
	if err != nil {
		return fmt.Errorf("contextop: widen %s: %w", r.ID, err)
	}
	return store.WidenRule(ctx, workspace.Rule{
		ID:      widenedID(r.Project, r.ID),
		Kind:    WidenedKind,
		Origin:  r.Project,
		Payload: body,
		At:      r.At,
	})
}

// Narrow takes a widened rule back out of force across the workspace. It is
// what dropping or reverting a widened rule does, and narrowing one the
// workspace does not hold is not an error.
func Narrow(ctx context.Context, store RuleStore, project workspace.ProjectKey, id string) error {
	return store.NarrowRule(ctx, widenedID(project, id))
}

// WidenedRules reads back every rule in force across the workspace.
func WidenedRules(ctx context.Context, store RuleStore) ([]WidenedRule, error) {
	held, err := store.WidenedRules(ctx, WidenedKind)
	if err != nil {
		return nil, err
	}
	out := make([]WidenedRule, 0, len(held))
	for _, rule := range held {
		var w WidenedRule
		if err := json.Unmarshal(rule.Payload, &w); err != nil {
			return nil, fmt.Errorf("contextop: read widened rule %s: %w", rule.ID, err)
		}
		if w.At.IsZero() {
			w.At = rule.At
		}
		if w.Project == "" {
			w.Project = rule.Origin
		}
		out = append(out, w)
	}
	return out, nil
}

// widenedID addresses a widened rule in the workspace. Two projects that
// widened their own operation 7 are two rules, so the project goes in the key.
func widenedID(project workspace.ProjectKey, id string) string {
	return string(project) + "\x00" + id
}

// ResolveRequest names the point a resolution answers for.
type ResolveRequest struct {
	// Project is the project being checked. An operation recorded in another
	// project answers here only if it was widened to the workspace.
	Project workspace.ProjectKey
	// Coordinates are the axes of the point (project.MergeCoordinates). A rule
	// scoped to coordinates answers only where they match.
	Coordinates map[string]string
	// Held are the terms the project's own stores already answer for: its terms
	// store and its voice profile. A workspace-wide rule about one of them is
	// left out, because the more specific rule wins.
	Held []string
}

// Resolution is what the context operations add to the vocabulary a project's
// own stores already carry.
type Resolution struct {
	// Binding are workspace-wide established rules. They are in force at the
	// severity each one carries, the same as a rule in the project's own store.
	Binding []profile.TermRule
	// Advisory are the suggestions and the contested rules: advice nobody has
	// established, or that disagrees with another rule. They
	// are reported and can never fail a check.
	Advisory []profile.TermRule
}

// Empty reports that the resolution adds nothing.
func (r Resolution) Empty() bool { return len(r.Binding) == 0 && len(r.Advisory) == 0 }

// RuleSets projects a resolution into the rule sets the vocabulary matcher
// takes, binding rules first.
//
// Both sets are forbidden-term sets: a rule says a word should be written
// another way, which is what a forbidden term with a replacement says. The
// advisory set carries the marker that holds its hits at neutral severity, so a
// suggestion is reported everywhere a rule would be and fails nothing.
func (r Resolution) RuleSets() []profile.TermRuleSet {
	var sets []profile.TermRuleSet
	if len(r.Binding) > 0 {
		sets = append(sets, profile.TermRuleSet{Rules: r.Binding, Kind: profile.VocabForbidden})
	}
	if len(r.Advisory) > 0 {
		sets = append(sets, profile.TermRuleSet{Rules: r.Advisory, Kind: profile.VocabForbidden, Suggested: true})
	}
	return sets
}

// Resolve answers what the operation log and the workspace's widened rules say
// at one point.
//
// The ladder runs from the most specific source to the least: what the
// project's own stores hold hides a workspace-wide rule about the same term,
// and a suggestion recorded in this project hides a workspace-wide suggestion
// about it. Within one level the newest operation about a term answers, because
// a later suggestion is a revision of an earlier one.
func Resolve(records []Record, widened []WidenedRule, req ResolveRequest) Resolution {
	held := map[string]bool{}
	for _, term := range req.Held {
		if key := termKey(term); key != "" {
			held[key] = true
		}
	}

	var out Resolution

	// Workspace-wide established rules, beneath whatever the project itself says.
	seen := map[string]bool{}
	binding := make([]profile.TermRule, 0, len(widened))
	for _, w := range widened {
		rule, ok := w.Subject.Rule()
		if !ok || !w.Scope.Covers(req.Coordinates) {
			continue
		}
		key := termKey(rule.Term)
		if key == "" || held[key] || seen[key] {
			continue
		}
		seen[key] = true
		binding = append(binding, rule)
	}
	sortRules(binding)
	out.Binding = binding

	// Suggestions. Records arrive newest first from Ledger.Records, so the first
	// answer about a term is the latest revision of it.
	advisory := make([]profile.TermRule, 0, len(records))
	advised := map[string]bool{}
	for _, r := range order(records) {
		if !r.Status.Advises() || !r.Kind.Bears() {
			continue
		}
		if !answersIn(r, req.Project) || !r.Scope.Covers(req.Coordinates) {
			continue
		}
		rule, ok := r.Rule()
		if !ok {
			continue
		}
		key := termKey(rule.Term)
		if key == "" || advised[key] {
			continue
		}
		advised[key] = true
		advisory = append(advisory, rule)
	}
	sortRules(advisory)
	out.Advisory = advisory
	return out
}

// answersIn reports whether an operation's rule reaches a project: its own
// always, and another's only once widened to the workspace.
func answersIn(r Record, project workspace.ProjectKey) bool {
	if r.Scope.Level == LevelWorkspace {
		return true
	}
	return r.Project == project
}

// order returns the records newest first, whatever order they arrived in, so
// the latest statement about a term is the one that answers.
func order(records []Record) []Record {
	out := make([]Record, len(records))
	copy(out, records)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq > out[j].Seq })
	return out
}

// sortRules orders rules by term so a resolution reads the same twice.
func sortRules(rules []profile.TermRule) {
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Term < rules[j].Term })
}

// termKey folds a term for comparison, the way the vocabulary matcher folds it.
func termKey(term string) string { return strings.ToLower(strings.TrimSpace(term)) }
