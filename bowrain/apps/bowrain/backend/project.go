package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// RunConstraintsInfo mirrors model.RunConstraints for the frontend.
type RunConstraintsInfo struct {
	Deletable   bool `json:"deletable,omitempty"`
	Cloneable   bool `json:"cloneable,omitempty"`
	Reorderable bool `json:"reorderable,omitempty"`
}

// TextRunInfo is a plain text chunk.
type TextRunInfo struct {
	Text string `json:"text"`
}

// PlaceholderRunInfo is a self-closing inline code.
type PlaceholderRunInfo struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	SubType     string              `json:"subType,omitempty"`
	Data        string              `json:"data"`
	Equiv       string              `json:"equiv"`
	Disp        string              `json:"disp,omitempty"`
	Constraints *RunConstraintsInfo `json:"constraints,omitempty"`
}

// PcOpenRunInfo is the opening half of a paired inline code.
type PcOpenRunInfo struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	SubType     string              `json:"subType,omitempty"`
	Data        string              `json:"data"`
	Equiv       string              `json:"equiv"`
	Disp        string              `json:"disp,omitempty"`
	Constraints *RunConstraintsInfo `json:"constraints,omitempty"`
}

// PcCloseRunInfo is the closing half of a paired inline code.
type PcCloseRunInfo struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	SubType string `json:"subType,omitempty"`
	Data    string `json:"data"`
	Equiv   string `json:"equiv,omitempty"`
}

// SubRunInfo is a sub-filter reference.
type SubRunInfo struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Equiv string `json:"equiv"`
}

// PluralRunInfo is a structured plural construct.
type PluralRunInfo struct {
	Pivot string               `json:"pivot"`
	Forms map[string][]RunInfo `json:"forms"`
}

// SelectRunInfo is a structured select construct.
type SelectRunInfo struct {
	Pivot string               `json:"pivot"`
	Cases map[string][]RunInfo `json:"cases"`
}

// RunInfo describes one inline-content primitive for the frontend.
// Exactly one of the pointer fields is non-nil per record.
type RunInfo struct {
	Text    *TextRunInfo        `json:"text,omitempty"`
	Ph      *PlaceholderRunInfo `json:"ph,omitempty"`
	PcOpen  *PcOpenRunInfo      `json:"pcOpen,omitempty"`
	PcClose *PcCloseRunInfo     `json:"pcClose,omitempty"`
	Sub     *SubRunInfo         `json:"sub,omitempty"`
	Plural  *PluralRunInfo      `json:"plural,omitempty"`
	Select  *SelectRunInfo      `json:"select,omitempty"`
}

// BlockInfo is a serializable representation of a translatable block.
type BlockInfo struct {
	ID           string               `json:"id"`
	SourceRuns   []RunInfo            `json:"sourceRuns,omitempty"`
	TargetRuns   map[string][]RunInfo `json:"targetRuns,omitempty"`
	Translatable bool                 `json:"translatable"`
	Properties   map[string]string    `json:"properties"`
}

// UpdateBlockRequest holds parameters for updating a block target.
type UpdateBlockRequest struct {
	ProjectID    string `json:"project_id"`
	ItemName     string `json:"item_name"`
	BlockID      string `json:"block_id"`
	TargetLocale string `json:"target_locale"`
	Text         string `json:"text"`
}

// UpdateBlockTargetRunsRequest holds parameters for updating a
// block target with a structured Run sequence.
type UpdateBlockTargetRunsRequest struct {
	ProjectID    string    `json:"project_id"`
	ItemName     string    `json:"item_name"`
	BlockID      string    `json:"block_id"`
	TargetLocale string    `json:"target_locale"`
	Runs         []RunInfo `json:"runs"`
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
		return nil, errors.New("project name is required")
	}
	if sourceLang == "" {
		return nil, errors.New("source language is required")
	}
	if len(targetLangs) == 0 {
		return nil, errors.New("at least one target language is required")
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

// flattenTargetRuns returns the plain-text flattening of a block's
// target-locale runs. Used by the editor/backend tests so they
// don't each reimplement the walker.
func flattenTargetRuns(b BlockInfo, locale string) string {
	return b.FlattenTarget(locale)
}

// FlattenSource returns the plain-text flattening of SourceRuns.
func (b BlockInfo) FlattenSource() string {
	return flattenRunInfos(b.SourceRuns)
}

// FlattenTarget returns the plain-text flattening of target runs.
func (b BlockInfo) FlattenTarget(locale string) string {
	return flattenRunInfos(b.TargetRuns[locale])
}

// flattenRunInfos flattens a RunInfo slice into plain text.
func flattenRunInfos(runs []RunInfo) string {
	var buf []rune
	flattenRunInfosTo(&buf, runs)
	return string(buf)
}

func flattenRunInfosTo(buf *[]rune, runs []RunInfo) {
	for _, r := range runs {
		switch {
		case r.Text != nil:
			*buf = append(*buf, []rune(r.Text.Text)...)
		case r.Ph != nil:
			*buf = append(*buf, '{')
			*buf = append(*buf, []rune(r.Ph.Equiv)...)
			*buf = append(*buf, '}')
		case r.Sub != nil:
			*buf = append(*buf, '[')
			*buf = append(*buf, []rune(r.Sub.Equiv)...)
			*buf = append(*buf, ']')
		case r.Plural != nil:
			if form, ok := r.Plural.Forms["other"]; ok {
				flattenRunInfosTo(buf, form)
				continue
			}
			for _, form := range r.Plural.Forms {
				flattenRunInfosTo(buf, form)
				break
			}
		case r.Select != nil:
			if form, ok := r.Select.Cases["other"]; ok {
				flattenRunInfosTo(buf, form)
				continue
			}
			for _, form := range r.Select.Cases {
				flattenRunInfosTo(buf, form)
				break
			}
		}
	}
}
