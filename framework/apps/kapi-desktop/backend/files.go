package backend

import (
	"os"
	"path/filepath"
	"strings"
)

// FileMatch represents a file matched by content patterns.
type FileMatch struct {
	Path     string `json:"path"`
	Format   string `json:"format,omitempty"`
	Relative string `json:"relative"`
	Pattern  string `json:"pattern"` // which content entry matched
}

// GetBasePath returns the effective base path for a project tab.
// Uses the project's BasePath if set, otherwise the .kapi file's parent directory.
func (a *App) GetBasePath(tabID string) string {
	op := a.getOpenProject(tabID)
	if op == nil {
		return ""
	}
	if op.Project.BasePath != "" {
		if filepath.IsAbs(op.Project.BasePath) {
			return op.Project.BasePath
		}
		// Relative base_path: resolve against the .kapi file's directory.
		if op.Path != "" {
			return filepath.Join(filepath.Dir(op.Path), op.Project.BasePath)
		}
	}
	// Default: .kapi file's directory, or working directory if unsaved.
	if op.Path != "" {
		return filepath.Dir(op.Path)
	}
	wd, _ := os.Getwd()
	return wd
}

// MatchContent resolves content patterns against the filesystem.
// Uses GetBasePath to determine the root directory for glob resolution.
func (a *App) MatchContent(tabID string) ([]FileMatch, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, nil
	}

	basePath := a.GetBasePath(tabID)
	var matches []FileMatch

	for _, entry := range op.Project.Content {
		if entry.Path == "" {
			continue
		}

		pattern := entry.Path
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(basePath, pattern)
		}

		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil || info.IsDir() {
				continue
			}

			rel, _ := filepath.Rel(basePath, f)
			format := entry.Format
			if format == "" {
				format = detectFormatByExtension(f)
			}
			matches = append(matches, FileMatch{
				Path:     f,
				Format:   format,
				Relative: rel,
				Pattern:  entry.Path,
			})
		}
	}
	return matches, nil
}

// DetectFormat returns the detected format name for a file path based on its extension.
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
