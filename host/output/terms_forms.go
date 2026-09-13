package output

import (
	"fmt"
	"io"
	"strings"
)

// TermsRejectedForm is a proposed form a filter dropped, with the reason.
type TermsRejectedForm struct {
	Form   string `json:"form"`
	Reason string `json:"reason"`
}

// TermsExpandEntry is what `kapi terms expand` proposed for one term.
type TermsExpandEntry struct {
	ConceptID string              `json:"concept_id"`
	Locale    string              `json:"locale"`
	Term      string              `json:"term"`
	Forms     []string            `json:"forms"`
	Rejected  []TermsRejectedForm `json:"rejected,omitempty"`
}

// TermsExpandOutput is the result of `kapi terms expand`.
type TermsExpandOutput struct {
	Target  string             `json:"target"`
	DryRun  bool               `json:"dry_run"`
	Terms   []TermsExpandEntry `json:"terms"`
	Changed int                `json:"changed"`
	Written bool               `json:"written"`
}

// FormatText lists each term with the forms proposed for it and what the
// filters dropped, so a short list reads as a decision rather than as a thin
// answer.
func (o TermsExpandOutput) FormatText(w io.Writer) error {
	if len(o.Terms) == 0 {
		fmt.Fprintln(w, "No terms to expand. Terms that already declare forms are skipped; --overwrite asks about them again.")
		return nil
	}
	for _, e := range o.Terms {
		forms := "no forms"
		if len(e.Forms) > 0 {
			forms = "+ " + strings.Join(e.Forms, ", ")
		}
		fmt.Fprintf(w, "  %-8s %-28s %s%s\n", e.Locale, e.Term, forms, rejectedSuffix(e.Rejected))
	}
	switch {
	case o.Changed == 0:
		fmt.Fprintln(w, "\nNothing to write.")
	case o.DryRun:
		fmt.Fprintf(w, "\n%d term(s) would gain forms in %s. Re-run without --dry-run to write them.\n", o.Changed, o.Target)
	case o.Written:
		fmt.Fprintf(w, "\n%d term(s) expanded in %s. Review the diff before committing.\n", o.Changed, o.Target)
	default:
		fmt.Fprintf(w, "\n%s already carries these forms.\n", o.Target)
	}
	return nil
}

func rejectedSuffix(rejected []TermsRejectedForm) string {
	if len(rejected) == 0 {
		return ""
	}
	parts := make([]string, 0, len(rejected))
	for _, r := range rejected {
		parts = append(parts, fmt.Sprintf("%s (%s)", r.Form, r.Reason))
	}
	return "   dropped: " + strings.Join(parts, ", ")
}

// TermsProblem is one finding of `kapi terms validate`.
type TermsProblem struct {
	ConceptID string `json:"concept_id,omitempty"`
	Locale    string `json:"locale,omitempty"`
	Term      string `json:"term,omitempty"`
	Message   string `json:"message"`
	Warning   bool   `json:"warning,omitempty"`
}

// TermsValidateOutput is the result of `kapi terms validate`.
type TermsValidateOutput struct {
	Valid    bool           `json:"valid"`
	Source   string         `json:"source"`
	Concepts int            `json:"concepts"`
	Problems []TermsProblem `json:"problems"`
}

// FormatText renders the verdict, the errors behind it, then the warnings.
func (o TermsValidateOutput) FormatText(w io.Writer) error {
	var errs, warns []TermsProblem
	for _, p := range o.Problems {
		if p.Warning {
			warns = append(warns, p)
		} else {
			errs = append(errs, p)
		}
	}
	if len(errs) > 0 {
		fmt.Fprintf(w, "INVALID  %s: %d problem(s):\n", o.Source, len(errs))
		for _, p := range errs {
			writeTermsProblem(w, p)
		}
	} else {
		fmt.Fprintf(w, "VALID  %s (%d concepts)\n", o.Source, o.Concepts)
	}
	if len(warns) > 0 {
		fmt.Fprintf(w, "%d warning(s):\n", len(warns))
		for _, p := range warns {
			writeTermsProblem(w, p)
		}
	}
	return nil
}

func writeTermsProblem(w io.Writer, p TermsProblem) {
	var where []string
	if p.ConceptID != "" {
		where = append(where, p.ConceptID)
	}
	if p.Locale != "" {
		where = append(where, p.Locale)
	}
	if strings.TrimSpace(p.Term) != "" {
		where = append(where, fmt.Sprintf("%q", p.Term))
	}
	if len(where) == 0 {
		fmt.Fprintf(w, "  - %s\n", p.Message)
		return
	}
	fmt.Fprintf(w, "  - %s: %s\n", strings.Join(where, " "), p.Message)
}
