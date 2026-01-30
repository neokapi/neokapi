package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/google/uuid"
)

// ProjectInfo describes a translation project exposed to the frontend.
type ProjectInfo struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	SourceLocale  string        `json:"source_locale"`
	TargetLocales []string      `json:"target_locales"`
	Path          string        `json:"path"`
	Files         []ProjectFile `json:"files"`
	CreatedAt     string        `json:"created_at"`
	ModifiedAt    string        `json:"modified_at"`
}

// ProjectFile describes a file within a project.
type ProjectFile struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	Size       int64  `json:"size"`
	BlockCount int    `json:"block_count"`
	WordCount  int    `json:"word_count"`
}

// BlockInfo is a serializable representation of a translatable block.
type BlockInfo struct {
	ID           string            `json:"id"`
	Source       string            `json:"source"`
	Targets      map[string]string `json:"targets"`
	Translatable bool              `json:"translatable"`
	HasSpans     bool              `json:"has_spans"`
	Properties   map[string]string `json:"properties"`
}

// UpdateBlockRequest holds parameters for updating a block target.
type UpdateBlockRequest struct {
	ProjectID    string `json:"project_id"`
	FileName     string `json:"file_name"`
	BlockID      string `json:"block_id"`
	TargetLocale string `json:"target_locale"`
	Text         string `json:"text"`
}

// AITranslateFileRequest holds parameters for AI-translating a file.
type AITranslateFileRequest struct {
	ProjectID        string `json:"project_id"`
	FileName         string `json:"file_name"`
	TargetLocale     string `json:"target_locale"`
	Provider         string `json:"provider"`
	APIKey           string `json:"api_key"`
	Model            string `json:"model"`
	ProviderConfigID string `json:"provider_config_id,omitempty"`
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

// project is the internal representation of a translation project.
type project struct {
	info  ProjectInfo
	files map[string]*projectFileData
	dirty bool
}

// projectFileData holds the parsed content of a file within a project.
type projectFileData struct {
	originalPath string
	format       string
	parts        []*model.Part
	sourceBytes  []byte
}

// projectStore manages all open projects.
type projectStore struct {
	mu       sync.RWMutex
	projects map[string]*project
}

func newProjectStore() *projectStore {
	return &projectStore{
		projects: make(map[string]*project),
	}
}

func (s *projectStore) get(id string) (*project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	if !ok {
		return nil, fmt.Errorf("project %q not found", id)
	}
	return p, nil
}

func (s *projectStore) put(p *project) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projects[p.info.ID] = p
}

func (s *projectStore) remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.projects, id)
}

func (s *projectStore) all() []ProjectInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ProjectInfo
	for _, p := range s.projects {
		result = append(result, p.info)
	}
	return result
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

	now := time.Now().UTC().Format(time.RFC3339)
	p := &project{
		info: ProjectInfo{
			ID:            uuid.New().String(),
			Name:          name,
			SourceLocale:  sourceLang,
			TargetLocales: targetLangs,
			Files:         []ProjectFile{},
			CreatedAt:     now,
			ModifiedAt:    now,
		},
		files: make(map[string]*projectFileData),
	}

	a.projects.put(p)
	return &p.info, nil
}

// GetProject returns the current project info.
func (a *App) GetProject(projectID string) (*ProjectInfo, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}
	return &p.info, nil
}

// ListProjects returns all open projects.
func (a *App) ListProjects() []ProjectInfo {
	return a.projects.all()
}

// CloseProject closes a project and releases its resources.
func (a *App) CloseProject(projectID string) error {
	_, err := a.projects.get(projectID)
	if err != nil {
		return err
	}
	a.projects.remove(projectID)
	return nil
}

// AddFiles imports files into a project, auto-detecting format and extracting blocks.
func (a *App) AddFiles(projectID string, filePaths []string) (*ProjectInfo, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	for _, filePath := range filePaths {
		info, err := os.Stat(filePath)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", filePath, err)
		}
		if info.IsDir() {
			continue
		}

		fileName := filepath.Base(filePath)

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

		// Parse with format reader
		reader, err := a.formatReg.NewReader(fmtName)
		if err != nil {
			continue
		}

		f, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", filePath, err)
		}

		doc := &model.RawDocument{
			URI:          filePath,
			SourceLocale: model.LocaleID(p.info.SourceLocale),
			Encoding:     "UTF-8",
			Reader:       f,
		}

		if err := reader.Open(ctx, doc); err != nil {
			f.Close()
			return nil, fmt.Errorf("parse %q: %w", filePath, err)
		}

		var parts []*model.Part
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				reader.Close()
				return nil, fmt.Errorf("read %q: %w", filePath, result.Error)
			}
			parts = append(parts, result.Part)
		}
		reader.Close()

		// Count blocks and words
		blockCount := 0
		wordCount := 0
		for _, pt := range parts {
			if pt.Type == model.PartBlock {
				block, ok := pt.Resource.(*model.Block)
				if ok {
					blockCount++
					wordCount += countWords(block.SourceText())
				}
			}
		}

		// Store file data
		p.files[fileName] = &projectFileData{
			originalPath: filePath,
			format:       fmtName,
			parts:        parts,
			sourceBytes:  data,
		}

		// Update project info
		p.info.Files = append(p.info.Files, ProjectFile{
			Name:       fileName,
			Format:     fmtName,
			Size:       info.Size(),
			BlockCount: blockCount,
			WordCount:  wordCount,
		})
	}

	p.info.ModifiedAt = time.Now().UTC().Format(time.RFC3339)
	p.dirty = true
	return &p.info, nil
}

// RemoveFile removes a file from the project.
func (a *App) RemoveFile(projectID, fileName string) (*ProjectInfo, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	if _, ok := p.files[fileName]; !ok {
		return nil, fmt.Errorf("file %q not found in project", fileName)
	}

	delete(p.files, fileName)

	// Update file list
	var updated []ProjectFile
	for _, f := range p.info.Files {
		if f.Name != fileName {
			updated = append(updated, f)
		}
	}
	p.info.Files = updated
	p.info.ModifiedAt = time.Now().UTC().Format(time.RFC3339)
	p.dirty = true

	return &p.info, nil
}

// ListProjectFiles returns the files in a project.
func (a *App) ListProjectFiles(projectID string) ([]ProjectFile, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}
	return p.info.Files, nil
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

// countCharsNoSpace counts Unicode runes excluding spaces.
func countCharsNoSpace(text string) int {
	count := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	return count
}

// fileExtension returns the file extension without dot, lowercased.
func fileExtension(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimPrefix(strings.ToLower(ext), ".")
}
