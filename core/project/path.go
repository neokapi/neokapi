package project

import (
	"path/filepath"
	"strings"
)

// ResolvePathPattern expands the `{lang}` placeholder in a path pattern.
func ResolvePathPattern(pattern, lang string) string {
	return strings.ReplaceAll(pattern, "{lang}", lang)
}

// ResolveFormat returns the format ID, treating "$auto" and the empty
// string as equivalent (both meaning auto-detect).
func ResolveFormat(format string) string {
	if format == "$auto" {
		return ""
	}
	return format
}

// ExpandTemplate expands path-template variables in `template` using
// `localPath` as the source path. Supported variables:
//
//	{path}     — relative path without extension
//	{filename} — filename with extension
//	{basename} — filename without extension
func ExpandTemplate(template, localPath string) string {
	result := template
	pathNoExt := strings.TrimSuffix(localPath, filepath.Ext(localPath))
	result = strings.ReplaceAll(result, "{path}", pathNoExt)
	filename := filepath.Base(localPath)
	result = strings.ReplaceAll(result, "{filename}", filename)
	basename := strings.TrimSuffix(filename, filepath.Ext(filename))
	result = strings.ReplaceAll(result, "{basename}", basename)
	return result
}
