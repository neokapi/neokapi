package termbase

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// TermCandidate is one scanned term row from a backend candidate query, before
// the shared lookup applies validity/status/score filtering and hydrates the
// owning concept. Term carries the matched term's own fields, including its
// per-dialect-reconstructed Validity; the shared code fills in Concept.
type TermCandidate struct {
	ConceptID string
	Term      Term
}

// TermCandidateSource is the dialect seam for the shared single-term tiered
// lookup: the exact / normalized / fuzzy candidate queries plus a concept
// hydrator. The tier gating, validity-scope filter, status filter, Levenshtein
// fuzzy scoring and score-descending sort all live once in LookupTiered, so the
// framework SQLite and bowrain Postgres termbases share one ranking policy.
//
// Because the validity-scope filter now lives in the shared path (fed by each
// backend's per-dialect validity reconstruction), Postgres honors term validity
// (valid_from/valid_to) by construction — fixing the drift where its fuzzy,
// exact and normalized paths ignored validity entirely while SQLite honored it.
//
// The candidate callbacks return raw rows (no filtering, no concept hydration);
// they must populate Term.Text, Term.Locale, Term.Status and Term.Validity, and
// may leave the rest zero.
type TermCandidateSource struct {
	// Exact returns terms whose (case-folded) text equals the query text in the
	// source locale, honoring the option's project/source scope.
	Exact func(ctx context.Context, sourceText string, opts LookupOptions) ([]TermCandidate, error)
	// Normalized returns terms whose normalized text equals normalizedSource.
	Normalized func(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error)
	// FuzzyCandidates returns a candidate pool for Levenshtein scoring against
	// normalizedSource (the backend picks its own retrieval, e.g. a trigram
	// pre-filter with a length-bounded full-scan fallback).
	FuzzyCandidates func(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error)
	// Concept hydrates the full owning concept (with all its terms).
	Concept func(ctx context.Context, conceptID string) (Concept, error)
}

// LookupTiered runs the shared exact→normalized→fuzzy tiered term lookup against
// a backend's candidate source. Tiers stop early: a non-empty higher tier
// suppresses the lower ones. Every candidate must pass the validity-scope and
// status filters; fuzzy candidates additionally must reach opts.MinScore by
// Levenshtein ratio. Results sort by score descending.
func LookupTiered(ctx context.Context, sourceText string, opts LookupOptions, src TermCandidateSource) ([]TermMatch, error) {
	if sourceText == "" {
		return nil, nil
	}
	opts = ApplyLookupDefaults(opts)
	modeEnabled := MatchModesEnabled(opts.MatchModes)
	normalizedSource := NormalizeTerm(sourceText)
	var matches []TermMatch

	if modeEnabled[model.MatchStrategyExact] {
		cands, err := src.Exact(ctx, sourceText, opts)
		if err != nil {
			return nil, err
		}
		matches = append(matches, buildTermMatches(ctx, cands, 1.0, model.MatchStrategyExact, "", opts, src.Concept)...)
	}

	if modeEnabled[model.MatchStrategyNormalized] && len(matches) == 0 {
		cands, err := src.Normalized(ctx, normalizedSource, opts)
		if err != nil {
			return nil, err
		}
		matches = append(matches, buildTermMatches(ctx, cands, 0.95, model.MatchStrategyNormalized, "", opts, src.Concept)...)
	}

	if modeEnabled[model.MatchStrategyFuzzy] && len(matches) == 0 {
		cands, err := src.FuzzyCandidates(ctx, normalizedSource, opts)
		if err != nil {
			return nil, err
		}
		matches = append(matches, buildTermMatches(ctx, cands, 0, model.MatchStrategyFuzzy, normalizedSource, opts, src.Concept)...)
	}

	// Score descending, with a deterministic tiebreak (concept ID, then term
	// text) so equal-score matches order identically across backends.
	slices.SortFunc(matches, func(a, b TermMatch) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Concept.ID, b.Concept.ID); c != 0 {
			return c
		}
		return cmp.Compare(a.Term.Text, b.Term.Text)
	})
	return matches, nil
}

// buildTermMatches applies the shared per-candidate filters and concept
// hydration. For the fuzzy tier (fuzzyKey != "") the score is the Levenshtein
// ratio against the candidate's normalized text, gated by opts.MinScore; the
// exact and normalized tiers use fixedScore. A candidate whose concept fails to
// hydrate is skipped (matching the historical per-backend behavior).
func buildTermMatches(ctx context.Context, cands []TermCandidate, fixedScore float64, mt model.MatchStrategy, fuzzyKey string, opts LookupOptions, hydrate func(context.Context, string) (Concept, error)) []TermMatch {
	var matches []TermMatch
	for _, c := range cands {
		if !MatchesScope(c.Term.Validity, opts.Scope) {
			continue
		}
		if !MatchesStatus(c.Term.Status, opts.StatusFilter) {
			continue
		}
		score := fixedScore
		if fuzzyKey != "" {
			score = sievepen.LevenshteinRatio(fuzzyKey, NormalizeTerm(c.Term.Text))
			if score < opts.MinScore {
				continue
			}
		}
		concept, err := hydrate(ctx, c.ConceptID)
		if err != nil {
			continue
		}
		matches = append(matches, TermMatch{
			Concept:   concept,
			Term:      c.Term,
			Score:     score,
			MatchType: mt,
		})
	}
	return matches
}

// LocaleTerm is one term with its owning concept, the unit the shared LookupAll
// scan consumes. Concept carries at least ProjectID (for the project-priority
// preference); Term carries at least Text and Validity.
type LocaleTerm struct {
	Concept Concept
	Term    Term
}

// LookupAllTiered finds every occurrence of the given terms in sourceText,
// de-duplicating identical (text, position) hits and preferring a
// project-scoped concept when the option names a project. Validity-scope
// filtering lives here so both backends honor it identically. Results sort by
// start position, then longest match first.
//
// Postgres inherits the (text, position) de-duplication by construction; its
// termbase is workspace-scoped and carries no project_id, so the
// project-priority preference is inert there while the de-duplication and
// validity filtering are shared.
func LookupAllTiered(sourceText string, opts LookupOptions, terms []LocaleTerm) []TermMatch {
	if sourceText == "" {
		return nil
	}
	var matches []TermMatch
	lowerSource := strings.ToLower(sourceText)

	type matchKey struct {
		text string
		pos  int
	}
	seen := make(map[matchKey]int)

	for _, entry := range terms {
		if !MatchesScope(entry.Term.Validity, opts.Scope) {
			continue
		}
		searchIn := sourceText
		searchFor := entry.Term.Text
		if !opts.CaseSensitive {
			searchIn = lowerSource
			searchFor = strings.ToLower(entry.Term.Text)
		}
		if searchFor == "" {
			continue
		}

		offset := 0
		for {
			idx := strings.Index(searchIn[offset:], searchFor)
			if idx < 0 {
				break
			}
			pos := offset + idx
			key := matchKey{text: searchFor, pos: pos}

			m := TermMatch{
				Concept:   entry.Concept,
				Term:      entry.Term,
				Score:     1.0,
				MatchType: model.MatchStrategyExact,
				Position:  model.TextRange{Start: pos, End: pos + len(searchFor)},
			}

			if existingIdx, exists := seen[key]; exists {
				if opts.ProjectID != "" && entry.Concept.ProjectID == opts.ProjectID &&
					matches[existingIdx].Concept.ProjectID != opts.ProjectID {
					matches[existingIdx] = m
				}
			} else {
				seen[key] = len(matches)
				matches = append(matches, m)
			}

			offset = pos + len(searchFor)
		}
	}

	// Start ascending, then longer match first, then a deterministic concept-ID
	// tiebreak so fully-tied matches order identically across backends.
	slices.SortFunc(matches, func(a, b TermMatch) int {
		if c := cmp.Compare(a.Position.Start, b.Position.Start); c != 0 {
			return c
		}
		if c := cmp.Compare(b.Position.End, a.Position.End); c != 0 {
			return c
		}
		return cmp.Compare(a.Concept.ID, b.Concept.ID)
	})
	return matches
}
