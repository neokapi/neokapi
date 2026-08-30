package project

import (
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/locale"
)

// Rendering a locale into a path is the one place a style other than BCP-47
// belongs.
//
// Inside kapi a locale is canonical BCP-47. On disk it is whatever the project
// declared: a repository whose tooling expects `messages_nb_NO.json` says
// `locale_format: posix` and gets those filenames, while its recipe, its
// stores, its status output and its checks all keep saying `nb-NO`. The
// projection happens here, at the boundary, and nowhere else.
//
// The `In` variants take the declared format; the bare forms render the locale
// as written. Two callers pass a sentinel rather than a locale — the catalog
// scanner and the desktop's output lister both substitute a marker they later
// match on — and a sentinel is not a locale to be projected.

// ResolvePathPattern expands the `{lang}` placeholder in a path pattern,
// rendering the locale exactly as given.
func ResolvePathPattern(pattern, lang string) string {
	return strings.ReplaceAll(pattern, "{lang}", lang)
}

// ResolvePathPatternIn expands `{lang}`, rendering the locale in the project's
// declared style. An empty format means BCP-47, which is the default and leaves
// the locale untouched.
func ResolvePathPatternIn(pattern, lang, localeFormat string) string {
	return ResolvePathPattern(pattern, locale.FormatCode(lang, localeFormat))
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

// ResolveTargetPath expands a content item's target template for one source
// file and target language, returning the output path (relative to the project
// root, in OS-native separators).
//
// base is the directory the source path is made relative to — it controls
// {path}/{dir}/{relpath} and how much of the source tree a directory-mirror
// target reproduces. When base is "", it defaults to the glob's fixed prefix
// (GlobFixedPrefix(itemPath)), so `input/docs/*.md` mirrors just filenames while
// `input/**/*.md` (or an explicit base) mirrors the subtree.
//
// Tokens (all optional):
//
//	{lang}     target language
//	{relpath}  source path relative to base, WITH extension   (docs/api.md)
//	{path}     source path relative to base, WITHOUT extension (docs/api)
//	{dir}      directory portion of {relpath}, "" at the root  (docs)
//	{filename} source filename with extension                  (api.md)
//	{name}     source filename without extension                (api)  [alias {basename}]
//	{ext}      source extension without the dot                 (md)
//	*          legacy shorthand for {name}
//
// When the target (after {lang} expansion) denotes a directory — it ends with
// "/", or its final segment has no extension and contains no wildcard or token —
// the source's {relpath} is appended. So `output/{lang}/` (or `output/{lang}`)
// mirrors the source tree under that root, the intuitive zero-token form.
func ResolveTargetPath(itemPath, base, target, source, lang string) string {
	return ResolveTargetPathIn(itemPath, base, target, source, lang, "")
}

// ResolveTargetPathIn is ResolveTargetPath rendering the locale in the
// project's declared style (see the note above ResolvePathPattern). An empty
// format means BCP-47.
func ResolveTargetPathIn(itemPath, base, target, source, lang, localeFormat string) string {
	lang = locale.FormatCode(lang, localeFormat)
	source = filepath.ToSlash(source)
	if base == "" {
		base = GlobFixedPrefix(itemPath)
	}
	base = filepath.ToSlash(base)
	if base != "" && !strings.HasSuffix(base, "/") {
		base += "/"
	}
	rel := strings.TrimPrefix(source, base)

	out := ResolvePathPattern(target, lang)

	if isDirectoryTarget(out) {
		out = strings.TrimRight(out, "/")
		if out == "" {
			out = rel
		} else {
			out += "/" + rel
		}
		return filepath.FromSlash(out)
	}

	out = ExpandTemplate(out, rel)
	name := format.Stem(rel)
	out = strings.ReplaceAll(out, "*", name)
	return filepath.FromSlash(out)
}

// isDirectoryTarget reports whether target (after {lang} expansion) denotes a
// directory to mirror into rather than a filename template. True when it ends
// with "/", is empty, or its final segment carries no file extension and no
// wildcard/template token.
func isDirectoryTarget(target string) bool {
	if target == "" || strings.HasSuffix(target, "/") {
		return true
	}
	last := target
	if i := strings.LastIndex(target, "/"); i >= 0 {
		last = target[i+1:]
	}
	if strings.ContainsAny(last, "*?[{") {
		return false // glob or token segment → filename template
	}
	return format.Ext(last) == ""
}

// ExpandTemplate expands path-template tokens in `template` using `localPath`
// (the source path relative to the resolution base, slash-separated). See
// ResolveTargetPath for the supported token set.
func ExpandTemplate(template, localPath string) string {
	localPath = filepath.ToSlash(localPath)
	filename := filepath.Base(localPath)
	// A compound suffix such as ".kbf.json" has to come off whole, or {name}
	// keeps a stray ".kbf" and the expanded target gains a second suffix.
	ext := format.Ext(filename)
	name := strings.TrimSuffix(filename, ext)
	dir := filepath.Dir(localPath)
	if dir == "." {
		dir = ""
	}

	r := template
	r = strings.ReplaceAll(r, "{relpath}", localPath)
	r = strings.ReplaceAll(r, "{path}", strings.TrimSuffix(localPath, ext))
	r = strings.ReplaceAll(r, "{dir}", dir)
	r = strings.ReplaceAll(r, "{filename}", filename)
	r = strings.ReplaceAll(r, "{name}", name)
	r = strings.ReplaceAll(r, "{basename}", name)
	r = strings.ReplaceAll(r, "{ext}", strings.TrimPrefix(ext, "."))
	return r
}
