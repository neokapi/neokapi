package cli

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// newConvCmd builds the convert command (standalone `kconv` root and the hidden
// `kapi convert` proxy).
func newConvCmd(a *App) *cobra.Command {
	var (
		to        string
		outPath   string
		targetLoc string
	)

	cmd := &cobra.Command{
		Use:     "convert [flags] [FILE...]",
		Short:   "Convert files between formats (Markdown, HTML, DocLang, …)",
		GroupID: "advanced",
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
  kconv scan.docling.JSON --to html
  kconv messages.xliff --to md --target fr
  docling convert in.pdf --to json | kconv -f docling --to md`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			toFmt, err := a.ResolveTargetFormat(to, outPath)
			if err != nil {
				return err
			}
			return a.RunConv(cmd.Context(), args, toFmt, model.LocaleID(targetLoc), outPath)
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
