package output

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/neokapi/neokapi/core/graph"
)

// TermbaseRelationEntry represents a single concept relation.
type TermbaseRelationEntry struct {
	ID           string          `json:"id"`
	SourceID     string          `json:"source_id"`
	TargetID     string          `json:"target_id"`
	RelationType string          `json:"relation_type"`
	Note         string          `json:"note,omitempty"`
	Validity     *graph.Validity `json:"validity,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// TermbaseRelateOutput represents the result of adding a relation.
type TermbaseRelateOutput struct {
	Relation TermbaseRelationEntry `json:"relation"`
	DBPath   string                `json:"db_path"`
}

func (o TermbaseRelateOutput) FormatText(w io.Writer) error {
	r := o.Relation
	fmt.Fprintf(w, "Added relation %s: %s -[%s]-> %s\n", r.ID, r.SourceID, r.RelationType, r.TargetID)
	if v := FormatValidity(r.Validity); v != "" {
		fmt.Fprintf(w, "  Validity: %s\n", v)
	}
	if r.Note != "" {
		fmt.Fprintf(w, "  Note:     %s\n", r.Note)
	}
	return nil
}

// TermbaseRelationsOutput represents a listing of concept relations.
type TermbaseRelationsOutput struct {
	Relations []TermbaseRelationEntry `json:"relations"`
	Total     int                     `json:"total"`
}

func (o TermbaseRelationsOutput) FormatText(w io.Writer) error {
	if len(o.Relations) == 0 {
		fmt.Fprintln(w, "No relations found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  ID\tSOURCE\tRELATION\tTARGET\tVALIDITY\tNOTE\n")
	fmt.Fprintf(tw, "  --\t------\t--------\t------\t--------\t----\n")
	for _, r := range o.Relations {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.SourceID, r.RelationType, r.TargetID, FormatValidity(r.Validity), r.Note)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nTotal: %d relation(s)\n", o.Total)
	return nil
}

// TermbaseUnrelateOutput represents the result of removing a relation.
type TermbaseUnrelateOutput struct {
	RelationID string `json:"relation_id"`
	DBPath     string `json:"db_path"`
}

func (o TermbaseUnrelateOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Removed relation %s from %s\n", o.RelationID, o.DBPath)
	return nil
}

// TermbaseShowTerm represents one term of a concept in the show view.
type TermbaseShowTerm struct {
	Text         string          `json:"text"`
	Locale       string          `json:"locale"`
	Status       string          `json:"status,omitempty"`
	PartOfSpeech string          `json:"part_of_speech,omitempty"`
	Note         string          `json:"note,omitempty"`
	Validity     *graph.Validity `json:"validity,omitempty"`
}

// TermbaseShowRelation represents one relation of a concept in the show view,
// annotated with its direction and the other concept's display term.
type TermbaseShowRelation struct {
	ID           string          `json:"id"`
	Direction    string          `json:"direction"` // "outgoing" or "incoming"
	RelationType string          `json:"relation_type"`
	ConceptID    string          `json:"concept_id"`             // the concept on the other side
	ConceptTerm  string          `json:"concept_term,omitempty"` // its display term, when resolvable
	Note         string          `json:"note,omitempty"`
	Validity     *graph.Validity `json:"validity,omitempty"`
}

// TermbaseShowOutput represents the full detail of one concept: definition,
// terms per locale with status and validity, and relations in both directions.
type TermbaseShowOutput struct {
	ID         string                 `json:"id"`
	Domain     string                 `json:"domain,omitempty"`
	Definition string                 `json:"definition,omitempty"`
	Source     string                 `json:"source,omitempty"`
	Properties map[string]string      `json:"properties,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Terms      []TermbaseShowTerm     `json:"terms"`
	Relations  []TermbaseShowRelation `json:"relations"`
}

func (o TermbaseShowOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Concept: %s\n", o.ID)
	if o.Domain != "" {
		fmt.Fprintf(w, "  Domain:      %s\n", o.Domain)
	}
	if o.Definition != "" {
		fmt.Fprintf(w, "  Definition:  %s\n", o.Definition)
	}
	if o.Source != "" {
		fmt.Fprintf(w, "  Source:      %s\n", o.Source)
	}
	if len(o.Properties) > 0 {
		keys := make([]string, 0, len(o.Properties))
		for k := range o.Properties {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		fmt.Fprintln(w, "  Properties:")
		for _, k := range keys {
			fmt.Fprintf(w, "    %s: %s\n", k, o.Properties[k])
		}
	}

	if len(o.Terms) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Terms:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "    LOCALE\tTERM\tSTATUS\tVALIDITY\tNOTE\n")
		fmt.Fprintf(tw, "    ------\t----\t------\t--------\t----\n")
		for _, t := range o.Terms {
			fmt.Fprintf(tw, "    %s\t%s\t%s\t%s\t%s\n",
				t.Locale, t.Text, t.Status, FormatValidity(t.Validity), t.Note)
		}
		tw.Flush()
	}

	if len(o.Relations) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Relations:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, r := range o.Relations {
			other := r.ConceptID
			if r.ConceptTerm != "" {
				other = fmt.Sprintf("%s (%s)", r.ConceptID, r.ConceptTerm)
			}
			edge := fmt.Sprintf("-[%s]->", r.RelationType)
			if r.Direction == "incoming" {
				edge = fmt.Sprintf("<-[%s]-", r.RelationType)
			}
			fmt.Fprintf(tw, "    %s\t%s\t%s\t%s\t%s\n",
				edge, other, r.ID, FormatValidity(r.Validity), r.Note)
		}
		tw.Flush()
	}
	return nil
}

// FormatValidity renders a validity as a compact human-readable string, e.g.
// "from 2026-01-01 to 2026-06-01; market=dach". A nil validity (always valid)
// renders as the empty string.
func FormatValidity(v *graph.Validity) string {
	if v == nil {
		return ""
	}
	var parts []string
	var interval []string
	if v.ValidFrom != nil {
		interval = append(interval, "from "+formatValidityTime(*v.ValidFrom))
	}
	if v.ValidTo != nil {
		interval = append(interval, "to "+formatValidityTime(*v.ValidTo))
	}
	if len(interval) > 0 {
		parts = append(parts, strings.Join(interval, " "))
	}
	if len(v.Tags) > 0 {
		tags := make([]string, 0, len(v.Tags))
		for k := range v.Tags {
			tags = append(tags, k)
		}
		slices.Sort(tags)
		for i, k := range tags {
			tags[i] = k + "=" + v.Tags[k]
		}
		parts = append(parts, strings.Join(tags, " "))
	}
	return strings.Join(parts, "; ")
}

// formatValidityTime renders midnight-UTC instants as plain dates and
// everything else as RFC3339, matching how the bounds are usually entered.
func formatValidityTime(t time.Time) string {
	u := t.UTC()
	if u.Equal(u.Truncate(24 * time.Hour)) {
		return u.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}
