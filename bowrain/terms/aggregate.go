package terms

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

// TermStatusOrder is the lifecycle vocabulary in reading order, so a status
// breakdown lists the same statuses in the same order everywhere.
var TermStatusOrder = []model.TermStatus{
	model.TermPreferred,
	model.TermApproved,
	model.TermAdmitted,
	model.TermProposed,
	model.TermDeprecated,
	model.TermForbidden,
}

// ConceptCounts is the workspace's vocabulary counted by term lifecycle status.
// A concept counts once under every status one of its terms carries, so the
// per-status counts overlap and do not sum to Total.
type ConceptCounts struct {
	Total    int                      `json:"total"`
	ByStatus map[model.TermStatus]int `json:"by_status"`
}

// LocaleCoverage is one locale's share of the workspace's concepts: how many
// define a term in it, out of how many exist.
type LocaleCoverage struct {
	Locale  model.LocaleID `json:"locale"`
	Present int            `json:"present"`
	Total   int            `json:"total"`
	Pct     int            `json:"pct"`
}

// ConceptAggregates is the optional capability a terms store implements to
// answer workspace-wide concept aggregates in the database. A store without it
// is aggregated in Go over a full read (CountsFromConcepts,
// CoverageFromConcepts), which is correct but reads every concept.
type ConceptAggregates interface {
	ConceptCounts(ctx context.Context) (ConceptCounts, error)
	ConceptLocaleCoverage(ctx context.Context) ([]LocaleCoverage, error)
}

// RecentSearcher is the optional capability a terms store implements to order a
// concept page by last update in the database, so "recently changed" spans the
// whole workspace rather than one page of it.
type RecentSearcher interface {
	SearchRecent(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error)
}

var (
	_ ConceptAggregates = (*PostgresStore)(nil)
	_ RecentSearcher    = (*PostgresStore)(nil)
)

// ConceptCounts counts the workspace's concepts, and the concepts carrying a
// term at each lifecycle status, in two grouped queries.
func (tb *PostgresStore) ConceptCounts(ctx context.Context) (ConceptCounts, error) {
	out := ConceptCounts{ByStatus: map[model.TermStatus]int{}}

	total, err := tb.Count(ctx)
	if err != nil {
		return out, err
	}
	out.Total = total

	rows, err := tb.db.QueryContext(ctx,
		`SELECT status, COUNT(DISTINCT concept_id) FROM tb_terms
		 WHERE workspace_id = $1 GROUP BY status`, tb.workspaceID)
	if err != nil {
		return out, fmt.Errorf("count concepts by status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return out, fmt.Errorf("scan concept status count: %w", err)
		}
		out.ByStatus[model.TermStatus(status)] = n
	}
	return out, rows.Err()
}

// ConceptLocaleCoverage reports, for every locale that appears in the
// workspace, how many concepts define a term in it — over every concept, not a
// sample. Rows are ordered by coverage, then locale.
func (tb *PostgresStore) ConceptLocaleCoverage(ctx context.Context) ([]LocaleCoverage, error) {
	total, err := tb.Count(ctx)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}

	rows, err := tb.db.QueryContext(ctx,
		`SELECT t.locale, COUNT(DISTINCT t.concept_id) AS present
		 FROM tb_terms t
		 JOIN tb_concepts c ON c.workspace_id = t.workspace_id AND c.id = t.concept_id
		 WHERE t.workspace_id = $1 AND t.locale <> ''
		 GROUP BY t.locale
		 ORDER BY present DESC, t.locale`, tb.workspaceID)
	if err != nil {
		return nil, fmt.Errorf("concept locale coverage: %w", err)
	}
	defer rows.Close()

	var out []LocaleCoverage
	for rows.Next() {
		var locale string
		var present int
		if err := rows.Scan(&locale, &present); err != nil {
			return nil, fmt.Errorf("scan locale coverage: %w", err)
		}
		out = append(out, LocaleCoverage{
			Locale:  model.LocaleID(locale),
			Present: present,
			Total:   total,
			Pct:     coveragePct(present, total),
		})
	}
	return out, rows.Err()
}

// SearchRecent pages concepts newest-updated first. It is the LIKE path, whose
// ORDER BY is already the concept's last update; the trigram path ranks by
// similarity instead and cannot answer a recency page.
func (tb *PostgresStore) SearchRecent(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error) {
	return tb.pgSearchLike(ctx, query, sourceLocale, targetLocale, offset, limit)
}

// CountsFromConcepts is the in-Go equivalent of ConceptCounts, for a store
// without the database capability.
func CountsFromConcepts(concepts []fw.Concept) ConceptCounts {
	out := ConceptCounts{Total: len(concepts), ByStatus: map[model.TermStatus]int{}}
	for _, cp := range concepts {
		seen := map[model.TermStatus]bool{}
		for _, t := range cp.Terms {
			if t.Status == "" || seen[t.Status] {
				continue
			}
			seen[t.Status] = true
			out.ByStatus[t.Status]++
		}
	}
	return out
}

// CoverageFromConcepts is the in-Go equivalent of ConceptLocaleCoverage, for a
// store without the database capability.
func CoverageFromConcepts(concepts []fw.Concept) []LocaleCoverage {
	total := len(concepts)
	if total == 0 {
		return nil
	}
	present := map[model.LocaleID]int{}
	for _, cp := range concepts {
		seen := map[model.LocaleID]bool{}
		for _, t := range cp.Terms {
			if t.Locale == "" || seen[t.Locale] {
				continue
			}
			seen[t.Locale] = true
			present[t.Locale]++
		}
	}
	out := make([]LocaleCoverage, 0, len(present))
	for locale, n := range present {
		out = append(out, LocaleCoverage{
			Locale:  locale,
			Present: n,
			Total:   total,
			Pct:     coveragePct(n, total),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Present != out[j].Present {
			return out[i].Present > out[j].Present
		}
		return strings.Compare(string(out[i].Locale), string(out[j].Locale)) < 0
	})
	return out
}

// coveragePct is present/total as a rounded 0-100 integer.
func coveragePct(present, total int) int {
	if total == 0 {
		return 0
	}
	return int(math.Round(float64(present) / float64(total) * 100))
}
