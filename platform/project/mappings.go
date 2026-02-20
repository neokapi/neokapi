package project

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ResolveRemotePath resolves a local path to a remote path using mappings.
// Returns the remote path and format, or an error if no mapping matches.
func (p *Project) ResolveRemotePath(localPath string) (remotePath string, format string, err error) {
	// Normalize local path (relative to project root)
	relPath, err := p.RelativePath(p.ResolvePath(localPath))
	if err != nil {
		return "", "", fmt.Errorf("relativize path: %w", err)
	}

	// Try each mapping in order
	for _, m := range p.Config.Mappings {
		// Check if local path matches the glob pattern
		matched, err := doublestar.Match(m.Local, relPath)
		if err != nil {
			continue
		}

		if matched {
			// Apply template substitution
			remote := expandTemplate(m.Remote, relPath)
			return remote, m.Format, nil
		}
	}

	return "", "", fmt.Errorf("no mapping found for local path: %s", localPath)
}

// ResolveLocalPath resolves a remote path to a local path using mappings.
// Returns the local path and format, or an error if no mapping matches.
func (p *Project) ResolveLocalPath(remotePath string) (localPath string, format string, err error) {
	// This is a simplified implementation
	// In production, would need reverse template matching

	for _, m := range p.Config.Mappings {
		// For now, just check if the remote template is a simple pattern
		// Full implementation would do reverse template resolution
		if strings.Contains(m.Remote, remotePath) {
			// Rough match - would need proper implementation
			local := p.ResolvePath(m.Local)
			return local, m.Format, nil
		}
	}

	return "", "", fmt.Errorf("no mapping found for remote path: %s", remotePath)
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
