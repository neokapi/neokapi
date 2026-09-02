package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/structdoc"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
)

// kconv — the format-aware `cat`-family converter: read any format kapi
// understands and re-express it in another. It is the clean projection of the
// content model + the structural-role layer: headings, lists, tables and inline
// formatting are carried by role, so a Word .docx, a DocLang document or a
// Docling JSON all project to clean Markdown / HTML (and back to DocLang),
// driven by the role layer rather than the source bytes.
//
// `kconv report.docx --to md`        → Markdown on stdout
// `kconv report.dclg.xml -o out.html` → HTML file
// `kconv fr.xliff --to md --target fr` → the French translation as Markdown
// `kconv ~/Downloads/* --to md -o out/` → one .md per input, in out/
// `kconv -r docs --to html -o site/`  → the docs tree, mirrored as HTML
//
// Like the rest of the toolbox it is exposed as a `kconv` busybox symlink and as
// the hidden `kapi convert` subcommand. A same-format conversion (e.g. .docx →
// .docx) still round-trips faithfully via the skeleton; a cross-format one
// projects through the model.

// ConvOptions carries one `kconv` invocation's flags.
type ConvOptions struct {
	// To is the resolved target format.
	To registry.FormatID
	// TargetLocale writes that translation instead of the source ("" = source).
	TargetLocale model.LocaleID
	// Output is -o: a single file, an output directory (a trailing separator, or
	// a path that already is one), or "" for standard output.
	Output string
	// Recursive walks directory arguments, like `kgrep -r`.
	Recursive bool
}

// RunConv converts each input file (or stdin) into the target format.
func (a *App) RunConv(ctx context.Context, args []string, opts ConvOptions) error {
	hadError := false
	files, err := expandInputs(args, opts.Recursive, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "kconv: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}
	outDir, err := prepareOutputDir(opts.Output, len(files))
	if err != nil {
		return err
	}
	// The tree the outputs mirror. Computed from the resolved inputs rather than
	// the arguments, so it is the same whether the shell expanded the glob or
	// kapi did, and whether the user named a directory or a pattern.
	root := ""
	if outDir != "" {
		root = commonDir(files)
	}
	claimed := map[string]string{}

	for _, file := range files {
		outPath := opts.Output
		if outDir != "" {
			dest, derr := a.convOutputPath(outDir, root, file, opts.To)
			if derr == nil {
				derr = claimOutput(claimed, dest, file)
			}
			if derr == nil {
				derr = os.MkdirAll(filepath.Dir(dest), 0o755)
			}
			if derr != nil {
				hadError = true
				fmt.Fprintf(os.Stderr, "kconv: %s: %v\n", DisplayName(file), derr)
				continue
			}
			outPath = dest
		}
		start := time.Now()
		cerr := a.convertDocument(ctx, file, opts.To, opts.TargetLocale, outPath)
		if cerr == nil && a.ConvTiming {
			// Self-reported so the number excludes process start-up — the
			// comparable figure for a conversion library. Milliseconds with the
			// literal "ms" so it is trivially machine-parseable.
			fmt.Fprintf(os.Stderr, "kconv: converted %s in %.2fms\n",
				DisplayName(file), float64(time.Since(start).Nanoseconds())/1e6)
		}
		if cerr != nil {
			// Ctrl-C is a global interrupt: stop and let cli.Run map it to exit 130.
			if errors.Is(cerr, context.Canceled) {
				return cerr
			}
			if errors.Is(cerr, ErrBinaryStdout) {
				cerr = errBinaryStdout("pass -o FILE, redirect stdout to a file, or pass `-o -` to write it here anyway")
			}
			// Report the bad file and continue, so one failure doesn't abort the
			// rest — matching kgrep/ksed/kcat.
			hadError = true
			fmt.Fprintf(os.Stderr, "kconv: %s: %v\n", DisplayName(file), cerr)
			continue
		}
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}

// convertDocument reads path (or stdin), detects its input format, and writes it
// as toFmt — to outPath when set, else stdout. With targetLoc empty the source
// is projected (a monolingual conversion); with a locale the writer emits that
// translation. The skeleton store and source bytes are wired to the writer ONLY
// for a same-format conversion; for a cross-format one they would be foreign to
// the writer, so it reconstructs from the content model + structural layer.
func (a *App) convertDocument(ctx context.Context, path string, toFmt registry.FormatID, targetLoc model.LocaleID, outPath string) error {
	// `-o -` is standard output named explicitly, not a file called "-".
	stdout := outPath == StdoutName
	if stdout {
		outPath = ""
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
	inFmt, err := a.resolveFormatFrom(path, sniff)
	if err != nil {
		return err
	}

	reader, err := a.FormatReg.NewReader(registry.FormatID(inFmt))
	if err != nil {
		return fmt.Errorf("no reader for format %q: %w", inFmt, err)
	}
	defer reader.Close()
	writer, err := a.conversionWriter(toFmt)
	if err != nil {
		return fmt.Errorf("no writer for format %q: %w", toFmt, err)
	}

	sameFormat := reader.Name() == writer.Name()
	// A cross-format conversion reconstructs the target from the content model.
	// Only a generative writer can do that; a skeleton-bound format (docx, odt,
	// idml, epub, …) writes back into its own original file and cannot be a
	// target. Check the declared capability (no plugin load) and fail cleanly.
	if !sameFormat {
		if info := a.FormatReg.FormatInfo(toFmt); info != nil && info.HasWriter {
			if info.Interchange {
				return fmt.Errorf("cannot convert to %q: it is a bilingual translation-interchange format. Use `kapi extract --format %s` (it captures the source skeleton so `kapi merge` can round-trip translations back into the original) rather than `convert`", toFmt, toFmt)
			}
			if !info.Generative {
				return fmt.Errorf("cannot convert to %q: it is a packaged format that can only be written by updating an existing %s file, not generated from %s", toFmt, toFmt, inFmt)
			}
		}
	}
	// Same-format conversion is a faithful round-trip through the typed skeleton.
	// NewWiredSkeleton stamps the store with the source format's origin and
	// connects the writer only when it is that same format — so a cross-format
	// target can never consume this foreign skeleton (it reconstructs from the
	// model). A store that cannot be created fails the conversion rather than
	// quietly turning a faithful round-trip into a re-serialization.
	if sameFormat {
		store, skelErr := format.NewWiredSkeleton(reader, writer)
		if skelErr != nil {
			return fmt.Errorf("cannot convert %s: %w", DisplayName(path), skelErr)
		}
		if store != nil {
			defer store.Close()
		}
	}

	doc := &model.RawDocument{
		URI:          DisplayName(path),
		SourceLocale: model.LocaleID(a.SourceLocale()),
		Encoding:     a.InputEncoding(),
	}
	if err := src.rawDocument(doc); err != nil {
		return err
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open %s: %w", DisplayName(path), err)
	}

	// The writer is configured before the read, not after, so the Part stream
	// can go straight from one to the other. Collecting it first — which is all
	// this used to do, only to replay the slice into the channel below — held
	// the whole document for no reason, and did it a second time inside a writer
	// that was already collecting. See #1710: neither buffer is visible while
	// the other one exists.
	flushOut := func() error { return nil }
	if outPath != "" {
		if err := writer.SetOutput(outPath); err != nil {
			return err
		}
	} else {
		// Binary output at a terminal is refused here (see binaryout.go); `-o -`
		// is the explicit "yes, stdout anyway", as it is for curl.
		out := io.Writer(os.Stdout)
		if !stdout {
			out, flushOut = a.guardedStdout()
		}
		if err := writer.SetOutputWriter(out); err != nil {
			return err
		}
	}
	// Only a same-format round-trip consumes the original: a cross-format
	// target reconstructs from the content model, and handing it foreign bytes
	// would materialize the input for nothing.
	if sameFormat {
		if err := bindWriterSource(writer, path, outPath, src); err != nil {
			return err
		}
	}
	writer.SetEncoding(a.InputEncoding())
	writer.SetLocale(targetLoc)

	// readCtx lets a writer that stops early stop the reader with it, rather
	// than leaving it blocked on a send nobody will receive.
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()

	ch := make(chan *model.Part, 64)
	readErrCh := make(chan error, 1)
	go func() {
		defer close(ch)
		var rerr error
		for res := range reader.Read(readCtx) {
			if res.Error != nil {
				rerr = res.Error
				cancelRead()
				continue // keep draining so the reader can close its channel
			}
			if res.Part == nil {
				continue
			}
			select {
			case ch <- res.Part:
			case <-readCtx.Done():
			}
		}
		readErrCh <- rerr
	}()

	werr := writer.Write(ctx, ch)
	cancelRead()
	for range ch { //nolint:revive // drain, so the forwarder finishes if the writer stopped early
	}
	// The read error wins: a writer handed a truncated stream usually fails
	// with something less informative than whatever stopped the reader.
	if readErr := <-readErrCh; readErr != nil {
		return readErr
	}
	if werr != nil {
		return fmt.Errorf("write %s: %w", DisplayName(path), werr)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close %s: %w", DisplayName(path), err)
	}
	// After Close, so a document shorter than the sniff length is judged on the
	// whole of itself rather than on however much the writer had flushed.
	return flushOut()
}

// prepareOutputDir decides whether -o names an output directory rather than an
// output file, and creates it when it does. A trailing separator is the
// explicit spelling (`-o out/`), and a path that already is a directory is
// taken as one. Returns "" when -o names a single file (or is absent).
//
// Without a directory, -o stays single-input: one output filename cannot hold
// several documents, and silently converting only the last one is the failure
// mode this rejects.
func prepareOutputDir(out string, inputs int) (string, error) {
	// "" and "-" are both standard output; neither names a directory, and the
	// several-inputs rule below does not apply to a stream.
	if out == "" || out == StdoutName {
		return "", nil
	}
	isDir := strings.HasSuffix(out, "/") || strings.HasSuffix(out, string(os.PathSeparator))
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		isDir = true
	}
	if !isDir {
		if inputs > 1 {
			return "", fmt.Errorf(
				"-o %s names one output file but %d inputs were given. Pass a directory instead (e.g. `-o %s/`) to write one file per input, or omit -o to write to stdout",
				out, inputs, format.TrimExt(out))
		}
		return "", nil
	}
	dir := filepath.Clean(out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create output directory %s: %w", dir, err)
	}
	return dir, nil
}

// convOutputPath is where one input lands under an output directory: its stem,
// re-extensioned for the target format, under the input's own position in the
// tree relative to root. So `kconv 'docs/**/*.md' --to html -o site/` writes
// site/guide/intro.html for docs/guide/intro.md — the shape survives the
// conversion, and two same-named files in different directories cannot collide.
func (a *App) convOutputPath(dir, root, input string, toFmt registry.FormatID) (string, error) {
	if input == "" || input == StdinName {
		return "", errors.New("standard input has no filename to derive an output name from. Name the file (-o out.md) instead of a directory")
	}
	ext := a.formatExt(toFmt)
	if ext == "" {
		return "", fmt.Errorf("no filename extension is registered for %q. Write a single file with `-o NAME.EXT` instead of a directory", toFmt)
	}
	dest := filepath.Join(dir, subdirUnder(root, input), format.Stem(input)+ext)
	if abs, err := filepath.Abs(input); err == nil && sameFile(abs, dest) {
		return "", fmt.Errorf("the conversion would overwrite the input (%s). Write to a different -o directory", dest)
	}
	return dest, nil
}

// subdirUnder returns input's directory relative to root, or "" when it does
// not lie under root (a mixed absolute/relative argument list, say) — in which
// case the file flattens into the output directory and claimOutput catches any
// resulting collision.
func subdirUnder(root, input string) string {
	if root == "" {
		return ""
	}
	rel, err := filepath.Rel(root, filepath.Dir(filepath.Clean(input)))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ""
	}
	return rel
}

// commonDir is the deepest directory every input sits under — the root of the
// tree the output mirrors. It needs no provenance from the arguments: a list of
// files in one directory shares that directory (so the output is flat), and
// `docs/**/*.md` shares docs/ (so the output keeps the sub-tree). Returns ""
// when the inputs share nothing, e.g. an absolute path beside a relative one.
func commonDir(files []string) string {
	var common []string
	seen := false
	for _, f := range files {
		if f == "" || f == StdinName {
			continue
		}
		dir := filepath.Dir(filepath.Clean(f))
		segs := strings.Split(filepath.ToSlash(dir), "/")
		if !filepath.IsAbs(dir) && dir != "." {
			// Root every relative path at ".", so `docs/a.md src/b.md` shares the
			// working directory and mirrors both trees, rather than sharing
			// nothing and flattening two files into one directory.
			segs = append([]string{"."}, segs...)
		}
		if !seen {
			common, seen = segs, true
			continue
		}
		n := 0
		for n < len(common) && n < len(segs) && common[n] == segs[n] {
			n++
		}
		common = common[:n]
	}
	if len(common) == 0 {
		return ""
	}
	if len(common) == 1 && common[0] == "" {
		return string(filepath.Separator) // every input is at the filesystem root
	}
	return filepath.Clean(filepath.FromSlash(strings.Join(common, "/")))
}

// claimOutput records dest as this run's output for input, and rejects a second
// input that resolves to the same file. Two inputs whose stems collide (a.md
// and a.html converted to Markdown) would otherwise leave one silently
// overwritten by the other, with the run reporting success.
func claimOutput(claimed map[string]string, dest, input string) error {
	key := dest
	if abs, err := filepath.Abs(dest); err == nil {
		key = abs
	}
	if prev, dup := claimed[key]; dup {
		return fmt.Errorf("would overwrite %s, already converted from %s. Convert them separately, or to different directories", dest, prev)
	}
	claimed[key] = input
	return nil
}

// formatExt is the filename extension a converted file gets: the target
// format's first registered extension, which is its preferred spelling
// (".md" over ".markdown", the compound ".dclg.xml" for DocLang).
func (a *App) formatExt(id registry.FormatID) string {
	if info := a.FormatReg.FormatInfo(id); info != nil && len(info.Extensions) > 0 {
		return info.Extensions[0]
	}
	return ""
}

// conversionWriter returns the writer for a `convert` target. JSON and YAML
// document conversions are served by the STRUCTURAL writers — an array of block
// records (the `kapi inspect` shape) — rather than the catalog key→value
// writers. A document (DocLang, Markdown, HTML, docx, …) has no catalog keys, so
// the catalog writers collapse every block onto the empty key; the structural
// writers capture its structure instead. The catalog json/yaml writers remain
// the i18n round-trip format, reached by translate/merge/same-format editing or
// an explicit `--to json-catalog` / `--to yaml-catalog` target. Every other
// target uses its registered writer.
func (a *App) conversionWriter(toFmt registry.FormatID) (format.DataFormatWriter, error) {
	switch toFmt {
	case "json":
		return structdoc.NewJSONWriter(), nil
	case "yaml":
		return structdoc.NewYAMLWriter(), nil
	}
	return a.FormatReg.NewWriter(toFmt)
}

// ResolveTargetFormat resolves the conversion target: an explicit --to (a
// registered writer's format id, or an extension like "md"/".md"), else the
// format inferred from the -o output extension. Returns an error when neither
// yields a writable format.
func (a *App) ResolveTargetFormat(to, outPath string) (registry.FormatID, error) {
	if to != "" {
		id := registry.FormatID(preset.ParseFormatRef(to).RegistryName())
		if a.FormatReg.HasWriter(id) {
			return id, nil
		}
		if det := a.writerByExt("." + strings.TrimPrefix(strings.ToLower(to), ".")); det != "" {
			return det, nil
		}
		return "", fmt.Errorf("unknown target format %q. Try a format id (markdown, html, doclang) or an extension (md, html)", to)
	}
	if outPath != "" && outPath != StdoutName {
		if det := a.writerForOutputPath(outPath); det != "" {
			return det, nil
		}
		// An output directory carries no extension to infer from, and saying
		// "cannot infer a target format from out" reads like a broken filename.
		if info, err := os.Stat(outPath); (err == nil && info.IsDir()) ||
			strings.HasSuffix(outPath, "/") || strings.HasSuffix(outPath, string(os.PathSeparator)) {
			return "", fmt.Errorf("an output directory sets no target format. Pass --to (e.g. `--to md -o %s`)", outPath)
		}
		return "", fmt.Errorf("cannot infer a target format from %q. Pass --to", filepath.Base(outPath))
	}
	return "", errors.New("specify a target format with --to (e.g. --to markdown) or an output file with -o")
}

// writerForOutputPath resolves the writer format for an output filename,
// honouring the compound ".dclg.xml" DocLang extension before the plain ".xml".
func (a *App) writerForOutputPath(path string) registry.FormatID {
	if strings.HasSuffix(strings.ToLower(path), ".dclg.xml") && a.FormatReg.HasWriter("doclang") {
		return "doclang"
	}
	return a.writerByExt(format.Ext(path))
}

// writerByExt returns the highest-priority writable format registered for ext,
// or "" when none.
func (a *App) writerByExt(ext string) registry.FormatID {
	if ext == "" {
		return ""
	}
	if det, err := a.FormatReg.Detect(ext, registry.DetectOptions{ExtensionOnly: true}); err == nil && det != "" && a.FormatReg.HasWriter(det) {
		return det
	}
	return ""
}
