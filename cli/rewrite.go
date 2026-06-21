package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"
)

// newRewriteCmd builds the `kapi rewrite` command: an AI-driven, format-aware
// stream editor. It is the content-editing counterpart to `ksed` — where ksed
// applies a regex substitution, rewrite hands the human-readable text of each
// block to an LLM with a plain-language instruction, then writes the document
// back in the same format, byte-for-byte except for the edited text.
//
// The faithful round-trip is the whole point: a .docx keeps its styles, a JSON
// catalog keeps its keys and shape, inline codes/placeholders survive — your AI
// edits the content inside the file, not the file. Use --diff to preview the
// before/after of every changed block and write nothing.
func (a *App) newRewriteCmd() *cobra.Command {
	var (
		instruction string
		provider    string
		modelName   string
		apiKey      string
		credential  string
		diff        bool
	)

	cmd := &cobra.Command{
		Use:     "rewrite [flags] --instruction TEXT FILE...",
		Short:   "Rewrite the text/content inside files with an AI instruction (faithful round-trip)",
		GroupID: "content",
		Long: `Rewrite the human-readable text inside any supported format with an LLM,
following a plain-language --instruction, then write the document back in the
same format. Only the editable text changes — a .docx keeps its styles, a JSON
catalog keeps its keys and shape, and inline codes/placeholders are preserved.

By default the rewritten document is written to standard output; use -i to edit
files in place, optionally keeping a backup (--in-place=.bak). With --diff
nothing is written — kapi prints a unified before/after diff of every changed
block so you can review the edit first.

This is the safe way to let an AI assistant edit content it otherwise cannot
open: the structure-preserving pipeline guarantees the rewrite lands only in the
text, never the surrounding markup.

With no FILE, or when FILE is "-", standard input is read (not valid with -i).`,
		Example: `  kapi rewrite --instruction "make it more concise" guide.md
  kapi rewrite --instruction "use UK spelling" --in-place=.bak *.docx
  kapi rewrite --instruction "friendlier tone" --diff locales/en.json
  kapi rewrite --instruction "fix typos" --provider anthropic -i report.docx`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if instruction == "" {
				return errors.New("an --instruction is required (what should the rewrite do?)")
			}

			inPlace := cmd.Flags().Changed("in-place")
			if inPlace && diff {
				return errors.New("--diff previews changes without writing; it cannot be combined with -i/--in-place")
			}
			backupSuffix := ""
			if inPlace {
				if v, _ := cmd.Flags().GetString("in-place"); v != sentinelInPlace {
					backupSuffix = v
				}
			}

			t, err := a.buildRewriteTool(instruction, provider, modelName, apiKey, credential)
			if err != nil {
				return err
			}

			if diff {
				return a.runRewriteDiff(cmd.Context(), args, t, cmd.OutOrStdout())
			}
			return a.runRewrite(cmd.Context(), args, t, inPlace, backupSuffix)
		},
	}

	f := cmd.Flags()
	f.StringVar(&instruction, "instruction", "", "plain-language instruction describing how to rewrite the text (required)")
	f.StringVar(&provider, "provider", "", "AI provider (default: anthropic, or the configured default)")
	f.StringVar(&modelName, "model", "", "AI model name")
	f.StringVar(&apiKey, "api-key", "", "API key for the AI provider")
	f.StringVar(&credential, "credential", "", "saved credential name (see 'kapi credentials list')")
	f.BoolVar(&diff, "diff", false, "preview a unified diff of changed blocks and write nothing")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input/output format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input/output encoding")

	// -i edits in place; --in-place=.bak also keeps a backup. (The sed-style
	// attached short form -i.bak only works for ksed, which normalizes argv in
	// its busybox/proxy entry; a normal cobra subcommand cannot, so the backup
	// form is the long --in-place=SUFFIX.)
	f.StringP("in-place", "i", "", "edit files in place; keep a backup with --in-place=.bak")
	f.Lookup("in-place").NoOptDefVal = sentinelInPlace

	return cmd
}

// buildRewriteTool constructs the rewrite tool through the registry so the
// shared config preprocessor resolves saved credentials and configured
// provider/model defaults, then type-asserts the framework BaseTool the faithful
// editDocument path drives.
func (a *App) buildRewriteTool(instruction, provider, modelName, apiKey, credential string) (*tool.BaseTool, error) {
	config := map[string]any{"instruction": instruction}
	if provider != "" {
		config["provider"] = provider
	}
	if modelName != "" {
		config["model"] = modelName
	}
	if apiKey != "" {
		config["apiKey"] = apiKey
	}
	if credential != "" {
		config["credential"] = credential
	}
	t, err := a.ToolReg.NewToolWithConfig(registry.ToolID("rewrite"), config, a.TargetLang)
	if err != nil {
		return nil, err
	}
	bt, ok := t.(*tool.BaseTool)
	if !ok {
		return nil, fmt.Errorf("rewrite: unexpected tool type %T", t)
	}
	return bt, nil
}

// runRewrite applies the rewrite tool to each input file, writing the
// reconstructed document in place or to standard output. Mirrors runSed.
func (a *App) runRewrite(ctx context.Context, args []string, t *tool.BaseTool, inPlace bool, backupSuffix string) error {
	hadError := false
	files, err := expandInputs(args, false, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "rewrite: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}
	for _, file := range files {
		// "" writeLocale: the rewrite edits source, so the document round-trips
		// monolingually in its own format.
		if err := a.editDocument(ctx, file, t, "", inPlace, backupSuffix, os.Stdout); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			hadError = true
			fmt.Fprintf(os.Stderr, "rewrite: %s: %v\n", displayName(file), err)
		}
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}

// runRewriteDiff streams each file's blocks, runs the rewrite producer over each
// one, and prints a unified before/after diff of every block whose text the
// rewrite changed — writing nothing. This is the reviewable-diff dry run.
func (a *App) runRewriteDiff(ctx context.Context, args []string, t *tool.BaseTool, out io.Writer) error {
	hadError := false
	files, err := expandInputs(args, false, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "rewrite: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}
	changed := 0
	for _, file := range files {
		n, derr := a.rewriteDiffFile(ctx, file, t, out)
		if derr != nil {
			if errors.Is(derr, context.Canceled) {
				return derr
			}
			hadError = true
			fmt.Fprintf(os.Stderr, "rewrite: %s: %v\n", displayName(file), derr)
			continue
		}
		changed += n
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	if changed == 0 {
		fmt.Fprintln(os.Stderr, "rewrite: no changes")
	}
	return nil
}

// rewriteDiffFile prints the per-block unified diff for one file and returns the
// number of changed blocks. The block source is rewritten in memory only (the
// producer's plan is applied to the streamed block); nothing is written to disk.
func (a *App) rewriteDiffFile(ctx context.Context, file string, t *tool.BaseTool, out io.Writer) (int, error) {
	changed := 0
	label := displayName(file)
	_, err := a.streamBlocks(ctx, file, func(index int, b *model.Block) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		before := model.RunsText(b.Source)
		part := &model.Part{Type: model.PartBlock, Resource: b}
		if _, aerr := t.ApplyContext(ctx, part); aerr != nil {
			return aerr
		}
		after := model.RunsText(b.Source)
		if before == after {
			return nil
		}
		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(before),
			B:        difflib.SplitLines(after),
			FromFile: fmt.Sprintf("%s:%d (before)", label, index),
			ToFile:   fmt.Sprintf("%s:%d (after)", label, index),
			Context:  3,
		}
		text, derr := difflib.GetUnifiedDiffString(diff)
		if derr != nil {
			return derr
		}
		if _, werr := out.Write([]byte(text)); werr != nil {
			return werr
		}
		changed++
		return nil
	})
	return changed, err
}
