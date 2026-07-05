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
	"path/filepath"

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

// ResolveFormatName picks the format for a path + content. An explicit --format
// wins; otherwise it runs the framework's canonical detection cascade
// (extension → container-aware content sniffing) and falls back to plain text.
//
// For stdin there is no filename, so detection is purely content-based — it
// routes through the same Detector.Detect path as files, which means piped
// documents (a .docx, a JSON catalog) are recognised via content sniffing, and
// only genuinely unidentifiable input falls back to plain text. This is the one
// place the toolbox decides a format, so both files and stdin share it.
func (a *App) ResolveFormatName(path string, content []byte) string {
	if a.FormatFlag != "" {
		return preset.ParseFormatRef(a.FormatFlag).RegistryName()
	}
	// stdin carries no usable path; let Detect skip the extension stage.
	detectPath := path
	if detectPath == StdinName {
		detectPath = ""
	}
	if name, err := a.FormatReg.Detector().Detect(detectPath, bytes.NewReader(content), ""); err == nil && name != "" {
		return name
	}
	return FallbackFormat
}

// StreamBlocks opens path (or stdin), detects its format, and calls fn for each
// Block part in document order. Read-only — the backbone of cat and grep.
func (a *App) StreamBlocks(ctx context.Context, path string, fn func(index int, b *model.Block) error) (string, error) {
	// A `container!entry` locator reads just that one entry, not the whole archive.
	if loc, ok := ParseEntryLocator(path); ok {
		return a.streamEntryBlocks(ctx, loc, fn)
	}
	content, err := readContent(ctx, path)
	if err != nil {
		return "", err
	}
	fmtName := a.ResolveFormatName(path, content)
	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmtName, fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}
	defer reader.Close()

	doc := &model.RawDocument{
		URI:          DisplayName(path),
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
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
	if loc, ok := ParseEntryLocator(path); ok {
		return a.editArchiveEntry(ctx, loc, t, writeLocale, inPlace, backupSuffix, out)
	}
	if container.IsContainerPath(path) {
		return a.editArchiveAll(ctx, path, t, writeLocale, inPlace, backupSuffix, out)
	}
	if inPlace && (path == "" || path == StdinName) {
		return errors.New("in-place editing requires a file argument")
	}
	content, err := readContent(ctx, path)
	if err != nil {
		return err
	}
	fmtName := a.ResolveFormatName(path, content)

	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}
	writer, err := a.FormatReg.NewWriter(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("%q is not editable (no writer) — read it with kcat; see editable formats with `kapi formats list`", fmtName)
	}

	// Wire skeleton store when both sides support it (byte-for-byte round-trip).
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			if store, serr := format.NewSkeletonStore(); serr == nil {
				defer store.Close()
				emitter.SetSkeletonStore(store)
				consumer.SetSkeletonStore(store)
			}
		}
	}

	doc := &model.RawDocument{
		URI:          DisplayName(path),
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
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
		if backupSuffix != "" {
			if err := os.WriteFile(path+backupSuffix, content, 0o644); err != nil {
				return fmt.Errorf("write backup: %w", err)
			}
		}
		if err := writer.SetOutput(path); err != nil {
			return err
		}
		if sps, ok := writer.(format.SourcePathSetter); ok && filepath.IsAbs(path) {
			sps.SetSourcePath(path)
		} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
			ocs.SetOriginalContent(content)
		}
	} else {
		if err := writer.SetOutputWriter(out); err != nil {
			return err
		}
		if ocs, ok := writer.(format.OriginalContentSetter); ok {
			ocs.SetOriginalContent(content)
		}
	}
	writer.SetEncoding(a.Encoding)
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

// expandInputs turns command-line file arguments into a concrete file list. No
// args means "read standard input" ([StdinName]). With recursive, directory
// arguments are walked (skipping hidden dirs and junk); without it, a directory
// argument is reported as skipped, mirroring `grep` / `cat` on a directory.
//
// Junk files (editor lock/metadata stubs such as Office's "~$…" owner files and
// macOS "._…" AppleDouble files) are silently dropped however they arrive —
// explicitly named, glob-expanded by the shell, or found during a recursive
// walk. They are never valid content, so processing them only ever yields a
// parse error; skipping keeps `kcat ~/Downloads/*` from tripping over a stray
// "~$report.docx" that Word left behind.
func expandInputs(args []string, recursive bool, onSkip func(path string, err error)) ([]string, error) {
	if len(args) == 0 {
		return []string{StdinName}, nil
	}
	var files []string
	for _, arg := range args {
		if arg == StdinName {
			files = append(files, arg)
			continue
		}
		// A `container!entry` locator (AD-026 §6) names one file inside an archive;
		// keep it verbatim — os.Stat on the whole string would fail. The archive
		// part's existence was already verified by ParseEntryLocator.
		if HasEntryLocator(arg) {
			files = append(files, arg)
			continue
		}
		info, err := os.Stat(arg)
		if err != nil {
			if onSkip != nil {
				onSkip(arg, err)
			}
			continue
		}
		if info.IsDir() {
			if !recursive {
				if onSkip != nil {
					onSkip(arg, errors.New("is a directory"))
				}
				continue
			}
			walked, werr := walkDirFiles(arg)
			if werr != nil {
				return nil, werr
			}
			files = append(files, walked...)
			continue
		}
		// Drop editor lock/metadata stubs silently — they are never content, so
		// skipping is not an error (exit status stays 0). walkDirFiles applies
		// the same filter to recursively discovered files.
		if isJunkFile(filepath.Base(arg)) {
			continue
		}
		files = append(files, arg)
	}
	return files, nil
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
