package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// StatsRecord is the content-metrics summary for one file (or the grand total).
// It is the overview complement to `kapi inspect` (per-block detail): the same
// generic, source-side shape for any format, so a content/RAG pipeline or an AI
// assistant can size and survey a document before processing it. Word, character
// and segment counts cover the translatable content; block and role counts cover
// the whole document.
type StatsRecord struct {
	File              string         `json:"file,omitempty"`
	Blocks            int            `json:"blocks"`
	Translatable      int            `json:"translatable"`
	NonTranslatable   int            `json:"non_translatable"`
	Words             int            `json:"words"`
	Characters        int            `json:"characters"`
	CharactersNoSpace int            `json:"characters_no_space"`
	Segments          int            `json:"segments"`
	ByRole            map[string]int `json:"by_role,omitempty"`
}

// StatsOutput is the structured result of a `kapi stats` run: a per-file record
// plus the grand total.
type StatsOutput struct {
	Files []StatsRecord `json:"files"`
	Total StatsRecord   `json:"total"`
}

// FormatText renders the stats as a human-readable table.
func (o StatsOutput) FormatText(w io.Writer) error {
	fileWidth := len("FILE")
	for _, r := range o.Files {
		if len(r.File) > fileWidth {
			fileWidth = len(r.File)
		}
	}
	fileWidth += 2

	header := func() {
		fmt.Fprintf(w, "%-*s %7s %7s %9s %9s %9s\n", fileWidth, "FILE", "BLOCKS", "TRANS", "WORDS", "CHARS", "SEGMENTS")
	}
	row := func(r StatsRecord) {
		fmt.Fprintf(w, "%-*s %7d %7d %9d %9d %9d\n", fileWidth, r.File, r.Blocks, r.Translatable, r.Words, r.Characters, r.Segments)
	}

	if len(o.Files) > 1 {
		header()
		for _, r := range o.Files {
			row(r)
		}
		fmt.Fprintln(w, strings.Repeat("─", fileWidth+7+7+9+9+9+5))
		total := o.Total
		total.File = fmt.Sprintf("Total (%d files)", len(o.Files))
		row(total)
	} else {
		// Single file: a vertical, label:value summary reads better than one row.
		r := o.Total
		fmt.Fprintf(w, "Blocks:                %7d\n", r.Blocks)
		fmt.Fprintf(w, "  translatable:        %7d\n", r.Translatable)
		fmt.Fprintf(w, "  non-translatable:    %7d\n", r.NonTranslatable)
		fmt.Fprintf(w, "Words:                 %7d\n", r.Words)
		fmt.Fprintf(w, "Characters:            %7d\n", r.Characters)
		fmt.Fprintf(w, "  (no spaces):         %7d\n", r.CharactersNoSpace)
		fmt.Fprintf(w, "Segments:              %7d\n", r.Segments)
	}

	if len(o.Total.ByRole) > 0 {
		fmt.Fprintln(w, "\nBy role:")
		roles := make([]string, 0, len(o.Total.ByRole))
		for role := range o.Total.ByRole {
			roles = append(roles, role)
		}
		sort.Slice(roles, func(i, j int) bool {
			if o.Total.ByRole[roles[i]] != o.Total.ByRole[roles[j]] {
				return o.Total.ByRole[roles[i]] > o.Total.ByRole[roles[j]]
			}
			return roles[i] < roles[j]
		})
		for _, role := range roles {
			fmt.Fprintf(w, "  %-20s %7d\n", role, o.Total.ByRole[role])
		}
	}
	return nil
}

// NewStatsCmd builds `kapi stats`: a content-metrics overview for any file —
// blocks, words, characters, segments, and a by-role breakdown. It is the
// aggregate sibling of `kapi inspect` (per-block detail) and shares the same
// --json plumbing, so an AI assistant or pipeline can size content the same way
// it reads or checks it.
func (a *App) NewStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stats [files...]",
		Short:   "Summarize content metrics for files — blocks, words, characters, segments, by role",
		GroupID: "analysis",
		Args:    cobra.ArbitraryArgs,
		Long: `Summarize the content of one or more files: total and translatable blocks,
word and character counts (with and without spaces), segments, and a breakdown
by structural role (heading, paragraph, list-item, table-cell, …). Any format —
a Word document, a JSON catalog, Markdown, HTML — yields the same shape.

Word, character, and segment counts cover the translatable content; block and
role counts cover the whole document. Prints a human table by default; --json
emits the structured record for piping into a pipeline.

With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  kapi stats report.docx
  kapi stats --json docs/*.md | jq '.total.words'
  cat page.html | kapi stats -f html`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runStats(cmd, args)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input encoding")
	return cmd
}

func (a *App) runStats(cmd *cobra.Command, args []string) error {
	a.InitRegistries()
	ctx := cmdContext(cmd)

	hadError := false
	files, err := expandInputs(args, false, func(path string, err error) {
		hadError = true
		fmt.Fprintf(cmd.ErrOrStderr(), "kapi stats: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}

	out := StatsOutput{Total: StatsRecord{ByRole: map[string]int{}}}
	for _, file := range files {
		rec, ferr := a.fileStats(ctx, file)
		if ferr != nil {
			if ctx.Err() != nil {
				return ferr
			}
			hadError = true
			fmt.Fprintf(cmd.ErrOrStderr(), "kapi stats: %s: %v\n", displayName(file), ferr)
			continue
		}
		out.Files = append(out.Files, rec)
		out.Total.add(rec)
	}

	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}

// fileStats streams one file's blocks and computes its content metrics.
func (a *App) fileStats(ctx context.Context, file string) (StatsRecord, error) {
	rec := StatsRecord{File: displayName(file), ByRole: map[string]int{}}
	_, err := a.streamBlocks(ctx, file, func(_ int, b *model.Block) error {
		rec.Blocks++
		if s, ok := b.Structure(); ok && s.Role != "" {
			rec.ByRole[s.Role]++
		}
		if !b.Translatable {
			rec.NonTranslatable++
			return nil
		}
		rec.Translatable++
		text := b.SourceText()
		rec.Words += b.WordCount()
		rec.Characters += utf8.RuneCountInString(text)
		rec.CharactersNoSpace += countNonSpace(text)
		rec.Segments += b.SourceSegmentCount()
		return nil
	})
	return rec, err
}

// add accumulates another record's metrics into r (the running total).
func (r *StatsRecord) add(o StatsRecord) {
	r.Blocks += o.Blocks
	r.Translatable += o.Translatable
	r.NonTranslatable += o.NonTranslatable
	r.Words += o.Words
	r.Characters += o.Characters
	r.CharactersNoSpace += o.CharactersNoSpace
	r.Segments += o.Segments
	for role, n := range o.ByRole {
		r.ByRole[role] += n
	}
}

// countNonSpace counts the runes in s that are not whitespace.
func countNonSpace(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}
