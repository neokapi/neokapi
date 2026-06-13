package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// This file holds the text/JSON output types for the brand knowledge-graph
// commands (`kapi concepts`, `kapi experiments`, `kapi terms pull`) that read
// the governed vocabulary through the bowrain plugin (Bowrain AD-021).

// ---------------------------------------------------------------------------
// concepts
// ---------------------------------------------------------------------------

// ConceptTerm is a single term within a concept, in one locale.
type ConceptTerm struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
	Status string `json:"status,omitempty"`
}

// ConceptListEntry is one concept row in `kapi concepts list`.
type ConceptListEntry struct {
	ID         string        `json:"id"`
	Domain     string        `json:"domain,omitempty"`
	Definition string        `json:"definition,omitempty"`
	Terms      []ConceptTerm `json:"terms,omitempty"`
}

// ConceptListOutput is the result of `kapi concepts list`.
type ConceptListOutput struct {
	Concepts   []ConceptListEntry `json:"concepts"`
	Shown      int                `json:"shown"`
	TotalCount int                `json:"total_count"`
}

func (o ConceptListOutput) FormatText(w io.Writer) error {
	if len(o.Concepts) == 0 {
		fmt.Fprintln(w, "No concepts found.")
		return nil
	}

	idW := 7  // "CONCEPT"
	domW := 6 // "DOMAIN"
	for _, c := range o.Concepts {
		if len(c.ID) > idW {
			idW = len(c.ID)
		}
		if len(c.Domain) > domW {
			domW = len(c.Domain)
		}
	}
	idW += 2
	domW += 2

	fmt.Fprintf(w, "  %-*s %-*s %s\n", idW, "CONCEPT", domW, "DOMAIN", "TERMS")
	fmt.Fprintf(w, "  %-*s %-*s %s\n", idW, "-------", domW, "------", "-----")
	for _, c := range o.Concepts {
		domain := c.Domain
		if domain == "" {
			domain = "-"
		}
		fmt.Fprintf(w, "  %-*s %-*s %s\n", idW, c.ID, domW, domain, conceptTermSummary(c.Terms, c.Definition))
	}

	if o.TotalCount > o.Shown {
		fmt.Fprintf(w, "\n%d concept(s) shown (of %d total)\n", o.Shown, o.TotalCount)
	} else {
		fmt.Fprintf(w, "\n%d concept(s)\n", o.Shown)
	}
	return nil
}

// conceptTermSummary renders a compact preview of a concept's terms, falling
// back to its definition when no terms are present.
func conceptTermSummary(terms []ConceptTerm, definition string) string {
	if len(terms) == 0 {
		return truncate(definition, 60)
	}
	const max = 4
	parts := make([]string, 0, max)
	for i, t := range terms {
		if i >= max {
			parts = append(parts, fmt.Sprintf("(+%d more)", len(terms)-max))
			break
		}
		parts = append(parts, fmt.Sprintf("%s [%s]", t.Text, t.Locale))
	}
	return strings.Join(parts, ", ")
}

// ConceptRelationEntry is one typed edge touching a concept.
type ConceptRelationEntry struct {
	Type     string `json:"type"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Note     string `json:"note,omitempty"`
}

// ConceptShowOutput is the result of `kapi concepts show <concept-id>`.
type ConceptShowOutput struct {
	ID         string                 `json:"id"`
	ProjectID  string                 `json:"project_id,omitempty"`
	Domain     string                 `json:"domain,omitempty"`
	Definition string                 `json:"definition,omitempty"`
	Properties map[string]string      `json:"properties,omitempty"`
	Terms      []ConceptTerm          `json:"terms,omitempty"`
	Relations  []ConceptRelationEntry `json:"relations,omitempty"`
	CreatedAt  string                 `json:"created_at,omitempty"`
	UpdatedAt  string                 `json:"updated_at,omitempty"`
}

func (o ConceptShowOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Concept: %s\n", o.ID)
	if o.Domain != "" {
		fmt.Fprintf(w, "Domain:  %s\n", o.Domain)
	}
	if o.Definition != "" {
		fmt.Fprintf(w, "Definition: %s\n", o.Definition)
	}

	if len(o.Terms) > 0 {
		fmt.Fprintln(w, "\nTerms:")
		byLocale := map[string][]ConceptTerm{}
		var locales []string
		for _, t := range o.Terms {
			if _, seen := byLocale[t.Locale]; !seen {
				locales = append(locales, t.Locale)
			}
			byLocale[t.Locale] = append(byLocale[t.Locale], t)
		}
		sort.Strings(locales)
		for _, loc := range locales {
			fmt.Fprintf(w, "  %s\n", loc)
			for _, t := range byLocale[loc] {
				if t.Status != "" {
					fmt.Fprintf(w, "    %s (%s)\n", t.Text, t.Status)
				} else {
					fmt.Fprintf(w, "    %s\n", t.Text)
				}
			}
		}
	}

	if len(o.Relations) > 0 {
		fmt.Fprintln(w, "\nRelations:")
		for _, r := range o.Relations {
			other := r.TargetID
			arrow := "->"
			if r.TargetID == o.ID && r.SourceID != "" {
				other = r.SourceID
				arrow = "<-"
			}
			line := fmt.Sprintf("  %s %s %s", r.Type, arrow, other)
			if r.Note != "" {
				line += " — " + r.Note
			}
			fmt.Fprintln(w, line)
		}
	}

	if o.UpdatedAt != "" {
		fmt.Fprintf(w, "\nUpdated: %s\n", o.UpdatedAt)
	}
	return nil
}

// ConceptStoryItem is one event on a concept's timeline.
type ConceptStoryItem struct {
	Kind    string    `json:"kind"`
	At      time.Time `json:"at"`
	Actor   string    `json:"actor,omitempty"`
	Summary string    `json:"summary,omitempty"`
	Ref     string    `json:"ref,omitempty"`
}

// ConceptStoryOutput is the result of `kapi concepts story <concept-id>`.
type ConceptStoryOutput struct {
	ConceptID string             `json:"concept_id"`
	Entries   []ConceptStoryItem `json:"entries"`
}

func (o ConceptStoryOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Story of concept %s\n", o.ConceptID)
	if len(o.Entries) == 0 {
		fmt.Fprintln(w, "  (no recorded events)")
		return nil
	}
	for _, e := range o.Entries {
		when := e.At.Format("2006-01-02 15:04")
		fmt.Fprintf(w, "  %s  [%s]", when, e.Kind)
		if e.Actor != "" {
			fmt.Fprintf(w, " %s", e.Actor)
		}
		if e.Summary != "" {
			fmt.Fprintf(w, " — %s", e.Summary)
		}
		if e.Ref != "" {
			fmt.Fprintf(w, " (%s)", e.Ref)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// ---------------------------------------------------------------------------
// experiments (change-sets)
// ---------------------------------------------------------------------------

// ExperimentEntry is one change-set row in `kapi experiments list`.
type ExperimentEntry struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	MergedAt    *time.Time `json:"merged_at,omitempty"`
}

// ExperimentListOutput is the result of `kapi experiments list`.
type ExperimentListOutput struct {
	Experiments []ExperimentEntry `json:"experiments"`
}

func (o ExperimentListOutput) FormatText(w io.Writer) error {
	if len(o.Experiments) == 0 {
		fmt.Fprintln(w, "No experiments found.")
		return nil
	}

	idW, statusW := 2, 6
	for _, e := range o.Experiments {
		if len(e.ID) > idW {
			idW = len(e.ID)
		}
		if len(e.Status) > statusW {
			statusW = len(e.Status)
		}
	}
	idW += 2
	statusW += 2

	fmt.Fprintf(w, "  %-*s %-*s %-19s %s\n", idW, "ID", statusW, "STATUS", "CREATED", "NAME")
	fmt.Fprintf(w, "  %-*s %-*s %-19s %s\n", idW, "--", statusW, "------", "-------", "----")
	for _, e := range o.Experiments {
		fmt.Fprintf(w, "  %-*s %-*s %-19s %s\n",
			idW, e.ID, statusW, e.Status, e.CreatedAt.Format("2006-01-02 15:04"), e.Name)
	}
	fmt.Fprintf(w, "\n%d experiment(s)\n", len(o.Experiments))
	return nil
}

// ExperimentOp is one ordered operation within a change-set.
type ExperimentOp struct {
	Seq int64  `json:"seq"`
	Op  string `json:"op"`
}

// ExperimentReview is one reviewer's verdict on a change-set.
type ExperimentReview struct {
	Reviewer string `json:"reviewer"`
	Verdict  string `json:"verdict"`
	Comment  string `json:"comment,omitempty"`
}

// ExperimentPilot binds a change-set to a project content stream as a shadow.
type ExperimentPilot struct {
	ProjectID string `json:"project_id"`
	Stream    string `json:"stream"`
}

// ExperimentShowOutput is the result of `kapi experiments show <id>`.
type ExperimentShowOutput struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Status      string             `json:"status"`
	Governed    bool               `json:"governed"`
	CreatedBy   string             `json:"created_by,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	SubmittedAt *time.Time         `json:"submitted_at,omitempty"`
	MergedAt    *time.Time         `json:"merged_at,omitempty"`
	Ops         []ExperimentOp     `json:"ops,omitempty"`
	Reviews     []ExperimentReview `json:"reviews,omitempty"`
	Pilots      []ExperimentPilot  `json:"pilots,omitempty"`
}

func (o ExperimentShowOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Experiment: %s\n", o.ID)
	fmt.Fprintf(w, "Name:    %s\n", o.Name)
	fmt.Fprintf(w, "Status:  %s\n", o.Status)
	if o.Governed {
		fmt.Fprintln(w, "Governed: yes (carries a governed operation)")
	}
	if o.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", o.Description)
	}
	if o.CreatedBy != "" {
		fmt.Fprintf(w, "Created by: %s (%s)\n", o.CreatedBy, o.CreatedAt.Format("2006-01-02 15:04"))
	}
	if o.SubmittedAt != nil {
		fmt.Fprintf(w, "Submitted:  %s\n", o.SubmittedAt.Format("2006-01-02 15:04"))
	}
	if o.MergedAt != nil {
		fmt.Fprintf(w, "Merged:     %s\n", o.MergedAt.Format("2006-01-02 15:04"))
	}

	if len(o.Ops) > 0 {
		fmt.Fprintf(w, "\nOperations (%d):\n", len(o.Ops))
		for _, op := range o.Ops {
			fmt.Fprintf(w, "  %d. %s\n", op.Seq, op.Op)
		}
	}
	if len(o.Reviews) > 0 {
		fmt.Fprintf(w, "\nReviews (%d):\n", len(o.Reviews))
		for _, r := range o.Reviews {
			line := fmt.Sprintf("  %s: %s", r.Reviewer, r.Verdict)
			if r.Comment != "" {
				line += " — " + r.Comment
			}
			fmt.Fprintln(w, line)
		}
	}
	if len(o.Pilots) > 0 {
		fmt.Fprintf(w, "\nPilots (%d):\n", len(o.Pilots))
		for _, p := range o.Pilots {
			fmt.Fprintf(w, "  %s @ %s\n", p.ProjectID, p.Stream)
		}
	}
	return nil
}

// BlastRadiusProject is the per-project slice of a blast-radius report.
type BlastRadiusProject struct {
	ProjectID      string `json:"project_id"`
	ProjectName    string `json:"project_name,omitempty"`
	AffectedBlocks int    `json:"affected_blocks"`
	NewViolations  int    `json:"new_violations"`
	Resolved       int    `json:"resolved"`
	Words          int    `json:"words"`
}

// ExperimentBlastRadiusOutput is the result of
// `kapi experiments blast-radius <id>`.
type ExperimentBlastRadiusOutput struct {
	ChangesetID    string               `json:"changeset_id"`
	TotalBlocks    int                  `json:"total_blocks"`
	AffectedBlocks int                  `json:"affected_blocks"`
	NewViolations  int                  `json:"new_violations"`
	Resolved       int                  `json:"resolved"`
	Words          int                  `json:"words"`
	Projects       []BlastRadiusProject `json:"projects,omitempty"`
}

func (o ExperimentBlastRadiusOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Blast radius of experiment %s\n", o.ChangesetID)
	fmt.Fprintf(w, "  Affected blocks: %d of %d\n", o.AffectedBlocks, o.TotalBlocks)
	fmt.Fprintf(w, "  New violations:  %d\n", o.NewViolations)
	fmt.Fprintf(w, "  Resolved:        %d\n", o.Resolved)
	fmt.Fprintf(w, "  Words affected:  %d\n", o.Words)

	if len(o.Projects) > 0 {
		fmt.Fprintln(w, "\nPer project:")
		nameW := 7 // "PROJECT"
		for _, p := range o.Projects {
			label := projectLabel(p)
			if len(label) > nameW {
				nameW = len(label)
			}
		}
		nameW += 2
		fmt.Fprintf(w, "  %-*s %8s %8s %8s %8s\n", nameW, "PROJECT", "BLOCKS", "NEW", "RESOLVED", "WORDS")
		for _, p := range o.Projects {
			fmt.Fprintf(w, "  %-*s %8d %8d %8d %8d\n",
				nameW, projectLabel(p), p.AffectedBlocks, p.NewViolations, p.Resolved, p.Words)
		}
	}
	return nil
}

func projectLabel(p BlastRadiusProject) string {
	if p.ProjectName != "" {
		return p.ProjectName
	}
	return p.ProjectID
}

// ---------------------------------------------------------------------------
// terms pull
// ---------------------------------------------------------------------------

// TermsPullOutput is the result of `kapi terms pull`.
type TermsPullOutput struct {
	DBPath    string `json:"db_path"`
	Workspace string `json:"workspace,omitempty"`
	Server    string `json:"server,omitempty"`
	Concepts  int    `json:"concepts"`
	Terms     int    `json:"terms"`
	Relations int    `json:"relations"`
}

func (o TermsPullOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Pulled %d concept(s), %d term(s), %d relation(s) from the workspace knowledge graph\n",
		o.Concepts, o.Terms, o.Relations)
	if o.Workspace != "" {
		fmt.Fprintf(w, "Workspace: %s\n", o.Workspace)
	}
	fmt.Fprintf(w, "Termbase:  %s\n", o.DBPath)
	fmt.Fprintln(w, "\n'kapi verify --terms' now gates offline against the governed terminology.")
	return nil
}

// truncate shortens s to at most n characters on a rune boundary, appending an
// ellipsis when it had to cut.
func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
