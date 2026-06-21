package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// inspectBlock is one content block in `kapi inspect` output: the parsed text
// plus a stable content-hash anchor and the block's structural role. It is the
// read end of read → retrieve → edit → write-back — an AI agent or RAG pipeline
// reads these records, retrieves against the anchors, and writes edits back to
// the same blocks (by ID) with the toolbox or a Transform tool.
type inspectBlock struct {
	File   string `json:"file"`
	Number int    `json:"number"` // 1-based, increments across all input files
	ID     string `json:"id,omitempty"`
	// ContentHash is model.ComputeContentHash(Text): a SHA-256 over the block's
	// NORMALIZED (whitespace-trimmed) source text, not over the raw Text field.
	// Recompute it with model.ComputeContentHash, not a bare sha256(text).
	ContentHash string `json:"content_hash,omitempty"`
	Role        string `json:"role,omitempty"`
	Level       int    `json:"level,omitempty"`
	Text        string `json:"text"`
}

// NewInspectCmd builds `kapi inspect`: parse any format into one anchored,
// structured record per content block. Prints a JSON array by default; --jsonl
// streams one object per line for piping into an ingestion pipeline.
func (a *App) NewInspectCmd() *cobra.Command {
	var jsonl bool
	cmd := &cobra.Command{
		Use:     "inspect [flags] [FILE...]",
		Short:   "Parse any format into anchored, structured content blocks",
		GroupID: "processing",
		Long: `Parse each file into one record per content block: the text, a stable
content-hash anchor, and the block's structural role (heading, list-item,
table-cell, …) and nesting level. Any format — a Word document, a JSON catalog,
Markdown, HTML — yields the same shape, so an AI agent or RAG pipeline can read
content, retrieve against the anchors, and write edits back to the same blocks.

Prints a JSON array by default; --jsonl streams one object per line (JSONL).
With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  kapi inspect report.docx
  kapi inspect --jsonl docs/*.md | jq .content_hash
  cat page.html | kapi inspect -f html`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runInspect(cmd.Context(), cmd, args, jsonl)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&jsonl, "jsonl", false, "stream one JSON object per line (JSONL) instead of a JSON array")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input encoding")
	return cmd
}

func (a *App) runInspect(ctx context.Context, cmd *cobra.Command, args []string, jsonl bool) error {
	hadError := false
	files, err := expandInputs(args, false, func(path string, err error) {
		hadError = true
		fmt.Fprintf(cmd.ErrOrStderr(), "kapi inspect: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	if !jsonl {
		enc.SetIndent("", "  ")
	}
	var blocks []inspectBlock // accumulated for the JSON-array form
	n := 0

	for _, file := range files {
		_, ferr := a.streamBlocks(ctx, file, func(_ int, b *model.Block) error {
			text := b.SourceText()
			if text == "" {
				return nil
			}
			n++
			rec := inspectBlock{
				File:        displayName(file),
				Number:      n,
				ID:          b.ID,
				ContentHash: model.ComputeContentHash(text),
				Text:        text,
			}
			if s, ok := b.Structure(); ok {
				rec.Role, rec.Level = s.Role, s.Level
			}
			if jsonl {
				return enc.Encode(rec) // Encode writes one object + newline = JSONL
			}
			blocks = append(blocks, rec)
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

	if !jsonl {
		if err := enc.Encode(blocks); err != nil {
			return err
		}
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}
