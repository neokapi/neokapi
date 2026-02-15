package backend

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/store"
	"github.com/gokapi/gokapi/lib/termbase"
	"github.com/google/uuid"
)

// TermInfo is the frontend-facing representation of a term.
type TermInfo struct {
	Text         string `json:"text"`
	Locale       string `json:"locale"`
	Status       string `json:"status"`
	PartOfSpeech string `json:"part_of_speech,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Note         string `json:"note,omitempty"`
}

// ConceptInfo is the frontend-facing representation of a concept.
type ConceptInfo struct {
	ID         string            `json:"id"`
	Domain     string            `json:"domain"`
	Definition string            `json:"definition"`
	Terms      []TermInfo        `json:"terms"`
	Properties map[string]string `json:"properties,omitempty"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

// TermSearchResult holds a page of term search results.
type TermSearchResult struct {
	Concepts   []ConceptInfo `json:"concepts"`
	TotalCount int           `json:"total_count"`
}

// TermLookupResult holds results from a term lookup.
type TermLookupResult struct {
	Matches []TermMatchInfo `json:"matches"`
}

// TermMatchInfo is the frontend-facing representation of a term match.
type TermMatchInfo struct {
	SourceTerm  string     `json:"source_term"`
	ConceptID   string     `json:"concept_id"`
	Domain      string     `json:"domain"`
	Score       float64    `json:"score"`
	MatchType   string     `json:"match_type"`
	Status      string     `json:"status"`
	TargetTerms []TermInfo `json:"target_terms"`
	Position    struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"position"`
}

// AddConceptRequest holds parameters for adding a concept.
type AddConceptRequest struct {
	ProjectID  string     `json:"project_id"`
	Domain     string     `json:"domain"`
	Definition string     `json:"definition"`
	Terms      []TermInfo `json:"terms"`
}

// UpdateConceptRequest holds parameters for updating a concept.
type UpdateConceptRequest struct {
	ProjectID  string     `json:"project_id"`
	ConceptID  string     `json:"concept_id"`
	Domain     string     `json:"domain"`
	Definition string     `json:"definition"`
	Terms      []TermInfo `json:"terms"`
}

// getOrCreateTB lazily initializes the app-level in-memory termbase.
func (a *App) getOrCreateTB() *termbase.InMemoryTermBase {
	if a.tb != nil {
		return a.tb
	}
	a.tb = termbase.NewInMemoryTermBase()
	return a.tb
}

func conceptToInfo(c termbase.Concept) ConceptInfo {
	terms := make([]TermInfo, len(c.Terms))
	for i, t := range c.Terms {
		terms[i] = TermInfo{
			Text:         t.Text,
			Locale:       string(t.Locale),
			Status:       string(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
	}
	return ConceptInfo{
		ID:         c.ID,
		Domain:     c.Domain,
		Definition: c.Definition,
		Terms:      terms,
		Properties: c.Properties,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

func termsFromInfo(terms []TermInfo) []termbase.Term {
	result := make([]termbase.Term, len(terms))
	for i, t := range terms {
		result[i] = termbase.Term{
			Text:         t.Text,
			Locale:       model.LocaleID(t.Locale),
			Status:       model.TermStatus(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
		if result[i].Status == "" {
			result[i].Status = model.TermApproved
		}
	}
	return result
}

// GetTerms searches the termbase.
func (a *App) GetTerms(projectID, query, sourceLocale, targetLocale string, offset, limit int) (*TermSearchResult, error) {
	tb := a.getOrCreateTB()
	results, total := tb.Search(query, sourceLocale, targetLocale, offset, limit)
	infos := make([]ConceptInfo, len(results))
	for i, c := range results {
		infos[i] = conceptToInfo(c)
	}
	return &TermSearchResult{
		Concepts:   infos,
		TotalCount: total,
	}, nil
}

// GetTermCount returns the total number of concepts in the termbase.
func (a *App) GetTermCount(projectID string) (int, error) {
	return a.getOrCreateTB().Count(), nil
}

// AddConcept adds a new concept to the termbase.
func (a *App) AddConcept(req AddConceptRequest) (*ConceptInfo, error) {
	tb := a.getOrCreateTB()
	concept := termbase.Concept{
		ID:         uuid.New().String(),
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      termsFromInfo(req.Terms),
	}

	if err := tb.AddConcept(concept); err != nil {
		return nil, err
	}

	// Retrieve to get normalized timestamps.
	stored, _ := tb.GetConcept(concept.ID)
	info := conceptToInfo(stored)
	return &info, nil
}

// UpdateConcept updates an existing concept in the termbase.
func (a *App) UpdateConcept(req UpdateConceptRequest) error {
	tb := a.getOrCreateTB()
	concept := termbase.Concept{
		ID:         req.ConceptID,
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      termsFromInfo(req.Terms),
	}

	return tb.AddConcept(concept)
}

// DeleteConcept removes a concept from the termbase.
func (a *App) DeleteConcept(projectID, conceptID string) error {
	tb := a.getOrCreateTB()
	return tb.DeleteConcept(conceptID)
}

// LookupTerms looks up terms matching the given text.
func (a *App) LookupTerms(projectID, text, sourceLocale, targetLocale string) (*TermLookupResult, error) {
	tb := a.getOrCreateTB()
	matches := tb.LookupAll(text, termbase.LookupOptions{
		SourceLocale: model.LocaleID(sourceLocale),
	})

	infos := make([]TermMatchInfo, len(matches))
	for i, m := range matches {
		var targetTerms []TermInfo
		for _, t := range m.Concept.TargetTerms(model.LocaleID(targetLocale)) {
			targetTerms = append(targetTerms, TermInfo{
				Text:   t.Text,
				Locale: string(t.Locale),
				Status: string(t.Status),
			})
		}
		infos[i] = TermMatchInfo{
			SourceTerm:  m.Term.Text,
			ConceptID:   m.Concept.ID,
			Domain:      m.Concept.Domain,
			Score:       m.Score,
			MatchType:   string(m.MatchType),
			Status:      string(m.Term.Status),
			TargetTerms: targetTerms,
		}
		infos[i].Position.Start = m.Position.Start
		infos[i].Position.End = m.Position.End
	}

	return &TermLookupResult{Matches: infos}, nil
}

// ImportTermsCSV imports terms from CSV content into the termbase.
func (a *App) ImportTermsCSV(projectID, csvContent, sourceLocale, targetLocale, domain string, hasHeader bool) (int, error) {
	tb := a.getOrCreateTB()
	count, err := termbase.ImportCSV(tb, strings.NewReader(csvContent), termbase.CSVImportOptions{
		SourceLocale: model.LocaleID(sourceLocale),
		TargetLocale: model.LocaleID(targetLocale),
		Domain:       domain,
		HasHeader:    hasHeader,
	})
	if err != nil {
		return 0, fmt.Errorf("import CSV: %w", err)
	}

	return count, nil
}

// ImportTermsJSON imports terms from JSON content into the termbase.
func (a *App) ImportTermsJSON(projectID, jsonContent string) (int, error) {
	tb := a.getOrCreateTB()
	count, err := termbase.ImportJSON(tb, strings.NewReader(jsonContent))
	if err != nil {
		return 0, fmt.Errorf("import JSON: %w", err)
	}

	return count, nil
}

// ExportTermsJSON exports the termbase as JSON.
func (a *App) ExportTermsJSON(projectID, name string) (string, error) {
	tb := a.getOrCreateTB()
	var buf bytes.Buffer
	if err := termbase.ExportJSON(tb, &buf, name); err != nil {
		return "", fmt.Errorf("export JSON: %w", err)
	}

	return buf.String(), nil
}

// TermEnforceItem runs terminology enforcement on a project item.
func (a *App) TermEnforceItem(projectID, itemName, targetLocale string) ([]TermEnforceResult, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tb := a.getOrCreateTB()
	if tb.Count() == 0 {
		return nil, nil
	}

	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	srcLocale := proj.SourceLocale
	tgtLocale := model.LocaleID(targetLocale)

	var results []TermEnforceResult
	for _, sb := range storedBlocks {
		block := sb.Block
		if !block.Translatable {
			continue
		}
		if !block.HasTarget(tgtLocale) {
			continue
		}

		sourceText := block.SourceText()
		targetText := block.TargetText(tgtLocale)

		matches := tb.LookupAll(sourceText, termbase.LookupOptions{
			SourceLocale: srcLocale,
			StatusFilter: []model.TermStatus{model.TermPreferred, model.TermApproved},
		})

		for _, m := range matches {
			targetTerms := m.Concept.TargetTerms(tgtLocale)
			if len(targetTerms) == 0 {
				continue
			}
			found := false
			for _, tt := range targetTerms {
				if strings.Contains(strings.ToLower(targetText), strings.ToLower(tt.Text)) {
					found = true
					break
				}
			}
			if !found {
				var expected []string
				for _, tt := range targetTerms {
					expected = append(expected, tt.Text)
				}
				results = append(results, TermEnforceResult{
					BlockID:      block.ID,
					SourceTerm:   m.Term.Text,
					ConceptID:    m.Concept.ID,
					Expected:     expected,
					SourceText:   sourceText,
					TargetText:   targetText,
					SourceLocale: string(srcLocale),
					TargetLocale: string(tgtLocale),
				})
			}
		}
	}

	return results, nil
}

// TermEnforceResult represents a terminology violation in a block.
type TermEnforceResult struct {
	BlockID      string   `json:"block_id"`
	SourceTerm   string   `json:"source_term"`
	ConceptID    string   `json:"concept_id"`
	Expected     []string `json:"expected"`
	SourceText   string   `json:"source_text"`
	TargetText   string   `json:"target_text"`
	SourceLocale string   `json:"source_locale"`
	TargetLocale string   `json:"target_locale"`
}
