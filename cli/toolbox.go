package cli

// Toolbox: format-aware reimaginings of the classic Unix text utilities —
// cat, grep, sed, plus a convert (conv) verb — that operate on the
// *translatable text* of any format kapi understands (Word .docx, JSON
// catalogs, XLIFF, Markdown, …) rather than raw bytes. They share kapi's
// reader/writer pipeline, so `kgrep` greps the prose inside a .docx, `ksed`
// rewrites it and saves the document back faithfully, and `kconv` re-expresses
// it in another format.
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
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/spf13/cobra"
)

// stdinName is the conventional path token for standard input.
const stdinName = "-"

// fallbackFormat is used when neither extension nor content sniffing resolves a
// format — keeps stdin and unknown files working as plain text, like the
// classic tools.
const fallbackFormat = "plaintext"

// mapToolboxErr maps a toolbox utility's RunE result to the grep-style exit
// code contract: nil on match (0), ErrSilentExit on no-match (1, message
// suppressed), context.Canceled on interrupt (130), and any other operational
// trouble (bad pattern, unreadable file, …) to ExitUsage (2) — matching
// grep/sed/cat and the utilities' own --help. The underlying message is
// preserved.
func mapToolboxErr(err error) error {
	if err == nil || errors.Is(err, ErrSilentExit) || errors.Is(err, context.Canceled) {
		return err
	}
	return WithExitCode(ExitUsage, err)
}

// BusyboxRoot returns a standalone root command when prog names a multi-call
// toolbox utility (kgrep / ksed / kcat / kconv, with an optional .exe suffix), or nil
// when it does not — signalling the caller to run the normal kapi root. The
// returned command owns the app lifecycle (config load, Init, Shutdown) so the
// utility behaves identically whether launched as `kgrep` or `kapi grep`.
func BusyboxRoot(app *App, prog string) *cobra.Command {
	prog = strings.TrimSuffix(strings.ToLower(filepath.Base(prog)), ".exe")
	var cmd *cobra.Command
	switch prog {
	case "kgrep":
		cmd = app.newGrepCmd()
	case "ksed":
		cmd = app.newSedCmd()
		// Faithful `ksed -i.bak` (attached backup suffix) needs arg rewriting.
		cmd.SetArgs(NormalizeSedInPlaceArgs(os.Args[1:]))
	case "kcat":
		cmd = app.newCatCmd()
	case "kconv":
		cmd = app.newConvCmd()
	default:
		return nil
	}
	// Rebrand the usage line from "grep …" to "kgrep …" (keep the arg spec).
	if i := strings.IndexByte(cmd.Use, ' '); i > 0 {
		cmd.Use = prog + cmd.Use[i:]
	} else {
		cmd.Use = prog
	}
	cmd.GroupID = ""
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return err
		}
		ApplyAppInitializers(app)
		return nil
	}
	cmd.PersistentPostRun = func(*cobra.Command, []string) { app.Shutdown() }
	if inner := cmd.RunE; inner != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error { return mapToolboxErr(inner(c, args)) }
	}
	return cmd
}

// NewToolboxProxies returns the hidden `kapi grep|sed|cat` subcommands. Each is
// a thin proxy with DisableFlagParsing set, so kapi's persistent flags are NOT
// merged into it — the toolbox utilities keep their full classic option surface
// (including -v / -c, which kapi's globals would otherwise shadow). The proxy
// delegates the raw argument list to the very same standalone command the
// kgrep / ksed / kcat binaries run, so `kapi grep` and `kgrep` behave
// identically. They are Hidden so `kapi --help` steers users to the dedicated
// kgrep / ksed / kcat commands.
func (a *App) NewToolboxProxies() []*cobra.Command {
	proxy := func(verb, short string, build func() *cobra.Command, normalize func([]string) []string) *cobra.Command {
		return &cobra.Command{
			Use:                verb,
			Short:              short,
			GroupID:            "content",
			Hidden:             true,
			DisableFlagParsing: true, // do not inherit/parse kapi's persistent flags
			RunE: func(cmd *cobra.Command, args []string) error {
				if normalize != nil {
					args = normalize(args)
				}
				std := build()
				std.Use = "kapi " + std.Use
				std.SilenceUsage = true
				std.SilenceErrors = true
				std.SetArgs(args)
				return mapToolboxErr(std.ExecuteContext(cmd.Context()))
			},
		}
	}
	return []*cobra.Command{
		proxy("grep", "Search the translatable text of files (use kgrep)", a.newGrepCmd, nil),
		proxy("sed", "Stream-edit the translatable text of files (use ksed)", a.newSedCmd, NormalizeSedInPlaceArgs),
		proxy("cat", "Print the translatable text of files (use kcat)", a.newCatCmd, nil),
		proxy("convert", "Convert files between formats (use kconv)", a.newConvCmd, nil),
	}
}

// displayName is the file label used in output and error messages; stdin shows
// as the conventional "(standard input)".
func displayName(path string) string {
	if path == "" || path == stdinName {
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
	if path != "" && path != stdinName {
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

// resolveFormatName picks the format for a path + content. An explicit --format
// wins; otherwise it runs the framework's canonical detection cascade
// (extension → container-aware content sniffing) and falls back to plain text.
//
// For stdin there is no filename, so detection is purely content-based — it
// routes through the same Detector.Detect path as files, which means piped
// documents (a .docx, a JSON catalog) are recognised via content sniffing, and
// only genuinely unidentifiable input falls back to plain text. This is the one
// place the toolbox decides a format, so both files and stdin share it.
func (a *App) resolveFormatName(path string, content []byte) string {
	if a.FormatFlag != "" {
		return preset.ParseFormatRef(a.FormatFlag).RegistryName()
	}
	// stdin carries no usable path; let Detect skip the extension stage.
	detectPath := path
	if detectPath == stdinName {
		detectPath = ""
	}
	if name, err := a.FormatReg.Detector().Detect(detectPath, bytes.NewReader(content), ""); err == nil && name != "" {
		return name
	}
	return fallbackFormat
}

// streamBlocks opens path (or stdin), detects its format, and calls fn for each
// Block part in document order. Read-only — the backbone of cat and grep.
func (a *App) streamBlocks(ctx context.Context, path string, fn func(index int, b *model.Block) error) (string, error) {
	content, err := readContent(ctx, path)
	if err != nil {
		return "", err
	}
	fmtName := a.resolveFormatName(path, content)
	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmtName, fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}
	defer reader.Close()

	doc := &model.RawDocument{
		URI:          displayName(path),
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmtName, fmt.Errorf("open %s: %w", displayName(path), err)
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

// editDocument reads path, applies the tool to every part, then writes the
// reconstructed document — in place (with optional backup) or to out. The
// skeleton store is wired between reader and writer so faithful formats (e.g.
// .docx) round-trip structure while only the edited text changes. writeLocale
// selects which locale the writer emits ("" = source / monolingual round-trip).
func (a *App) editDocument(ctx context.Context, path string, t *tool.BaseTool, writeLocale model.LocaleID, inPlace bool, backupSuffix string, out io.Writer) error {
	if inPlace && (path == "" || path == stdinName) {
		return errors.New("in-place editing requires a file argument")
	}
	content, err := readContent(ctx, path)
	if err != nil {
		return err
	}
	fmtName := a.resolveFormatName(path, content)

	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}
	writer, err := a.FormatReg.NewWriter(registry.FormatID(fmtName))
	if err != nil {
		return fmt.Errorf("%q is a read-only format (no writer) — read it with kcat instead", fmtName)
	}

	// Wire skeleton store when both sides support it (faithful round-trip).
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
		URI:          displayName(path),
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return fmt.Errorf("open %s: %w", displayName(path), err)
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
		p, aerr := t.Apply(res.Part)
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
		return fmt.Errorf("write %s: %w", displayName(path), err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close %s: %w", displayName(path), err)
	}
	return nil
}

// expandInputs turns command-line file arguments into a concrete file list. No
// args means "read standard input" ([stdinName]). With recursive, directory
// arguments are walked (skipping hidden dirs and junk); without it, a directory
// argument is reported as skipped, mirroring `grep` / `cat` on a directory.
func expandInputs(args []string, recursive bool, onSkip func(path string, err error)) ([]string, error) {
	if len(args) == 0 {
		return []string{stdinName}, nil
	}
	var files []string
	for _, arg := range args {
		if arg == stdinName {
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
		files = append(files, arg)
	}
	return files, nil
}

// useColor resolves the --color mode (auto/always/never) against the terminal
// and the NO_COLOR convention.
func useColor(mode string) bool {
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
