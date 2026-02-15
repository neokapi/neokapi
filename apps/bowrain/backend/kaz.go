package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gokapi/gokapi/core/kaz"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/sievepen"
	"github.com/gokapi/gokapi/lib/termbase"
	"github.com/google/uuid"
)

// SaveProjectAs saves the project as a .kaz package to the given path.
func (a *App) SaveProjectAs(projectID, path string) error {
	p, err := a.projects.get(projectID)
	if err != nil {
		return err
	}

	// Ensure .kaz extension
	if !strings.HasSuffix(strings.ToLower(path), ".kaz") {
		path += ".kaz"
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}
	defer f.Close()

	// Build pack items from project items
	var packItems []kaz.PackItem
	for _, pi := range p.info.Items {
		id, ok := p.items[pi.Name]
		if !ok {
			continue
		}

		itemType := id.itemType
		if itemType == "" {
			itemType = "file"
		}

		packItems = append(packItems, kaz.PackItem{
			Name:        pi.Name,
			Type:        itemType,
			Format:      id.format,
			SourceBytes: id.sourceBytes,
			Parts:       id.parts,
		})
	}

	packOpts := kaz.PackOptions{
		Name:          p.info.Name,
		SourceLocale:  p.info.SourceLocale,
		TargetLocales: p.info.TargetLocales,
		Items:         packItems,
	}

	// Serialize TM entries if present
	if p.tm != nil && p.tm.Count() > 0 {
		tmData, err := exportTMJSON(p.tm)
		if err == nil {
			packOpts.TMData = tmData
		}
	}

	// Serialize termbase concepts if present
	if p.tb != nil {
		var buf bytes.Buffer
		if err := termbase.ExportJSON(p.tb, &buf, p.info.Name); err == nil && buf.Len() > 0 {
			packOpts.TermsData = buf.Bytes()
		}
	}

	err = kaz.Pack(f, packOpts)
	if err != nil {
		return fmt.Errorf("pack: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	p.info.Path = path
	p.info.ModifiedAt = now
	p.dirty = false

	return nil
}

// SaveProject saves the project to its current path.
func (a *App) SaveProject(projectID string) error {
	p, err := a.projects.get(projectID)
	if err != nil {
		return err
	}
	if p.info.Path == "" {
		return fmt.Errorf("project has no save path; use SaveProjectAs")
	}
	return a.SaveProjectAs(projectID, p.info.Path)
}

// OpenProjectDialog shows a native file dialog and opens the selected .kaz project.
func (a *App) OpenProjectDialog() (*ProjectInfo, error) {
	if a.app == nil {
		return nil, fmt.Errorf("application not initialized")
	}
	path, err := a.app.Dialog.OpenFile().
		SetTitle("Open a Project").
		AddFilter("Kaz Packages (*.kaz)", "*.kaz").
		PromptForSingleSelection()
	if err != nil {
		return nil, fmt.Errorf("file dialog: %w", err)
	}
	if path == "" {
		return nil, nil // user cancelled
	}
	return a.OpenProject(path)
}

// SaveProjectDialog shows a native save dialog and saves the project.
func (a *App) SaveProjectDialog(projectID string) error {
	p, err := a.projects.get(projectID)
	if err != nil {
		return err
	}

	defaultName := p.info.Name + ".kaz"
	path, err := a.app.Dialog.SaveFile().
		SetMessage("Save Project").
		SetFilename(defaultName).
		AddFilter("Kaz Packages (*.kaz)", "*.kaz").
		PromptForSingleSelection()
	if err != nil {
		return fmt.Errorf("file dialog: %w", err)
	}
	if path == "" {
		return nil // user cancelled
	}
	return a.SaveProjectAs(projectID, path)
}

// AddItemsDialog shows a native file dialog and adds the selected files to the project.
func (a *App) AddItemsDialog(projectID string) (*ProjectInfo, error) {
	paths, err := a.app.Dialog.OpenFile().
		SetTitle("Add Files to Project").
		PromptForMultipleSelection()
	if err != nil {
		return nil, fmt.Errorf("file dialog: %w", err)
	}
	if len(paths) == 0 {
		return nil, nil // user cancelled
	}
	return a.AddItems(projectID, paths)
}

// OpenProject opens a .kaz package and loads it into memory.
func (a *App) OpenProject(path string) (*ProjectInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}

	pkg, err := kaz.Unpack(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("unpack %q: %w", path, err)
	}

	// Create project
	projectID := uuid.New().String()
	p := &project{
		info: ProjectInfo{
			ID:            projectID,
			Name:          pkg.Manifest.Name,
			SourceLocale:  pkg.Manifest.SourceLocale,
			TargetLocales: pkg.Manifest.TargetLocales,
			Path:          path,
			Items:         []ProjectItem{},
			CreatedAt:     pkg.Manifest.CreatedAt,
			ModifiedAt:    pkg.Manifest.ModifiedAt,
		},
		items: make(map[string]*projectItemData),
	}

	ctx := context.Background()

	// Process each item from manifest
	for _, mi := range pkg.Manifest.Items {
		sourceBytes := pkg.Items[mi.Path]
		blockIndex := pkg.Blocks[mi.Path]
		previewHTML := pkg.Previews[mi.Path]

		var parts []*model.Part

		// Path 1: Re-parse source item if available
		if len(sourceBytes) > 0 {
			parsed, err := a.parseItem(ctx, mi.Path, mi.Format, sourceBytes, pkg.Manifest.SourceLocale)
			if err == nil {
				parts = parsed

				// Restore translations from block index into parts
				if blockIndex != nil {
					restoreTranslations(parts, blockIndex)
				}
			}
		}

		// Path 2: Reconstruct from block index if no source or parsing failed
		if len(parts) == 0 && blockIndex != nil {
			parts = kaz.ReconstructParts(blockIndex)
		}

		// Recalculate counts from block index or parts
		blockCount := mi.BlockCount
		wordCount := mi.WordCount
		if blockIndex != nil {
			blockCount = len(blockIndex.Blocks)
			wordCount = 0
			for _, b := range blockIndex.Blocks {
				wordCount += countWords(b.Source)
			}
		}

		itemType := mi.Type
		if itemType == "" {
			itemType = "file"
		}

		p.items[mi.Path] = &projectItemData{
			format:      mi.Format,
			itemType:    itemType,
			parts:       parts,
			sourceBytes: sourceBytes,
			previewHTML: previewHTML,
			blockIndex:  blockIndex,
		}

		p.info.Items = append(p.info.Items, ProjectItem{
			Name:       mi.Path,
			Format:     mi.Format,
			Type:       itemType,
			Size:       int64(len(sourceBytes)),
			BlockCount: blockCount,
			WordCount:  wordCount,
		})
	}

	// Restore TM entries if present in package
	if len(pkg.TMData) > 0 {
		tm, err := getOrCreateTM(p)
		if err == nil {
			importTMJSON(tm, pkg.TMData)
		}
	}

	// Restore termbase concepts if present in package
	if len(pkg.TermsData) > 0 {
		tb := getOrCreateTB(p)
		termbase.ImportJSON(tb, bytes.NewReader(pkg.TermsData))
	}

	a.projects.put(p)
	return &p.info, nil
}

// tmJSONEntry is a simple JSON-serializable representation of a TM entry.
type tmJSONEntry struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// exportTMJSON serializes all TM entries as JSON.
func exportTMJSON(tm *sievepen.SQLiteTM) ([]byte, error) {
	entries := tm.Entries()
	jEntries := make([]tmJSONEntry, len(entries))
	for i, e := range entries {
		jEntries[i] = tmJSONEntry{
			ID:           e.ID,
			Source:       e.SourceText(),
			Target:       e.TargetText(),
			SourceLocale: string(e.SourceLocale),
			TargetLocale: string(e.TargetLocale),
			CreatedAt:    e.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    e.UpdatedAt.Format(time.RFC3339),
		}
	}
	return json.Marshal(jEntries)
}

// importTMJSON deserializes TM entries from JSON and adds them to the TM.
func importTMJSON(tm *sievepen.SQLiteTM, data []byte) {
	var entries []tmJSONEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	for _, e := range entries {
		createdAt, _ := time.Parse(time.RFC3339, e.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, e.UpdatedAt)
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		if updatedAt.IsZero() {
			updatedAt = time.Now()
		}
		entry := sievepen.TMEntry{
			ID:           e.ID,
			Source:       model.NewFragment(e.Source),
			Target:       model.NewFragment(e.Target),
			SourceLocale: model.LocaleID(e.SourceLocale),
			TargetLocale: model.LocaleID(e.TargetLocale),
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		}
		tm.Add(entry)
	}
}

// parseItem parses source bytes using the appropriate format reader.
func (a *App) parseItem(ctx context.Context, name, format string, data []byte, sourceLocale string) ([]*model.Part, error) {
	reader, err := a.formatReg.NewReader(format)
	if err != nil {
		return nil, err
	}

	doc := &model.RawDocument{
		URI:          name,
		SourceLocale: model.LocaleID(sourceLocale),
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return nil, err
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return nil, result.Error
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	return parts, nil
}

// restoreTranslations copies target translations from a block index into the Part stream.
func restoreTranslations(parts []*model.Part, blockIndex *kaz.BlockIndex) {
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}
		b := blockIndex.BlockByID(block.ID)
		if b == nil {
			continue
		}
		for locale, text := range b.Targets {
			if text != "" {
				block.SetTargetText(model.LocaleID(locale), text)
			}
		}
	}
}
