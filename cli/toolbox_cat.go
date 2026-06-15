package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// catBlock is one block in --json output.
type catBlock struct {
	File   string `json:"file"`
	Number int    `json:"number"`
	ID     string `json:"id,omitempty"`
	Text   string `json:"text"`
}

// newCatCmd builds the cat command. Used as the standalone `kcat` root and,
// via newToolboxProxies, behind the detached `kapi cat` subcommand.
func (a *App) newCatCmd() *cobra.Command {
	var (
		number    bool
		showIDs   bool
		targetLoc string
	)

	cmd := &cobra.Command{
		Use:     "cat [flags] [FILE...]",
		Short:   "Print the translatable text of files, block by block",
		GroupID: "content",
		Long: `Print the human-readable text extracted from each file, one block per line,
regardless of the underlying format. A Word .docx, a JSON catalog and an XLIFF
file all print as their plain prose, with the markup and structure stripped.

With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  kcat report.docx
  kcat -n locales/en.json
  kcat --target fr messages.xliff
  cat raw.txt | kcat -f plaintext`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runCat(cmd.Context(), cmd, args, catOptions{
				number:    number,
				showIDs:   showIDs,
				targetLoc: model.LocaleID(targetLoc),
			})
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&number, "number", "n", false, "number the output blocks")
	f.BoolVar(&showIDs, "id", false, "prefix each block with its source ID")
	f.StringVar(&targetLoc, "target", "", "print the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input encoding")
	f.Bool("json", false, "emit blocks as JSON instead of plain text")
	return cmd
}

type catOptions struct {
	number    bool
	showIDs   bool
	targetLoc model.LocaleID
}

func (a *App) runCat(ctx context.Context, cmd *cobra.Command, args []string, opts catOptions) error {
	hadError := false
	files, err := expandInputs(args, false, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "kcat: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	var jsonBlocks []catBlock
	n := 0

	for _, file := range files {
		_, ferr := a.streamBlocks(ctx, file, func(_ int, b *model.Block) error {
			text, ok := blockScopeText(b, opts.targetLoc)
			if !ok {
				return nil
			}
			n++
			if jsonOut {
				jsonBlocks = append(jsonBlocks, catBlock{File: displayName(file), Number: n, ID: b.ID, Text: text})
				return nil
			}
			printCatLine(os.Stdout, n, b.ID, text, opts)
			return nil
		})
		if ferr != nil {
			// A cancelled context (Ctrl-C) is a global interrupt, not a per-file
			// error: stop now and let cli.Run map it to exit 130 with no message.
			if errors.Is(ferr, context.Canceled) {
				return ferr
			}
			// Otherwise report the bad file and carry on, so one unparseable file
			// doesn't abort the rest — matching kgrep/ksed and GNU `cat`.
			hadError = true
			fmt.Fprintf(os.Stderr, "kcat: %s: %v\n", displayName(file), ferr)
			continue
		}
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(jsonBlocks); err != nil {
			return err
		}
	}
	if hadError {
		// A read error occurred (warning already printed); exit 2 (trouble),
		// matching the grep-style toolbox exit-code contract and kgrep.
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}

// blockScopeText returns the text for the requested locale scope: the source
// when targetLoc is empty, otherwise that target. ok is false when there is no
// such text (e.g. a block with no translation for the locale).
func blockScopeText(b *model.Block, targetLoc model.LocaleID) (string, bool) {
	if targetLoc == "" {
		s := b.SourceText()
		return s, s != ""
	}
	if !b.HasTarget(targetLoc) {
		return "", false
	}
	return b.TargetText(targetLoc), true
}

func printCatLine(w *os.File, n int, id, text string, opts catOptions) {
	var prefix string
	if opts.number {
		prefix += fmt.Sprintf("%6d\t", n)
	}
	if opts.showIDs && id != "" {
		prefix += id + "\t"
	}
	fmt.Fprintf(w, "%s%s\n", prefix, text)
}
