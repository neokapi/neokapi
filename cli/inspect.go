package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structrec"
	"github.com/spf13/cobra"
)

// NewInspectCmd builds `kapi inspect`: parse any format into one anchored,
// structured record per content block — the text, a stable content-hash anchor,
// and the block's structural role and nesting level. The same record shape backs
// `kapi convert <doc> --to json|yaml`. Prints a JSON array by default; --jsonl
// streams one object per line, --yaml emits a YAML sequence.
func (a *App) NewInspectCmd() *cobra.Command {
	var (
		jsonl  bool
		asYAML bool
	)
	cmd := &cobra.Command{
		Use:     "inspect [flags] [FILE...]",
		Short:   "Parse any format into anchored, structured content blocks",
		GroupID: "processing",
		Long: `Parse each file into one record per content block: the text, a stable
content-hash anchor, and the block's structural role (heading, list-item,
table-cell, …) and nesting level. Any format — a Word document, a JSON catalog,
Markdown, HTML — yields the same shape, so an AI agent or RAG pipeline can read
content, retrieve against the anchors, and write edits back to the same blocks.

Prints a JSON array by default; --jsonl streams one JSON object per line (JSONL)
for piping into an ingestion pipeline; --yaml emits a YAML sequence.
With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  kapi inspect report.docx
  kapi inspect --jsonl docs/*.md | jq .content_hash
  kapi inspect --yaml report.dclg.xml
  cat page.html | kapi inspect -f html`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonl && asYAML {
				return errors.New("--jsonl and --yaml are mutually exclusive")
			}
			outFormat := "json"
			switch {
			case jsonl:
				outFormat = "jsonl"
			case asYAML:
				outFormat = "yaml"
			}
			return a.runInspect(cmd.Context(), cmd, args, outFormat)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&jsonl, "jsonl", false, "stream one JSON object per line (JSONL) instead of a JSON array")
	f.BoolVar(&asYAML, "yaml", false, "emit a YAML sequence instead of a JSON array")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input encoding")
	return cmd
}

func (a *App) runInspect(ctx context.Context, cmd *cobra.Command, args []string, outFormat string) error {
	hadError := false
	files, err := expandInputs(args, false, func(path string, err error) {
		hadError = true
		fmt.Fprintf(cmd.ErrOrStderr(), "kapi inspect: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}

	streaming := outFormat == "jsonl"
	enc := json.NewEncoder(cmd.OutOrStdout())
	var recs []structrec.Record // accumulated for the array (json/yaml) forms
	n := 0

	for _, file := range files {
		_, ferr := a.streamBlocks(ctx, file, func(_ int, b *model.Block) error {
			if b.SourceText() == "" {
				return nil
			}
			n++
			rec := structrec.FromBlock(n, b, b.SourceRuns())
			rec.File = displayName(file)
			if streaming {
				return enc.Encode(rec) // Encode writes one object + newline = JSONL
			}
			recs = append(recs, rec)
			return nil
		})
		if ferr != nil {
			// A cancelled context (Ctrl-C) is a global interrupt; stop now. Any
			// other error is per-file: report it and carry on, matching kcat.
			if errors.Is(ferr, context.Canceled) {
				return ferr
			}
			hadError = true
			fmt.Fprintf(cmd.ErrOrStderr(), "kapi inspect: %s: %v\n", displayName(file), ferr)
			continue
		}
	}

	if !streaming {
		var (
			out  []byte
			mErr error
		)
		if outFormat == "yaml" {
			out, mErr = structrec.MarshalYAML(recs)
		} else {
			out, mErr = structrec.MarshalJSONArray(recs)
		}
		if mErr != nil {
			return mErr
		}
		if _, wErr := cmd.OutOrStdout().Write(out); wErr != nil {
			return wErr
		}
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}
