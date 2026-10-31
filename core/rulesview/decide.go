package rulesview

import (
	"iter"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
)

// The questions `kapi rules` asks a person.
//
// Each suggestion is one question with the evidence as its options: each form
// of a name, with the blocks and files that write it, or each side of a
// disagreement. Beside each option is what keeping it does: the blocks a check
// then reports, and where the rule applies. Questions builds the questions
// from a digest; a Counter fills in the counts from the blocks of the
// project's source, matched with FormMatcher, the matcher a check's usage
// count uses.

// The kinds of decision a person is asked about.
const (
	DecisionName         = "name"
	DecisionWord         = "word"
	DecisionNote         = "note"
	DecisionMemory       = "memory"
	DecisionDisagreement = "disagreement"
)

// The questions a decision asks.
const (
	questionWhichForm = "Which one do your files use?"
	questionKeep      = "Keep this as a rule?"
	questionWhichSide = "Which one holds?"
)

// Decision is one question.
type Decision struct {
	// ID names the suggestion, and Short is the form a person types. A
	// disagreement has no id of its own: each option carries its side's.
	ID    string `json:"id,omitempty"`
	Short string `json:"short,omitempty"`
	Kind  string `json:"kind"`
	// Question is what the person is asked.
	Question string `json:"question"`
	// Reason says what a disagreement is about.
	Reason string `json:"reason,omitempty"`
	// Sentence is the suggestion as a sentence.
	Sentence string `json:"sentence"`
	// Options are the forms of a name, or the sides of a disagreement.
	// Chosen is the option a keep takes unless the person picks another: the
	// form the suggestion names, or the side the files use most.
	Options []Option `json:"options,omitempty"`
	Chosen  int      `json:"chosen"`
	// Consequence says what keeping it does, for a decision with no options.
	Consequence string `json:"consequence,omitempty"`
	// Seen is where the suggestion was seen.
	Seen *contextop.Evidence `json:"seen,omitempty"`
	// Applies is where a kept rule holds.
	Applies string `json:"applies"`
	// SuggestedBy names who suggested it.
	SuggestedBy string `json:"suggested_by,omitempty"`

	// Subject is what the question is about, Scope where the suggestion
	// holds, and NoticedBy who suggested it: the first side's, for a
	// disagreement.
	Subject   contextop.Subject `json:"-"`
	Scope     contextop.Scope   `json:"-"`
	NoticedBy contextop.Actor   `json:"-"`
}

// Option is one answer to a question.
type Option struct {
	// Form is one wording of a name; ID and Short name one side of a
	// disagreement, and Sentence states it.
	Form     string `json:"form,omitempty"`
	ID       string `json:"id,omitempty"`
	Short    string `json:"short,omitempty"`
	Sentence string `json:"sentence,omitempty"`
	// Blocks and Files count where the project's source writes the form, or
	// the wording the side says to use.
	Blocks int `json:"blocks"`
	Files  int `json:"files"`
	// Consequence says what keeping this option does.
	Consequence string `json:"consequence"`

	// Subject, Scope and NoticedBy are the side's, for a disagreement. An
	// option that is a form of a name carries the question's scope alone.
	Subject   contextop.Subject `json:"-"`
	Scope     contextop.Scope   `json:"-"`
	NoticedBy contextop.Actor   `json:"-"`
}

// Block is one block of the project's source as a count reads it: the file it
// sits in, the coordinates of that file's point, and its source text.
type Block struct {
	Path        string
	Coordinates map[string]string
	Text        string
}

// Questions reads the suggestions waiting in a digest as questions, without
// counts: each disagreement between two or more rules first, then every other
// suggestion a person can keep or drop. records are the log's records as the
// reader sees them, which give each suggestion its scope.
func Questions(d Digest, records []contextop.Record) []Decision {
	byID := make(map[string]contextop.Record, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	out := []Decision{}
	seen := map[string]bool{}
	var items []Item
	for _, c := range d.Conflicts {
		for _, side := range c.Sides {
			seen[side.ID] = true
		}
		if len(c.Sides) > 1 {
			out = append(out, disagreement(c, byID))
			continue
		}
		items = append(items, c.Sides...)
	}
	for _, t := range d.Suggested {
		for _, g := range t.Groups {
			for _, it := range g.Items {
				if !seen[it.ID] && (it.Keepable || it.Droppable) {
					items = append(items, it)
					seen[it.ID] = true
				}
			}
		}
	}
	for _, it := range items {
		out = append(out, suggestion(it, byID[it.ID].Scope))
	}
	return out
}

// NeedsBlocks reports whether any question counts uses of a wording, and so
// whether the blocks of the project's source are worth reading.
func NeedsBlocks(decisions []Decision) bool {
	return slices.ContainsFunc(decisions, func(d Decision) bool {
		return d.Kind == DecisionDisagreement || d.Subject.Kind == contextop.SubjectTerm
	})
}

// disagreement asks which side of a disagreement holds.
func disagreement(c Conflict, byID map[string]contextop.Record) Decision {
	d := Decision{Kind: DecisionDisagreement, Question: questionWhichSide, Reason: c.Reason}
	names := true
	for _, side := range c.Sides {
		r := byID[side.ID]
		if ThemeOf(side.Subject) != ThemeNames {
			names = false
		}
		d.Options = append(d.Options, Option{
			ID: side.ID, Short: side.Short, Sentence: side.Sentence,
			Subject: side.Subject, Scope: r.Scope, NoticedBy: side.NoticedBy,
		})
		if d.Seen == nil && side.Quote != nil {
			d.Seen = side.Quote
		}
		if d.Scope.Level == "" {
			d.Scope, d.Subject, d.NoticedBy = r.Scope, side.Subject, side.NoticedBy
		}
	}
	if names {
		d.Question = questionWhichForm
	}
	return d
}

// suggestion asks about one suggestion at scope: which of its forms the files
// use, for a name, and whether to keep it otherwise.
func suggestion(it Item, scope contextop.Scope) Decision {
	d := Decision{
		ID: it.ID, Short: it.Short, Sentence: it.Sentence, Seen: it.Quote,
		Question: questionKeep, SuggestedBy: ActorPhrase(it.NoticedBy, false),
		Subject: it.Subject, Scope: scope, NoticedBy: it.NoticedBy,
	}
	switch {
	case it.Subject.Kind == contextop.SubjectMemory:
		d.Kind = DecisionMemory
	case it.Subject.Kind == contextop.SubjectNote || it.Subject.Kind == contextop.SubjectNone:
		d.Kind = DecisionNote
	case ThemeOf(it.Subject) == ThemeNames:
		d.Kind, d.Question = DecisionName, questionWhichForm
		rule, _ := it.Subject.Rule()
		for _, form := range ruleForms(rule) {
			d.Options = append(d.Options, Option{Form: form, Scope: scope})
		}
	default:
		d.Kind = DecisionWord
	}
	return d
}

// ruleForms are a name's forms: the one the rule says to write first, then
// the ones it avoids, each once.
func ruleForms(rule profile.TermRule) []string {
	var out []string
	for _, f := range append([]string{rule.Replacement, rule.Term}, rule.Forms...) {
		if f = strings.TrimSpace(f); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// Label is the wording a question is about, for a list that shows each
// question on one line. For a name it is the form the suggestion says to
// write, and for a word rule the wording it avoids, which both sides of a
// disagreement share. A content-memory pair gives its source, and a note its
// prose.
func (d Decision) Label() string {
	if rule, ok := d.Subject.Rule(); ok {
		if d.Kind == DecisionName {
			return rule.Replacement
		}
		return rule.Term
	}
	if m := d.Subject.Memory; m != nil {
		return m.Source
	}
	return d.Subject.Text
}

// Avoided counts the wordings a word rule's check reports: the term and its
// other forms. A question about anything else counts none.
func (d Decision) Avoided() int {
	rule, ok := d.Subject.Rule()
	if !ok || d.Kind == DecisionName {
		return 0
	}
	n := 0
	for _, f := range append([]string{rule.Term}, rule.Forms...) {
		if strings.TrimSpace(f) != "" && f != rule.Replacement {
			n++
		}
	}
	return n
}

// Counter counts uses of a wording in the blocks of a project's source and
// states what keeping a rule does.
type Counter struct {
	// Space is the project's axis space, which says whether a scope covers a
	// block's coordinates.
	Space axis.Space
	// Applies says where a rule at a scope holds, in a person's words ("the
	// whole project", "docs, guides"), and whether that is less than the
	// whole project.
	Applies func(contextop.Scope) (string, bool)
	// Blocks are the blocks of the project's source. Nil counts no use.
	Blocks iter.Seq[Block]
}

// Fill counts each option's uses and states what keeping it does.
func (c Counter) Fill(d *Decision) {
	d.Applies = c.applies(d.Scope)
	switch d.Kind {
	case DecisionName:
		rule, _ := d.Subject.Rule()
		caseSensitive := rule.MatchesCase()
		for i := range d.Options {
			o := &d.Options[i]
			o.Blocks, o.Files = c.Uses(d.Scope, []string{o.Form}, caseSensitive)
			o.Consequence = c.Consequence(*d, o.Form)
		}
	case DecisionDisagreement:
		best := -1
		for i := range d.Options {
			o := &d.Options[i]
			if rule, ok := o.Subject.Rule(); ok && rule.Replacement != "" {
				o.Blocks, o.Files = c.Uses(o.Scope, []string{rule.Replacement}, rule.MatchesCase())
			}
			label := ""
			if rule, ok := o.Subject.Rule(); ok {
				label = rule.Replacement
			}
			o.Consequence = c.keepConsequence(o.Subject, o.Scope, label, ThemeOf(o.Subject) == ThemeNames)
			if best < 0 || o.Blocks > d.Options[best].Blocks {
				best = i
			}
		}
		d.Chosen = max(best, 0)
	default:
		d.Consequence = c.keepConsequence(d.Subject, d.Scope, "", false)
	}
}

// Consequence states what keeping a name with form as the one to write does.
// The form may be one the person typed rather than one of the options.
func (c Counter) Consequence(d Decision, form string) string {
	subject := d.Subject
	if form != "" {
		subject, _ = EditSubject(subject, form, nil)
	}
	return c.keepConsequence(subject, d.Scope, form, true)
}

// applies is where a rule at scope holds, empty when the counter has no way
// to say.
func (c Counter) applies(scope contextop.Scope) string {
	if c.Applies == nil {
		return ""
	}
	where, _ := c.Applies(scope)
	return where
}

// keepConsequence says what keeping subject at scope does: the blocks a check
// then reports, and where it applies. label is the wording the kept rule
// writes, empty to call it "it"; name counts the avoided wordings as the
// other forms of one name rather than listing them.
func (c Counter) keepConsequence(subject contextop.Subject, scope contextop.Scope, label string, name bool) string {
	where := " Applies to " + c.applies(scope) + "."
	switch subject.Kind {
	case contextop.SubjectNote, contextop.SubjectNone:
		return "If you keep it: it is stated to writers and assistants where it applies. No check reads it." + where
	case contextop.SubjectMemory:
		return "If you keep it: the content memory holds this wording." + where
	}
	rule, _ := subject.Rule()
	avoided := slices.DeleteFunc(append([]string{rule.Term}, rule.Forms...), func(f string) bool {
		return strings.TrimSpace(f) == "" || f == rule.Replacement
	})
	blocks, _ := c.Uses(scope, avoided, rule.MatchesCase())
	lead := "If you keep it: "
	if label != "" {
		lead = "If you keep " + strconv.Quote(label) + ": "
	}
	var what string
	switch {
	case name && blocks == 0:
		what = "no block uses " + otherForms(len(avoided)) + "."
	case name:
		what = otherForms(len(avoided)) + " " + become(len(avoided)) + " in " + BlockCount(blocks) + "."
	case blocks == 0:
		what = "no block uses " + quotedAlternatives(avoided) + "."
	default:
		what = quotedAlternatives(avoided) + " " + become(len(avoided)) + " in " + BlockCount(blocks) + "."
	}
	if rule.Advisory {
		what += " They report and fail nothing."
	}
	return lead + what + where
}

// Uses counts the blocks at scope that write any of forms, and the files they
// sit in.
func (c Counter) Uses(scope contextop.Scope, forms []string, caseSensitive bool) (blocks, files int) {
	re := FormMatcher(forms, caseSensitive)
	if re == nil || c.Blocks == nil {
		return 0, 0
	}
	return countBlocks(c.Blocks, c.Space, scope, re)
}

// countBlocks counts the blocks at scope whose text re matches, and the files
// they sit in.
func countBlocks(blocks iter.Seq[Block], space axis.Space, scope contextop.Scope, re *regexp.Regexp) (int, int) {
	n := 0
	inFiles := map[string]bool{}
	for b := range blocks {
		if !scope.CoversIn(space, b.Coordinates) || !re.MatchString(b.Text) {
			continue
		}
		n++
		inFiles[b.Path] = true
	}
	return n, len(inFiles)
}

// otherForms names the forms a kept name avoids: "the other form", "the other
// 4 forms".
func otherForms(n int) string {
	if n == 1 {
		return "the other form"
	}
	return "the other " + strconv.Itoa(n) + " forms"
}

// become is the verb for n avoided forms turning into findings.
func become(n int) string {
	if n == 1 {
		return "becomes a finding"
	}
	return "become findings"
}

// BlockCount renders a count of blocks: "1 block", "3 blocks".
func BlockCount(n int) string {
	if n == 1 {
		return "1 block"
	}
	return strconv.Itoa(n) + " blocks"
}

// quotedAlternatives lists wordings in double quotes, the way a sentence
// lists alternatives: "a", "a" or "b".
func quotedAlternatives(forms []string) string {
	quoted := make([]string, len(forms))
	for i, f := range forms {
		quoted[i] = `"` + f + `"`
	}
	return OrList(quoted)
}
