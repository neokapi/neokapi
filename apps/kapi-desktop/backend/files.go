package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/ignore"
	"github.com/neokapi/neokapi/core/project"
)

// FileMatch represents a file matched by content patterns.
type FileMatch struct {
	Path       string `json:"path"`
	Format     string `json:"format,omitempty"`
	Relative   string `json:"relative"`
	Pattern    string `json:"pattern"`
	Collection string `json:"collection,omitempty"`
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
	home, _ := os.UserHomeDir()
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

	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return nil, nil
	}

	ig := ignore.ForProjectDir(basePath)

	var matches []FileMatch
	for _, entry := range op.Project.Content {
		if entry.Path == "" {
			continue
		}

		// Reject patterns that escape the project root.
		if strings.Contains(entry.Path, "..") {
			continue
		}

		// Reject absolute paths.
		if filepath.IsAbs(entry.Path) {
			continue
		}

		pattern := filepath.Join(basePath, entry.Path)
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil || info.IsDir() {
				continue
			}

			// Verify the resolved file is within the project root.
			absFile, _ := filepath.Abs(f)
			absBase, _ := filepath.Abs(basePath)
			if !strings.HasPrefix(absFile, absBase+string(filepath.Separator)) {
				continue
			}

			rel, _ := filepath.Rel(basePath, f)

			// Skip ignored files.
			if ig.Match(filepath.ToSlash(rel), false) {
				continue
			}

			format := entry.Format
			if format == "" {
				format = detectFormatByExtension(f)
			}
			matches = append(matches, FileMatch{
				Path:       f,
				Format:     format,
				Relative:   rel,
				Pattern:    entry.Path,
				Collection: entry.Collection,
			})
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
	detected, err := a.formatReg.DetectByExtension(ext)
	if err != nil {
		return ""
	}
	return detected
}

// ValidateContentPath checks if a content path pattern is safe (no .., no absolute).
func (a *App) ValidateContentPath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain '..' — all paths are relative to the project directory")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("path must be relative to the project directory")
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
// ignored files (project.kapi, hidden files, .kapiignore entries).
func (a *App) IsEmptyProject(tabID string) bool {
	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return true
	}
	ig := ignore.ForProjectDir(basePath)
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return true
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if ig.Match(name, e.IsDir()) {
			continue
		}
		return false
	}
	return true
}

// ListProjectFiles returns all files in the project directory recursively.
// Files matching .kapiignore / KAPI_IGNORE patterns are excluded.
func (a *App) ListProjectFiles(tabID string) ([]ProjectFileInfo, error) {
	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return nil, nil
	}
	ig := ignore.ForProjectDir(basePath)
	var files []ProjectFileInfo
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Always skip hidden entries.
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(basePath, path)
		if rel == "." {
			return nil
		}
		if ig.Match(filepath.ToSlash(rel), info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		pf := ProjectFileInfo{
			Path:     path,
			Relative: rel,
			Size:     info.Size(),
			IsDir:    info.IsDir(),
		}
		if !info.IsDir() {
			pf.Format = detectFormatByExtension(path)
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
		return fmt.Errorf("project has no base path")
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
		op.Project.Content = append(op.Project.Content, project.ContentEntry{
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
		return "", fmt.Errorf("project has no base path")
	}
	// Safety: reject destDir with ..
	if strings.Contains(destDir, "..") {
		return "", fmt.Errorf("destination must not contain '..'")
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

func detectFormatByExtension(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return "json"
	case ".xliff", ".xlf":
		return "xliff"
	case ".po":
		return "po"
	case ".properties":
		return "java-properties"
	case ".yaml", ".yml":
		return "yaml"
	case ".xml":
		return "xml"
	case ".html", ".htm":
		return "html"
	case ".md", ".markdown":
		return "markdown"
	case ".txt":
		return "plaintext"
	case ".ts", ".tsx":
		return "typescript"
	case ".csv":
		return "csv"
	default:
		return ""
	}
}
