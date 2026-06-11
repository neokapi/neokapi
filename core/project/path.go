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

// GlobFixedPrefix returns the fixed directory prefix of a glob pattern —
// everything before the first glob metacharacter (*, ?, [, {). For a
// literal path (no glob characters) it returns the directory portion with
// a trailing separator, or "" for a bare filename.
func GlobFixedPrefix(pattern string) string {
	for i, c := range pattern {
		if c == '*' || c == '?' || c == '[' || c == '{' {
			return pattern[:i]
		}
	}
	dir := filepath.Dir(pattern)
	if dir == "." {
		return ""
	}
	return dir + string(filepath.Separator)
}

// ResolveTargetPath expands a content item's target template for one
// source file and target language. {lang} expands to lang; {path},
// {filename}, and {basename} expand from the source path relative to the
// item pattern's fixed prefix, so a `docs/**/*.md` item mirrors its
// subtree under the target root. A bare `*` expands to the source
// basename without extension (legacy shorthand).
func ResolveTargetPath(itemPath, target, source, lang string) string {
	out := ResolvePathPattern(target, lang)
	rel := strings.TrimPrefix(source, GlobFixedPrefix(itemPath))
	out = ExpandTemplate(out, rel)
	base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	return strings.ReplaceAll(out, "*", base)
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
