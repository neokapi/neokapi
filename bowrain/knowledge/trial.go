package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
)

// The trial: what the checks say on one stream, before and after.
//
// The blast radius counts. A trial NAMES: for the one content stream a pilot
// binds the draft to, it runs the same matchers the checks run — the voice
// vocabulary matcher and the terms lookup — under the live graph and under the
// graph the draft would produce, and reports the findings each side raises. A
// reviewer reads the two lists and decides whether the draft says what they
// meant it to say.
//
// What is live and what is computed, precisely, because the difference matters:
//
//   - The VOICE half is live on the stream. StartPilot materializes a candidate
//     profile and binds it to the stream's profile property, and the profile
//     resolver reads that rung, so a check running on this stream really does
//     resolve through the draft. VoiceBound reports whether that binding is in
//     place right now.
//   - The TERMS half is computed here. No check resolves terms per stream — the
//     terms read every check goes through names no stream — so the pilot's terms
//     shadow is deliberately invisible to them (see terms.ShadowIDPrefix). This
//     report applies the draft's ops to an in-memory copy of the graph and looks
//     the block up under both, which is the same lookup with a different graph
//     under it, but it is a computation and not a resolution.
//
// Saying so is the point. A trial that claimed to be a live check on both halves
// would be inviting a reviewer to trust a mechanism that is not there.

// TrialFinding is one named finding on one block: enough to recognize the rule
// that fired and the text it fired on.
type TrialFinding struct {
	// Kind is "term" or "voice" — which half of the gate raised it.
	Kind string `json:"kind"`
	// Rule names what fired: the designation for a term finding, the matched
	// term for a voice finding.
	Rule string `json:"rule"`
	// Replacement is what the rule says to write instead, when it says.
	Replacement string `json:"replacement,omitempty"`
	// Severity is the voice half's severity; empty for a term finding.
	Severity string `json:"severity,omitempty"`
	// ConceptID locates a term finding in the graph.
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

	// VoiceBound is the candidate profile the pilot bound to this stream, when
	// one is bound. Its presence is what makes the voice half of this report a
	// description of what checks on this stream actually resolve, rather than
	// only of what they would resolve.
	VoiceBound string `json:"voice_bound,omitempty"`
	// TermsComputed is always true and says so on the wire: no check resolves
	// terms per stream, so the terms half of this report is applied here rather
	// than resolved on the branch.
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
	pairs, err := e.voicePairs(ctx, ops)
	if err != nil {
		return nil, fmt.Errorf("build voice candidates: %w", err)
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

		raised, cleared, err := diffFindings(ctx, before, after, pairs, locale, text)
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
	report.VoiceBound = e.boundVoiceProfile(ctx, cs, ops, projectID, stream)
	return report, nil
}

// boundVoiceProfile returns the pilot candidate profile currently bound to the
// stream, or "" when none is. It reports the binding rather than assuming it:
// a pilot started before the draft grew its voice ops has no candidate, and a
// stream someone re-pointed by hand has someone else's.
func (e *Engine) boundVoiceProfile(ctx context.Context, cs ChangeSet, ops []ChangeSetOp, projectID, stream string) string {
	ids := voiceProfileIDs(ops)
	if len(ids) == 0 {
		return ""
	}
	streams, err := e.streamStore()
	if err != nil {
		return ""
	}
	s, err := streams.GetStream(ctx, projectID, stream)
	if err != nil || s == nil || s.Properties == nil {
		return ""
	}
	current := s.Properties[coreprofile.PropertyProfileID]
	for _, id := range ids {
		if current == pilotProfileID(cs.ID, stream, id) {
			return current
		}
	}
	return ""
}

// diffFindings names what each side of the gate raises on one block's text under
// the two graphs. Both halves reuse the matchers the checks reuse — the same
// MatchVocabulary the voice-vocabulary tool runs, the same LookupAll the term
// gate runs — so a trial can never disagree with the check it predicts for a
// reason of its own.
func diffFindings(ctx context.Context, before, after *terms.InMemoryStore, pairs []profilePair, locale model.LocaleID, text string) (raised, cleared []TrialFinding, err error) {
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

	for _, pr := range pairs {
		baseHits := coreprofile.MatchVocabulary(pr.baseline, text)
		candHits := coreprofile.MatchVocabulary(pr.candidate, text)
		baseKeys := vocabKeySet(baseHits)
		candKeys := vocabKeySet(candHits)
		for _, k := range sortedKeys(candKeys) {
			if _, ok := baseKeys[k]; !ok {
				raised = append(raised, candKeys[k])
			}
		}
		for _, k := range sortedKeys(baseKeys) {
			if _, ok := candKeys[k]; !ok {
				cleared = append(cleared, baseKeys[k])
			}
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

// vocabKeySet keys each vocabulary hit by category and byte range, matching how
// core/profile diffs the two profiles' hits: both sides score identical text, so
// positions align.
func vocabKeySet(hits []coreprofile.VocabHit) map[string]TrialFinding {
	out := make(map[string]TrialFinding, len(hits))
	for _, h := range hits {
		key := fmt.Sprintf("%s|%d|%d", h.Category, h.Start, h.End)
		out[key] = TrialFinding{
			Kind:        "voice",
			Rule:        h.Term,
			Replacement: h.Replacement,
			Severity:    string(h.Severity),
			ConceptID:   h.ConceptID,
		}
	}
	return out
}

func sortedKeys(m map[string]TrialFinding) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
