package project

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ResolveRemotePath resolves a local path to a remote path using content entries.
// Returns the remote path and format, or an error if no content entry matches.
func (p *Project) ResolveRemotePath(localPath string) (remotePath string, format string, err error) {
	// Normalize local path (relative to project root)
	relPath, err := p.RelativePath(p.ResolvePath(localPath))
	if err != nil {
		return "", "", fmt.Errorf("relativize path: %w", err)
	}

	// Try each content entry in order
	for _, c := range p.Config.Content {
		// Resolve the glob pattern (expand {lang} with the entry's effective language).
		lang := c.EffectiveLanguage(p.Config.SourceLocale())
		pattern := resolvePathPattern(c.Path, lang)

		// Check if local path matches the glob pattern
		matched, err := doublestar.Match(pattern, relPath)
		if err != nil {
			continue
		}

		if matched {
			// Use the base to strip prefix, or fall back to the path itself.
			if c.Base != "" {
				base := resolvePathPattern(c.Base, lang)
				relFromBase := strings.TrimPrefix(relPath, base)
				relFromBase = strings.TrimPrefix(relFromBase, "/")
				return relFromBase, resolveFormat(c.Format), nil
			}
			return relPath, resolveFormat(c.Format), nil
		}
	}

	return "", "", fmt.Errorf("no content entry found for local path: %s", localPath)
}

// ResolveLocalPath resolves a remote path to a local path using content entries.
// Returns the local path and format, or an error if no content entry matches.
func (p *Project) ResolveLocalPath(remotePath string) (localPath string, format string, err error) {
	for _, c := range p.Config.Content {
		lang := c.EffectiveLanguage(p.Config.SourceLocale())
		pattern := resolvePathPattern(c.Path, lang)
		if strings.Contains(pattern, remotePath) {
			local := p.ResolvePath(pattern)
			return local, resolveFormat(c.Format), nil
		}
	}

	return "", "", fmt.Errorf("no content entry found for remote path: %s", remotePath)
}

// expandTemplate expands a template string with path variables.
// Supports: {path}, {filename}, {basename}
func expandTemplate(template string, localPath string) string {
	result := template

	// {path} - full relative path without extension
	pathNoExt := strings.TrimSuffix(localPath, filepath.Ext(localPath))
	result = strings.ReplaceAll(result, "{path}", pathNoExt)

	// {filename} - filename with extension
	filename := filepath.Base(localPath)
	result = strings.ReplaceAll(result, "{filename}", filename)

	// {basename} - filename without extension
	basename := strings.TrimSuffix(filename, filepath.Ext(filename))
	result = strings.ReplaceAll(result, "{basename}", basename)

	return result
}

// ResolvePathPattern expands {lang} placeholders in a path pattern.
func ResolvePathPattern(pattern, lang string) string {
	return strings.ReplaceAll(pattern, "{lang}", lang)
}

// resolvePathPattern is an internal alias for ResolvePathPattern.
func resolvePathPattern(pattern, lang string) string {
	return ResolvePathPattern(pattern, lang)
}

// ResolveFormat returns the format, treating "$auto" and empty as equivalent (auto-detect).
func ResolveFormat(format string) string {
	if format == "$auto" {
		return ""
	}
	return format
}

// resolveFormat is an internal alias for ResolveFormat.
func resolveFormat(format string) string {
	return ResolveFormat(format)
}
