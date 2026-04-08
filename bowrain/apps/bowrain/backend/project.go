package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// ProjectInfo describes a translation project exposed to the frontend.
type ProjectInfo struct {
	ID                    string        `json:"id"`
	Name                  string        `json:"name"`
	DefaultSourceLanguage string        `json:"default_source_language"`
	TargetLanguages       []string      `json:"target_languages"`
	Path                  string        `json:"path"`
	Items                 []ProjectItem `json:"items"`
	CreatedAt             string        `json:"created_at"`
	ModifiedAt            string        `json:"modified_at"`
}

// ProjectItem describes an item within a project.
type ProjectItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Format     string `json:"format"`
	Type       string `json:"type"` // "file", "data", etc.
	Size       int64  `json:"size"`
	BlockCount int    `json:"block_count"`
	WordCount  int    `json:"word_count"`
}

// SpanInfo describes an inline span element for the frontend.
type SpanInfo struct {
	SpanType    string `json:"span_type"` // "opening", "closing", "placeholder"
	Type        string `json:"type"`      // Semantic type from vocabulary (e.g., "fmt:bold")
	ID          string `json:"id"`
	Data        string `json:"data"`                   // original markup: "<b>", "</b>", "<br/>"
	SubType     string `json:"sub_type,omitempty"`     // Format-specific refinement (e.g., "html:b")
	DisplayText string `json:"display_text,omitempty"` // Human-readable label (e.g., "[B]")
	EquivText   string `json:"equiv_text,omitempty"`   // Plain text equivalent
	Deletable   bool   `json:"deletable,omitempty"`
	Cloneable   bool   `json:"cloneable,omitempty"`
	CanReorder  bool   `json:"can_reorder,omitempty"`
}

// BlockInfo is a serializable representation of a translatable block.
type BlockInfo struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"`
	SourceCoded  string            `json:"source_coded,omitempty"`
	SourceSpans  []SpanInfo        `json:"source_spans,omitempty"`
	Targets      map[string]string `json:"targets"`
	TargetsCoded map[string]string `json:"targets_coded,omitempty"`
	Translatable bool              `json:"translatable"`
	HasSpans     bool              `json:"has_spans"`
	Properties   map[string]string `json:"properties"`
}

// UpdateBlockRequest holds parameters for updating a block target.
type UpdateBlockRequest struct {
	ProjectID    string `json:"project_id"`
	ItemName     string `json:"item_name"`
	BlockID      string `json:"block_id"`
	TargetLocale string `json:"target_locale"`
	Text         string `json:"text"`
}

// UpdateBlockTargetCodedRequest holds parameters for updating a block target with coded text and spans.
type UpdateBlockTargetCodedRequest struct {
	ProjectID    string     `json:"project_id"`
	ItemName     string     `json:"item_name"`
	BlockID      string     `json:"block_id"`
	TargetLocale string     `json:"target_locale"`
	CodedText    string     `json:"coded_text"`
	Spans        []SpanInfo `json:"spans"`
}

// TranslationStats holds statistics about a translation operation.
type TranslationStats struct {
	TotalBlocks      int `json:"total_blocks"`
	TranslatedBlocks int `json:"translated_blocks"`
	WordCount        int `json:"word_count"`
}

// WordCountResult holds word and character counts.
type WordCountResult struct {
	SourceWords int            `json:"source_words"`
	SourceChars int            `json:"source_chars"`
	TargetWords map[string]int `json:"target_words"`
	TargetChars map[string]int `json:"target_chars"`
}

// CreateProject creates a new translation project.
func (a *App) CreateProject(name, sourceLang string, targetLangs []string) (*ProjectInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}
	if sourceLang == "" {
		return nil, fmt.Errorf("source language is required")
	}
	if len(targetLangs) == 0 {
		return nil, fmt.Errorf("at least one target language is required")
	}

	locales := make([]model.LocaleID, len(targetLangs))
	for i, l := range targetLangs {
		locales[i] = model.LocaleID(l)
	}

	p := &store.Project{
		Name:                  name,
		DefaultSourceLanguage: model.LocaleID(sourceLang),
		TargetLanguages:       locales,
		Properties:            map[string]string{},
	}

	ctx := context.Background()
	if err := a.store.CreateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	info := projectToInfo(p)
	return &info, nil
}

// GetProject returns the current project info.
func (a *App) GetProject(projectID string) (*ProjectInfo, error) {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		info, err := a.remote.GetProject(ws, projectID)
		if err != nil {
			a.goOffline()
			// Fall through to local.
		} else {
			return info, nil
		}
	}
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return buildProjectInfo(ctx, a.store, proj)
}

// ListProjects returns all open projects.
func (a *App) ListProjects() []ProjectInfo {
	if a.isConnected() {
		a.mu.RLock()
		ws := a.activeWS
		a.mu.RUnlock()
		projects, err := a.remote.ListProjects(ws)
		if err == nil {
			return projects
		}
		// Fall through to local on error.
	}
	ctx := context.Background()
	projects, err := a.store.ListProjects(ctx)
	if err != nil {
		return []ProjectInfo{}
	}
	result := make([]ProjectInfo, len(projects))
	for i, p := range projects {
		result[i] = projectToInfo(p)
	}
	return result
}

// CloseProject closes a project and releases its resources.
func (a *App) CloseProject(projectID string) error {
	ctx := context.Background()
	return a.store.DeleteProject(ctx, projectID)
}

// AddItems imports items into a project, auto-detecting format and extracting blocks.
func (a *App) AddItems(projectID string, filePaths []string) (*ProjectInfo, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	for _, filePath := range filePaths {
		info, err := os.Stat(filePath)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", filePath, err)
		}
		if info.IsDir() {
			continue
		}

		itemName := filepath.Base(filePath)

		// Detect format
		fmtName, err := a.DetectFormat(filePath)
		if err != nil {
			continue // skip unsupported formats
		}

		// Read file bytes
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", filePath, err)
		}

		// Parse with format reader using editor.ParseItem.
		reader, err := a.formatReg.NewReader(registry.FormatID(fmtName))
		if err != nil {
			continue
		}

		doc := &model.RawDocument{
			URI:          filePath,
			SourceLocale: proj.DefaultSourceLanguage,
			Encoding:     "UTF-8",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		}

		result, err := editor.ParseItem(ctx, reader, doc, string(proj.DefaultSourceLanguage), fmtName, itemName)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", filePath, err)
		}

		// Store item with block index and preview HTML.
		item := &store.Item{
			Name:        itemName,
			Format:      fmtName,
			ItemType:    "file",
			BlockIndex:  result.BlockIndexJSON,
			PreviewHTML: result.PreviewHTML,
			Properties:  map[string]string{},
		}
		if err := a.store.StoreItem(ctx, projectID, "main", item); err != nil {
			return nil, fmt.Errorf("store item %q: %w", itemName, err)
		}

		// Store extracted blocks.
		if len(result.Blocks) > 0 {
			if err := a.store.StoreBlocksForItem(ctx, projectID, "main", itemName, result.Blocks); err != nil {
				return nil, fmt.Errorf("store blocks for %q: %w", itemName, err)
			}
		}
	}

	return buildProjectInfo(ctx, a.store, proj)
}

// RemoveItem removes an item from the project.
func (a *App) RemoveItem(projectID, itemName string) (*ProjectInfo, error) {
	ctx := context.Background()
	proj, err := a.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if err := a.store.DeleteItem(ctx, projectID, "main", itemName); err != nil {
		return nil, err
	}

	return buildProjectInfo(ctx, a.store, proj)
}

// ListProjectFiles returns the items in a project.
func (a *App) ListProjectFiles(projectID string) ([]ProjectItem, error) {
	ctx := context.Background()
	items, err := a.store.ListItems(ctx, projectID, "main")
	if err != nil {
		return nil, err
	}

	result := make([]ProjectItem, 0, len(items))
	for _, item := range items {
		blocks, err := a.store.GetBlocks(ctx, store.BlockQuery{
			ProjectID: projectID,
			Stream:    "main",
			ItemName:  item.Name,
		})
		if err != nil {
			return nil, err
		}

		wordCount := 0
		for _, sb := range blocks {
			if sb.Block.Translatable {
				wordCount += countWords(sb.Block.SourceText())
			}
		}

		result = append(result, ProjectItem{
			ID:         item.ID,
			Name:       item.Name,
			Format:     item.Format,
			Type:       item.ItemType,
			Size:       0,
			BlockCount: len(blocks),
			WordCount:  wordCount,
		})
	}
	return result, nil
}

// projectToInfo converts a store.Project to a ProjectInfo (without items).
func projectToInfo(p *store.Project) ProjectInfo {
	locales := make([]string, len(p.TargetLanguages))
	for i, l := range p.TargetLanguages {
		locales[i] = string(l)
	}
	return ProjectInfo{
		ID:                    p.ID,
		Name:                  p.Name,
		DefaultSourceLanguage: string(p.DefaultSourceLanguage),
		TargetLanguages:       locales,
		Items:                 []ProjectItem{},
		CreatedAt:             p.CreatedAt.Format(time.RFC3339),
		ModifiedAt:            p.UpdatedAt.Format(time.RFC3339),
	}
}

// buildProjectInfo builds a full ProjectInfo with items from store data.
func buildProjectInfo(ctx context.Context, cs store.ContentStore, proj *store.Project) (*ProjectInfo, error) {
	info := projectToInfo(proj)

	items, err := cs.ListItems(ctx, proj.ID, "main")
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	for _, item := range items {
		blocks, err := cs.GetBlocks(ctx, store.BlockQuery{
			ProjectID: proj.ID,
			Stream:    "main",
			ItemName:  item.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("get blocks for %q: %w", item.Name, err)
		}

		wordCount := 0
		for _, sb := range blocks {
			if sb.Block.Translatable {
				wordCount += countWords(sb.Block.SourceText())
			}
		}

		info.Items = append(info.Items, ProjectItem{
			ID:         item.ID,
			Name:       item.Name,
			Format:     item.Format,
			Type:       item.ItemType,
			Size:       0,
			BlockCount: len(blocks),
			WordCount:  wordCount,
		})
	}

	return &info, nil
}

// countWords counts words in text by splitting on whitespace.
func countWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// countChars counts Unicode runes in text.
func countChars(text string) int {
	return len([]rune(text))
}

// fileExtension returns the file extension without dot, lowercased.
func fileExtension(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimPrefix(strings.ToLower(ext), ".")
}
