package host

import (
	"context"
	"errors"
	"fmt"
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
//
// Like the rest of the toolbox it is exposed as a `kconv` busybox symlink and as
// the hidden `kapi convert` subcommand. A same-format conversion (e.g. .docx →
// .docx) still round-trips faithfully via the skeleton; a cross-format one
// projects through the model.

// RunConv converts each input file (or stdin) into the target format.
func (a *App) RunConv(ctx context.Context, args []string, toFmt registry.FormatID, targetLoc model.LocaleID, outPath string) error {
	hadError := false
	files, err := expandInputs(args, false, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "kconv: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}
	if outPath != "" && len(files) > 1 {
		return errors.New("-o accepts a single input file; convert files one at a time (or omit -o to write to stdout)")
	}

	for _, file := range files {
		start := time.Now()
		cerr := a.convertDocument(ctx, file, toFmt, targetLoc, outPath)
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
	src, err := openDocSource(ctx, path)
	if err != nil {
		return err
	}
	defer src.Close()

	sniff, err := src.seeker()
	if err != nil {
		return err
	}
	inFmt := a.resolveFormatFrom(path, sniff)

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
				return fmt.Errorf("cannot convert to %q: it is a bilingual translation-interchange format — use `kapi extract --format %s` (it captures the source skeleton so `kapi merge` can round-trip translations back into the original), not `convert`", toFmt, toFmt)
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
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
	}
	if err := src.rawDocument(doc); err != nil {
		return err
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open %s: %w", DisplayName(path), err)
	}

	var parts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return res.Error
		}
		if res.Part != nil {
			parts = append(parts, res.Part)
		}
	}

	if outPath != "" {
		if err := writer.SetOutput(outPath); err != nil {
			return err
		}
	} else if err := writer.SetOutputWriter(os.Stdout); err != nil {
		return err
	}
	// Only a same-format round-trip consumes the original: a cross-format
	// target reconstructs from the content model, and handing it foreign bytes
	// would materialize the input for nothing.
	if sameFormat {
		if err := bindWriterSource(writer, path, outPath, src); err != nil {
			return err
		}
	}
	writer.SetEncoding(a.Encoding)
	writer.SetLocale(targetLoc)

	ch := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
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
		return "", fmt.Errorf("unknown target format %q — try a format id (markdown, html, doclang) or an extension (md, html)", to)
	}
	if outPath != "" {
		if det := a.writerForOutputPath(outPath); det != "" {
			return det, nil
		}
		return "", fmt.Errorf("cannot infer a target format from %q — pass --to", filepath.Base(outPath))
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
