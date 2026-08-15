package terms

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// FuzzyLengthWindow returns the inclusive term-length bounds a fuzzy candidate
// must fall within to be able to reach minScore, for a query of queryLen runes.
// Both backends' full-scan fallbacks pre-filter on it.
//
// The fuzzy score is memory.LevenshteinRatio, which is 1 - dist/max(m,n), and
// the edit distance between two strings is at least their length difference.
// So the best a candidate of length n can score against a query of length m is
//
//	1 - |m-n|/max(m,n) = min(m,n)/max(m,n)
//
// which reaches minScore only while minScore*m <= n <= m/minScore. Deriving the
// window from minScore keeps the pre-filter consistent with the score gate that
// follows it. The fixed ±30% window it replaces was not: any caller asking for
// a minScore below 0.7 lost candidates that would have passed, silently and
// only on the full-scan path, so the two backends disagreed exactly when their
// trigram pre-filters happened to fall through differently.
func FuzzyLengthWindow(queryLen int, minScore float64) (minLen, maxLen int) {
	if queryLen <= 0 {
		return 0, 0
	}
	// A non-positive or impossible minScore bounds nothing; scan unrestricted
	// rather than invent a window.
	if minScore <= 0 || minScore > 1 {
		return 0, math.MaxInt32
	}
	// Floor/ceil rather than truncate, so a boundary-length candidate is
	// admitted to the pool and rejected (or kept) by the score gate itself.
	return int(math.Floor(float64(queryLen) * minScore)),
		int(math.Ceil(float64(queryLen) / minScore))
}

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
// framework SQLite and bowrain Postgres terms stores share one ranking policy.
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
			score = memory.LevenshteinRatio(fuzzyKey, NormalizeTerm(c.Term.Text))
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
// Occurrences are found by the shared check.TermMatcher — the same rule the
// voice-profile vocabulary, the do-not-translate check and the occurrence graph
// use — so a word is a hit for the whole gate or for none of it. Where two
// declared terms cover the same words, the longer one wins: a project that has
// declared `mooring_id` a concept of its own has said that those characters are
// not a use of the retired `mooring` inside them, and reporting both would be
// the graph contradicting itself.
//
// Postgres inherits the (text, position) de-duplication by construction; its
// terms is workspace-scoped and carries no project_id, so the
// project-priority preference is inert there while the de-duplication and
// validity filtering are shared.
func LookupAllTiered(sourceText string, opts LookupOptions, terms []LocaleTerm) []TermMatch {
	if sourceText == "" {
		return nil
	}
	var matches []TermMatch
	prepared := check.PrepareText(sourceText)

	type matchKey struct {
		text string
		pos  int
	}
	seen := make(map[matchKey]int)

	for _, entry := range terms {
		if !MatchesScope(entry.Term.Validity, opts.Scope) {
			continue
		}
		matcher := check.NewTermMatcher(entry.Term.Text)
		if opts.CaseSensitive {
			matcher = check.NewCaseSensitiveTermMatcher(entry.Term.Text)
		}
		if matcher.Empty() {
			continue
		}
		key := strings.ToLower(entry.Term.Text)

		for _, hit := range matcher.FindIn(prepared) {
			k := matchKey{text: key, pos: hit[0]}

			m := TermMatch{
				Concept:   entry.Concept,
				Term:      entry.Term,
				Score:     1.0,
				MatchType: model.MatchStrategyExact,
				Position:  model.TextRange{Start: hit[0], End: hit[1]},
			}

			if existingIdx, exists := seen[k]; exists {
				if opts.ProjectID != "" && entry.Concept.ProjectID == opts.ProjectID &&
					matches[existingIdx].Concept.ProjectID != opts.ProjectID {
					matches[existingIdx] = m
				}
				continue
			}
			seen[k] = len(matches)
			matches = append(matches, m)
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
	return dropCoveredMatches(matches)
}

// dropCoveredMatches applies the longest-declared-match preference: a match
// whose span falls strictly inside another match's span is not reported.
// Matches that merely overlap, and matches covering exactly the same span, are
// all kept — neither term contains the other, so both are genuinely used.
//
// It relies on the caller's sort (start ascending, then longest first), which
// puts a covering match before everything it covers.
func dropCoveredMatches(matches []TermMatch) []TermMatch {
	if len(matches) < 2 {
		return matches
	}
	kept := matches[:0]
	var cover model.TextRange
	covered := false
	for _, m := range matches {
		if covered && cover.End >= m.Position.End &&
			!(cover.Start == m.Position.Start && cover.End == m.Position.End) {
			continue
		}
		kept = append(kept, m)
		if !covered || m.Position.End > cover.End {
			cover, covered = m.Position, true
		}
	}
	return kept
}
