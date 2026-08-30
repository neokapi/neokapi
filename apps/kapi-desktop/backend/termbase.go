package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// --- DTOs ---

// ConceptDTO is the frontend-facing terms concept.
type ConceptDTO struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Domain     string            `json:"domain"`
	Definition string            `json:"definition"`
	Source     string            `json:"source"` // "terminology" or "brand_vocabulary"
	Terms      []TermDTO         `json:"terms"`
	Properties map[string]string `json:"properties,omitempty"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

// TermDTO is the frontend-facing term within a concept.
type TermDTO struct {
	Text           string `json:"text"`
	Locale         string `json:"locale"`
	Status         string `json:"status"` // preferred, approved, admitted, proposed, deprecated, forbidden
	PartOfSpeech   string `json:"part_of_speech,omitempty"`
	Gender         string `json:"gender,omitempty"`
	Note           string `json:"note,omitempty"`
	CompetitorTerm bool   `json:"competitor_term,omitempty"`
	// Validity carries the term's temporal/tag scoping (the constraints +
	// derived-geography axis the concept dashboard renders). nil = always valid.
	Validity *ValidityDTO `json:"validity,omitempty"`
}

// TermSearchResult is the paginated result from SearchTerms.
type TermSearchResult struct {
	Concepts   []ConceptDTO `json:"concepts"`
	TotalCount int          `json:"total_count"`
}

// TermsStats is the stats response for an open terms.
type TermsStats struct {
	Count int    `json:"count"`
	Path  string `json:"path"`
}

// AddConceptRequest is the request to add a new concept.
type AddConceptRequest struct {
	ProjectID  string    `json:"project_id"`
	Domain     string    `json:"domain"`
	Definition string    `json:"definition"`
	Terms      []TermDTO `json:"terms"`
}

// UpdateConceptRequest is the request to update a concept.
type UpdateConceptRequest struct {
	ConceptID  string    `json:"concept_id"`
	ProjectID  string    `json:"project_id"`
	Domain     string    `json:"domain"`
	Definition string    `json:"definition"`
	Terms      []TermDTO `json:"terms"`
}

// --- Conversion helpers ---

func conceptToDTO(c terms.Concept) ConceptDTO {
	terms := make([]TermDTO, 0, len(c.Terms))
	for _, t := range c.Terms {
		terms = append(terms, TermDTO{
			Text:           t.Text,
			Locale:         string(t.Locale),
			Status:         string(t.Status),
			PartOfSpeech:   t.PartOfSpeech,
			Gender:         t.Gender,
			Note:           t.Note,
			CompetitorTerm: t.CompetitorTerm,
			Validity:       validityToDTO(t.Validity),
		})
	}
	return ConceptDTO{
		ID:         c.ID,
		ProjectID:  c.ProjectID,
		Domain:     c.Domain,
		Definition: c.Definition,
		Source:     string(c.Source),
		Terms:      terms,
		Properties: c.Properties,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

func dtoToTerms(dtos []TermDTO) []terms.Term {
	ts := make([]terms.Term, 0, len(dtos))
	for _, d := range dtos {
		ts = append(ts, terms.Term{
			Text:           d.Text,
			Locale:         model.LocaleID(d.Locale),
			Status:         model.TermStatus(d.Status),
			PartOfSpeech:   d.PartOfSpeech,
			Gender:         d.Gender,
			Note:           d.Note,
			CompetitorTerm: d.CompetitorTerm,
			Validity:       validityFromDTO(d.Validity),
		})
	}
	return ts
}

// --- Resource discovery ---

// ListNamedTerms returns the named terms stores under the config dir's terms/.
func (a *App) ListNamedTerms() []ResourceInfo {
	return listNamedResources("terms")
}

// --- Lifecycle ---

// OpenTerms opens a SQLite terms file and returns a handle ID.
func (a *App) OpenTerms(path string) (string, error) {
	tb, err := terms.NewSQLiteStore(path)
	if err != nil {
		return "", fmt.Errorf("open terms %q: %w", path, err)
	}
	return a.tbHandles.Open(tb), nil
}

// OpenTermsDialog shows a native file dialog to open a terms store.
func (a *App) OpenTermsDialog() (string, error) {
	if a.app == nil {
		return "", nil
	}
	path, err := a.app.Dialog.OpenFile().
		AddFilter("Termbases", "*.db").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	return a.OpenTerms(path)
}

// CreateTerms creates a new empty terms at the given path.
func (a *App) CreateTerms(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	return a.OpenTerms(path)
}

// CreateNamedTerms creates a named terms store under the config dir's terms/.
func (a *App) CreateNamedTerms(name string) (string, error) {
	dir := namedResourceDir("terms")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create terms stores directory: %w", err)
	}
	path := filepath.Join(dir, name+".db")
	return a.OpenTerms(path)
}

// CloseTerms closes an open terms by handle.
func (a *App) CloseTerms(handle string) {
	_ = a.tbHandles.Close(handle)
}

// GetTermsStats returns stats for an open terms.
func (a *App) GetTermsStats(handle string) *TermsStats {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil
	}
	count, err := tb.Count(context.Background())
	if err != nil {
		return nil
	}
	return &TermsStats{Count: count}
}

// GetTermsActivityStats returns daily concept counts over time.
func (a *App) GetTermsActivityStats(handle string) []terms.ActivityStat {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil
	}
	stats, err := tb.ActivityStats(context.Background())
	if err != nil {
		return nil
	}
	return stats
}

// GetTermsLocaleStats returns term counts grouped by locale.
func (a *App) GetTermsLocaleStats(handle string) []terms.LocaleStat {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil
	}
	stats, err := tb.LocaleStats(context.Background())
	if err != nil {
		return nil
	}
	return stats
}

// --- CRUD ---

// SearchTerms searches terms concepts by query with pagination.
func (a *App) SearchTerms(handle, query, srcLocale, tgtLocale string, offset, limit int) *TermSearchResult {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return &TermSearchResult{}
	}
	concepts, total, err := tb.Search(context.Background(), query, model.LocaleID(srcLocale), model.LocaleID(tgtLocale), offset, limit)
	if err != nil {
		return &TermSearchResult{}
	}
	dtos := make([]ConceptDTO, 0, len(concepts))
	for _, c := range concepts {
		dtos = append(dtos, conceptToDTO(c))
	}
	return &TermSearchResult{Concepts: dtos, TotalCount: total}
}

// GetConcept returns a single concept by ID.
func (a *App) GetConcept(handle, conceptID string) *ConceptDTO {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil
	}
	concept, found, err := tb.GetConcept(context.Background(), conceptID)
	if err != nil || !found {
		return nil
	}
	dto := conceptToDTO(concept)
	return &dto
}

// AddConcept adds a new concept to the terms store.
func (a *App) AddConcept(handle string, req AddConceptRequest) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("terms handle %q not found", handle)
	}
	concept := terms.Concept{
		ID:         id.New(),
		ProjectID:  req.ProjectID,
		Domain:     req.Domain,
		Definition: req.Definition,
		Source:     terms.TermSourceTerminology,
		Terms:      dtoToTerms(req.Terms),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	return tb.AddConcept(context.Background(), concept)
}

// UpdateConcept updates an existing concept.
func (a *App) UpdateConcept(handle string, req UpdateConceptRequest) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("terms handle %q not found", handle)
	}
	existing, found, err := tb.GetConcept(context.Background(), req.ConceptID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("concept %q not found", req.ConceptID)
	}
	existing.ProjectID = req.ProjectID
	existing.Domain = req.Domain
	existing.Definition = req.Definition
	existing.Terms = dtoToTerms(req.Terms)
	existing.UpdatedAt = time.Now()
	return tb.AddConcept(context.Background(), existing) // AddConcept with same ID = update
}

// DeleteConcept deletes a single concept.
func (a *App) DeleteConcept(handle, conceptID string) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("terms handle %q not found", handle)
	}
	return tb.DeleteConcept(context.Background(), conceptID)
}

// DeleteConcepts deletes multiple concepts.
func (a *App) DeleteConcepts(handle string, conceptIDs []string) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("terms handle %q not found", handle)
	}
	for _, cid := range conceptIDs {
		if err := tb.DeleteConcept(context.Background(), cid); err != nil {
			return err
		}
	}
	return nil
}
