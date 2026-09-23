package host

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
)

// FormatText renders one operation as the line a person reads.
func (o ContextOperation) FormatText(w io.Writer) error {
	if _, err := fmt.Fprintln(w, o.line()); err != nil {
		return err
	}
	if o.Landed != "" {
		if _, err := fmt.Fprintf(w, "  written to %s\n", o.Landed); err != nil {
			return err
		}
	}
	return nil
}

// line is one operation on one line: its id, when it happened, who did it, what
// they did, what it was about and where it stands. The operation's kind is
// named once: a term rule reads `observe term "Quickcast", not "Quick cast"`,
// and a note reads `observe "the docs address the reader as you"`.
func (o ContextOperation) line() string {
	parts := []string{
		"#" + o.ID,
		o.At.Format(time.RFC3339),
		o.Actor.String(),
		string(o.Kind),
	}
	switch o.Subject.Kind {
	case contextop.SubjectNone:
	case contextop.SubjectNote:
		parts = append(parts, o.Subject.Phrase())
	default:
		parts = append(parts, o.Subject.Describe())
	}
	if o.Correction != nil {
		parts = append(parts, fmt.Sprintf("%q became %q", o.Correction.From, o.Correction.To))
	}
	if o.Target != "" {
		parts = append(parts, "of #"+o.Target)
	}
	if o.TargetSession != "" {
		parts = append(parts, "of session "+o.TargetSession)
	}
	if o.Kind.Bears() {
		status := string(o.Status)
		if len(o.ContestedBy) > 0 {
			others := make([]string, len(o.ContestedBy))
			for i, id := range o.ContestedBy {
				others[i] = "#" + id
			}
			status += " by " + strings.Join(others, ", ")
		}
		parts = append(parts, "["+status+"]")
	}
	return strings.Join(parts, "  ")
}

// FormatText renders a context history.
func (l ContextOperationList) FormatText(w io.Writer) error {
	if len(l.Operations) == 0 {
		_, err := fmt.Fprintln(w, "No context operations are recorded here yet.")
		return err
	}
	for _, op := range l.Operations {
		if _, err := fmt.Fprintln(w, op.line()); err != nil {
			return err
		}
		for _, e := range op.Evidence {
			if _, err := fmt.Fprintln(w, "  seen in "+describeEvidence(e)); err != nil {
				return err
			}
		}
		if op.Note != "" {
			if _, err := fmt.Fprintln(w, "  "+op.Note); err != nil {
				return err
			}
		}
	}
	return nil
}

// describeEvidence renders one piece of evidence: where it was seen and what it
// said there.
func describeEvidence(e contextop.Evidence) string {
	where := e.Path
	if e.Unit != "" {
		if where != "" {
			where += " "
		}
		where += e.Unit
	}
	if where == "" {
		where = "an unnamed place"
	}
	if e.Quote != "" {
		return fmt.Sprintf("%s: %q", where, e.Quote)
	}
	return where
}

// FormatText renders what a revert undid.
func (r ContextRevertResult) FormatText(w io.Writer) error {
	what := fmt.Sprintf("%d operation(s)", len(r.Reverted))
	if r.Session != "" {
		what = fmt.Sprintf("%s recorded by session %s", what, r.Session)
	}
	if _, err := fmt.Fprintf(w, "Reverted %s.\n", what); err != nil {
		return err
	}
	for _, op := range r.Reverted {
		if _, err := fmt.Fprintln(w, "  "+op.line()); err != nil {
			return err
		}
	}
	for _, retracted := range r.Retracted {
		if _, err := fmt.Fprintf(w, "  taken back out of %s\n", retracted); err != nil {
			return err
		}
	}
	return nil
}

// FormatText renders what a keep did: one line per rule kept, where it was
// written, and each suggestion left for a person to choose about.
func (r ContextKeepResult) FormatText(w io.Writer) error {
	if len(r.Kept) == 0 && len(r.Skipped) == 0 {
		_, err := fmt.Fprintln(w, "Nothing was waiting to be kept.")
		return err
	}
	for _, op := range r.Kept {
		if err := op.FormatText(w); err != nil {
			return err
		}
	}
	for _, skip := range r.Skipped {
		if _, err := fmt.Fprintf(w, "Left #%s: %s. Choose between them before keeping it.\n", skip.ID, skip.Reason); err != nil {
			return err
		}
	}
	return nil
}
