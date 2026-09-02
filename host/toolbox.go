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
	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
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
	if name, err := a.FormatReg.Detector().Detect(detectPath, content, ""); err == nil && name != "" {
		return name, true
	}
	return "", false
}

// StreamBlocks opens path (or stdin), detects its format, and calls fn for each
// Block part in document order. Read-only — the backbone of cat and grep.
func (a *App) StreamBlocks(ctx context.Context, path string, fn func(index int, b *model.Block) error) (string, error) {
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
	fmtName, err := a.resolveFormatFrom(path, sniff)
	if err != nil {
		return "", err
	}
	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmtName, fmt.Errorf("no reader for format %q: %w", fmtName, err)
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

// EditDocument reads path, applies the tool to every part, then writes the
// reconstructed document — in place (with optional backup) or to out. The
// skeleton store is wired between reader and writer so structure-preserving
// formats (e.g. .docx) round-trip byte-for-byte while only the edited text
// changes. writeLocale
// selects which locale the writer emits ("" = source / monolingual round-trip).
func (a *App) EditDocument(ctx context.Context, path string, t *tool.BaseTool, writeLocale model.LocaleID, inPlace bool, backupSuffix string, out io.Writer) error {
	// A `container!entry` locator edits one inner file; a bare container path edits
	// every eligible entry. Both repack through the container binding (AD-026 §6) —
	// the archive format has no writer of its own.
	if loc, ok := parseEntryLocator(path); ok {
		return a.editArchiveEntry(ctx, loc, t, writeLocale, inPlace, backupSuffix, out)
	}
	if container.IsContainerPath(path) {
		return a.editArchiveAll(ctx, path, t, writeLocale, inPlace, backupSuffix, out)
	}
	if inPlace && (path == "" || path == StdinName) {
		return errors.New("in-place editing requires a file argument")
	}
	src, err := openDocSource(ctx, path)
	if err != nil {
		return err
	}
	defer src.Close()

	sniff, err := src.seeker()
	if err != nil {
		return err
	}
	fmtName, err := a.resolveFormatFrom(path, sniff)
	if err != nil {
		return err
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}
	writer, err := a.FormatReg.NewWriter(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("%q is not editable (no writer). Read it with kcat; see editable formats with `kapi formats`", fmtName)
	}

	// Wire skeleton store when both sides support it (byte-for-byte round-trip).
	// A store that cannot be created fails the edit: this path rewrites the file
	// IN PLACE, so degrading silently would replace a faithfully-preserved
	// document with a reconstruction of itself — the original bytes are gone and
	// the command still reports success.
	store, skelErr := format.NewWiredSkeleton(reader, writer)
	if skelErr != nil {
		reader.Close()
		return fmt.Errorf("cannot edit %s: %w", DisplayName(path), skelErr)
	}
	if store != nil {
		defer store.Close()
	}

	doc := &model.RawDocument{
		URI:          DisplayName(path),
		SourceLocale: model.LocaleID(a.SourceLocale()),
		Encoding:     a.InputEncoding(),
	}
	if err := src.rawDocument(doc); err != nil {
		reader.Close()
		return err
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return fmt.Errorf("open %s: %w", DisplayName(path), err)
	}

	var outParts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			reader.Close()
			return res.Error
		}
		if res.Part == nil {
			continue
		}
		// ApplyContext (not Apply) so a network-backed tool — e.g. the AI
		// rewrite tool — honours cancellation/deadlines on its provider calls
		// while the document is streamed. Pure-text tools (ksed) ignore ctx.
		p, aerr := t.ApplyContext(ctx, res.Part)
		if aerr != nil {
			reader.Close()
			return aerr
		}
		if p != nil {
			outParts = append(outParts, p)
		}
	}
	reader.Close()

	if inPlace {
		// In place, the original must be held in memory: SetOutput truncates
		// the file the writer would otherwise re-read for its skeleton, so a
		// source-path binding here would hand the writer an empty document.
		content, berr := src.bytes()
		if berr != nil {
			return berr
		}
		if backupSuffix != "" {
			if err := os.WriteFile(path+backupSuffix, content, 0o644); err != nil {
				return fmt.Errorf("write backup: %w", err)
			}
		}
		if err := writer.SetOutput(path); err != nil {
			return err
		}
		if ocs, ok := writer.(format.OriginalContentSetter); ok {
			ocs.SetOriginalContent(content)
		}
	} else {
		if err := writer.SetOutputWriter(out); err != nil {
			return err
		}
		// Writing elsewhere leaves the input intact, so a package writer can
		// re-read it from disk rather than take a second copy of it.
		if err := bindWriterSource(writer, path, "", src); err != nil {
			return err
		}
	}
	writer.SetEncoding(a.InputEncoding())
	writer.SetLocale(writeLocale)

	ch := make(chan *model.Part, len(outParts)+1)
	for _, p := range outParts {
		ch <- p
	}
	close(ch)
	if err := writer.Write(ctx, ch); err != nil {
		return fmt.Errorf("write %s: %w", DisplayName(path), err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close %s: %w", DisplayName(path), err)
	}
	return nil
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
