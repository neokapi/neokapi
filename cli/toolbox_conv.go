package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/spf13/cobra"
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

// newConvCmd builds the convert command (standalone `kconv` root and the hidden
// `kapi convert` proxy).
func (a *App) newConvCmd() *cobra.Command {
	var (
		to        string
		outPath   string
		targetLoc string
	)

	cmd := &cobra.Command{
		Use:     "convert [flags] [FILE...]",
		Short:   "Convert files between formats (Markdown, HTML, DocLang, …)",
		GroupID: "content",
		Long: `Convert the content of each file into another format, driven by the structural
role layer rather than the source bytes. Headings, lists, tables and inline
formatting are carried across, so a Word .docx, a DocLang document and a Docling
JSON all project to clean Markdown or HTML — and source or translated content
re-expresses as DocLang.

The target format is taken from --to, or inferred from the -o output extension.
With no -o, the result is written to standard output. With no FILE, or "-",
standard input is read.

A same-format conversion (e.g. .docx → .docx) round-trips faithfully via the
document skeleton; a cross-format conversion reconstructs from the content model.`,
		Example: `  kconv report.docx --to md
  kconv report.dclg.xml -o report.html
  kconv scan.docling.json --to html
  kconv messages.xliff --to md --target fr
  docling convert in.pdf --to json | kconv -f docling --to md`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			toFmt, err := a.resolveTargetFormat(to, outPath)
			if err != nil {
				return err
			}
			return a.runConv(cmd.Context(), args, toFmt, model.LocaleID(targetLoc), outPath)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&to, "to", "t", "", "target format (e.g. markdown, html, doclang, or an extension like md)")
	f.StringVarP(&outPath, "output", "o", "", "output file (format inferred from its extension; default: stdout)")
	f.StringVar(&targetLoc, "target", "", "convert the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input/output encoding")
	return cmd
}

// runConv converts each input file (or stdin) into the target format.
func (a *App) runConv(ctx context.Context, args []string, toFmt registry.FormatID, targetLoc model.LocaleID, outPath string) error {
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
		if err := a.convertDocument(ctx, file, toFmt, targetLoc, outPath); err != nil {
			return fmt.Errorf("%s: %w", displayName(file), err)
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
	content, err := readContent(ctx, path)
	if err != nil {
		return err
	}
	inFmt := a.resolveFormatName(path, content)

	reader, err := a.FormatReg.NewReader(registry.FormatID(inFmt))
	if err != nil {
		return fmt.Errorf("no reader for format %q: %w", inFmt, err)
	}
	defer reader.Close()
	writer, err := a.FormatReg.NewWriter(toFmt)
	if err != nil {
		return fmt.Errorf("no writer for format %q: %w", toFmt, err)
	}

	sameFormat := reader.Name() == writer.Name()
	if sameFormat {
		if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
			if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
				if store, serr := format.NewSkeletonStore(); serr == nil {
					defer store.Close()
					emitter.SetSkeletonStore(store)
					consumer.SetSkeletonStore(store)
				}
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
		return fmt.Errorf("open %s: %w", displayName(path), err)
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
		if sameFormat {
			if sps, ok := writer.(format.SourcePathSetter); ok && filepath.IsAbs(path) {
				sps.SetSourcePath(path)
			} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
				ocs.SetOriginalContent(content)
			}
		}
	} else {
		if err := writer.SetOutputWriter(os.Stdout); err != nil {
			return err
		}
		if sameFormat {
			if ocs, ok := writer.(format.OriginalContentSetter); ok {
				ocs.SetOriginalContent(content)
			}
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
		return fmt.Errorf("write %s: %w", displayName(path), err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close %s: %w", displayName(path), err)
	}
	return nil
}

// resolveTargetFormat resolves the conversion target: an explicit --to (a
// registered writer's format id, or an extension like "md"/".md"), else the
// format inferred from the -o output extension. Returns an error when neither
// yields a writable format.
func (a *App) resolveTargetFormat(to, outPath string) (registry.FormatID, error) {
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
	return a.writerByExt(filepath.Ext(strings.ToLower(path)))
}

// writerByExt returns the highest-priority writable format registered for ext,
// or "" when none.
func (a *App) writerByExt(ext string) registry.FormatID {
	if ext == "" {
		return ""
	}
	if det, err := a.FormatReg.DetectByExtension(ext); err == nil && det != "" && a.FormatReg.HasWriter(det) {
		return det
	}
	return ""
}
