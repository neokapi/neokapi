package host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// commentFormatterTimeout bounds one run of a comment language's formatter.
const commentFormatterTimeout = time.Minute

// commentRewriteDeclarer is implemented by a comment provider whose language a
// plugin declares writable, and returns that declaration.
type commentRewriteDeclarer interface {
	Rewrite() *manifest.CommentRewrite
}

// formattedRewriter is a plugin language's comment provider held to the
// formatter the project of one file uses, so comment.Contain compares each
// rewrite of that file with it.
type formattedRewriter struct {
	comment.Provider
	comment.Rewriter
	*commentFormatter
}

// withCommentFormatter returns p held to the formatter file's project uses when
// p writes a language a plugin declares writable, and p itself otherwise.
func withCommentFormatter(p comment.Provider, file string) comment.Provider {
	d, ok := p.(commentRewriteDeclarer)
	r, writes := p.(comment.Rewriter)
	if !ok || !writes || d.Rewrite() == nil {
		return p
	}
	return &formattedRewriter{Provider: p, Rewriter: r, commentFormatter: resolveCommentFormatter(p, file, d.Rewrite())}
}

// commentFormatter runs a formatter a plugin declares for a comment language
// (manifest.CommentFormatter) as a subprocess, reading a file's bytes on
// standard input.
//
// The formatter runs in the host rather than in the plugin because it belongs
// to the file's project: its configuration and ignore files sit beside the file,
// and the plugin reads bytes and never sees the project. It runs in the
// directory holding the marker that selected it, with the file's own path, so
// the configuration and ignore files that apply to the file apply to what it
// is given.
//
// A formatter that is not installed, or that leaves a file at the path it is
// given as it is, did not run, and every call returns an error wrapping
// comment.ErrFormatterNotRun: a rewrite is written only when a formatter that
// formats the file agrees with it.
type commentFormatter struct {
	provider comment.Provider
	decl     manifest.CommentFormatter
	canary   []byte
	// command is decl.Command with its executable resolved, and dir the
	// directory it runs in. notRun says why no formatter runs, when none does.
	command []string
	dir     string
	notRun  string
	// formatted holds output by input, and verified the canary's outcome by
	// path, for the life of one file's edits.
	formatted map[[sha256.Size]byte][]byte
	verified  map[string]error
}

var _ comment.Formatter = (*commentFormatter)(nil)

// resolveCommentFormatter finds the formatter the project of file uses. The
// directories from file's up to the root are searched in turn for the markers
// each declared formatter lists, and the nearest directory holding one decides.
// The formatter's executable is looked for in node_modules/.bin in that
// directory and above it, and then on PATH.
func resolveCommentFormatter(p comment.Provider, file string, decl *manifest.CommentRewrite) *commentFormatter {
	f := &commentFormatter{
		provider:  p,
		canary:    []byte(decl.FormatterCanary),
		formatted: map[[sha256.Size]byte][]byte{},
		verified:  map[string]error{},
	}
	abs, err := resolvedPath(file)
	if err != nil {
		f.notRun = err.Error()
		return f
	}
	for dir := filepath.Dir(abs); ; dir = filepath.Dir(dir) {
		for _, candidate := range decl.Formatters {
			if marker, ok := formatterMarkerIn(dir, candidate.Detect); ok {
				f.decl, f.dir = candidate, dir
				bin, found := findFormatterBinary(dir, candidate.Command[0])
				if !found {
					f.notRun = fmt.Sprintf("%s formats the files under %s, as %s says, and %s is not installed in node_modules/.bin there or above it, or on PATH",
						candidate.Name, DisplayName(dir), marker, candidate.Command[0])
					return f
				}
				f.command = append([]string{bin}, candidate.Command[1:]...)
				return f
			}
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	var markers []string
	for _, candidate := range decl.Formatters {
		for _, m := range candidate.Detect {
			markers = append(markers, m.File)
		}
	}
	f.notRun = fmt.Sprintf("no formatter for %s is configured for %s: kapi looks for %s in the file's directory and the directories above it",
		p.Language(), DisplayName(file), strings.Join(markers, ", "))
	return f
}

// resolvedPath is the absolute path of name with every symbolic link resolved.
// A formatter matches its configuration and ignore files against the path its
// working directory resolves to, so a file reached through a linked directory
// is given to it by that path, and an ignore file that lists the file applies.
func resolvedPath(name string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return abs, nil
	}
	return filepath.Join(dir, filepath.Base(abs)), nil
}

// formatterMarkerIn returns the first marker in markers that dir holds.
func formatterMarkerIn(dir string, markers []manifest.CommentFormatterMarker) (string, bool) {
	for _, m := range markers {
		path := filepath.Join(dir, m.File)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if m.Contains == "" || bytes.Contains(data, []byte(m.Contains)) {
			return path, true
		}
	}
	return "", false
}

// findFormatterBinary resolves a formatter's executable: an absolute path as it
// is, else node_modules/.bin in dir or a directory above it, else PATH.
func findFormatterBinary(dir, name string) (string, bool) {
	if filepath.IsAbs(name) {
		info, err := os.Stat(name)
		return name, err == nil && !info.IsDir()
	}
	for d := dir; ; d = filepath.Dir(d) {
		candidate := filepath.Join(d, "node_modules", ".bin", name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		if parent := filepath.Dir(d); parent == d {
			break
		}
	}
	path, err := exec.LookPath(name)
	return path, err == nil
}

// FormatterName implements comment.Formatter.
func (f *commentFormatter) FormatterName() string {
	if f.decl.Name == "" {
		return "formatter"
	}
	return f.decl.Name
}

// FormatterCanary implements comment.Formatter with the canary the manifest
// declares.
func (f *commentFormatter) FormatterCanary() []byte { return f.canary }

// Format implements comment.Formatter. Before the formatter's output for a
// path is trusted, the formatter must rewrite the canary given that path.
func (f *commentFormatter) Format(name string, src []byte) ([]byte, error) {
	if f.notRun != "" {
		return nil, fmt.Errorf("%s: %w", f.notRun, comment.ErrFormatterNotRun)
	}
	if err := f.verify(name); err != nil {
		return nil, err
	}
	return f.run(name, src)
}

// Disagreements implements comment.Formatter by locating the formatter's
// output with the provider (comment.CompareFormatted).
func (f *commentFormatter) Disagreements(name string, src []byte, located *comment.File) ([]comment.Disagreement, error) {
	formatted, err := f.Format(name, src)
	if err != nil {
		return nil, err
	}
	return comment.CompareFormatted(f.provider, name, src, formatted, located)
}

// verify runs the canary through the formatter at name once, and reports the
// formatter as not run when it leaves the canary as it is.
func (f *commentFormatter) verify(name string) error {
	if err, done := f.verified[name]; done {
		return err
	}
	out, err := f.run(name, f.canary)
	switch {
	case err != nil:
		err = fmt.Errorf("%s could not format its canary at %s: %w: %w", f.FormatterName(), DisplayName(name), err, comment.ErrFormatterNotRun)
	case bytes.Equal(out, f.canary):
		err = fmt.Errorf("%s leaves a file at %s as it is, as a formatter set to ignore the file does, so its agreement shows nothing: %w",
			f.FormatterName(), DisplayName(name), comment.ErrFormatterNotRun)
	}
	f.verified[name] = err
	return err
}

// run formats src as the file at name, reusing the output for input it has
// already formatted.
func (f *commentFormatter) run(name string, src []byte) ([]byte, error) {
	key := sha256.Sum256(append([]byte(name+"\x00"), src...))
	if out, ok := f.formatted[key]; ok {
		return out, nil
	}
	abs, err := resolvedPath(name)
	if err != nil {
		return nil, err
	}
	args := make([]string, len(f.command)-1)
	for i, a := range f.command[1:] {
		args[i] = strings.ReplaceAll(a, "{file}", abs)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commentFormatterTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.command[0], args...)
	cmd.Dir = f.dir
	cmd.Stdin = bytes.NewReader(src)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, fmt.Errorf("run %s: %s: %w", f.FormatterName(), detail, comment.ErrFormatterNotRun)
		}
		return nil, fmt.Errorf("%s exited with %d: %s", f.FormatterName(), exit.ExitCode(), detail)
	}
	out := stdout.Bytes()
	f.formatted[key] = out
	return out, nil
}
