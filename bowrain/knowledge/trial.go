package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
)

// A trial compares term findings for a pilot's content stream under the live
// graph and the proposed graph.
//
// The evaluation is simulated here. Regular term checks do not resolve by
// stream, so they cannot see the pilot shadow (terms.ShadowIDPrefix). The trial
// applies draft operations to an in-memory graph and compares lookups against
// that graph with lookups against the live graph.

// TrialFinding is one named finding on one block: enough to recognize the rule
// that fired and the text it fired on.
type TrialFinding struct {
	// Kind is "term": the gate that raised it.
	Kind string `json:"kind"`
	// Rule names what fired: the designation.
	Rule string `json:"rule"`
	// Replacement is what the rule says to write instead, when it says.
	Replacement string `json:"replacement,omitempty"`
	// ConceptID locates the finding in the graph.
	ConceptID string `json:"concept_id,omitempty"`

	BlockID        string         `json:"block_id"`
	ItemName       string         `json:"item_name"`
	CollectionName string         `json:"collection_name,omitempty"`
	Locale         model.LocaleID `json:"locale"`
	Text           string         `json:"text"`
}

// TrialReport is the findings diff for one pilot stream.
type TrialReport struct {
	ChangesetID string `json:"changeset_id"`
	ProjectID   string `json:"project_id"`
	Stream      string `json:"stream"`

	// TotalBlocks is the (block, locale) rows scanned; ChangedBlocks the rows
	// whose finding set differs between the two graphs.
	TotalBlocks   int `json:"total_blocks"`
	ChangedBlocks int `json:"changed_blocks"`

	// Raised are the findings the draft adds, Cleared the ones it removes.
	// Both are capped — a trial is read, not exported.
	Raised  []TrialFinding `json:"raised"`
	Cleared []TrialFinding `json:"cleared"`
	// RaisedTotal and ClearedTotal count before the cap, so a truncated list
	// still says how much it is a sample of.
	RaisedTotal  int `json:"raised_total"`
	ClearedTotal int `json:"cleared_total"`

	// TermsComputed is always true and says so on the wire: no check resolves
	// terms per stream, so this report is applied here rather than resolved on
	// the branch.
	TermsComputed bool `json:"terms_computed"`

	Partial       bool      `json:"partial,omitempty"`
	PartialReason string    `json:"partial_reason,omitempty"`
	ComputedAt    time.Time `json:"computed_at"`
}

// DefaultTrialFindings caps each side of the diff when EvalOptions.MaxSamples is
// not set.
const DefaultTrialFindings = 50

// TrialFindings runs the check matchers over one project's stream under the live
// graph and under the graph the change-set's ops would produce, and reports the
// findings each side raises. Nothing is persisted.
//
// The stream is walked alone: a trial answers "what changes HERE", so including
// main would fold in content nobody bound the draft to and make the diff read as
// the workspace's rather than the branch's.
func (e *Engine) TrialFindings(ctx context.Context, workspaceID string, cs ChangeSet, ops []ChangeSetOp, projectID, stream string, opts EvalOptions) (*TrialReport, error) {
	if projectID == "" || stream == "" {
		return nil, errors.New("knowledge: TrialFindings requires a project and a stream")
	}

	before, err := e.buildBeforeTerms(ctx, ops)
	if err != nil {
		return nil, fmt.Errorf("build before terms: %w", err)
	}
	after, err := ApplyOpsToTerms(ctx, before, ops)
	if err != nil {
		return nil, fmt.Errorf("build after terms: %w", err)
	}

	limit := opts.MaxSamples
	if limit <= 0 {
		limit = DefaultTrialFindings
	}

	report := &TrialReport{
		ChangesetID:   cs.ID,
		ProjectID:     projectID,
		Stream:        stream,
		TermsComputed: true,
		ComputedAt:    time.Now().UTC(),
	}

	scoped := opts
	scoped.ProjectID = projectID
	scoped.Streams = []string{stream}

	walkErr := e.walkBlocks(ctx, workspaceID, scoped, func(p *store.Project, st string, b *venue.StoredBlock, locale model.LocaleID, text, colID, colName string) error {
		report.TotalBlocks++

		raised, cleared, err := diffFindings(ctx, before, after, locale, text)
		if err != nil {
			return err
		}
		if len(raised) == 0 && len(cleared) == 0 {
			return nil
		}
		report.ChangedBlocks++

		locate := func(f TrialFinding) TrialFinding {
			f.BlockID = b.ID
			f.ItemName = b.ItemName
			f.CollectionName = colName
			f.Locale = locale
			f.Text = truncateText(text)
			return f
		}
		for _, f := range raised {
			report.RaisedTotal++
			if len(report.Raised) < limit {
				report.Raised = append(report.Raised, locate(f))
			}
		}
		for _, f := range cleared {
			report.ClearedTotal++
			if len(report.Cleared) < limit {
				report.Cleared = append(report.Cleared, locate(f))
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errBudgetExhausted) {
		return nil, walkErr
	}
	if errors.Is(walkErr, errBudgetExhausted) {
		report.Partial = true
		report.PartialReason = "the trial reached its time budget before it had covered the stream"
	}

	if report.Raised == nil {
		report.Raised = []TrialFinding{}
	}
	if report.Cleared == nil {
		report.Cleared = []TrialFinding{}
	}
	return report, nil
}

// diffFindings names what each side of the gate raises on one block's text under
// the two graphs. It reuses the LookupAll the term gate runs, so a trial can
// never disagree with the check it predicts for a reason of its own.
func diffFindings(ctx context.Context, before, after *terms.InMemoryStore, locale model.LocaleID, text string) (raised, cleared []TrialFinding, err error) {
	beforeSet, err := forbiddenTerms(ctx, before, locale, text)
	if err != nil {
		return nil, nil, err
	}
	afterSet, err := forbiddenTerms(ctx, after, locale, text)
	if err != nil {
		return nil, nil, err
	}
	for _, k := range sortedKeys(afterSet) {
		if _, ok := beforeSet[k]; !ok {
			raised = append(raised, afterSet[k])
		}
	}
	for _, k := range sortedKeys(beforeSet) {
		if _, ok := afterSet[k]; !ok {
			cleared = append(cleared, beforeSet[k])
		}
	}
	return raised, cleared, nil
}

// forbiddenTerms returns the forbidden designations a text contains under one
// graph, keyed by concept + lowered designation so the same term is comparable
// across the two graphs.
func forbiddenTerms(ctx context.Context, tb *terms.InMemoryStore, locale model.LocaleID, text string) (map[string]TrialFinding, error) {
	out := map[string]TrialFinding{}
	if tb == nil {
		return out, nil
	}
	matches, err := tb.LookupAll(ctx, text, terms.LookupOptions{SourceLocale: locale})
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		if m.Term.Status != model.TermForbidden {
			continue
		}
		key := m.Concept.ID + "|" + strings.ToLower(m.Term.Text)
		out[key] = TrialFinding{
			Kind:        "term",
			Rule:        m.Term.Text,
			ConceptID:   m.Concept.ID,
			Replacement: resolveReplacement(ctx, tb, m.Concept, locale),
		}
	}
	return out, nil
}

func sortedKeys(m map[string]TrialFinding) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
