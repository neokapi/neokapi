package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileMatch represents a file matched by content patterns.
type FileMatch struct {
	Path     string `json:"path"`
	Format   string `json:"format,omitempty"`
	Relative string `json:"relative"`
	Pattern  string `json:"pattern"`
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
func (a *App) MatchContent(tabID string) ([]FileMatch, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, nil
	}

	basePath := a.GetBasePath(tabID)
	if basePath == "" {
		return nil, nil
	}

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
