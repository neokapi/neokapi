package backend

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
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

// getOrCreateTB lazily initializes the app-level persistent SQLite termbase.
func (a *App) getOrCreateTB() (*termbase.SQLiteTermBase, error) {
	if a.tb != nil {
		return a.tb, nil
	}
	tbDir := filepath.Join(desktopConfigDir(), "termbase")
	os.MkdirAll(tbDir, 0755)
	tbPath := filepath.Join(tbDir, "default.db")
	tb, err := termbase.NewSQLiteTermBase(tbPath)
	if err != nil {
		return nil, err
	}
	a.tb = tb
	return tb, nil
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
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		result, err := a.remote.GetTerms(ws, query, sourceLocale, targetLocale, offset, limit)
		if err != nil {
			a.goOffline()
		} else {
			return result, nil
		}
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return nil, fmt.Errorf("init termbase: %w", err)
	}
	results, total, err := tb.Search(context.Background(), query, model.LocaleID(sourceLocale), model.LocaleID(targetLocale), offset, limit)
	if err != nil {
		return nil, err
	}
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
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		count, err := a.remote.GetTermCount(ws)
		if err != nil {
			a.goOffline()
		} else {
			return count, nil
		}
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return 0, fmt.Errorf("init termbase: %w", err)
	}
	return tb.Count(context.Background())
}

// AddConcept adds a new concept to the termbase.
func (a *App) AddConcept(req AddConceptRequest) (*ConceptInfo, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		info, err := a.remote.AddConcept(ws, req.Domain, req.Definition, req.Terms)
		if err != nil {
			a.goOffline()
			a.enqueue("add_concept", req)
		} else {
			return info, nil
		}
	} else if a.isOffline() {
		a.enqueue("add_concept", req)
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return nil, fmt.Errorf("init termbase: %w", err)
	}
	concept := termbase.Concept{
		ID:         id.New(),
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      termsFromInfo(req.Terms),
	}

	if err := tb.AddConcept(context.Background(), concept); err != nil {
		return nil, err
	}

	// Retrieve to get normalized timestamps.
	stored, _, _ := tb.GetConcept(context.Background(), concept.ID)
	info := conceptToInfo(stored)
	return &info, nil
}

// UpdateConcept updates an existing concept in the termbase.
func (a *App) UpdateConcept(req UpdateConceptRequest) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		err := a.remote.UpdateConcept(ws, req.ConceptID, req.Domain, req.Definition, req.Terms)
		if err != nil {
			a.goOffline()
			a.enqueue("update_concept", req)
		} else {
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("update_concept", req)
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return fmt.Errorf("init termbase: %w", err)
	}
	concept := termbase.Concept{
		ID:         req.ConceptID,
		Domain:     req.Domain,
		Definition: req.Definition,
		Terms:      termsFromInfo(req.Terms),
	}

	return tb.AddConcept(context.Background(), concept)
}

// DeleteConcept removes a concept from the termbase.
func (a *App) DeleteConcept(projectID, conceptID string) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		err := a.remote.DeleteConcept(ws, conceptID)
		if err != nil {
			a.goOffline()
			a.enqueue("delete_concept", deleteConceptPayload{ConceptID: conceptID})
		} else {
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("delete_concept", deleteConceptPayload{ConceptID: conceptID})
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return fmt.Errorf("init termbase: %w", err)
	}
	return tb.DeleteConcept(context.Background(), conceptID)
}

// LookupTerms looks up terms matching the given text.
func (a *App) LookupTerms(projectID, text, sourceLocale, targetLocale string) (*TermLookupResult, error) {
	tb, err := a.getOrCreateTB()
	if err != nil {
		return nil, fmt.Errorf("init termbase: %w", err)
	}
	matches, err := tb.LookupAll(context.Background(), text, termbase.LookupOptions{
		SourceLocale: model.LocaleID(sourceLocale),
	})
	if err != nil {
		return nil, err
	}

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
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		count, err := a.remote.ImportTermsCSV(ws, csvContent, sourceLocale, targetLocale, domain, hasHeader)
		if err != nil {
			a.goOffline()
		} else {
			return count, nil
		}
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return 0, fmt.Errorf("init termbase: %w", err)
	}
	count, err := termbase.ImportCSV(context.Background(), tb, strings.NewReader(csvContent), termbase.CSVImportOptions{
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
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		count, err := a.remote.ImportTermsJSON(ws, jsonContent)
		if err != nil {
			a.goOffline()
		} else {
			return count, nil
		}
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return 0, fmt.Errorf("init termbase: %w", err)
	}
	count, err := termbase.ImportJSON(context.Background(), tb, strings.NewReader(jsonContent))
	if err != nil {
		return 0, fmt.Errorf("import JSON: %w", err)
	}

	return count, nil
}

// ExportTermsJSON exports the termbase as JSON.
func (a *App) ExportTermsJSON(projectID, name string) (string, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		result, err := a.remote.ExportTermsJSON(ws, name)
		if err != nil {
			a.goOffline()
		} else {
			return result, nil
		}
	}
	tb, err := a.getOrCreateTB()
	if err != nil {
		return "", fmt.Errorf("init termbase: %w", err)
	}
	var buf bytes.Buffer
	if err := termbase.ExportJSON(context.Background(), tb, &buf, name); err != nil {
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

	tb, err := a.getOrCreateTB()
	if err != nil {
		return nil, fmt.Errorf("init termbase: %w", err)
	}
	if count, cerr := tb.Count(ctx); cerr != nil {
		return nil, cerr
	} else if count == 0 {
		return nil, nil
	}

	storedBlocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	srcLocale := proj.DefaultSourceLanguage
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

		matches, err := tb.LookupAll(ctx, sourceText, termbase.LookupOptions{
			SourceLocale: srcLocale,
			StatusFilter: []model.TermStatus{model.TermPreferred, model.TermApproved},
		})
		if err != nil {
			return nil, err
		}

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
