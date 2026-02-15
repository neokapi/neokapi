package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/sievepen"
	"github.com/google/uuid"
)

// TMEntryInfo is the frontend-facing representation of a TM entry.
type TMEntryInfo struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	UpdatedAt    string `json:"updated_at"`
}

// TMSearchResult holds a page of TM search results.
type TMSearchResult struct {
	Entries    []TMEntryInfo `json:"entries"`
	TotalCount int           `json:"total_count"`
}

// TMUpdateRequest holds parameters for updating a TM entry.
type TMUpdateRequest struct {
	ProjectID    string `json:"project_id"`
	EntryID      string `json:"entry_id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
}

// getOrCreateTM lazily initializes the app-level persistent SQLite TM.
func (a *App) getOrCreateTM() (*sievepen.SQLiteTM, error) {
	if a.tm != nil {
		return a.tm, nil
	}
	tmPath := a.tmPath
	if tmPath == "" {
		home, _ := os.UserHomeDir()
		tmDir := filepath.Join(home, ".config", "gokapi", "tm")
		os.MkdirAll(tmDir, 0755)
		tmPath = filepath.Join(tmDir, "default.db")
	}
	tm, err := sievepen.NewSQLiteTM(tmPath)
	if err != nil {
		return nil, err
	}
	a.tm = tm
	return tm, nil
}

// entryToInfo converts a sievepen.TMEntry to a TMEntryInfo.
func entryToInfo(e sievepen.TMEntry) TMEntryInfo {
	return TMEntryInfo{
		ID:           e.ID,
		Source:       e.SourceText(),
		Target:       e.TargetText(),
		SourceLocale: string(e.SourceLocale),
		TargetLocale: string(e.TargetLocale),
		UpdatedAt:    e.UpdatedAt.Format(time.RFC3339),
	}
}

// GetTMEntries searches the TM with optional query and locale filters.
func (a *App) GetTMEntries(projectID, query, sourceLocale, targetLocale string, offset, limit int) (*TMSearchResult, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		result, err := a.remote.GetTMEntries(ws, query, sourceLocale, targetLocale, offset, limit)
		if err != nil {
			a.goOffline()
			// Fall through to local TM.
		} else {
			return result, nil
		}
	}
	tm, err := a.getOrCreateTM()
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
	}

	entries, total := tm.SearchEntries(query, sourceLocale, targetLocale, offset, limit)
	infos := make([]TMEntryInfo, len(entries))
	for i, e := range entries {
		infos[i] = entryToInfo(e)
	}

	return &TMSearchResult{
		Entries:    infos,
		TotalCount: total,
	}, nil
}

// GetTMCount returns the total number of entries in the TM.
func (a *App) GetTMCount(projectID string) (int, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		count, err := a.remote.GetTMCount(ws)
		if err != nil {
			a.goOffline()
		} else {
			return count, nil
		}
	}
	tm, err := a.getOrCreateTM()
	if err != nil {
		return 0, fmt.Errorf("init TM: %w", err)
	}

	return tm.Count(), nil
}

// UpdateTMEntry updates an existing TM entry.
func (a *App) UpdateTMEntry(req TMUpdateRequest) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		err := a.remote.UpdateTMEntry(ws, req.EntryID, req.Source, req.Target, req.SourceLocale, req.TargetLocale)
		if err != nil {
			a.goOffline()
			a.enqueue("update_tm_entry", req)
		} else {
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("update_tm_entry", req)
	}
	tm, err := a.getOrCreateTM()
	if err != nil {
		return fmt.Errorf("init TM: %w", err)
	}

	entry, ok := tm.GetEntry(req.EntryID)
	if !ok {
		return fmt.Errorf("TM entry %q not found", req.EntryID)
	}

	entry.Source = model.NewFragment(req.Source)
	entry.Target = model.NewFragment(req.Target)
	entry.SourceLocale = model.LocaleID(req.SourceLocale)
	entry.TargetLocale = model.LocaleID(req.TargetLocale)
	entry.UpdatedAt = time.Now()

	return tm.Add(entry)
}

// DeleteTMEntry deletes a TM entry by ID.
func (a *App) DeleteTMEntry(projectID, entryID string) error {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		err := a.remote.DeleteTMEntry(ws, entryID)
		if err != nil {
			a.goOffline()
			a.enqueue("delete_tm_entry", deleteTMPayload{EntryID: entryID})
		} else {
			return nil
		}
	} else if a.isOffline() {
		a.enqueue("delete_tm_entry", deleteTMPayload{EntryID: entryID})
	}
	tm, err := a.getOrCreateTM()
	if err != nil {
		return fmt.Errorf("init TM: %w", err)
	}

	return tm.Delete(entryID)
}

// AddTMEntry adds a new entry to the TM.
func (a *App) AddTMEntry(projectID, source, target, sourceLocale, targetLocale string) (*TMEntryInfo, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		info, err := a.remote.AddTMEntry(ws, source, target, sourceLocale, targetLocale)
		if err != nil {
			a.goOffline()
			a.enqueue("add_tm_entry", addTMPayload{
				Source: source, Target: target, SourceLocale: sourceLocale, TargetLocale: targetLocale,
			})
			// Fall through to local.
		} else {
			return info, nil
		}
	} else if a.isOffline() {
		a.enqueue("add_tm_entry", addTMPayload{
			Source: source, Target: target, SourceLocale: sourceLocale, TargetLocale: targetLocale,
		})
	}
	tm, err := a.getOrCreateTM()
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
	}

	now := time.Now()
	entry := sievepen.TMEntry{
		ID:           uuid.New().String(),
		Source:       model.NewFragment(source),
		Target:       model.NewFragment(target),
		SourceLocale: model.LocaleID(sourceLocale),
		TargetLocale: model.LocaleID(targetLocale),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := tm.Add(entry); err != nil {
		return nil, err
	}

	info := entryToInfo(entry)
	return &info, nil
}
