package host

// Toolbox: format-aware reimaginings of the classic Unix text utilities —
// cat, grep, sed, diff, plus a convert (conv) verb — that operate on the
// text/content inside any format kapi understands (JSON catalogs, Markdown,
// HTML, Word documents, …) rather than raw bytes. They share kapi's
// reader/writer pipeline, so `kgrep` greps the prose inside a .docx, `ksed`
// rewrites it and saves the document back byte-for-byte, and `kconv`
// re-expresses it in another format.
//
// Each is exposed two ways:
//   - as a kapi subcommand: `kapi grep`, `kapi sed`, `kapi cat`, `kapi convert`
//   - as a multi-call ("busybox") binary: the kapi binary, when invoked through
//     a `kgrep` / `ksed` / `kcat` / `kconv` symlink, dispatches to the matching
//     command as a standalone root (see BusyboxRoot). One binary, four extra
//     names, no extra size.
//
// In standalone form the commands carry the full classic option surface
// (including the -v / -c shorthands kapi's persistent flags otherwise reserve);
// as kapi subcommands the few conflicting shorthands fall back to long flags.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
)

// StdinName is the conventional path token for standard input.
const StdinName = "-"

// StdoutName is the same token on the output side: `kconv -o -` is the explicit
// "write to standard output", which is also how it says "yes, even if the
// document is binary and stdout is a terminal" (see binaryout.go). curl spells
// the same opt-in `--output -`.
const StdoutName = "-"

// FallbackFormat is used when neither extension nor content sniffing resolves a
// format — keeps stdin and unknown files working as plain text, like the
// classic tools.
const FallbackFormat = "plaintext"

// MapToolboxErr maps a toolbox utility's RunE result to the grep-style exit
// code contract: nil on match (0), ErrSilentExit on no-match (1, message
// suppressed), context.Canceled on interrupt (130), and any other operational
// trouble (bad pattern, unreadable file, …) to ExitUsage (2) — matching
// grep/sed/cat and the utilities' own --help. The underlying message is
// preserved.
func MapToolboxErr(err error) error {
	if err == nil || errors.Is(err, ErrSilentExit) || errors.Is(err, context.Canceled) {
		return err
	}
	return WithExitCode(ExitUsage, err)
}

// DisplayName is the file label used in output and error messages; stdin shows
// as the conventional "(standard input)".
func DisplayName(path string) string {
	if path == "" || path == StdinName {
		return "(standard input)"
	}
	return path
}

// readContent reads a file path, or standard input when path is "" or "-".
//
// A terminal stdin read blocks until EOF, so we run it on a goroutine and race
// it against ctx: cli.Run traps SIGINT and turns it into context cancellation
// (it does not let the signal kill the process), and a plain io.ReadAll would
// never observe that — Ctrl-C on `kcat` with no FILE would hang. Racing ctx
// lets the command return context.Canceled (→ exit 130) while the orphaned read
// goroutine is torn down at process exit.
func readContent(ctx context.Context, path string) ([]byte, error) {
	if path != "" && path != StdinName {
		return os.ReadFile(path)
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	// Snapshot os.Stdin here, in the caller, rather than inside the goroutine.
	// We deliberately leak this goroutine (the select below returns on ctx.Done
	// without waiting for it), so reading the os.Stdin global from inside it would
	// race any later restore of os.Stdin — exactly what tests that swap stdin do.
	stdin := os.Stdin
	go func() {
		data, err := io.ReadAll(stdin)
		done <- result{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-done:
		return r.data, r.err
	}
}

// ErrBinaryContent reports input that no format claimed and whose bytes are
// binary. Reading it as plain text — the fallback that serves extensionless
// prose — would print or convert the raw bytes, so the toolbox refuses instead.
// See the guard in binary.go.
var ErrBinaryContent = errors.New("binary file")

// errBinaryInput is the per-file error the toolbox reports for binary input.
// The message names the escape hatch, because a binary file kapi *does*
// understand is exactly what --format is for.
func errBinaryInput() error {
	return fmt.Errorf("%w: no text format detected; pass -f FORMAT to read it as one anyway", ErrBinaryContent)
}

// ResolveFormatName picks the format for a path + content. An explicit --format
// wins; otherwise it runs the framework's canonical detection cascade
// (extension → container-aware content sniffing) and falls back to plain text.
//
// For stdin there is no filename, so detection is purely content-based — it
// routes through the same Detector.Detect path as files, which means piped
// documents (a .docx, a JSON catalog) are recognised via content sniffing, and
// only genuinely unidentifiable input falls back to plain text. This is the one
// place the toolbox decides a format, so both files and stdin share it — which
// is also why the binary guard lives here: every toolbox command inherits it
// from this one function.
func (a *App) ResolveFormatName(path string, content []byte) (string, error) {
	if name, ok := a.explicitOrDetected(path, bytes.NewReader(content)); ok {
		return name, nil
	}
	if a.binaryBytes(content) {
		return "", errBinaryInput()
	}
	return FallbackFormat, nil
}

// resolveFormatFrom is ResolveFormatName over a seekable stream rather than a
// buffer. The detector reads a prefix and seeks back, so an open file resolves
// its format without being read into memory — which is the whole point for a
// package format, where the buffer would be the file.
func (a *App) resolveFormatFrom(path string, content io.ReadSeeker) (string, error) {
	if name, ok := a.explicitOrDetected(path, content); ok {
		return name, nil
	}
	if a.binaryStream(content) {
		return "", errBinaryInput()
	}
	return FallbackFormat, nil
}

// explicitOrDetected runs the two stages that can name a format — an explicit
// --format, then the detection cascade — and reports whether either did. A
// false result is the fallback case, and the only one the binary guard sees.
func (a *App) explicitOrDetected(path string, content io.ReadSeeker) (string, bool) {
	if a.FormatFlag != "" {
		return preset.ParseFormatRef(a.FormatFlag).RegistryName(), true
	}
	// stdin carries no usable path; let Detect skip the extension stage.
	detectPath := path
	if detectPath == StdinName {
		detectPath = ""
	}
	if name, ok := a.resolveNamedFile(detectPath, content); ok {
		return name, true
	}
	if name, err := a.FormatReg.Detector().Detect(detectPath, content, ""); err == nil && name != "" {
		return name, true
	}
	return "", false
}

// resolveNamedFile resolves a file with an extension through the registry's
// engine resolver (the one every named-file entry point uses), sniffing the
// content it is handed when the chosen engine's formats need telling apart.
// A path without an extension, or one no format claims, reports false so the
// caller can sniff the content on its own.
func (a *App) resolveNamedFile(path string, content io.ReadSeeker) (string, bool) {
	if format.Ext(path) == "" {
		return "", false
	}
	opts := registry.DetectOptions{}
	if a.ProjectContext != nil {
		opts = a.ProjectContext.DetectOptions(nil)
	}
	if content != nil {
		opts.Content = func() (io.ReadSeeker, error) {
			if _, err := content.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			return unclosable{content}, nil
		}
	}
	name, err := a.FormatReg.Detect(path, opts)
	if content != nil {
		_, _ = content.Seek(0, io.SeekStart)
	}
	if err != nil || name == "" {
		return "", false
	}
	return string(name), true
}

// unclosable hands a stream to a sniff that closes what it is given, without
// closing the caller's stream.
type unclosable struct{ io.ReadSeeker }

// StreamBlocks opens path (or stdin), detects its format, and calls fn for each
// Block part in document order. Read-only — the backbone of cat and grep.
func (a *App) StreamBlocks(ctx context.Context, path string, fn func(index int, b *model.Block) error) (string, error) {
	return a.streamBlocks(ctx, path, fn)
}

// openReader builds the reader for a document and names its format: the
// --format flag, with a preset it names applied to the reader, and otherwise
// the format detection finds in content.
func (a *App) openReader(path string, content io.ReadSeeker) (string, format.DataFormatReader, error) {
	if a.FormatFlag != "" {
		reader, name, err := a.NewConfiguredReader(a.FormatFlag)
		if err != nil {
			return "", nil, err
		}
		return name, reader, nil
	}
	name, err := a.resolveFormatFrom(path, content)
	if err != nil {
		return "", nil, err
	}
	reader, err := a.FormatReg.NewReader(registry.FormatID(name))
	if err != nil {
		return name, nil, fmt.Errorf("no reader for format %q: %w", name, err)
	}
	return name, reader, nil
}

func (a *App) streamBlocks(ctx context.Context, path string, fn func(index int, b *model.Block) error) (string, error) {
	// A `container!entry` locator reads just that one entry, not the whole archive.
	if loc, ok := parseEntryLocator(path); ok {
		return a.streamEntryBlocks(ctx, loc, fn)
	}
	src, err := openDocSource(ctx, path)
	if err != nil {
		return "", err
	}
	defer src.Close()

	sniff, err := src.seeker()
	if err != nil {
		return "", err
	}
	fmtName, reader, err := a.openReader(path, sniff)
	if err != nil {
		return fmtName, err
	}
	defer reader.Close()

	doc := &model.RawDocument{
		URI:          DisplayName(path),
		SourceLocale: model.LocaleID(a.SourceLocale()),
		Encoding:     a.InputEncoding(),
	}
	if err := src.rawDocument(doc); err != nil {
		return fmtName, err
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmtName, fmt.Errorf("open %s: %w", DisplayName(path), err)
	}

	index := 0
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return fmtName, res.Error
		}
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b != nil {
			if err := fn(index, b); err != nil {
				return fmtName, err
			}
			index++
		}
	}
	return fmtName, nil
}

// expandInputs turns a Unix-filter utility's file arguments into a concrete
// file list: the toolbox spelling of [App.ResolveInputs]. No args means "read
// standard input" ([StdinName]) — but only when stdin is redirected, so `kcat`
// on a bare terminal reports what it wants instead of blocking silently. With
// recursive, directory arguments are walked; without it a directory argument is
// reported as skipped, mirroring `grep` / `cat` on a directory. Glob patterns
// expand in-process regardless, so a quoted `'src/**'` works the same in every
// shell.
//
// Junk files (editor lock/metadata stubs such as Office's "~$…" owner files and
// macOS "._…" AppleDouble files) are silently dropped however they arrive —
// explicitly named, glob-expanded by the shell, or found during a recursive
// walk. They are never valid content, so processing them only ever yields a
// parse error; skipping keeps `kcat ~/Downloads/*` from tripping over a stray
// "~$report.docx" that Word left behind.
func expandInputs(args []string, recursive bool, onSkip func(path string, err error)) ([]string, error) {
	opts := InputOptions{
		Fallback:                FallbackStdinOnly,
		RequireRecursiveForDirs: true,
		Recursive:               recursive,
		OnSkip:                  onSkip,
	}
	if len(args) == 0 {
		if stdinIsPipe() {
			return []string{StdinName}, nil
		}
		return nil, WithExitCode(ExitUsage, fmt.Errorf(
			"no input. Pass files or a glob, pipe content in, or pass `-` to read standard input: %w", ErrNoInput))
	}
	return expandArgs(args, opts)
}

// UseColor resolves the --color mode (auto/always/never) against the terminal
// and the NO_COLOR convention.
func UseColor(mode string) bool {
	switch mode {
	case "always", "yes", "force":
		return true
	case "never", "no", "none":
		return false
	default: // auto
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		return isatty.IsTerminal(os.Stdout.Fd())
	}
}
