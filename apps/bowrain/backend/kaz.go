package backend

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// kazManifest is the manifest.yaml inside a .kaz package.
type kazManifest struct {
	Name            string            `yaml:"name"`
	Version         string            `yaml:"version"`
	GokapiVersion   string            `yaml:"gokapi_version"`
	SourceLocale    string            `yaml:"source_locale"`
	TargetLocales   []string          `yaml:"target_locales"`
	CreatedAt       string            `yaml:"created_at"`
	ModifiedAt      string            `yaml:"modified_at"`
	FormatsRequired []string          `yaml:"formats_required"`
	PluginsRequired []string          `yaml:"plugins_required"`
	Files           []kazFileManifest `yaml:"files"`
}

// kazFileManifest describes a file entry in the manifest.
type kazFileManifest struct {
	Path       string `yaml:"path"`
	Format     string `yaml:"format"`
	Size       int64  `yaml:"size"`
	BlockCount int    `yaml:"block_count"`
	WordCount  int    `yaml:"word_count"`
}

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

	w := zip.NewWriter(f)
	defer w.Close()

	// Collect format names
	formatSet := make(map[string]bool)
	var fileManifests []kazFileManifest

	// Write source files and collect XLIFF data
	for _, pf := range p.info.Files {
		fd, ok := p.files[pf.Name]
		if !ok {
			continue
		}

		formatSet[fd.format] = true

		// Write source file
		fw, err := w.Create("files/" + pf.Name)
		if err != nil {
			return fmt.Errorf("write file %q: %w", pf.Name, err)
		}
		if _, err := fw.Write(fd.sourceBytes); err != nil {
			return err
		}

		// Write XLIFF work file
		xliffData, err := a.partsToXLIFF(fd.parts, p.info.SourceLocale, p.info.TargetLocales)
		if err == nil && len(xliffData) > 0 {
			xfw, err := w.Create("xliff/" + pf.Name + ".xlf")
			if err != nil {
				return fmt.Errorf("write xliff %q: %w", pf.Name, err)
			}
			if _, err := xfw.Write(xliffData); err != nil {
				return err
			}
		}

		fileManifests = append(fileManifests, kazFileManifest{
			Path:       pf.Name,
			Format:     fd.format,
			Size:       pf.Size,
			BlockCount: pf.BlockCount,
			WordCount:  pf.WordCount,
		})
	}

	var formats []string
	for f := range formatSet {
		formats = append(formats, f)
	}

	// Build manifest
	now := time.Now().UTC().Format(time.RFC3339)
	manifest := kazManifest{
		Name:            p.info.Name,
		Version:         "1.0",
		GokapiVersion:   "0.1.0",
		SourceLocale:    p.info.SourceLocale,
		TargetLocales:   p.info.TargetLocales,
		CreatedAt:       p.info.CreatedAt,
		ModifiedAt:      now,
		FormatsRequired: formats,
		PluginsRequired: []string{},
		Files:           fileManifests,
	}

	manifestData, err := yaml.Marshal(&manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	mw, err := w.Create("manifest.yaml")
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if _, err := mw.Write(manifestData); err != nil {
		return err
	}

	// Create empty dirs for structure
	if _, err := w.Create("assets/"); err != nil {
		return err
	}
	if _, err := w.Create("tm/"); err != nil {
		return err
	}

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

// OpenProject opens a .kaz package and loads it into memory.
func (a *App) OpenProject(path string) (*ProjectInfo, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer r.Close()

	// Find and read manifest
	var manifest kazManifest
	manifestFound := false
	fileContents := make(map[string][]byte)

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open entry %q: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read entry %q: %w", f.Name, err)
		}

		if f.Name == "manifest.yaml" {
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("parse manifest: %w", err)
			}
			manifestFound = true
		} else if strings.HasPrefix(f.Name, "files/") && !strings.HasSuffix(f.Name, "/") {
			name := strings.TrimPrefix(f.Name, "files/")
			fileContents[name] = data
		}
	}

	if !manifestFound {
		return nil, fmt.Errorf("manifest.yaml not found in %q", path)
	}

	// Create project
	projectID := uuid.New().String()
	p := &project{
		info: ProjectInfo{
			ID:            projectID,
			Name:          manifest.Name,
			SourceLocale:  manifest.SourceLocale,
			TargetLocales: manifest.TargetLocales,
			Path:          path,
			Files:         []ProjectFile{},
			CreatedAt:     manifest.CreatedAt,
			ModifiedAt:    manifest.ModifiedAt,
		},
		files: make(map[string]*projectFileData),
	}

	ctx := context.Background()

	// Process each file from manifest
	for _, mf := range manifest.Files {
		data, ok := fileContents[mf.Path]
		if !ok {
			continue
		}

		// Try to parse with format reader
		reader, err := a.formatReg.NewReader(mf.Format)
		if err != nil {
			// Store without parsing
			p.files[mf.Path] = &projectFileData{
				format:      mf.Format,
				sourceBytes: data,
			}
			p.info.Files = append(p.info.Files, ProjectFile{
				Name:       mf.Path,
				Format:     mf.Format,
				Size:       mf.Size,
				BlockCount: mf.BlockCount,
				WordCount:  mf.WordCount,
			})
			continue
		}

		doc := &model.RawDocument{
			URI:          mf.Path,
			SourceLocale: model.LocaleID(manifest.SourceLocale),
			Encoding:     "UTF-8",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		}

		if err := reader.Open(ctx, doc); err != nil {
			reader.Close()
			continue
		}

		var parts []*model.Part
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				break
			}
			parts = append(parts, result.Part)
		}
		reader.Close()

		// Recalculate counts from actual data
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

		p.files[mf.Path] = &projectFileData{
			format:      mf.Format,
			parts:       parts,
			sourceBytes: data,
		}

		p.info.Files = append(p.info.Files, ProjectFile{
			Name:       mf.Path,
			Format:     mf.Format,
			Size:       int64(len(data)),
			BlockCount: blockCount,
			WordCount:  wordCount,
		})
	}

	a.projects.put(p)
	return &p.info, nil
}

// partsToXLIFF converts parts to a simple XLIFF representation.
// This is a minimal serialization of blocks for the .kaz package.
func (a *App) partsToXLIFF(parts []*model.Part, sourceLang string, targetLangs []string) ([]byte, error) {
	writer, err := a.formatReg.NewWriter("xliff")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := writer.SetOutputWriter(&buf); err != nil {
		return nil, err
	}
	writer.SetLocale(model.LocaleID(sourceLang))

	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)

	ctx := context.Background()
	if err := writer.Write(ctx, ch); err != nil {
		return nil, err
	}
	writer.Close()

	return buf.Bytes(), nil
}
