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
}

// MatchContent resolves content patterns from the current project against the filesystem.
// The basePath is the directory to resolve relative globs from (typically the .kapi file's directory).
func (a *App) MatchContent(tabID, basePath string) ([]FileMatch, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, nil
	}
	proj := op.Project

	var matches []FileMatch
	for _, entry := range proj.Content {
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
