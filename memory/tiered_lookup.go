package memory

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// CandidateSource is the dialect seam for the shared tiered-lookup
// orchestration: the two backend-specific queries a store must supply. The
// tier loop, tag-mismatch penalty, exact-ambiguity demotion, project-priority
// boost, sort order and result limiting all live once in TieredLookup, so the
// SQLite (framework) and Postgres (bowrain) stores share one ranking. Both
// callbacks return framework Entry values already hydrated with their
// variants.
type CandidateSource struct {
	// Exact returns entries whose variant column (plain / struct_key /
	// general_key) equals key in sourceLocale, honoring the option's project
	// scope. The column argument is one of "plain", "struct_key",
	// "general_key".
	Exact func(ctx context.Context, column, key string, sourceLocale model.LocaleID, opts LookupOptions) ([]Entry, error)
	// FuzzyCandidates returns a candidate pool for Levenshtein scoring against
	// the three keys in sourceLocale, honoring the option's project scope.
	FuzzyCandidates func(ctx context.Context, plainKey, structKey, generalKey string, sourceLocale model.LocaleID, opts LookupOptions) ([]Entry, error)
}

// TieredLookup runs the shared exact→fuzzy tiered match against a backend's
// candidate source. It is the single home of the content memory ranking policy; the SQLite
// and Postgres stores both delegate here so their results stay identical.
//
// Tiers 1-3 are exact matches on the generalized, structural and plain keys.
// Plain-text equality with a differing inline-code structure takes the
// industry tag-mismatch penalty (ScoreNearExact). Several full-score exacts
// with differing targets are settled by the approval nearest the asking point
// (resolveNearestApproval), and demote to ScoreNearExact when there is no point
// to measure from (demoteAmbiguousExacts). When MinScore requires exact-only results the
// fuzzy tier is skipped. Tiers 4-6 score fuzzy candidates by Levenshtein
// ratio, with a small same-project boost. Results sort by score, then
// match-type priority, then entry ID (sortMatches) and are capped at
// MaxResults.
func TieredLookup(
	ctx context.Context,
	plainKey, structKey, generalKey string,
	entityAnnotations []*model.EntityAnnotation,
	sourceLocale, targetLocale model.LocaleID,
	opts LookupOptions,
	src CandidateSource,
) ([]Match, error) {
	// The candidate queries key on the variant rows, which the write path stores
	// under canonical locales; the lookup asks in the same form.
	sourceLocale = model.NormalizeLocale(sourceLocale)
	targetLocale = model.NormalizeLocale(targetLocale)
	var matches []Match
	seen := make(map[string]bool)
	modeEnabled := MatchModesEnabled(opts.MatchModes)

	add := func(entry Entry, score float64, mt MatchType) {
		if seen[entry.ID] {
			return
		}
		// Entry must have the requested target variant.
		if !entry.HasLocale(targetLocale) {
			return
		}
		seen[entry.ID] = true
		var adaptations []EntityAdaptation
		if mt == MatchGeneralizedExact || mt == MatchGeneralizedFuzzy {
			adaptations = ComputeEntityAdaptations(entry, sourceLocale, targetLocale, entityAnnotations)
		}
		matches = append(matches, Match{
			Entry:             entry,
			Score:             score,
			MatchType:         mt,
			ProjectID:         entry.ProjectID,
			EntityAdaptations: adaptations,
		})
	}

	// Tier 1-3: exact matches via indexed variant columns.
	if modeEnabled[MatchModeGeneralized] {
		entries, err := src.Exact(ctx, "general_key", generalKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, MatchGeneralizedExact)
		}
	}
	if modeEnabled[MatchModeStructural] {
		entries, err := src.Exact(ctx, "struct_key", structKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, MatchStructuralExact)
		}
	}
	if modeEnabled[MatchModePlain] {
		entries, err := src.Exact(ctx, "plain", plainKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			// Plain-text equality is not a full match when the inline-code
			// structure differs (the industry "tag mismatch" penalty): a
			// bare heading must not score 1.0 against an icon-button entry
			// just because the words coincide.
			score := 1.0
			if entryStruct := NormalizeText(model.RunsStructuralText(e.Variant(sourceLocale))); entryStruct != structKey {
				score = ScoreNearExact
			}
			add(e, score, MatchExact)
		}
	}

	// Several full-score exacts with differing targets are not one answer. A
	// caller that named the point it is asking from gets the approval nearest
	// that point; a caller that named none has no way to prefer one, so none of
	// them is THE translation and all demote to ScoreNearExact.
	if !resolveNearestApproval(matches, targetLocale, opts.Point) {
		demoteAmbiguousExacts(matches, targetLocale)
	}

	if len(matches) > 0 && opts.MinScore >= 1.0 {
		kept := matches[:0]
		for _, m := range matches {
			if m.Score >= opts.MinScore {
				kept = append(kept, m)
			}
		}
		return LimitResults(sortMatches(kept), opts.MaxResults), nil
	}

	// Tier 4-6: fuzzy candidates via the backend's candidate pool + Levenshtein
	// scoring.
	candidates, err := src.FuzzyCandidates(ctx, plainKey, structKey, generalKey, sourceLocale, opts)
	if err != nil {
		return nil, err
	}
	// Convert the fixed query keys to runes once, not per candidate.
	genKeyRunes := []rune(generalKey)
	structKeyRunes := []rune(structKey)
	plainKeyRunes := []rune(plainKey)
	for _, entry := range candidates {
		if seen[entry.ID] {
			continue
		}
		srcRuns := entry.Variant(sourceLocale)
		if len(srcRuns) == 0 {
			continue
		}
		var bestScore float64
		var bestType MatchType
		if modeEnabled[MatchModeGeneralized] {
			s := LevenshteinRatioRunes(genKeyRunes, []rune(NormalizeText(model.RunsGeneralizedText(srcRuns))))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchGeneralizedFuzzy
			}
		}
		if modeEnabled[MatchModeStructural] {
			s := LevenshteinRatioRunes(structKeyRunes, []rune(NormalizeText(model.RunsStructuralText(srcRuns))))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchStructuralFuzzy
			}
		}
		if modeEnabled[MatchModePlain] {
			s := LevenshteinRatioRunes(plainKeyRunes, []rune(NormalizeText(model.FlattenRuns(srcRuns))))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchFuzzy
			}
		}
		if bestScore < opts.MinScore {
			continue
		}
		if opts.ProjectID != "" && entry.ProjectID == opts.ProjectID && bestScore < 1.0 {
			bestScore += 0.03
			if bestScore > 1.0 {
				bestScore = 1.0
			}
		}
		add(entry, bestScore, bestType)
	}

	return LimitResults(sortMatches(matches), opts.MaxResults), nil
}
