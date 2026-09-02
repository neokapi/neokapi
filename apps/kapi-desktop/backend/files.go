package backend

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// FileMatch represents a file matched by content patterns.
type FileMatch struct {
	Path     string `json:"path"`
	Format   string `json:"format,omitempty"`
	Relative string `json:"relative"`
	// Pattern is the EFFECTIVE glob, with any collection `base:` folded in, so
	// it does not equal the path the recipe declares.
	Pattern    string `json:"pattern"`
	Collection string `json:"collection,omitempty"`
	// CollectionIndex and ItemIndex address the recipe entry this file came
	// from. A surface joining files back to the rows a person edits uses these:
	// matching the declared path against Pattern silently finds nothing for any
	// collection that declares a base.
	CollectionIndex int `json:"collection_index"`
	ItemIndex       int `json:"item_index"`
}

// GetBasePath returns the project root: the .kapi file's parent directory.
// For unsaved projects, returns the user's home directory.
func (a *App) GetBasePath(tabID string) string {
	op := a.getOpenProject(tabID)
	if op == nil {
		return ""
	}
	if op.Path != "" {
		return filepath.Dir(op.Path)
	}
	home, _ := userHomeDir()
	return home
}

// MatchContent resolves content patterns against the filesystem.
// All patterns are relative to the .kapi file's directory.
// Patterns containing ".." are rejected for safety.
// Files matched by .kapiignore / KAPI_IGNORE are excluded.
func (a *App) MatchContent(tabID string) ([]FileMatch, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, nil
	}

	ctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := ctx.ResolveContent(a.formatReg)
	if err != nil {
		return nil, err
	}

	matches := make([]FileMatch, len(resolved))
	for i, rf := range resolved {
		matches[i] = FileMatch{
			Path:            rf.Path,
			Format:          rf.Format,
			Relative:        rf.Relative,
			Pattern:         rf.Pattern,
			Collection:      rf.Collection,
			CollectionIndex: rf.CollectionIndex,
			ItemIndex:       rf.ItemIndex,
		}
	}
	return matches, nil
}

// DetectFormat returns the detected format name for a file path.
func (a *App) DetectFormat(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return ""
	}
	detected, err := a.formatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true})
	if err != nil {
		return ""
	}
	return string(detected)
}

// ValidateContentPath checks if a content path pattern is safe (no .., no absolute).
func (a *App) ValidateContentPath(path string) error {
	if strings.Contains(path, "..") {
		return errors.New("path must not contain '..': all paths are relative to the project directory")
	}
	if filepath.IsAbs(path) {
		return errors.New("path must be relative to the project directory")
	}
	return nil
}

// ProjectFileInfo describes a file in the project directory.
type ProjectFileInfo struct {
	Path     string `json:"path"`
	Relative string `json:"relative"`
	Format   string `json:"format,omitempty"`
	Size     int64  `json:"size"`
	IsDir    bool   `json:"is_dir"`
}

// IsEmptyProject returns true if the project directory contains only
// ignored files (kapi.yaml, hidden files, .kapiignore entries), using the
// shared project walk so "empty" agrees with what ListProjectFiles shows.
func (a *App) IsEmptyProject(tabID string) bool {
	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return true
	}
	empty := true
	_ = project.WalkProjectDir(basePath, func(string, os.FileInfo) error {
		empty = false
		return filepath.SkipAll
	})
	return empty
}

// ListProjectFiles returns all files in the project directory recursively.
// Files matching .kapiignore / KAPI_IGNORE patterns are excluded.
// Format detection is scoped to the project's declared plugins.
func (a *App) ListProjectFiles(tabID string) ([]ProjectFileInfo, error) {
	op := a.getOpenProject(tabID)
	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return nil, nil
	}

	// Use project-scoped format detection when a project is available.
	var ctx *project.ProjectContext
	if op != nil {
		ctx = project.NewProjectContext(op.Project, op.Path)
	}

	var files []ProjectFileInfo
	err := project.WalkProjectDir(basePath, func(rel string, info os.FileInfo) error {
		path := filepath.Join(basePath, rel)
		pf := ProjectFileInfo{
			Path:     path,
			Relative: rel,
			Size:     info.Size(),
			IsDir:    info.IsDir(),
		}
		if !info.IsDir() {
			if ctx != nil {
				pf.Format = ctx.DetectFormat(a.formatReg, path)
			} else {
				pf.Format = a.DetectFormat(path)
			}
		}
		files = append(files, pf)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list project files: %w", err)
	}
	return files, nil
}

// ApplyTemplate sets up a project directory from a template name.
// Supported templates:
//   - "input-output": Creates ./input/ and ./output/{lang}/ directories,
//     adds content pattern input/* → output/{lang}/*.
//   - "empty": No-op, just keeps the empty project.
func (a *App) ApplyTemplate(tabID, template string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return errors.New("project has no base path")
	}
	switch template {
	case "input-output":
		// Create directories.
		if err := os.MkdirAll(filepath.Join(basePath, "input"), 0o755); err != nil {
			return fmt.Errorf("create input dir: %w", err)
		}
		if err := os.MkdirAll(filepath.Join(basePath, "output"), 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		// Add content pattern.
		op.Project.Collections = append(op.Project.Collections, project.Collection{
			Path:   "input/*",
			Target: "output/{lang}/*",
		})
		if err := project.Save(op.Path, op.Project); err != nil {
			return fmt.Errorf("save project: %w", err)
		}
	case "empty":
		// No-op.
	default:
		return fmt.Errorf("unknown template %q", template)
	}
	return nil
}

// CopyFileToProject copies a file into the project directory, preserving the filename.
// If destDir is non-empty, places the file under that subdirectory (e.g. "input").
// Returns the relative path of the copied file.
func (a *App) CopyFileToProject(tabID, srcPath, destDir string) (string, error) {
	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return "", errors.New("project has no base path")
	}
	// Safety: reject destDir with ..
	if strings.Contains(destDir, "..") {
		return "", errors.New("destination must not contain '..'")
	}
	targetDir := basePath
	if destDir != "" {
		targetDir = filepath.Join(basePath, destDir)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create target dir: %w", err)
	}
	name := filepath.Base(srcPath)
	destPath := filepath.Join(targetDir, name)

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("read source file: %w", err)
	}
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	rel, _ := filepath.Rel(basePath, destPath)
	return rel, nil
}

// AddFilesDialog opens a native file picker and copies selected files into the project.
// If destDir is non-empty, files are placed under that subdirectory.
// Returns the relative paths of all copied files.
func (a *App) AddFilesDialog(tabID, destDir string) ([]string, error) {
	if a.app == nil {
		return nil, nil
	}
	paths, err := a.app.Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false).
		PromptForMultipleSelection()
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	var results []string
	for _, src := range paths {
		rel, err := a.CopyFileToProject(tabID, src, destDir)
		if err != nil {
			return results, err
		}
		results = append(results, rel)
	}
	return results, nil
}
