package termcandidate

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// ScanIdentity is the identity the context scan files its candidates under.
// It carries the tool prefix (platauth.ToolIdentityPrefix), so the person
// who keeps a candidate reviews a tool's work.
const ScanIdentity = "tool/context-scan"

// Candidate is one term to file, with its reach and the coordinates it is
// scoped to.
type Candidate struct {
	Term        string
	Definition  string
	Domain      string
	Reach       Reach
	Coordinates map[string]string
}

// FileResult says what filing did with each candidate.
type FileResult struct {
	// Filed are the ids of the concepts filed.
	Filed []string
	// Refreshed are the ids of earlier candidates whose reach was updated.
	Refreshed []string
	// Existing are the terms a concept already carries.
	Existing []string
	// Rejected are the terms a person has said are not terms.
	Rejected []string
}

// File stores each candidate as a proposed concept of the project: one
// source-language term with status proposed, the definition the model gave,
// the reach and the scope. A candidate a concept of the project already
// carries (case-insensitively, in the source language) is not filed again;
// when that concept is a candidate this identity filed and nobody has kept,
// its reach and scope are brought up to date. A candidate in rejected (lower
// case) is skipped, and so is a repeat of an earlier candidate.
func File(ctx context.Context, tb terms.Terminology, projectID string, source model.LocaleID, cands []Candidate, rejected map[string]bool, by string, now time.Time) (FileResult, error) {
	var res FileResult
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return res, fmt.Errorf("read the terms store: %w", err)
	}
	byTerm := map[string]terms.Concept{}
	for _, cp := range concepts {
		if p := ProjectOf(cp); p != "" && p != projectID {
			continue
		}
		for _, t := range cp.Terms {
			if sameLanguage(t.Locale, source) {
				byTerm[strings.ToLower(strings.TrimSpace(t.Text))] = cp
			}
		}
	}
	done := map[string]bool{}
	for _, cand := range cands {
		text := strings.TrimSpace(cand.Term)
		key := strings.ToLower(text)
		if key == "" || done[key] {
			continue
		}
		done[key] = true
		if rejected[key] {
			res.Rejected = append(res.Rejected, text)
			continue
		}
		if cp, ok := byTerm[key]; ok {
			if !IsCandidate(cp, source) || cp.Properties[PropProposedBy] != by {
				res.Existing = append(res.Existing, text)
				continue
			}
			before := maps.Clone(cp.Properties)
			cp.Properties = maps.Clone(cp.Properties)
			scope(&cp, cand)
			if maps.Equal(before, cp.Properties) {
				res.Existing = append(res.Existing, text)
				continue
			}
			cp.UpdatedAt = now
			if err := tb.AddConcept(ctx, cp); err != nil {
				return res, fmt.Errorf("refresh candidate %q: %w", text, err)
			}
			res.Refreshed = append(res.Refreshed, cp.ID)
			continue
		}
		cp := terms.Concept{
			ID:         id.New(),
			ProjectID:  projectID,
			Domain:     cand.Domain,
			Definition: cand.Definition,
			Terms:      []terms.Term{{Text: text, Locale: source, Status: model.TermProposed}},
			Properties: map[string]string{PropProposedBy: by, PropProject: projectID},
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		scope(&cp, cand)
		if err := tb.AddConcept(ctx, cp); err != nil {
			return res, fmt.Errorf("file candidate %q: %w", text, err)
		}
		byTerm[key] = cp
		res.Filed = append(res.Filed, cp.ID)
	}
	return res, nil
}

// scope writes a candidate's coordinates and reach onto its concept.
func scope(cp *terms.Concept, cand Candidate) {
	cp.ScopeToPoint("", cand.Coordinates)
	cand.Reach.Apply(cp)
}

// ProjectOf is the project a concept belongs to, "" for one of the whole
// workspace. A store that keeps no project column carries it in the
// PropProject property.
func ProjectOf(cp terms.Concept) string {
	if cp.ProjectID != "" {
		return cp.ProjectID
	}
	return cp.Properties[PropProject]
}

// IsCandidate reports whether a concept is waiting to be kept: its term in
// the source language has status proposed.
func IsCandidate(cp terms.Concept, source model.LocaleID) bool {
	for _, t := range cp.Terms {
		if sameLanguage(t.Locale, source) {
			return t.Status == model.TermProposed
		}
	}
	return false
}

// Promote sets a candidate's proposed source-language terms to preferred,
// and reports whether it changed any.
func Promote(cp *terms.Concept, source model.LocaleID) bool {
	changed := false
	cp.Terms = slices.Clone(cp.Terms)
	for i, t := range cp.Terms {
		if sameLanguage(t.Locale, source) && t.Status == model.TermProposed {
			cp.Terms[i].Status = model.TermPreferred
			changed = true
		}
	}
	return changed
}

// SourceText is a concept's term in the source language, or "".
func SourceText(cp terms.Concept, source model.LocaleID) string {
	for _, t := range cp.Terms {
		if sameLanguage(t.Locale, source) {
			return t.Text
		}
	}
	return ""
}

// sameLanguage reports whether two locales name one language: en-GB is
// English.
func sameLanguage(a, b model.LocaleID) bool {
	la := terms.BaseLanguage(a)
	return la != "" && la == terms.BaseLanguage(b)
}

// ProjectPropRejected is the project property listing the terms a person
// has said are not terms, lower case, one per line.
//
// It sits on the project rather than in the terms store or the context
// ledger. A tombstone concept would be read by every surface that lists
// concepts, and a ledger note would be dealt as a card of its own. The
// project row is read by both the server, which records a rejection, and
// the worker, which files candidates, and the list is small: it grows by
// one line per rejected candidate.
const ProjectPropRejected = "term_candidates.rejected"

// Rejected reads the rejected terms from a project's properties.
func Rejected(props map[string]string) map[string]bool {
	out := map[string]bool{}
	for line := range strings.SplitSeq(props[ProjectPropRejected], "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out[line] = true
		}
	}
	return out
}

// AddRejected records term as rejected in a project's properties, and
// reports whether the list changed. props must be non-nil.
func AddRejected(props map[string]string, term string) bool {
	key := strings.ToLower(strings.TrimSpace(term))
	if key == "" {
		return false
	}
	set := Rejected(props)
	if set[key] {
		return false
	}
	set[key] = true
	lines := slices.Sorted(maps.Keys(set))
	props[ProjectPropRejected] = strings.Join(lines, "\n")
	return true
}
