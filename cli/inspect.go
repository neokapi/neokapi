package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewInspectCmd builds `kapi inspect`: parse any format into one anchored,
// structured record per content block — the text, a stable content-hash anchor,
// and the block's structural role and nesting level. The same record shape backs
// `kapi kconv <doc> --to json|yaml`.
//
// Its shape selector is the shared `--output-format` axis (json / yaml / text),
// not private flags: inspect emits structured records by nature, so text mode
// renders the JSON array. `--jsonl` remains as the one genuinely different
// framing — a stream rather than a document — which the format axis has no
// spelling for.
func NewInspectCmd(a *App) *cobra.Command {
	var (
		jsonl   bool
		project []string
	)
	cmd := &cobra.Command{
		Use:     "inspect [flags] [FILE...]",
		Short:   "Parse any format into anchored, structured content blocks",
		GroupID: "advanced",
		Long: `Parse each file into one record per content block: the text, a stable
content-hash anchor, and the block's structural role (heading, list-item,
table-cell, …) and nesting level. Any format, whether a Word document, a JSON catalog,
Markdown or HTML, yields the same shape, so an AI agent or RAG pipeline can read
content, retrieve against the anchors, and write edits back to the same blocks.

Prints a JSON array by default; --output-format yaml emits a YAML sequence, and
--jsonl streams one JSON object per line (JSONL) for piping into an ingestion
pipeline.

Positional paths take glob patterns (` + "`**`" + ` recurses) and directories, expanded
by kapi itself. Inside a project, no FILE means the project's tracked content;
FILE "-" reads standard input.`,
		Example: `  kapi inspect report.docx
  kapi inspect --jsonl 'docs/**/*.md' | jq .content_hash
  kapi inspect --output-format yaml report.dclg.xml
  kapi inspect report.docx --project html,markdown   # each block rendered per format
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
			for _, p := range project {
				if !slices.Contains(supported, p) {
					return fmt.Errorf("--project: unsupported format %q (supported: %v)", p, supported)
				}
			}
			return a.RunInspect(cmd.Context(), cmd, args, outFormat, project)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&jsonl, "jsonl", false, "stream one JSON object per line (JSONL) instead of a JSON array")
	f.StringSliceVar(&project, "project", nil, "also render each block to these target formats (html, markdown, asciidoc) under \"projected\"")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	a.AddSourceLangFlag(f)
	a.AddEncodingFlag(f, "", "input encoding")
	return cmd
}
