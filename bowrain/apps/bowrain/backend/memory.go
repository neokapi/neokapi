package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// MemoryEntryInfo is the frontend-facing representation of a content-memory entry.
// The Bowrain desktop still exposes a bilingual shape over the API; it
// renders two locales at a time chosen by the user.
type MemoryEntryInfo struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	UpdatedAt    string `json:"updated_at"`
}

// MemorySearchResult holds a page of content-memory search results.
type MemorySearchResult struct {
	Entries    []MemoryEntryInfo `json:"entries"`
	TotalCount int               `json:"total_count"`
}

// MemoryUpdateRequest holds parameters for updating a content-memory entry.
type MemoryUpdateRequest struct {
	ProjectID    string `json:"project_id"`
	EntryID      string `json:"entry_id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
}

// getOrCreateMemory lazily initializes the app-level persistent SQLite content memory.
func (a *App) getOrCreateMemory() (*memory.SQLiteStore, error) {
	if a.tm != nil {
		return a.tm, nil
	}
	memoryPath := a.memoryPath
	if memoryPath == "" {
		memoryDir := filepath.Join(desktopConfigDir(), "tm")
		if err := os.MkdirAll(memoryDir, 0755); err != nil {
			return nil, fmt.Errorf("create tm dir: %w", err)
		}
		memoryPath = filepath.Join(memoryDir, "default.db")
	}
	tm, err := memory.NewSQLiteStore(memoryPath)
	if err != nil {
		return nil, err
	}
	a.tm = tm
	return tm, nil
}

// entryToInfo converts a memory.Entry to a MemoryEntryInfo for the bilingual
// view. It projects the entry's variants onto the requested (src, tgt) pair.
func entryToInfo(e memory.Entry, sourceLocale, targetLocale string) MemoryEntryInfo {
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
	return MemoryEntryInfo{
		ID:           e.ID,
		Source:       e.VariantText(srcLoc),
		Target:       e.VariantText(tgtLoc),
		SourceLocale: string(srcLoc),
		TargetLocale: string(tgtLoc),
		UpdatedAt:    e.UpdatedAt.Format(time.RFC3339),
	}
}

// GetMemoryEntries searches the content memory with optional query and locale filters.
func (a *App) GetMemoryEntries(projectID, query, sourceLocale, targetLocale string, offset, limit int) (*MemorySearchResult, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		result, err := client.GetMemoryEntries(context.Background(), ws, query, sourceLocale, targetLocale, offset, limit)
		if err != nil {
			a.goOffline()
			// Fall through to local content memory.
		} else {
			return editorMemoryResultToSearch(result), nil
		}
	}
	tm, err := a.getOrCreateMemory()
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}

	entries, total, err := tm.SearchEntries(context.Background(), memory.SearchParams{
		Query:         query,
		AnyLocale:     sourceLocale,
		RequireLocale: targetLocale,
		Offset:        offset,
		Limit:         limit,
	})
	if err != nil {
		return nil, err
	}
	infos := make([]MemoryEntryInfo, len(entries))
	for i, e := range entries {
		infos[i] = entryToInfo(e, sourceLocale, targetLocale)
	}

	return &MemorySearchResult{
		Entries:    infos,
		TotalCount: total,
	}, nil
}

// GetMemoryCount returns the total number of entries in the content memory.
func (a *App) GetMemoryCount(projectID string) (int, error) {
	if a.isConnected() {
		client, ws := a.editorRemote()
		count, err := client.GetMemoryCount(context.Background(), ws)
		if err != nil {
			a.goOffline()
		} else {
			return count, nil
		}
	}
	tm, err := a.getOrCreateMemory()
	if err != nil {
		return 0, fmt.Errorf("init content memory: %w", err)
	}

	return tm.Count(context.Background())
}

// UpdateMemoryEntry updates an existing content-memory entry. The server is authoritative for
// the content memory, so a successful online update skips the local mirror; the local write
// runs only offline (queued for replay) or in pure local mode.
func (a *App) UpdateMemoryEntry(req MemoryUpdateRequest) error {
	return a.writeThroughVoid(updateMemoryEntryOp{req},
		func() error {
			client, ws := a.editorRemote()
			return client.UpdateMemoryEntry(context.Background(), ws, req.EntryID, req.Source, req.Target, req.SourceLocale, req.TargetLocale)
		},
		func() error { return nil }, // server-authoritative: skip the local mirror on success
		func() error { return a.updateMemoryEntryLocal(req) },
	)
}

func (a *App) updateMemoryEntryLocal(req MemoryUpdateRequest) error {
	tm, err := a.getOrCreateMemory()
	if err != nil {
		return fmt.Errorf("init content memory: %w", err)
	}

	entry, ok, err := tm.GetEntry(context.Background(), req.EntryID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("content-memory entry %q not found", req.EntryID)
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

// DeleteMemoryEntry deletes a content-memory entry by ID.
func (a *App) DeleteMemoryEntry(projectID, entryID string) error {
	return a.writeThroughVoid(deleteMemoryEntryOp{EntryID: entryID},
		func() error {
			client, ws := a.editorRemote()
			return client.DeleteMemoryEntry(context.Background(), ws, entryID)
		},
		func() error { return nil }, // server-authoritative: skip the local mirror on success
		func() error {
			tm, err := a.getOrCreateMemory()
			if err != nil {
				return fmt.Errorf("init content memory: %w", err)
			}
			return tm.Delete(context.Background(), entryID)
		},
	)
}

// AddMemoryEntry adds a new entry to the content memory.
func (a *App) AddMemoryEntry(projectID, source, target, sourceLocale, targetLocale string) (*MemoryEntryInfo, error) {
	op := addMemoryEntryOp{Source: source, Target: target, SourceLocale: sourceLocale, TargetLocale: targetLocale}
	return writeThroughResult(a, op,
		func() (*MemoryEntryInfo, error) {
			client, ws := a.editorRemote()
			info, err := client.AddMemoryEntry(context.Background(), ws, source, target, sourceLocale, targetLocale)
			if err != nil {
				return nil, err
			}
			out := editorMemoryEntryToInfo(*info)
			return &out, nil
		},
		nil, // server returns the canonical entry; nothing to reconcile locally
		func() (*MemoryEntryInfo, error) {
			return a.addMemoryEntryLocal(source, target, sourceLocale, targetLocale)
		},
	)
}

func (a *App) addMemoryEntryLocal(source, target, sourceLocale, targetLocale string) (*MemoryEntryInfo, error) {
	tm, err := a.getOrCreateMemory()
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}

	now := time.Now()
	srcLoc := model.LocaleID(sourceLocale)
	tgtLoc := model.LocaleID(targetLocale)
	entry := memory.Entry{
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
