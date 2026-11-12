package host

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	editdiff "github.com/neokapi/neokapi/core/edit"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/memory"
)

// Fix candidates are the ready-made corrections a reviewer picks from for one
// translation: the draft with the term rules applied, the content memory's
// alternatives for the source, and on request an AI rewrite for a reason the
// reviewer gives. Each is a run sequence a set_content sends as it stands,
// with a word diff against the draft. Kapi Desktop, the CLI and the Bowrain
// server compute them here, so every surface offers the same fixes.

// FixKind says where a fix candidate comes from.
type FixKind string

const (
	// FixTerm is the draft with the replacements its term rules name.
	FixTerm FixKind = "term"
	// FixMemory is a content-memory translation of the same source.
	FixMemory FixKind = "memory"
	// FixAI is a model's rewrite of the draft for a given reason.
	FixAI FixKind = "ai"
)

// FixReason is why a reviewer says a translation is not right.
type FixReason string

const (
	FixReasonWrongTerm   FixReason = "wrong_term"
	FixReasonMeaning     FixReason = "meaning"
	FixReasonTone        FixReason = "tone"
	FixReasonPlaceholder FixReason = "placeholder"
	FixReasonGrammar     FixReason = "grammar"
)

// FixReasons lists every reason, in the order a reason sheet offers them.
var FixReasons = []FixReason{FixReasonWrongTerm, FixReasonMeaning, FixReasonTone, FixReasonPlaceholder, FixReasonGrammar}

// ParseFixReason reads a reason as a request names it. The empty string is no
// reason.
func ParseFixReason(s string) (FixReason, error) {
	r := FixReason(strings.TrimSpace(s))
	if r == "" || slices.Contains(FixReasons, r) {
		return r, nil
	}
	return "", fmt.Errorf("unknown reason %q: want one of wrong_term, meaning, tone, placeholder, grammar", s)
}

// describe is the reason as an instruction to a model reads it.
func (r FixReason) describe() string {
	switch r {
	case FixReasonWrongTerm:
		return "it uses the wrong term; use the terminology the rules give"
	case FixReasonMeaning:
		return "it does not say what the source says; keep the meaning of the source exactly"
	case FixReasonTone:
		return "its tone is wrong; match the voice the guidance describes"
	case FixReasonPlaceholder:
		return "a placeholder or inline code is wrong; keep every placeholder and code of the source, in an order the sentence needs"
	case FixReasonGrammar:
		return "its grammar or spelling is wrong; correct it and change nothing else"
	}
	return ""
}

// FixChange is one substitution a term fix made.
type FixChange struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// FixCandidate is one correction offered for a translation.
type FixCandidate struct {
	Kind FixKind `json:"kind"`
	// Label is the line a surface shows for the candidate.
	Label string `json:"label"`
	// Runs is the corrected translation, the content a set_content sends.
	Runs []model.Run `json:"runs"`
	// Text is Runs as a reader reads it, each code as the data it holds.
	Text string `json:"text"`
	// Diff is a word diff of the draft against Text.
	Diff editdiff.Diff `json:"diff"`
	// Reasons are the reasons the candidate addresses.
	Reasons []FixReason `json:"reasons"`
	// Changes are the substitutions of a term fix.
	Changes []FixChange `json:"changes,omitempty"`
	// Score is a content-memory match's score, from 0 to 100.
	Score *int `json:"score,omitempty"`
}

// DroppedFix is a candidate that was computed and not offered, with the
// reason.
type DroppedFix struct {
	Kind  FixKind `json:"kind"`
	Label string  `json:"label"`
	Why   string  `json:"why"`
}

// FixAIChain builds the tool chain an AI rewrite runs: a one-block translate
// configured with instruction, as the caller resolves its provider and the
// governance at the unit's point. The returned release frees what the
// assembly opened and is always safe to call.
type FixAIChain func(ctx context.Context, instruction string) ([]tool.Tool, func(), error)

// FixCandidatesRequest is one translation unit and what governs it.
type FixCandidatesRequest struct {
	// Block holds the source and the draft. It is read, never changed.
	Block *model.Block
	// Source and Target are the languages of the source and the draft.
	Source, Target model.LocaleID
	// Reason orders the candidates and steers the AI rewrite.
	Reason FixReason
	// TermRules are the word rules in force for the target language at the
	// unit's point. Their replacements make the term fix.
	TermRules []profile.TermRuleSet
	// Memory is the content memory alternatives are looked up in. Nil offers
	// none.
	Memory memory.ContentMemory
	// MemoryProjectID boosts matches from the unit's own project.
	MemoryProjectID string
	// AI, when set, asks for an AI rewrite through the chain it builds.
	AI FixAIChain
	// Findings are the unit's current findings, as lines, which the AI
	// rewrite is asked to resolve.
	Findings []string
	// Instruction is extra guidance for the AI rewrite.
	Instruction string
}

// FixCandidatesResult is what FixCandidates offers.
type FixCandidatesResult struct {
	Candidates []FixCandidate `json:"candidates"`
	Dropped    []DroppedFix   `json:"dropped"`
	// Notes say why a kind of candidate is missing.
	Notes []string `json:"notes"`
}

// memoryAlternatives is how many content-memory matches FixCandidates offers.
const memoryAlternatives = 3

// FixCandidates computes the corrections for one translation: the term fix
// (FixTerm), the content memory's alternatives (FixMemory) and, when req.AI
// is set, an AI rewrite (FixAI). A candidate identical to the draft, or to a
// candidate before it, is left out. Every candidate passes the inline-code
// guard a set_content of it meets in the change service; one that does not
// is listed under Dropped. The candidates are ordered by req.Reason.
func FixCandidates(ctx context.Context, req FixCandidatesRequest) (*FixCandidatesResult, error) {
	if req.Block == nil {
		return nil, errors.New("fix candidates: no block")
	}
	if req.Target == "" {
		return nil, errors.New("fix candidates: no target language")
	}
	draft := req.Block.TargetRuns(req.Target)
	out := &FixCandidatesResult{Candidates: []FixCandidate{}, Dropped: []DroppedFix{}, Notes: []string{}}
	seen := map[string]bool{fixKey(draft): true}
	offer := func(c FixCandidate) {
		key := fixKey(c.Runs)
		if seen[key] {
			return
		}
		if why := fixRefusal(req.Block, req.Target, c.Runs); why != "" {
			out.Dropped = append(out.Dropped, DroppedFix{Kind: c.Kind, Label: c.Label, Why: why})
			return
		}
		seen[key] = true
		c.Text = model.RenderRunsWithData(c.Runs)
		c.Diff = editdiff.Compare(model.RenderRunsWithData(draft), c.Text)
		out.Candidates = append(out.Candidates, c)
	}

	if len(draft) > 0 {
		if runs, changes := TermFix(draft, req.TermRules); len(changes) > 0 {
			offer(FixCandidate{Kind: FixTerm, Label: termFixLabel(changes), Runs: runs,
				Reasons: []FixReason{FixReasonWrongTerm}, Changes: changes})
		}
	}

	if req.Memory != nil {
		lookup := &model.Block{ID: "fix-lookup", Translatable: true, SourceLocale: req.Source}
		lookup.SetSourceRuns(req.Block.SourceRuns())
		opts := memory.DefaultLookupOptions()
		// One more than offered, since a match identical to the draft is left out.
		opts.MaxResults = memoryAlternatives + 1
		opts.ProjectID = req.MemoryProjectID
		matches, err := req.Memory.Lookup(ctx, lookup, req.Source, req.Target, opts)
		if err != nil {
			out.Notes = append(out.Notes, "the content memory could not be read: "+err.Error())
		}
		offered := 0
		for _, m := range matches {
			runs := copyRuns(m.Entry.Variant(req.Target))
			if len(runs) == 0 || offered == memoryAlternatives {
				continue
			}
			score := int(m.Score*100 + 0.5)
			label := fmt.Sprintf("Content memory, %d%% match", score)
			if !model.DiffRunCodes(req.Block.SourceRuns(), runs).Balanced() {
				out.Dropped = append(out.Dropped, DroppedFix{Kind: FixMemory, Label: label, Why: "it drops or adds a placeholder the source holds"})
				continue
			}
			before := len(out.Candidates)
			offer(FixCandidate{Kind: FixMemory, Label: label, Runs: runs, Score: &score,
				Reasons: []FixReason{FixReasonWrongTerm, FixReasonMeaning, FixReasonPlaceholder}})
			if len(out.Candidates) > before {
				offered++
			}
		}
	}

	if req.AI != nil {
		instruction := FixInstruction(model.RenderRunsWithData(draft), req.Reason, req.Findings, req.Instruction)
		chain, release, err := req.AI(ctx, instruction)
		if err != nil {
			out.Notes = append(out.Notes, "no AI rewrite: "+err.Error())
		} else {
			runs, perr := ProposeRewrite(ctx, chain, req.Block, req.Target)
			release()
			switch {
			case perr != nil:
				out.Notes = append(out.Notes, "no AI rewrite: "+perr.Error())
			default:
				reasons := []FixReason{}
				label := "AI rewrite"
				if req.Reason != "" {
					reasons = append(reasons, req.Reason)
					label = "AI rewrite for " + strings.ReplaceAll(string(req.Reason), "_", " ")
				}
				if !model.DiffRunCodes(req.Block.SourceRuns(), runs).Balanced() {
					out.Dropped = append(out.Dropped, DroppedFix{Kind: FixAI, Label: label, Why: "it drops or adds a placeholder the source holds"})
				} else {
					offer(FixCandidate{Kind: FixAI, Label: label, Runs: runs, Reasons: reasons})
				}
			}
		}
	}

	order := fixOrder(req.Reason)
	slices.SortStableFunc(out.Candidates, func(a, b FixCandidate) int {
		return slices.Index(order, a.Kind) - slices.Index(order, b.Kind)
	})
	return out, nil
}

// fixOrder is the order candidate kinds are offered in for a reason.
func fixOrder(r FixReason) []FixKind {
	switch r {
	case FixReasonMeaning, FixReasonPlaceholder:
		return []FixKind{FixMemory, FixAI, FixTerm}
	case FixReasonTone, FixReasonGrammar:
		return []FixKind{FixAI, FixMemory, FixTerm}
	}
	return []FixKind{FixTerm, FixMemory, FixAI}
}

// fixKey identifies a run sequence for the duplicate test.
func fixKey(runs []model.Run) string { return string(model.CanonicalRunsJSON(runs)) }

// termFixLabel names the substitutions a term fix made.
func termFixLabel(changes []FixChange) string {
	if len(changes) == 1 {
		return fmt.Sprintf("Use %q for %q", changes[0].To, changes[0].From)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		parts[i] = fmt.Sprintf("%q for %q", c.To, c.From)
	}
	return "Use " + strings.Join(parts, ", ")
}

// termFixPasses bounds the substitutions a term fix makes, so rules whose
// replacements match one another cannot keep rewriting the text.
const termFixPasses = 16

// TermFix applies the replacements of the term rules sets hold to runs, one
// at a time, through the fix `kapi fix` applies (check.Fix) and the change
// service's in-memory apply, so a replacement keeps every inline code and
// leaves words that hold a code alone. It returns the result and the
// substitutions; no substitution leaves runs as they were.
//
// Every rule with a replacement applies, an advisory one included: a
// reviewer asked for a fix. A suggested rule, which nobody has established,
// does not.
func TermFix(runs []model.Run, sets []profile.TermRuleSet) ([]model.Run, []FixChange) {
	if len(sets) == 0 || len(runs) == 0 {
		return runs, nil
	}
	work := &model.Block{ID: "term-fix", Translatable: true}
	work.SetSourceRuns(copyRuns(runs))
	var changes []FixChange
	skip := map[string]bool{}
	for range termFixPasses {
		cur := work.SourceRuns()
		text := model.RunsText(cur)
		findings := profile.HitsToFindings(profile.MatchTermRulesInRuns(sets, text, cur), text, cur)
		applied := false
		for _, f := range findings {
			if f.Suggested || skip[f.OriginalText+"\x00"+f.Metadata["replacement"]] {
				continue
			}
			rev := model.EditionRevision(work, model.EditionKey{})
			op := check.Fix(f, cur, change.Ref{Block: work.ID}, rev)
			if op == nil {
				continue
			}
			res := change.ApplyBlock(work, []change.Op{*op}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}})
			if len(res) != 1 || res[0].Status != change.OpApplied {
				skip[f.OriginalText+"\x00"+f.Metadata["replacement"]] = true
				continue
			}
			changes = addFixChange(changes, f.OriginalText, f.Metadata["replacement"])
			applied = true
			break
		}
		if !applied {
			break
		}
	}
	if len(changes) == 0 {
		return runs, nil
	}
	return work.SourceRuns(), changes
}

func addFixChange(changes []FixChange, from, to string) []FixChange {
	for i := range changes {
		if changes[i].From == from && changes[i].To == to {
			changes[i].Count++
			return changes
		}
	}
	return append(changes, FixChange{From: from, To: to, Count: 1})
}

// fixRefusal is why a set_content of runs on b's target edition would be
// refused by the change service's guards, or "" when it would land. It
// applies the operation to a copy of b in preview.
func fixRefusal(b *model.Block, target model.LocaleID, runs []model.Run) string {
	work := snapshotBlock(b)
	key := model.EditionKey{Locale: target}
	op := change.Op{
		Kind:    change.KindSetContent,
		At:      change.Ref{Block: b.ID, Edition: key},
		IfMatch: model.EditionRevision(work, key),
		Body:    &change.SetContent{Content: change.Content{Runs: copyRuns(runs)}},
	}
	res := change.ApplyBlock(work, []change.Op{op}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}, Preview: true})
	if len(res) == 1 && res[0].Error != nil {
		if res[0].Error.Code == change.CodeGuard {
			return "it would break a placeholder or inline code: " + res[0].Error.Message
		}
		return res[0].Error.Message
	}
	return ""
}

// FixInstruction is the translate instruction for a corrected translation:
// the current translation, the reviewer's reason, the findings to resolve,
// and any extra guidance. Kapi Desktop's "Fix with AI" and the fix sheet's AI
// rewrite send it.
func FixInstruction(current string, reason FixReason, findings []string, extra string) string {
	var sb strings.Builder
	sb.WriteString("Produce a corrected translation. The current translation is:\n")
	sb.WriteString(current)
	if d := reason.describe(); d != "" {
		sb.WriteString("\n\nThe reviewer says " + d + ".\n")
	}
	if len(findings) > 0 {
		sb.WriteString("\n\nIt has these review findings; resolve every one of them while preserving the meaning of the source:\n")
		for _, f := range findings {
			sb.WriteString("- " + f + "\n")
		}
	} else if reason == "" {
		sb.WriteString("\n\nImprove its accuracy and fluency while preserving the meaning of the source.\n")
	}
	if strings.TrimSpace(extra) != "" {
		sb.WriteString("\nAdditional guidance: " + extra)
	}
	return sb.String()
}

// ProposeRewrite runs chain (a one-block translate and the steps around it)
// over a copy of b and returns the translation it produced for target. b is
// left as it was. An empty translation is an error.
func ProposeRewrite(ctx context.Context, chain []tool.Tool, b *model.Block, target model.LocaleID) ([]model.Run, error) {
	work := snapshotBlock(b)
	work.Annotations = maps.Clone(b.Annotations)
	if err := RunTools(ctx, chain, work); err != nil {
		return nil, fmt.Errorf("ai rewrite: %w", err)
	}
	runs := work.TargetRuns(target)
	if strings.TrimSpace(model.RunsText(runs)) == "" && !model.RunsHaveInlineCodes(runs) {
		return nil, errors.New("the model returned an empty translation. Try a more specific instruction")
	}
	return runs, nil
}
