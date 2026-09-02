package cli

import "github.com/spf13/cobra"

// NewStatsCmd builds `kapi stats`: a content-metrics overview for any file —
// blocks, words, characters, segments, and a by-role breakdown. It is the
// aggregate sibling of `kapi inspect` (per-block detail) and shares the same
// --json plumbing, so an AI assistant or pipeline can size content the same way
// it reads or checks it.
func NewStatsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stats [files...]",
		Short:   "Summarize content metrics for files: blocks, words, characters, segments, by role",
		GroupID: "advanced",
		Args:    cobra.ArbitraryArgs,
		Long: `Summarize the content of one or more files: total and translatable blocks,
word and character counts (with and without spaces), segments, and a breakdown
by structural role (heading, paragraph, list-item, table-cell, …). Any format, whether
a Word document, a JSON catalog, Markdown or HTML, yields the same shape.

Word, character, and segment counts cover the translatable content; block and
role counts cover the whole document. Prints a human table by default;
--output-format json|yaml emits the structured record for piping into a pipeline.

Positional paths accept glob patterns and directories, expanded by kapi itself.
Quote the pattern and ` + "`**`" + ` recurses identically in every shell. Inside a project,
no FILE means the project's tracked content; FILE "-" reads standard input.`,
		Example: `  kapi stats report.docx
  kapi stats 'src/**'                       # every file under src, any depth
  kapi stats --output-format json 'docs/**/*.md' | jq '.total.words'
  kapi stats                                # inside a project: its tracked content
  cat page.html | kapi stats -f html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.RunStats(cmd, args)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	a.AddSourceLangFlag(f)
	a.AddEncodingFlag(f, "", "input encoding")
	return cmd
}
