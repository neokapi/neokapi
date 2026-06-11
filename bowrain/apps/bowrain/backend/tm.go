package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// TMEntryInfo is the frontend-facing representation of a TM entry.
// The Bowrain desktop still exposes a bilingual shape over the API; it
// renders two locales at a time chosen by the user.
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
		tmDir := filepath.Join(desktopConfigDir(), "tm")
		if err := os.MkdirAll(tmDir, 0755); err != nil {
			return nil, fmt.Errorf("create tm dir: %w", err)
		}
		tmPath = filepath.Join(tmDir, "default.db")
	}
	tm, err := sievepen.NewSQLiteTM(tmPath)
	if err != nil {
		return nil, err
	}
	a.tm = tm
	return tm, nil
}

// entryToInfo converts a sievepen.TMEntry to a TMEntryInfo for the bilingual
// view. It projects the entry's variants onto the requested (src, tgt) pair.
func entryToInfo(e sievepen.TMEntry, sourceLocale, targetLocale string) TMEntryInfo {
	srcLoc := model.LocaleID(sourceLocale)
	tgtLoc := model.LocaleID(targetLocale)
	if srcLoc == "" && e.HintSrcLang != "" {
		srcLoc = e.HintSrcLang
	}
	// Pick any other locale for target if not specified.
	if tgtLoc == "" {
		for loc := range e.Variants {
			if loc != srcLoc {
				tgtLoc = loc
				break
			}
		}
	}
	return TMEntryInfo{
		ID:           e.ID,
		Source:       e.VariantText(srcLoc),
		Target:       e.VariantText(tgtLoc),
		SourceLocale: string(srcLoc),
		TargetLocale: string(tgtLoc),
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

	entries, total, err := tm.SearchEntries(context.Background(), sievepen.SearchParams{
		Query:         query,
		AnyLocale:     sourceLocale,
		RequireLocale: targetLocale,
		Offset:        offset,
		Limit:         limit,
	})
	if err != nil {
		return nil, err
	}
	infos := make([]TMEntryInfo, len(entries))
	for i, e := range entries {
		infos[i] = entryToInfo(e, sourceLocale, targetLocale)
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

	return tm.Count(context.Background())
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

	entry, ok, err := tm.GetEntry(context.Background(), req.EntryID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("TM entry %q not found", req.EntryID)
	}

	srcLoc := model.LocaleID(req.SourceLocale)
	tgtLoc := model.LocaleID(req.TargetLocale)
	if entry.Variants == nil {
		entry.Variants = make(map[model.LocaleID][]model.Run)
	}
	entry.Variants[srcLoc] = []model.Run{{Text: &model.TextRun{Text: req.Source}}}
	entry.Variants[tgtLoc] = []model.Run{{Text: &model.TextRun{Text: req.Target}}}
	if entry.HintSrcLang == "" {
		entry.HintSrcLang = srcLoc
	}
	entry.UpdatedAt = time.Now()

	return tm.Add(context.Background(), entry)
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

	return tm.Delete(context.Background(), entryID)
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
	srcLoc := model.LocaleID(sourceLocale)
	tgtLoc := model.LocaleID(targetLocale)
	entry := sievepen.TMEntry{
		ID: id.New(),
		Variants: map[model.LocaleID][]model.Run{
			srcLoc: {{Text: &model.TextRun{Text: source}}},
			tgtLoc: {{Text: &model.TextRun{Text: target}}},
		},
		HintSrcLang: srcLoc,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := tm.Add(context.Background(), entry); err != nil {
		return nil, err
	}

	info := entryToInfo(entry, sourceLocale, targetLocale)
	return &info, nil
}
