package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
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
	// Revision is the server's revision of the entry, which an edit names.
	Revision string `json:"revision,omitempty"`
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
	// BaseRevision is the revision of the entry the edit was made on.
	BaseRevision string `json:"base_revision,omitempty"`
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
		Revision:     e.Properties[localRevisionKey],
	}
}

// localRevisionKey is the property under which the local memory keeps the
// server's revision of an entry it sent, so an edit made offline names the
// version of the entry the person saw.
const localRevisionKey = "bowrain:revision"

// errUnsentOffline is an offline edit of an entry this app holds without
// knowing the server's version of it: one an earlier version of the app made
// offline. The edit could only be refused when it reached the server.
var errUnsentOffline = errors.New("this entry was made by an earlier version of Bowrain and cannot be changed offline; change it when you are connected")

// errRevisionNotSent is a change that does not say which version of the
// entry it was made on.
var errRevisionNotSent = errors.New("this change does not say which version of the entry it was made on; reopen the entry and make the change again")

// revisionNotSent is why a change that names no revision is refused: an entry
// an earlier version of the app made offline holds none to send, and any
// other entry is to be reopened.
func (a *App) revisionNotSent(entryID string) error {
	if tm, err := a.getOrCreateMemory(); err == nil {
		if entry, ok, err := tm.GetEntry(context.Background(), entryID); err == nil && ok && entry.Properties[localRevisionKey] == "" {
			return errUnsentOffline
		}
	}
	return errRevisionNotSent
}

// recordLocalMemoryRevision keeps the server's revision of an entry the local
// memory holds, after a queued add or edit of it reached the server.
func (a *App) recordLocalMemoryRevision(id, revision string) {
	if id == "" || revision == "" {
		return
	}
	tm, err := a.getOrCreateMemory()
	if err != nil {
		return
	}
	ctx := context.Background()
	entry, ok, err := tm.GetEntry(ctx, id)
	if err != nil || !ok {
		return
	}
	if entry.Properties == nil {
		entry.Properties = map[string]string{}
	}
	entry.Properties[localRevisionKey] = revision
	if err := tm.Add(ctx, entry); err != nil {
		slog.Warn("bowrain: could not record a content-memory revision locally", "entry", id, "error", err)
	}
}

// foldIntoQueuedAdd puts an offline edit into the queued add of the same
// entry, when the add has not been sent: the server then receives the entry
// as the person left it. It reports whether it did.
func (a *App) foldIntoQueuedAdd(req MemoryUpdateRequest) (bool, error) {
	if a.offlineQueue == nil {
		return false, nil
	}
	ctx := a.queueCtx()
	pending, err := a.offlineQueue.Pending(ctx, string(opAddMemoryEntry))
	if err != nil {
		return false, err
	}
	for _, c := range pending {
		var add addMemoryEntryOp
		if json.Unmarshal([]byte(c.Payload), &add) != nil || add.ID != req.EntryID {
			continue
		}
		add.Source, add.Target = req.Source, req.Target
		add.SourceLocale, add.TargetLocale = req.SourceLocale, req.TargetLocale
		return a.offlineQueue.Rewrite(ctx, c.ID, add)
	}
	return false, nil
}

// foldIntoQueuedEdit puts an offline edit into the queued edit of the same
// entry, when that edit has not been sent. The queued edit keeps the revision
// it was made on: both are the person's, made on the version they saw, and
// the server takes the last wording as one edit. It reports whether it did.
func (a *App) foldIntoQueuedEdit(req MemoryUpdateRequest) (bool, error) {
	if a.offlineQueue == nil {
		return false, nil
	}
	ctx := a.queueCtx()
	pending, err := a.offlineQueue.Pending(ctx, string(opUpdateMemoryEntry))
	if err != nil {
		return false, err
	}
	for _, c := range slices.Backward(pending) {
		var queued updateMemoryEntryOp
		if json.Unmarshal([]byte(c.Payload), &queued) != nil || queued.EntryID != req.EntryID {
			continue
		}
		queued.Source, queued.Target = req.Source, req.Target
		queued.SourceLocale, queued.TargetLocale = req.SourceLocale, req.TargetLocale
		return a.offlineQueue.Rewrite(ctx, c.ID, queued)
	}
	return false, nil
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
	if a.isOffline() {
		// An entry whose add has not been sent takes the edit into the add.
		folded, err := a.foldIntoQueuedAdd(req)
		if err != nil {
			return err
		}
		if !folded {
			// A queued edit of the same entry takes the later wording.
			if folded, err = a.foldIntoQueuedEdit(req); err != nil {
				return err
			}
		}
		if folded {
			return a.updateMemoryEntryLocal(req)
		}
	}
	// The edit names the version of the entry the person saw, as the screen
	// they edited on carried it. The app never supplies one: a revision read at
	// save time would say what the store holds now, not what they saw.
	if req.BaseRevision == "" && (a.isConnected() || a.isOffline()) {
		return a.revisionNotSent(req.EntryID)
	}
	return a.writeThroughVoid(updateMemoryEntryOp{req},
		func() error {
			client, ws := a.editorRemote()
			_, err := client.UpdateMemoryEntry(context.Background(), ws, req.EntryID, req.Source, req.Target, req.SourceLocale, req.TargetLocale, req.BaseRevision)
			return err
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
//
// baseRevision is the revision of the entry the person saw, so a delete never
// removes text someone wrote after they read it.
func (a *App) DeleteMemoryEntry(projectID, entryID, baseRevision string) error {
	if a.isOffline() {
		// An entry whose add has not been sent was never on the server: the
		// add and any edit queued after it go, and nothing is sent.
		dropped, err := a.dropUnsentEntry(entryID)
		if err != nil {
			return err
		}
		if dropped {
			return a.deleteMemoryEntryLocal(entryID)
		}
	}
	// The delete names the version the person saw; the app never supplies one.
	if baseRevision == "" && (a.isConnected() || a.isOffline()) {
		return a.revisionNotSent(entryID)
	}
	return a.writeThroughVoid(deleteMemoryEntryOp{EntryID: entryID, BaseRevision: baseRevision},
		func() error {
			client, ws := a.editorRemote()
			return client.DeleteMemoryEntry(context.Background(), ws, entryID, baseRevision)
		},
		func() error { return nil }, // server-authoritative: skip the local mirror on success
		func() error { return a.deleteMemoryEntryLocal(entryID) },
	)
}

func (a *App) deleteMemoryEntryLocal(entryID string) error {
	tm, err := a.getOrCreateMemory()
	if err != nil {
		return fmt.Errorf("init content memory: %w", err)
	}
	return tm.Delete(context.Background(), entryID)
}

// dropUnsentEntry removes the queued add of entryID, and every edit queued
// after it, when the add has not been sent. It reports whether it did.
func (a *App) dropUnsentEntry(entryID string) (bool, error) {
	if a.offlineQueue == nil {
		return false, nil
	}
	ctx := a.queueCtx()
	adds, err := a.offlineQueue.Pending(ctx, string(opAddMemoryEntry))
	if err != nil {
		return false, err
	}
	found := false
	for _, c := range adds {
		var add addMemoryEntryOp
		if json.Unmarshal([]byte(c.Payload), &add) != nil || add.ID != entryID {
			continue
		}
		if found, err = a.offlineQueue.Discard(ctx, c.ID); err != nil {
			return false, err
		}
	}
	if !found {
		return false, nil
	}
	edits, err := a.offlineQueue.Pending(ctx, string(opUpdateMemoryEntry))
	if err != nil {
		return false, err
	}
	for _, c := range edits {
		var edit updateMemoryEntryOp
		if json.Unmarshal([]byte(c.Payload), &edit) == nil && edit.EntryID == entryID {
			if _, err := a.offlineQueue.Discard(ctx, c.ID); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

// AddMemoryEntry adds a new entry to the content memory.
func (a *App) AddMemoryEntry(projectID, source, target, sourceLocale, targetLocale string) (*MemoryEntryInfo, error) {
	// The entry's id is chosen here, so an entry made offline is stored under
	// the same id locally and on the server once the add is sent.
	entryID := id.New()
	op := addMemoryEntryOp{ID: entryID, Source: source, Target: target, SourceLocale: sourceLocale, TargetLocale: targetLocale}
	return writeThroughResult(a, op,
		func() (*MemoryEntryInfo, error) {
			client, ws := a.editorRemote()
			info, err := client.AddMemoryEntry(context.Background(), ws, "", source, target, sourceLocale, targetLocale)
			if err != nil {
				return nil, err
			}
			out := editorMemoryEntryToInfo(*info)
			return &out, nil
		},
		nil, // server returns the canonical entry; nothing to reconcile locally
		func() (*MemoryEntryInfo, error) {
			return a.addMemoryEntryLocal(entryID, source, target, sourceLocale, targetLocale)
		},
	)
}

func (a *App) addMemoryEntryLocal(entryID, source, target, sourceLocale, targetLocale string) (*MemoryEntryInfo, error) {
	tm, err := a.getOrCreateMemory()
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}

	now := time.Now()
	srcLoc := model.LocaleID(sourceLocale)
	tgtLoc := model.LocaleID(targetLocale)
	entry := memory.Entry{
		ID: entryID,
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
