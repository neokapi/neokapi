package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewInspectCmd builds `kapi inspect`: read any format into one record per
// content block, the read leg of the change contract. Each record carries the
// reference and revision an edit names, the text in placeholder form, the
// inline codes and plurals it holds, its other editions and the operations it
// accepts, so what kapi inspect prints is what kapi apply takes.
//
// Its shape selector is the shared `--output-format` axis (json / yaml / text),
// not private flags: inspect emits structured records by nature, so text mode
// renders the JSON array. `--jsonl` remains as the one genuinely different
// framing — a stream rather than a document — which the format axis has no
// spelling for.
func NewInspectCmd(a *App) *cobra.Command {
	var (
		jsonl  bool
		render []string
	)
	cmd := &cobra.Command{
		Use:     "inspect [flags] [FILE...]",
		Short:   "Read any format into content blocks, with the references and revisions an edit names",
		GroupID: "advanced",
		Long: `Read each file into one record per content block. Any format, whether a Word
document, a JSON catalog, Markdown or HTML, yields the same shape, so an agent
or a script can read content and send edits back to the same blocks with kapi
apply:

  ref         the block's reference, {"doc", "block", "edition"}, to copy into
              an operation's "at"
  rev         the revision of the block's text, to send as "if_match"
  text        the text, with inline codes as <x id="…"/> placeholders
  codes       each inline code by the id its placeholder shows: its type,
              its attributes, and the attributes an edit may change
  structures  each plural or select, with the path to each branch and the
              branch's text
  editions    the block's other editions, each with its revision and text,
              and the status the file records for it where it keeps one
  ops         the operations the block accepts
  role, level the block's structural role (heading, list-item, table-cell,
              …) and nesting level

Inside a project (the one -p names, or the one found from the working
directory) a file is named by its project-relative path and read with the
format and configuration the recipe binds, and each declared translation is
listed among the editions. A file outside the project is named by its absolute
path. A bilingual file whose reader has to be told the language of the
translation it holds, such as a PO catalog, lists it with --target-lang. A
source file whose comments are what kapi edits in it lists each comment, keyed
as kapi check reports it (func/Parse).

Prints a JSON array by default; --output-format yaml emits a YAML sequence, and
--jsonl streams one JSON object per line (JSONL) for piping into a script.

Positional paths take glob patterns (` + "`**`" + ` recurses) and directories, expanded
by kapi itself. Inside a project, no FILE means the project's tracked content;
FILE "-" reads standard input.`,
		Example: `  kapi inspect report.docx
  kapi inspect --jsonl 'docs/**/*.md' | jq '{ref, rev}'
  kapi inspect --output-format yaml report.dclg.xml
  kapi inspect report.docx --render html,markdown   # each block rendered per format
  kapi inspect -p site/kapi.yaml site/docs/guide.md
  cat page.html | kapi inspect -f html`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			outFormat := "json"
			switch {
			case jsonl:
				if output.ResolveFormat(cmd) == output.FormatYAML {
					return errors.New("--jsonl streams JSON objects; it cannot be combined with --output-format yaml")
				}
				outFormat = "jsonl"
			case output.ResolveFormat(cmd) == output.FormatYAML:
				outFormat = "yaml"
			}
			supported := formats.BlockFragmentFormats()
			for _, p := range render {
				if !slices.Contains(supported, p) {
					return WithExitCode(ExitUsage, fmt.Errorf("--render: unsupported format %q (supported: %v)", p, supported))
				}
			}
			return a.RunInspect(cmd.Context(), cmd, args, outFormat, render)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&jsonl, "jsonl", false, "stream one JSON object per line (JSONL) instead of a JSON array")
	f.StringSliceVar(&render, "render", nil, "also render each block to these formats (html, markdown, asciidoc) under \"projected\"")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: what the recipe binds, else auto-detect by extension/content)")
	a.AddEngineFlag(f)
	a.AddTargetLangFlag(f)
	a.AddEncodingFlag(f, "", "input encoding")
	AddProjectFlag(cmd)
	return cmd
}
