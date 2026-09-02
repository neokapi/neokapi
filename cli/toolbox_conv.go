package cli

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// newConvCmd builds the convert command (standalone `kconv` root and the hidden
// `kapi kconv` proxy).
func newConvCmd(a *App) *cobra.Command {
	var (
		to        string
		outPath   string
		targetLoc string
		recursive bool
	)

	cmd := &cobra.Command{
		Use:     "convert [flags] [FILE...]",
		Short:   "Convert files between formats (Markdown, HTML, DocLang, …)",
		GroupID: "advanced",
		Long: `Convert the content of each file into another format, driven by the structural
role layer rather than the source bytes. Headings, lists, tables and inline
formatting are carried across, so a Word .docx, a DocLang document and a Docling
JSON all project to clean Markdown or HTML, and source or translated content
re-expresses as DocLang.

The target format is taken from --to, or inferred from the -o output extension.
With no -o, the result is written to standard output. With no FILE, or "-",
standard input is read.

Converting to a binary format (.docx, .idml, …) writes a binary document, so
when standard output is a terminal that is refused rather than streamed at it.
Pass -o FILE, redirect stdout, or ask for it explicitly with "-o -". Redirected
or piped output is never touched.

Several files convert in one run. Give -o a directory (a trailing slash, or a
path that already is one) and each input is written to its own file there,
named after the input and re-extensioned for the target format; the directory
is created if it does not exist. With -r, directory arguments are walked and
the sub-tree is mirrored under the output directory.

A same-format conversion (e.g. .docx → .docx) round-trips faithfully via the
document skeleton; a cross-format conversion reconstructs from the content model.`,
		Example: `  kconv report.docx --to md
  kconv report.dclg.xml -o report.html
  kconv scan.docling.JSON --to html
  kconv ~/Downloads/* --to md -o converted/
  kconv -r docs --to html -o site/
  kconv messages.xliff --to md --target fr
  docling convert in.pdf --to json | kconv -f docling --to md`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			toFmt, err := a.ResolveTargetFormat(to, outPath)
			if err != nil {
				return err
			}
			return a.RunConv(cmd.Context(), args, ConvOptions{
				To:           toFmt,
				TargetLocale: model.LocaleID(targetLoc),
				Output:       outPath,
				Recursive:    recursive,
			})
		},
	}

	f := cmd.Flags()
	f.StringVarP(&to, "to", "t", "", "target format (e.g. markdown, html, doclang, or an extension like md)")
	f.StringVarP(&outPath, "output", "o", "", "output file, a directory (`-o out/`) to write one file per input, or - for standard output (default: stdout)")
	f.BoolVarP(&recursive, "recursive", "r", false, "recurse into directory arguments")
	f.StringVar(&targetLoc, "target", "", "convert the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	a.AddSourceLangFlag(f)
	a.AddEncodingFlag(f, "", "input/output encoding")
	f.BoolVar(&a.ConvTiming, "timing", false, "report each file's conversion time on stderr")
	return cmd
}
