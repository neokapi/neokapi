package terms

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// ApplyLookupDefaults fills unset LookupOptions fields with their defaults: a
// non-positive MinScore becomes 0.8. It also puts both locales in canonical
// form, the form the term rows are keyed by, so every backend's lookup asks
// the store the question it can answer.
func ApplyLookupDefaults(opts LookupOptions) LookupOptions {
	if opts.MinScore <= 0 {
		opts.MinScore = 0.8
	}
	opts.SourceLocale = model.NormalizeLocale(opts.SourceLocale)
	opts.TargetLocale = model.NormalizeLocale(opts.TargetLocale)
	return opts
}

// MatchModesEnabled returns the set of enabled match strategies. An empty modes
// slice enables all of exact, normalized, and fuzzy matching.
func MatchModesEnabled(modes []model.MatchStrategy) map[model.MatchStrategy]bool {
	if len(modes) == 0 {
		return map[model.MatchStrategy]bool{
			model.MatchStrategyExact:      true,
			model.MatchStrategyNormalized: true,
			model.MatchStrategyFuzzy:      true,
		}
	}
	m := make(map[model.MatchStrategy]bool)
	for _, mode := range modes {
		m[mode] = true
	}
	return m
}

func matchesSource(c Concept, filter []TermSource) bool {
	if len(filter) == 0 {
		return true
	}
	source := c.Source
	if source == "" {
		source = TermSourceTerminology
	}
	return slices.Contains(filter, source)
}

func matchesDomain(c Concept, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	for _, d := range domains {
		if strings.EqualFold(c.Domain, d) {
			return true
		}
	}
	return false
}

// MatchesStatus reports whether status is in the filter. An empty filter
// matches any status.
func MatchesStatus(status model.TermStatus, filter []model.TermStatus) bool {
	if len(filter) == 0 {
		return true
	}
	return slices.Contains(filter, status)
}

func matchesTerm(text, term string, caseSensitive bool) bool {
	if caseSensitive {
		return text == term
	}
	return strings.EqualFold(text, term)
}

// NormalizeTerm normalizes a term for comparison by applying Unicode NFC
// normalization, lowercasing, trimming whitespace, and collapsing internal
// whitespace to single spaces.
func NormalizeTerm(s string) string {
	s = norm.NFC.String(s)
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// FuzzyLengthWindow returns the inclusive term-length bounds a fuzzy candidate
// must fall within to be able to reach minScore, for a query of queryLen runes.
// SQL stores apply the window to stored normalized keys.
//
// The fuzzy score is memory.LevenshteinRatio, which is 1 - dist/max(m,n), and
// the edit distance between two strings is at least their length difference.
// So the best a candidate of length n can score against a query of length m is
//
//	1 - |m-n|/max(m,n) = min(m,n)/max(m,n)
//
// which reaches minScore only while minScore*m <= n <= m/minScore. Deriving the
// window from minScore bounds the scan for the selected score threshold.
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
		int(min(math.MaxInt32, math.Ceil(float64(queryLen)/minScore)))
}

// TermCandidate is one scanned term row from a backend candidate query, before
// the shared lookup applies validity/status/score filtering and hydrates the
// owning concept. Term carries the matched term's own fields, including its
// per-dialect-reconstructed Validity; the shared code fills in Concept.
type TermCandidate struct {
	ConceptID string
	Term      Term
}

// TermCandidateSource supplies term rows and their owning concepts to LookupTiered.
// Candidate queries apply locale and source constraints. The shared lookup applies
// scope, status, domain and score filters before selecting a tier.
type TermCandidateSource struct {
	// Exact returns terms whose (case-folded) text equals the query text in the
	// source locale, honoring the option's project/source scope.
	Exact func(ctx context.Context, sourceText string, opts LookupOptions) ([]TermCandidate, error)
	// Forms returns the terms in the source locale that declare forms, honoring
	// the option's project/source scope. The exact tier keeps those with a
	// declared form that is the whole query.
	Forms func(ctx context.Context, opts LookupOptions) ([]TermCandidate, error)
	// Normalized returns terms indexed under normalizedSource and rows that predate the index.
	Normalized func(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error)
	// FuzzyCandidates returns a candidate pool for Levenshtein scoring against
	// normalizedSource.
	FuzzyCandidates func(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error)
	// Concept hydrates the full owning concept (with all its terms).
	Concept func(ctx context.Context, conceptID string) (Concept, error)
}

// LookupTiered runs the shared exact→normalized→fuzzy tiered term lookup against
// a backend's candidate source. The exact tier finds a term under its text and
// under a declared form that is the whole query, read by the rule
// LookupAllTiered applies to running text, so "alerts" finds a term "alert"
// that lists it and reports the term. Tiers stop early: a non-empty higher tier
// suppresses the lower ones. Every candidate must pass the validity-scope and
// status filters; fuzzy candidates additionally must reach opts.MinScore by
// Levenshtein ratio. Results sort by score descending.
func LookupTiered(ctx context.Context, sourceText string, opts LookupOptions, src TermCandidateSource) ([]TermMatch, error) {
	return lookupTiers(ctx, sourceText, opts, func(mode model.MatchStrategy, key string, opts LookupOptions) ([]TermCandidate, error) {
		switch mode {
		case model.MatchStrategyExact:
			exact, err := src.Exact(ctx, key, opts)
			if err != nil {
				return nil, err
			}
			withForms, err := src.Forms(ctx, opts)
			if err != nil {
				return nil, err
			}
			return append(exact, formsNamingQuery(key, withForms, opts.CaseSensitive)...), nil
		case model.MatchStrategyNormalized:
			rows, err := src.Normalized(ctx, key, opts)
			if err != nil {
				return nil, err
			}
			var matched []TermCandidate
			for _, row := range rows {
				if NormalizeTerm(row.Term.Text) == key {
					matched = append(matched, row)
				}
			}
			return matched, nil
		default:
			return src.FuzzyCandidates(ctx, key, opts)
		}
	}, src.Concept)
}

func lookupTiers(ctx context.Context, text string, opts LookupOptions,
	candidates func(model.MatchStrategy, string, LookupOptions) ([]TermCandidate, error),
	hydrate func(context.Context, string) (Concept, error),
) ([]TermMatch, error) {
	if text == "" {
		return nil, nil
	}
	opts = ApplyLookupDefaults(opts)
	enabled := MatchModesEnabled(opts.MatchModes)
	normalized := NormalizeTerm(text)
	for _, mode := range []model.MatchStrategy{model.MatchStrategyExact, model.MatchStrategyNormalized, model.MatchStrategyFuzzy} {
		if !enabled[mode] {
			continue
		}
		key := normalized
		if mode == model.MatchStrategyExact {
			key = text
		}
		rows, err := candidates(mode, key, opts)
		if err != nil {
			return nil, err
		}
		var matches []TermMatch
		for _, row := range rows {
			if !opts.includesConcept(row.ConceptID) || !MatchesScope(row.Term.Validity, opts.Scope) || !MatchesStatus(row.Term.Status, opts.StatusFilter) {
				continue
			}
			score := 1.0
			switch mode {
			case model.MatchStrategyNormalized:
				score = 0.95
			case model.MatchStrategyFuzzy:
				score = memory.LevenshteinRatio(key, NormalizeTerm(row.Term.Text))
				if score < opts.MinScore {
					continue
				}
			}
			concept, err := hydrate(ctx, row.ConceptID)
			if err != nil {
				return nil, err
			}
			if matchesDomain(concept, opts.Domains) && matchesSource(concept, opts.SourceFilter) {
				matches = append(matches, TermMatch{Concept: concept, Term: row.Term, Score: score, MatchType: mode})
			}
		}
		if len(matches) > 0 {
			slices.SortFunc(matches, func(a, b TermMatch) int {
				return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(a.Concept.ID, b.Concept.ID), cmp.Compare(a.Term.Text, b.Term.Text))
			})
			return matches, nil
		}
	}
	return nil, nil
}

// formsNamingQuery keeps the candidates with a declared form that is the whole
// query.
func formsNamingQuery(query string, cands []TermCandidate, cased bool) []TermCandidate {
	var kept []TermCandidate
	for _, c := range cands {
		if declaresFormOf(c.Term, query, cased) {
			kept = append(kept, c)
		}
	}
	return kept
}

// declaresFormOf reports whether one of term's declared forms is the whole of
// query. The query is read as LookupAllTiered reads running text, through
// check.FindTermFormsIn: whole words, any run of whitespace between them, case
// folded unless cased. A single-term lookup and a scan of a passage therefore
// agree about which spellings are a use of the term.
func declaresFormOf(term Term, query string, cased bool) bool {
	query = strings.TrimSpace(query)
	forms := NormalizeForms(term.Text, term.Forms)
	if query == "" || len(forms) == 0 {
		return false
	}
	for _, hit := range check.FindTermFormsIn(check.PrepareText(query), forms, cased) {
		if hit[0] == 0 && hit[1] == len(query) {
			return true
		}
	}
	return false
}

// LocaleTerm is one term with its owning concept, the unit the shared LookupAll
// scan consumes. Concept carries at least ProjectID (for the project-priority
// preference); Term carries at least Text and Validity.
type LocaleTerm struct {
	Concept Concept
	Term    Term
}

// LookupAllTiered locates eligible terms and their declared forms in sourceText.
// Each match reports the term and the byte range of its spelling in the text.
// Duplicate occurrences prefer the requested project's concept, then the lowest
// concept ID. A longer declared match suppresses a term contained within it.
// Results are ordered by position, then by decreasing length.
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
		if !opts.includesConcept(entry.Concept.ID) || !matchesDomain(entry.Concept, opts.Domains) || !matchesSource(entry.Concept, opts.SourceFilter) || !MatchesScope(entry.Term.Validity, opts.Scope) {
			continue
		}
		surfaces := entry.Term.Surfaces()
		if len(surfaces) == 0 || check.NewTermMatcher(surfaces[0]).Empty() {
			continue
		}
		key := strings.ToLower(entry.Term.Text)

		for _, hit := range check.FindTermFormsIn(prepared, surfaces, opts.CaseSensitive) {
			k := matchKey{text: key, pos: hit[0]}

			m := TermMatch{
				Concept:   entry.Concept,
				Term:      entry.Term,
				Score:     1.0,
				MatchType: model.MatchStrategyExact,
				Position:  model.TextRange{Start: hit[0], End: hit[1]},
			}

			if existingIdx, exists := seen[k]; exists {
				previous := matches[existingIdx].Concept
				preferred := opts.ProjectID != "" && entry.Concept.ProjectID == opts.ProjectID
				previousPreferred := opts.ProjectID != "" && previous.ProjectID == opts.ProjectID
				if (preferred && !previousPreferred) || (preferred == previousPreferred && entry.Concept.ID < previous.ID) {
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

// dropCoveredMatches applies the longest-declared-match rule
// (check.KeepLongestDeclared, the one definition term-check applies to rule
// hits): a match whose span falls strictly inside another match's span is not
// reported, while matches that only overlap, or cover exactly the same span, are
// all kept. The caller's order is preserved for the matches that remain.
func dropCoveredMatches(matches []TermMatch) []TermMatch {
	if len(matches) < 2 {
		return matches
	}
	spans := make([]check.DeclaredSpan, len(matches))
	for i, m := range matches {
		spans[i] = check.DeclaredSpan{Start: m.Position.Start, End: m.Position.End, Index: i}
	}
	keep := make(map[int]bool, len(matches))
	for _, s := range check.KeepLongestDeclared(spans) {
		keep[s.Index] = true
	}
	kept := matches[:0]
	for i, m := range matches {
		if keep[i] {
			kept = append(kept, m)
		}
	}
	return kept
}

// MatchedConceptIDs returns the distinct concept IDs of a match set, in the
// order the matches list them.
func MatchedConceptIDs(matches []TermMatch) []string {
	seen := make(map[string]bool, len(matches))
	var ids []string
	for _, m := range matches {
		if !seen[m.Concept.ID] {
			seen[m.Concept.ID] = true
			ids = append(ids, m.Concept.ID)
		}
	}
	return ids
}

// HydrateMatchConcepts replaces each match's concept with the whole concept of
// the same ID from concepts: its properties and its terms in every locale. A SQL
// backend's LookupAll scans source-locale rows, which carry only part of each
// concept, then reads the matched concepts and passes them here. The match then
// holds what the in-memory store's match holds, including the target-locale
// wording a caller shows beside the source term. A match whose concept is
// missing from concepts keeps the one it has. Each match gets its own Terms
// slice.
func HydrateMatchConcepts(matches []TermMatch, concepts []Concept) {
	byID := make(map[string]Concept, len(concepts))
	for _, c := range concepts {
		byID[c.ID] = c
	}
	for i := range matches {
		c, ok := byID[matches[i].Concept.ID]
		if !ok {
			continue
		}
		c.Terms = slices.Clone(c.Terms)
		matches[i].Concept = c
	}
}
