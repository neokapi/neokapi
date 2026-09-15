package host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

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
// p writes a language a plugin declares writable, and p itself otherwise. The
// formatter runs only when trust allows it.
func withCommentFormatter(p comment.Provider, file string, trust *formatterTrust) comment.Provider {
	d, ok := p.(commentRewriteDeclarer)
	r, writes := p.(comment.Rewriter)
	if !ok || !writes || d.Rewrite() == nil {
		return p
	}
	return &formattedRewriter{Provider: p, Rewriter: r, commentFormatter: resolveCommentFormatter(p, file, d.Rewrite(), trust)}
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
// A formatter did not run when execution trust does not allow it
// (formatterTrust), when it is not installed, or when it leaves a file at the
// path it is given as it is. Every call then returns an error wrapping
// comment.ErrFormatterNotRun, so a rewrite is written only when a formatter that
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
// directory and above it, and then on PATH, and the formatter runs only when
// trust allows the command the marker selects, together with every
// configuration file it can load for file.
func resolveCommentFormatter(p comment.Provider, file string, decl *manifest.CommentRewrite, trust *formatterTrust) *commentFormatter {
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
				if reason := trust.refusal(marker, candidate.Name); reason != "" {
					f.notRun = reason
					return f
				}
				bin, skipped, found := findFormatterBinary(dir, candidate.Command[0])
				if !found {
					f.notRun = fmt.Sprintf("%s formats the files under %s, as %s says, and %s is not installed in node_modules/.bin there or above it, or on PATH",
						candidate.Name, DisplayName(dir), marker, candidate.Command[0])
					if len(skipped) > 0 {
						f.notRun += ", where kapi skips the entries that are not absolute paths: " + strings.Join(skipped, ", ")
					}
					return f
				}
				command := append([]string{bin}, candidate.Command[1:]...)
				configs := formatterConfigFiles(filepath.Dir(abs), dir, candidate.Detect)
				if err := trust.allow(marker, candidate.Name, command, configs); err != nil {
					f.notRun = err.Error()
					return f
				}
				f.command = command
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
		switch {
		case m.Key != "":
			if hasTopLevelKey(path, data, m.Key) {
				return path, true
			}
		case m.Contains == "" || bytes.Contains(data, []byte(m.Contains)):
			return path, true
		}
	}
	return "", false
}

// formatterConfigFiles lists the configuration files a formatter can load for a
// file in from: every file its markers name, in from and each directory above it
// up to to, the directory of the marker that selected it. A marker with a Key
// names a file the formatter loads only when it holds that key. A marker's
// Contains is not consulted, since the formatter loads such a file whatever it
// holds.
func formatterConfigFiles(from, to string, markers []manifest.CommentFormatterMarker) []string {
	var files []string
	for dir := from; ; dir = filepath.Dir(dir) {
		for _, m := range markers {
			path := filepath.Join(dir, m.File)
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			if m.Key != "" {
				data, err := os.ReadFile(path)
				if err != nil || !hasTopLevelKey(path, data, m.Key) {
					continue
				}
			}
			files = append(files, path)
		}
		if parent := filepath.Dir(dir); dir == to || parent == dir {
			break
		}
	}
	return files
}

// hasTopLevelKey reports whether data, the document at path, holds key at its
// top level with a value other than null, false, zero or an empty string. A
// file ending in .yaml or .yml is read as YAML and any other file as JSON, and a
// document that does not parse holds no key.
func hasTopLevelKey(path string, data []byte, key string) bool {
	var doc map[string]any
	var err error
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &doc)
	default:
		err = json.Unmarshal(data, &doc)
	}
	if err != nil {
		return false
	}
	switch v := doc[key].(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return true
	}
}

// findFormatterBinary resolves a formatter's executable: an absolute path as it
// is, else node_modules/.bin in dir or a directory above it, else a directory on
// PATH. A PATH entry that is not an absolute path names a directory relative to
// kapi's working directory, which can be the project, so it is skipped, and the
// entries skipped are returned quoted.
func findFormatterBinary(dir, name string) (string, []string, bool) {
	if filepath.IsAbs(name) {
		info, err := os.Stat(name)
		return name, nil, err == nil && !info.IsDir()
	}
	for d := dir; ; d = filepath.Dir(d) {
		candidate := filepath.Join(d, "node_modules", ".bin", name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil, true
		}
		if parent := filepath.Dir(d); parent == d {
			break
		}
	}
	absolute, skipped := splitPathEntries(os.Getenv("PATH"))
	for _, entry := range absolute {
		if bin, err := exec.LookPath(filepath.Join(entry, name)); err == nil {
			return bin, skipped, true
		}
	}
	return "", skipped, false
}

// splitPathEntries splits a PATH value into its entries that are absolute paths
// and the entries that are not, quoted. An empty entry names the working
// directory and is not absolute.
func splitPathEntries(value string) (absolute, skipped []string) {
	for _, entry := range filepath.SplitList(value) {
		if filepath.IsAbs(entry) {
			absolute = append(absolute, entry)
			continue
		}
		skipped = append(skipped, strconv.Quote(entry))
	}
	return absolute, skipped
}

// lookPathAbsolute finds the program name in the PATH entries that are absolute
// paths, which are the entries a formatter's environment holds (formatterEnv).
func lookPathAbsolute(name string) (string, bool) {
	absolute, _ := splitPathEntries(os.Getenv("PATH"))
	for _, entry := range absolute {
		if bin, err := exec.LookPath(filepath.Join(entry, name)); err == nil {
			return bin, true
		}
	}
	return "", false
}

// formatterEnv is the environment a formatter runs with: environ with every
// PATH entry that is not an absolute path removed. Such an entry names a
// directory relative to the formatter's working directory, which is the
// project's, so a program the formatter or its interpreter looked up there
// would be the project's.
func formatterEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, value, ok := strings.Cut(kv, "=")
		if ok && (name == "PATH" || runtime.GOOS == "windows" && strings.EqualFold(name, "PATH")) {
			absolute, _ := splitPathEntries(value)
			kv = name + "=" + strings.Join(absolute, string(os.PathListSeparator))
		}
		out = append(out, kv)
	}
	return out
}

// formatterRuntime is what a formatter's executable runs besides itself.
type formatterRuntime struct {
	// interpreter is the program that runs the executable: the one a script's
	// first line names, or the node a node_modules/.bin shim runs.
	interpreter string
	// script is the file a node_modules/.bin shim hands to node.
	script string
}

// cmdShimNode matches the line a node_modules/.bin shim written by npm's and
// pnpm's cmd-shim uses to run node on its target, and captures the target.
var cmdShimNode = regexp.MustCompile(`exec "\$basedir/node"\s+(?:"([^"]+)"|'([^']+)')`)

// formatterShebangBytes bounds how much of an executable is read to find what
// it runs.
const formatterShebangBytes = 64 << 10

// formatterRuntimeOf resolves what the executable at path runs, as the
// formatter's environment (formatterEnv) resolves it. A node_modules/.bin shim
// runs node from its own directory when that directory holds an executable
// node, and otherwise the node on PATH, on the target it names. A script whose
// first line is `#!/usr/bin/env name` runs name from PATH, and one naming an
// interpreter's path runs that interpreter. An executable without a first
// `#!` line runs nothing kapi can name.
func formatterRuntimeOf(path string) formatterRuntime {
	f, err := os.Open(path)
	if err != nil {
		return formatterRuntime{}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, formatterShebangBytes))
	if err != nil || !bytes.HasPrefix(data, []byte("#!")) {
		return formatterRuntime{}
	}
	dir := filepath.Dir(path)
	if m := cmdShimNode.FindSubmatch(data); m != nil {
		var rt formatterRuntime
		if node := filepath.Join(dir, "node"); isExecutableFile(node) {
			rt.interpreter = node
		} else if node, ok := lookPathAbsolute("node"); ok {
			rt.interpreter = node
		}
		target := string(m[1])
		if target == "" {
			target = string(m[2])
		}
		target = strings.ReplaceAll(target, "$basedir", dir)
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		rt.script = filepath.Clean(target)
		return rt
	}
	line, _, _ := bytes.Cut(data[2:], []byte("\n"))
	fields := strings.Fields(string(line))
	if len(fields) == 0 {
		return formatterRuntime{}
	}
	if filepath.Base(fields[0]) != "env" {
		return formatterRuntime{interpreter: fields[0]}
	}
	for _, name := range fields[1:] {
		if strings.HasPrefix(name, "-") || strings.Contains(name, "=") {
			continue
		}
		if interpreter, ok := lookPathAbsolute(name); ok {
			return formatterRuntime{interpreter: interpreter}
		}
		break
	}
	return formatterRuntime{}
}

// isExecutableFile reports whether path is a regular file some user may execute.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
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
// already formatted. A formatter with no resolved command runs nothing.
func (f *commentFormatter) run(name string, src []byte) ([]byte, error) {
	if len(f.command) == 0 {
		return nil, fmt.Errorf("no formatter command is resolved for %s: %w", DisplayName(name), comment.ErrFormatterNotRun)
	}
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
	cmd.Env = formatterEnv(os.Environ())
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
