package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// --- DTOs ---

// ConceptDTO is the frontend-facing termbase concept.
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

// TermbaseStats is the stats response for an open termbase.
type TermbaseStats struct {
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

func conceptToDTO(c termbase.Concept) ConceptDTO {
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

func dtoToTerms(dtos []TermDTO) []termbase.Term {
	terms := make([]termbase.Term, 0, len(dtos))
	for _, d := range dtos {
		terms = append(terms, termbase.Term{
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
	return terms
}

// --- Resource discovery ---

// ListNamedTermbases returns named termbases from KAPI_HOME/termbases/.
func (a *App) ListNamedTermbases() []ResourceInfo {
	return listNamedResources("termbases")
}

// --- Lifecycle ---

// OpenTermbase opens a SQLite termbase file and returns a handle ID.
func (a *App) OpenTermbase(path string) (string, error) {
	tb, err := termbase.NewSQLiteTermBase(path)
	if err != nil {
		return "", fmt.Errorf("open termbase %q: %w", path, err)
	}
	return a.tbHandles.Open(tb), nil
}

// OpenTermbaseDialog shows a native file dialog to open a termbase.
func (a *App) OpenTermbaseDialog() (string, error) {
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
	return a.OpenTermbase(path)
}

// CreateTermbase creates a new empty termbase at the given path.
func (a *App) CreateTermbase(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	return a.OpenTermbase(path)
}

// CreateNamedTermbase creates a new named termbase in KAPI_HOME/termbases/.
func (a *App) CreateNamedTermbase(name string) (string, error) {
	dir := namedResourceDir("termbases")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create termbases directory: %w", err)
	}
	path := filepath.Join(dir, name+".db")
	return a.OpenTermbase(path)
}

// CloseTermbase closes an open termbase by handle.
func (a *App) CloseTermbase(handle string) {
	_ = a.tbHandles.Close(handle)
}

// GetTermbaseStats returns stats for an open termbase.
func (a *App) GetTermbaseStats(handle string) *TermbaseStats {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil
	}
	count, err := tb.Count(context.Background())
	if err != nil {
		return nil
	}
	return &TermbaseStats{Count: count}
}

// GetTermbaseActivityStats returns daily concept counts over time.
func (a *App) GetTermbaseActivityStats(handle string) []termbase.ActivityStat {
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

// GetTermbaseLocaleStats returns term counts grouped by locale.
func (a *App) GetTermbaseLocaleStats(handle string) []termbase.LocaleStat {
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

// SearchTerms searches termbase concepts by query with pagination.
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

// AddConcept adds a new concept to the termbase.
func (a *App) AddConcept(handle string, req AddConceptRequest) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("termbase handle %q not found", handle)
	}
	concept := termbase.Concept{
		ID:         id.New(),
		ProjectID:  req.ProjectID,
		Domain:     req.Domain,
		Definition: req.Definition,
		Source:     termbase.TermSourceTerminology,
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
		return fmt.Errorf("termbase handle %q not found", handle)
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
		return fmt.Errorf("termbase handle %q not found", handle)
	}
	return tb.DeleteConcept(context.Background(), conceptID)
}

// DeleteConcepts deletes multiple concepts.
func (a *App) DeleteConcepts(handle string, conceptIDs []string) error {
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("termbase handle %q not found", handle)
	}
	for _, cid := range conceptIDs {
		if err := tb.DeleteConcept(context.Background(), cid); err != nil {
			return err
		}
	}
	return nil
}

// --- Import / Export ---

// ImportTermbaseCSVDialog shows a file dialog and imports a CSV termbase.
func (a *App) ImportTermbaseCSVDialog(handle, srcLocale, tgtLocale, domain string) (*ImportResult, error) {
	if a.app == nil {
		return nil, nil
	}
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("termbase handle %q not found", handle)
	}

	path, err := a.app.Dialog.OpenFile().
		AddFilter("CSV/TSV Files", "*.csv;*.tsv").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open CSV: %w", err)
	}
	defer f.Close()

	count, err := termbase.ImportCSV(context.Background(), tb, f, termbase.CSVImportOptions{
		SourceLocale: model.LocaleID(srcLocale),
		TargetLocale: model.LocaleID(tgtLocale),
		Domain:       domain,
		HasHeader:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("import CSV: %w", err)
	}
	return &ImportResult{Count: count}, nil
}

// ImportTermbaseJSONDialog shows a file dialog and imports a JSON termbase.
func (a *App) ImportTermbaseJSONDialog(handle string) (*ImportResult, error) {
	if a.app == nil {
		return nil, nil
	}
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("termbase handle %q not found", handle)
	}

	path, err := a.app.Dialog.OpenFile().
		AddFilter("JSON Files", "*.json").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open JSON: %w", err)
	}
	defer f.Close()

	count, err := termbase.ImportJSON(context.Background(), tb, f)
	if err != nil {
		return nil, fmt.Errorf("import JSON: %w", err)
	}
	return &ImportResult{Count: count}, nil
}

// ExportTermbaseJSONDialog shows a save dialog and exports the termbase as JSON.
func (a *App) ExportTermbaseJSONDialog(handle, name string) error {
	if a.app == nil {
		return nil
	}
	tb, ok := a.tbHandles.Get(handle)
	if !ok {
		return fmt.Errorf("termbase handle %q not found", handle)
	}

	path, err := a.app.Dialog.SaveFile().
		AddFilter("JSON Files", "*.json").
		SetFilename(name + "-termbase.json").
		PromptForSingleSelection()
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if !strings.HasSuffix(strings.ToLower(path), ".json") {
		path += ".json"
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create JSON: %w", err)
	}
	defer f.Close()

	return termbase.ExportJSON(context.Background(), tb, f, name)
}
