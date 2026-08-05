package output

import (
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/occurrence"
)

// TermsOccurrencesOutput is what `kapi terms occurrences` reports: every use of
// a term in the project's blocks, with where each one sits.
type TermsOccurrencesOutput struct {
	// Subject is the term or concept as asked for.
	Subject string `json:"subject"`
	// ConceptIDs are the concepts the subject resolved to.
	ConceptIDs []string `json:"concept_ids,omitempty"`
	// Terms are the term texts searched for.
	Terms []string `json:"terms,omitempty"`
	// Occurrences are the uses found, ordered by document and position.
	Occurrences []occurrence.Occurrence `json:"occurrences"`
	// Total is how many uses were found, before any limit.
	Total int `json:"total"`
	// Blocks is how many distinct blocks they fall in.
	Blocks int `json:"blocks"`
	// Truncated reports that a limit cut the list short.
	Truncated bool `json:"truncated,omitempty"`
}

// NewTermsOccurrencesOutput projects a query result for printing.
func NewTermsOccurrencesOutput(res *occurrence.Result) TermsOccurrencesOutput {
	if res == nil {
		return TermsOccurrencesOutput{}
	}
	return TermsOccurrencesOutput{
		Subject:     res.Subject,
		ConceptIDs:  res.ConceptIDs,
		Terms:       res.Terms,
		Occurrences: res.Occurrences,
		Total:       res.Total,
		Blocks:      res.Blocks,
		Truncated:   res.Truncated,
	}
}

func (o TermsOccurrencesOutput) FormatText(w io.Writer) error {
	if len(o.Occurrences) == 0 {
		// The term exists — an unknown one is an error, not this — so say what
		// is true: nothing in the extracted content uses it.
		fmt.Fprintf(w, "No block uses %q.\n", o.Subject)
		if len(o.ConceptIDs) > 0 {
			Note(w, "Concept: %s", joinAll(o.ConceptIDs))
		}
		return nil
	}

	t := NewTable(w).Accent(0).Headers(
		T("occurrences.header.document"), T("occurrences.header.block"),
		T("occurrences.header.locale"), T("occurrences.header.term"),
		T("occurrences.header.text"))
	s := t.Styles()
	for _, occ := range o.Occurrences {
		t.Row(
			s.Dim(occ.Document),
			s.Dim(occ.BlockID),
			localeLabel(occ.Locale),
			occ.Term,
			s.Text(occ.Snippet),
		)
	}
	t.Render()

	fmt.Fprintln(w)
	Note(w, "%d use(s) in %d block(s)", o.Total, o.Blocks)
	if o.Truncated {
		Note(w, "Showing %d. Use --limit to see more.", len(o.Occurrences))
	}
	return nil
}

// localeLabel names the language a use was found in. The source text carries no
// locale of its own — a block cache holds no opinion about which language it is
// in — so it is labelled for what it is.
func localeLabel(locale string) string {
	if locale == "" {
		return "source"
	}
	return locale
}

func joinAll(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
