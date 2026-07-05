package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/model"
)

// catBlock is one block in --json output.
type catBlock struct {
	File   string `json:"file"`
	Number int    `json:"number"`
	ID     string `json:"id,omitempty"`
	Text   string `json:"text"`
}

type CatOptions struct {
	Number       bool
	ShowIDs      bool
	TargetLocale model.LocaleID
}

func (a *App) RunCat(ctx context.Context, cmd Command, args []string, opts CatOptions) error {
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
		_, ferr := a.StreamBlocks(ctx, file, func(_ int, b *model.Block) error {
			text, ok := blockScopeText(b, opts.TargetLocale)
			if !ok {
				return nil
			}
			n++
			if jsonOut {
				jsonBlocks = append(jsonBlocks, catBlock{File: DisplayName(file), Number: n, ID: b.ID, Text: text})
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
			fmt.Fprintf(os.Stderr, "kcat: %s: %v\n", DisplayName(file), ferr)
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

func printCatLine(w *os.File, n int, id, text string, opts CatOptions) {
	var prefix string
	if opts.Number {
		prefix += fmt.Sprintf("%6d\t", n)
	}
	if opts.ShowIDs && id != "" {
		prefix += id + "\t"
	}
	fmt.Fprintf(w, "%s%s\n", prefix, text)
}
