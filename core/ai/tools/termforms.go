package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/check"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// Expanding a voice profile's vocabulary rules with the shapes each term takes.
//
// This is the authoring-time half of the answer to #2226. A rule names a word,
// prose uses that word inflected, and matching the bare string alone let "the
// platform utilizes your data" past a rule forbidding `utilize`.
//
// Generating the forms was tried first and is the wrong layer. English suffix
// rules over the term string worked for English, produced non-words for
// Norwegian, reached none of the forms Norwegian actually uses, and needed a
// minimum term length to stop `Go` matching inside "going". Morphology is
// per-language knowledge; LanguageTool and Acrolinx carry a linguistic pack per
// language to have it.
//
// A model has that knowledge for every language, and the cost of asking is
// paid once here rather than on every check. What lands in the profile is a
// list a person reads in a diff, and the check that consumes it stays exact,
// deterministic, free and language-neutral.

// MaxFormsPerTerm caps what one rule takes on.
//
// A long tail of rare forms trades precision at check time for violations
// nobody writes. Eight covers a Germanic noun's four cases with definite and
// plural, and an English verb several times over.
const MaxFormsPerTerm = 8

// TermExpansion is what the model proposed for one term.
type TermExpansion struct {
	Term string `json:"term"`
	// Forms are the other shapes, excluding Term itself.
	Forms []string `json:"forms"`
	// Rejected are forms that came back and were dropped, with the reason, so a
	// reviewer sees what was filtered rather than a silently shorter list.
	Rejected []RejectedForm `json:"rejected,omitempty"`
}

// RejectedForm is a proposed form that did not survive the checks below.
type RejectedForm struct {
	Form   string `json:"form"`
	Reason string `json:"reason"`
}

// ExpandTermForms asks a provider for the surface forms of each term, in one
// call, and returns them filtered.
//
// One call rather than one per term: the model reads a coherent vocabulary and
// a run over a profile of twenty rules costs one round trip.
func ExpandTermForms(ctx context.Context, p aiprovider.LLMProvider, terms []string, language string) ([]TermExpansion, error) {
	if p == nil {
		return nil, errors.New("term-forms: nil provider")
	}
	want := dedupeTerms(terms)
	if len(want) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(language) == "" {
		return nil, errors.New("term-forms: no language given, and the forms of a word depend on which one it is")
	}

	pr := prompt.TermForms{Language: language, MaxPerTerm: MaxFormsPerTerm}
	turns := pr.Turns(strings.Join(want, "\n"))
	ctx = prompt.WithID(ctx, prompt.IDTermForms)
	resp, err := p.ChatStructured(ctx, aiprovider.MessagesFromTurns(turns), termFormsSchema())
	if err != nil {
		return nil, fmt.Errorf("term-forms: %w", err)
	}

	var out struct {
		Terms []TermExpansion `json:"terms"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		return nil, fmt.Errorf("term-forms: %w", err)
	}
	return filterExpansions(want, out.Terms), nil
}

// filterExpansions keeps only forms this profile can safely match on.
//
// Every check here exists because a wrong form is worse than a missing one: it
// accuses text that broke no rule, in a gate, and the author never wrote it
// down themselves.
func filterExpansions(want []string, got []TermExpansion) []TermExpansion {
	asked := make(map[string]bool, len(want))
	for _, t := range want {
		asked[strings.ToLower(t)] = true
	}

	out := make([]TermExpansion, 0, len(got))
	for _, e := range got {
		if !asked[strings.ToLower(strings.TrimSpace(e.Term))] {
			// A term nobody asked about. Dropping it is safer than adding rules
			// to a profile from a model's own initiative.
			continue
		}
		kept := TermExpansion{Term: e.Term}
		seen := map[string]bool{strings.ToLower(e.Term): true}
		for _, f := range e.Forms {
			f = strings.TrimSpace(f)
			reason := rejectForm(e.Term, f, seen)
			if reason != "" {
				if f != "" {
					kept.Rejected = append(kept.Rejected, RejectedForm{Form: f, Reason: reason})
				}
				continue
			}
			seen[strings.ToLower(f)] = true
			kept.Forms = append(kept.Forms, f)
			if len(kept.Forms) >= MaxFormsPerTerm {
				break
			}
		}
		out = append(out, kept)
	}
	return out
}

// rejectForm returns why a proposed form cannot be used, or "" to keep it.
func rejectForm(term, form string, seen map[string]bool) string {
	switch {
	case form == "":
		return "empty"
	case seen[strings.ToLower(form)]:
		return "duplicate"
	case check.NewTermMatcher(form).Empty():
		// Nothing the matcher can look for: punctuation, or whitespace only.
		return "no matchable text"
	case strings.ContainsAny(form, "\n\t"):
		return "spans lines"
	case !sharesAPrefix(term, form):
		// A form of the same word starts like it. Anything else is a synonym,
		// a translation, or an invention, and the prompt asked for none of
		// those. This is the check that catches a model answering "harness"
		// for "leverage".
		return "not a form of the term"
	}
	return ""
}

// minSharedPrefix is how much of the term a form must open with.
//
// Three characters is enough to separate an inflection from a synonym while
// leaving room for a stem change: løsning/løsningene share seven, utilize/
// utilizing share six, run/ran share two and are the kind of irregular this
// deliberately does not try to keep.
const minSharedPrefix = 3

func sharesAPrefix(term, form string) bool {
	t := []rune(strings.ToLower(term))
	f := []rune(strings.ToLower(form))
	if len(t) < minSharedPrefix {
		// A term this short gets no forms at all, and the prefix rule cannot
		// help: for `Go` it admits both "gone" and "Golang", one a real form of
		// the verb and the other a different word entirely.
		//
		// Declining is the right way to be wrong here. A term of one or two
		// characters in a voice profile is almost always a product or a
		// technology — Go, AI, UI, R — where every inflection of the ordinary
		// word that shares its spelling is a false accusation. An author who
		// does mean the verb can write the forms themselves, which is one line
		// and unambiguous.
		return false
	}
	n := min(len(f), minSharedPrefix)
	return n == minSharedPrefix && string(t[:n]) == string(f[:n])
}

func dedupeTerms(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

// RuleTerms is every term in a profile's vocabulary, in rule order.
func RuleTerms(p *coreprofile.VoiceProfile) []string {
	if p == nil {
		return nil
	}
	var out []string
	for _, set := range [][]coreprofile.TermRule{
		p.Vocabulary.ForbiddenTerms, p.Vocabulary.CompetitorTerms, p.Vocabulary.PreferredTerms,
	} {
		for _, r := range set {
			out = append(out, r.Term)
		}
	}
	return out
}

func termFormsSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "term_forms",
		Description: "Surface forms of each term in the requested language",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"terms": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"term":  map[string]any{"type": "string"},
							"forms": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required":             []string{"term", "forms"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"terms"},
			"additionalProperties": false,
		},
	}
}
