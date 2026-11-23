package project

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/rootpath"
)

// Inside kapi a locale is canonical BCP-47. In a path it takes the style the
// project declared: `locale_format: posix` yields `messages_nb_NO.json`,
// while the recipe, stores, status output and checks keep `nb-NO`. This file
// is the only place that projection happens.
//
// The `In` variants take the declared format; the bare forms render the locale
// as written. The catalog scanner and the desktop's output lister pass a
// sentinel marker they later match on, which must stay unprojected.

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

// GlobFixedPrefix returns the fixed directory prefix of a glob pattern,
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
// base is the directory the source path is made relative to, it controls
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
// When the target (after {lang} expansion) denotes a directory, it ends with
// "/", or its final segment has no extension and contains no wildcard or token,
// the source's {relpath} is appended. So `output/{lang}/` (or `output/{lang}`)
// mirrors the source tree under that root, the intuitive zero-token form.
func ResolveTargetPath(itemPath, base, target, source, lang string) (string, error) {
	return ResolveTargetPathIn(itemPath, base, target, source, lang, "")
}

// ResolveTargetPathIn is ResolveTargetPath rendering the locale in the
// project's declared style (see the note above ResolvePathPattern). An empty
// format means BCP-47.
func ResolveTargetPathIn(itemPath, base, target, source, lang, localeFormat string) (string, error) {
	lang = locale.FormatCode(lang, localeFormat)
	if strings.Contains(target, "{lang}") && lang != "" {
		if err := CheckSegment("{lang}", lang); err != nil {
			return "", err
		}
	}
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
		return filepath.FromSlash(out), nil
	}

	out, err := ExpandTemplate(out, rel)
	if err != nil {
		return "", err
	}
	if strings.Contains(out, "*") {
		name := format.Stem(rel)
		if err := CheckSegment("*", name); err != nil {
			return "", err
		}
		out = strings.ReplaceAll(out, "*", name)
	}
	return filepath.FromSlash(out), nil
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
	if _, after, ok := strings.CutLast(target, "/"); ok {
		last = after
	}
	if strings.ContainsAny(last, "*?[{") {
		return false // glob or token segment → filename template
	}
	return format.Ext(last) == ""
}

// ExpandTemplate expands path-template tokens in `template` using `localPath`
// (the source path relative to the resolution base, slash-separated). See
// ResolveTargetPath for the supported token set.
func ExpandTemplate(template, localPath string) (string, error) {
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

	// The tokens that stand for one name are filled from a file's name, which
	// the repository chooses, so each must be one safe segment. {relpath},
	// {path} and {dir} are paths a walk or the person produced, and a walk
	// never yields a ".." segment.
	for _, v := range []struct{ token, value string }{
		{"{filename}", filename},
		{"{name}", name},
		{"{basename}", name},
		{"{ext}", strings.TrimPrefix(ext, ".")},
	} {
		if !strings.Contains(template, v.token) || (v.token == "{ext}" && v.value == "") {
			continue
		}
		if err := CheckSegment(v.token, v.value); err != nil {
			return "", err
		}
	}

	r := template
	r = strings.ReplaceAll(r, "{relpath}", localPath)
	r = strings.ReplaceAll(r, "{path}", strings.TrimSuffix(localPath, ext))
	r = strings.ReplaceAll(r, "{dir}", dir)
	r = strings.ReplaceAll(r, "{filename}", filename)
	r = strings.ReplaceAll(r, "{name}", name)
	r = strings.ReplaceAll(r, "{basename}", name)
	r = strings.ReplaceAll(r, "{ext}", strings.TrimPrefix(ext, "."))
	return r, nil
}

// UnsafeSegmentError says a value a path template was to be filled with is
// not a safe path segment: a file's name of ".." in place of {name} would
// climb out of the directory the template names.
type UnsafeSegmentError struct {
	Token string
	Value string
}

func (e *UnsafeSegmentError) Error() string {
	return fmt.Sprintf("%s would be filled with %q, which is not a safe path segment", e.Token, e.Value)
}

// TargetOutsideError says a target a template names leads outside the
// project once its links are resolved: the target file, or a directory on the
// way to it, is a symbolic link out of the project, or a link to nothing. kapi
// neither reads nor writes such a target. Err is the *rootpath.EscapeError,
// which names the link.
type TargetOutsideError struct {
	Target string
	Err    error
}

func (e *TargetOutsideError) Error() string { return e.Err.Error() }

// Unwrap returns Err.
func (e *TargetOutsideError) Unwrap() error { return e.Err }

// CheckTargetInside returns a *TargetOutsideError when target, a path beneath
// root as written, leads outside root once links are resolved. It returns nil
// for a target inside root, one not beneath root as written, and when root is
// empty.
func CheckTargetInside(root, target string) error {
	if root == "" || !rootpath.IsBeneath(root, target) {
		return nil
	}
	if _, err := rootpath.Contain(root, target); err != nil {
		if esc := (*rootpath.EscapeError)(nil); errors.As(err, &esc) {
			return &TargetOutsideError{Target: target, Err: err}
		}
	}
	return nil
}

// CheckSegment refuses a value that is not one path segment: empty, "." or
// "..", or holding a separator.
func CheckSegment(token, v string) error {
	if v == "" || v == "." || v == ".." || strings.ContainsAny(v, `/\`) {
		return &UnsafeSegmentError{Token: token, Value: v}
	}
	return nil
}
