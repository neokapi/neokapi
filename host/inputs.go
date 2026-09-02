package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/project"
)

// Input conventions (one resolver for every command that takes file inputs).
//
// Three rules hold uniformly across the command surface, so a pattern that
// works for `kapi stats` works identically for `kapi check`, `kapi inspect`,
// `kapi grep`, … :
//
//  1. Globs expand in-process. `kapi stats 'src/**'` behaves the same on zsh,
//     bash and PowerShell whether or not the shell expanded the pattern first,
//     and `**` always means "recursive" (doublestar), which `filepath.Glob`
//     cannot express. Quoting the pattern is the portable spelling; leaving it
//     unquoted still works because a shell-expanded list of concrete paths is
//     just the resolved form of the same input.
//
//  2. Directories are content, not errors. A directory argument expands to the
//     regular files beneath it, skipping hidden directories and editor junk.
//
//  3. No input never means "block on the terminal". With nothing to read, a
//     command falls back to the project's tracked content set when a recipe is
//     in scope, then to standard input only when stdin is actually a pipe or a
//     file. On an interactive terminal it reports what it wanted instead of
//     hanging with no prompt. Standard input is always available explicitly as
//     the conventional "-" argument.

// StdinFallback selects what a command does when it is given no file arguments.
type StdinFallback int

const (
	// FallbackProjectThenStdin is the default for content verbs (stats, check,
	// inspect, …): with a recipe in scope the project's tracked content set is
	// the implied input; otherwise a piped stdin is read. On an interactive
	// terminal the command reports how to give it input rather than blocking.
	FallbackProjectThenStdin StdinFallback = iota
	// FallbackStdinOnly is for the toolbox utilities that emulate a Unix filter
	// (kcat, kgrep, ksed): with no arguments they read stdin, like their
	// namesakes — but still only when stdin is not an interactive terminal.
	FallbackStdinOnly
	// FallbackNone is for commands that require explicit inputs and have no
	// implicit source at all.
	FallbackNone
)

// InputOptions tunes ResolveInputs for one command.
type InputOptions struct {
	// Fallback selects the no-argument behavior. Zero value is
	// FallbackProjectThenStdin.
	Fallback StdinFallback
	// RequireRecursiveForDirs keeps the POSIX `grep -r` contract: a directory
	// argument is skipped (reported through OnSkip) unless Recursive is set.
	// Off by default — directories expand like any other content selector.
	RequireRecursiveForDirs bool
	// Recursive is the command's -r/--recursive flag; only consulted when
	// RequireRecursiveForDirs is set.
	Recursive bool
	// Command names the command in diagnostics ("kapi stats: …").
	Command string
	// OnSkip receives arguments that could not be used (missing path, a
	// directory under POSIX rules). Skipping is reported, never fatal.
	OnSkip func(path string, err error)
}

// ErrNoInput is returned when a command has nothing to read and cannot fall
// back: no file arguments, no project content set, and an interactive stdin
// that the user never asked to be read. Blocking on a terminal for input the
// user did not know to type is never the right answer, so this is an error
// with an actionable message instead.
var ErrNoInput = errors.New("no input")

// ResolveInputs turns a command's positional arguments into a concrete file
// list, applying the three input conventions documented above. The returned
// slice is either a list of real paths (possibly with `container!entry`
// locators) or the single element StdinName.
func (a *App) ResolveInputs(cmd Command, args []string, opts InputOptions) ([]string, error) {
	if len(args) > 0 {
		return expandArgs(args, opts)
	}
	return a.implicitInputs(cmd, opts)
}

// implicitInputs resolves the no-arguments case.
func (a *App) implicitInputs(cmd Command, opts InputOptions) ([]string, error) {
	name := opts.Command
	if name == "" {
		name = "kapi"
	}

	if opts.Fallback == FallbackProjectThenStdin {
		files, projectPath, err := a.projectContentFiles(cmd)
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			if !a.Quiet {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: no files given, using the %d file(s) tracked by %s\n",
					name, len(files), relativeToCwd(projectPath))
			}
			return files, nil
		}
		if projectPath != "" {
			// A recipe is in scope but tracks nothing: say so rather than
			// silently falling through to stdin, which would look like a hang.
			return nil, WithExitCode(ExitUsage, fmt.Errorf(
				"%s: %s tracks no content. Add files with `kapi add <pattern>`, or pass paths explicitly: %w",
				name, DisplayName(projectPath), ErrNoInput))
		}
	}

	if opts.Fallback == FallbackNone {
		return nil, WithExitCode(ExitUsage, fmt.Errorf("%s: no input files given: %w", name, ErrNoInput))
	}

	if stdinIsPipe() {
		return []string{StdinName}, nil
	}
	return nil, WithExitCode(ExitUsage, fmt.Errorf(
		"%s: no input. Pass files or a glob (e.g. `%s 'src/**'`), pipe content in, or pass `-` to read standard input: %w",
		name, name, ErrNoInput))
}

// projectContentFiles resolves the tracked content set of the recipe in scope,
// returning the files and the recipe path. Both are empty when no recipe is in
// scope (or discovery is disabled by KAPI_NO_PROJECT).
func (a *App) projectContentFiles(cmd Command) ([]string, string, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		// A discovery failure is not this command's problem: treat it as "no
		// project" and let the stdin fallback below report the real situation.
		return nil, "", nil //nolint:nilerr // discovery trouble degrades to no-project
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, "", fmt.Errorf("load project: %w", err)
	}
	a.InitRegistries()
	pctx := project.NewProjectContext(proj, projectPath)
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, "", fmt.Errorf("resolve project content: %w", err)
	}
	// The content resolver returns absolute paths; report them the way the user
	// would type them, so findings and per-file rows read identically whether
	// the files were named or implied.
	files := make([]string, 0, len(resolved))
	for _, rf := range resolved {
		files = append(files, relativeToCwd(rf.Path))
	}
	return files, projectPath, nil
}

// relativeToCwd shortens an absolute path to a cwd-relative one when it lies
// under the working directory, and leaves it absolute otherwise.
func relativeToCwd(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// expandArgs expands explicit arguments: stdin tokens, archive locators, globs,
// directories and plain files.
func expandArgs(args []string, opts InputOptions) ([]string, error) {
	var files []string
	seen := map[string]struct{}{}
	add := func(p string) {
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		files = append(files, p)
	}

	for _, arg := range args {
		if arg == StdinName {
			add(arg)
			continue
		}
		// A `container!entry` locator (AD-026 §6) names one file inside an
		// archive; keep it verbatim — os.Stat on the whole string would fail.
		if hasEntryLocator(arg) {
			add(arg)
			continue
		}

		matches, err := expandOne(arg, opts)
		if err != nil {
			return nil, err
		}
		if matches == nil {
			// Reported through OnSkip inside expandOne.
			continue
		}
		for _, m := range matches {
			add(m)
		}
	}
	return files, nil
}

// expandOne resolves a single argument to zero or more concrete files. A nil
// (rather than empty) result means the argument was skipped and reported.
func expandOne(arg string, opts InputOptions) ([]string, error) {
	if looksLikeGlobArg(arg) {
		return expandGlobArg(arg, opts)
	}

	info, err := os.Stat(arg)
	if err != nil {
		skip(opts, arg, err)
		return nil, nil
	}
	if info.IsDir() {
		if opts.RequireRecursiveForDirs && !opts.Recursive {
			skip(opts, arg, errors.New("is a directory"))
			return nil, nil
		}
		return walkDirFiles(arg)
	}
	// Drop editor lock/metadata stubs silently — they are never content, so
	// skipping is not an error (exit status stays 0).
	if isJunkFile(filepath.Base(arg)) {
		return []string{}, nil
	}
	return []string{arg}, nil
}

// expandGlobArg expands a glob pattern in-process with doublestar semantics
// (`**` recursive, `{a,b}` alternation), relative to the pattern's own
// non-magic prefix so absolute and relative patterns behave alike. A directory
// that matches expands to the files beneath it, which is what makes
// `kapi stats 'src/**'` mean "everything under src".
func expandGlobArg(pattern string, opts InputOptions) ([]string, error) {
	root, rel := splitGlobRoot(pattern)
	rels, err := project.ExpandGlob(root, rel)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
	}
	// A wildcard does not reach into dot-directories, matching both the shell
	// and the directory walk: `kapi stats '**'` must not descend into .git.
	// Naming one explicitly (`.github/**/*.yml`) opts back in, so the hidden
	// tree is reachable — just never by accident.
	if !wantsHidden(rel) {
		rels = slices.DeleteFunc(rels, hasHiddenSegment)
	}
	if len(rels) == 0 {
		skip(opts, pattern, errors.New("no matches"))
		return nil, nil
	}
	sort.Strings(rels)

	var out []string
	for _, r := range rels {
		p := filepath.Join(root, filepath.FromSlash(r))
		info, err := os.Stat(p)
		if err != nil {
			// Report the drop instead of dropping it. Whether the user is told
			// used to depend only on whether they typed a wildcard: expandOne
			// calls skip() for an unreadable file named explicitly (which
			// resolveFiles turns into a non-zero exit), while a match from a glob
			// disappeared with no signal at all — `Processed 11 files` when the
			// glob matched 12, invisible to --on-skip and to the exit code.
			skip(opts, p, err)
			continue
		}
		if info.IsDir() {
			// `src/**` matches directories as well as files; a directory
			// contributes its files, so the pattern reads as "everything under
			// src" rather than erroring on each intermediate directory.
			walked, werr := walkDirFiles(p)
			if werr != nil {
				return nil, werr
			}
			out = append(out, walked...)
			continue
		}
		if isJunkFile(filepath.Base(p)) {
			continue
		}
		out = append(out, p)
	}
	if out == nil {
		// Matched only directories that contained nothing usable.
		return []string{}, nil
	}
	return out, nil
}

// splitGlobRoot splits a glob pattern into the longest leading directory
// prefix that contains no glob metacharacters, and the remaining pattern
// relative to it. doublestar matches against an fs.FS rooted at the prefix,
// which is also what keeps absolute patterns working.
func splitGlobRoot(pattern string) (root, rel string) {
	slashed := filepath.ToSlash(pattern)
	parts := strings.Split(slashed, "/")
	cut := 0
	for i, p := range parts {
		if isGlobPattern(p) {
			break
		}
		cut = i + 1
	}
	// The final component is the pattern itself even when it has no magic
	// (callers only reach here for magic patterns), so never consume it.
	if cut >= len(parts) {
		cut = len(parts) - 1
	}
	prefix := strings.Join(parts[:cut], "/")
	rel = strings.Join(parts[cut:], "/")
	switch {
	case prefix == "" && strings.HasPrefix(slashed, "/"):
		root = string(filepath.Separator)
	case prefix == "":
		root = "."
	case strings.HasPrefix(slashed, "/") && !strings.HasPrefix(prefix, "/"):
		root = string(filepath.Separator) + filepath.FromSlash(prefix)
	default:
		root = filepath.FromSlash(prefix)
	}
	if rel == "" {
		rel = "*"
	}
	return root, rel
}

// wantsHidden reports whether a pattern explicitly names a dot-prefixed
// segment, which opts its expansion into hidden trees.
func wantsHidden(pattern string) bool {
	for seg := range strings.SplitSeq(pattern, "/") {
		if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
			return true
		}
	}
	return false
}

// hasHiddenSegment reports whether a slash-separated relative path passes
// through a dot-prefixed directory or names a dotfile.
func hasHiddenSegment(rel string) bool {
	for seg := range strings.SplitSeq(rel, "/") {
		if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
			return true
		}
	}
	return false
}

// looksLikeGlobArg reports whether arg should be expanded as a glob pattern. A
// path that exists verbatim is never treated as a pattern, so a file
// legitimately named with brackets still resolves to itself.
func looksLikeGlobArg(arg string) bool {
	if !isGlobPattern(arg) {
		return false
	}
	if _, err := os.Lstat(arg); err == nil {
		return false
	}
	return true
}

func skip(opts InputOptions, path string, err error) {
	if opts.OnSkip != nil {
		opts.OnSkip(path, err)
	}
}

// stdinIsPipe reports whether standard input is redirected (a pipe or a
// regular file) rather than an interactive terminal. Only then may a command
// read it implicitly: on a terminal, reading would block with no prompt.
//
// It is a variable so tests can stand in for a terminal, which they otherwise
// never have — under `go test` stdin is always redirected, which is precisely
// the case that does not reproduce the reported hang.
var stdinIsPipe = func() bool {
	fd := os.Stdin.Fd()
	return !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd)
}
