package host

import (
	"context"
	"fmt"
	"io"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
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
	UniqueCharacters  int            `json:"unique_characters"`
	Segments          int            `json:"segments"`
	ByRole            map[string]int `json:"by_role,omitempty"`

	// charSet is the distinct-rune inventory of the translatable source text
	// (the aggregate that used to be the chars-listing tool — useful for font
	// subsetting). It backs UniqueCharacters and is unioned by add().
	charSet map[rune]struct{}
}

// StatsOutput is the structured result of a `kapi stats` run: a per-file record
// plus the grand total.
type StatsOutput struct {
	Files []StatsRecord `json:"files"`
	Total StatsRecord   `json:"total"`
}

// FormatText renders the stats as a human-readable table.
func (o StatsOutput) FormatText(w io.Writer) error {
	if len(o.Files) > 1 {
		t := output.NewTable(w).Accent(0).
			Headers("FILE", "BLOCKS", "TRANS", "WORDS", "CHARS", "SEGMENTS")
		for _, r := range o.Files {
			t.Rowf(r.File, r.Blocks, r.Translatable, r.Words, r.Characters, r.Segments)
		}
		total := o.Total
		t.Rowf(fmt.Sprintf("Total (%d files)", len(o.Files)),
			total.Blocks, total.Translatable, total.Words, total.Characters, total.Segments)
		t.Render()
	} else {
		// Single file: a vertical, label:value summary reads better than one row.
		r := o.Total
		fmt.Fprintf(w, "Blocks:                %7d\n", r.Blocks)
		fmt.Fprintf(w, "  translatable:        %7d\n", r.Translatable)
		fmt.Fprintf(w, "  non-translatable:    %7d\n", r.NonTranslatable)
		fmt.Fprintf(w, "Words:                 %7d\n", r.Words)
		fmt.Fprintf(w, "Characters:            %7d\n", r.Characters)
		fmt.Fprintf(w, "  (no spaces):         %7d\n", r.CharactersNoSpace)
		fmt.Fprintf(w, "  unique:              %7d\n", r.UniqueCharacters)
		fmt.Fprintf(w, "Segments:              %7d\n", r.Segments)
	}

	if len(o.Total.ByRole) > 0 {
		fmt.Fprintln(w)
		output.Title(w, "By role:")
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
		t := output.NewTable(w).Accent(0).Headers("ROLE", "BLOCKS")
		for _, role := range roles {
			t.Rowf(role, o.Total.ByRole[role])
		}
		t.Render()
	}
	return nil
}

func (a *App) RunStats(cmd Command, args []string) error {
	a.InitRegistries()
	ctx := CmdContext(cmd)

	hadError := false
	files, err := a.ResolveInputs(cmd, args, InputOptions{
		Command: "kapi stats",
		OnSkip: func(path string, err error) {
			hadError = true
			fmt.Fprintf(cmd.ErrOrStderr(), "kapi stats: %s: %v\n", path, err)
		},
	})
	if err != nil {
		return err
	}

	prog := a.NewProgress(cmd, "reading", len(files))
	defer prog.Done()
	out, err := a.computeStats(ctx, files, prog, func(file string, ferr error) {
		hadError = true
		fmt.Fprintf(cmd.ErrOrStderr(), "kapi stats: %s: %v\n", DisplayName(file), ferr)
	})
	prog.Done()
	if err != nil {
		return err
	}

	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}

// ComputeStats streams each file and aggregates the per-file records plus the
// grand total — the compute path shared by `kapi stats` and the stats MCP tool.
// When onError is non-nil a failing file is reported through it and skipped;
// with a nil onError the first failure is fatal. Context cancellation is
// always fatal.
func (a *App) ComputeStats(ctx context.Context, files []string, onError func(file string, err error)) (StatsOutput, error) {
	return a.computeStats(ctx, files, nil, onError)
}

// computeStats is ComputeStats with an optional progress reporter — reading a
// large content set is exactly the multi-second work the progress convention
// exists for.
func (a *App) computeStats(ctx context.Context, files []string, prog *Progress, onError func(file string, err error)) (StatsOutput, error) {
	out := StatsOutput{Total: StatsRecord{ByRole: map[string]int{}}}
	for _, file := range files {
		prog.Step(DisplayName(file))
		recs, ferr := a.fileStats(ctx, file)
		prog.Advance()
		if ferr != nil {
			if ctx.Err() != nil {
				return out, ferr
			}
			if onError == nil {
				return out, fmt.Errorf("%s: %w", DisplayName(file), ferr)
			}
			onError(file, ferr)
			continue
		}
		// One row per source: a plain file yields a single record; an archive
		// yields one per inner entry (keyed `<archive>!<entry>`).
		for _, rec := range recs {
			out.Files = append(out.Files, rec)
			out.Total.add(rec)
		}
	}
	return out, nil
}

// fileStats streams one source's blocks and computes content metrics, bucketed
// by origin: a plain file yields a single record; an archive yields one record
// per inner entry (keyed `<archive>!<entry>`) so the table breaks the container
// down by file rather than aggregating it into one opaque row.
func (a *App) fileStats(ctx context.Context, file string) ([]StatsRecord, error) {
	byLabel := map[string]*StatsRecord{}
	var order []string
	_, err := a.StreamBlocks(ctx, file, func(_ int, b *model.Block) error {
		label := entryLabel(DisplayName(file), b)
		rec := byLabel[label]
		if rec == nil {
			rec = &StatsRecord{File: label, ByRole: map[string]int{}}
			byLabel[label] = rec
			order = append(order, label)
		}
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
		if rec.charSet == nil {
			rec.charSet = map[rune]struct{}{}
		}
		for _, r := range text {
			rec.charSet[r] = struct{}{}
		}
		rec.Segments += b.SourceSegmentCount()
		return nil
	})
	if err != nil {
		return nil, err
	}
	recs := make([]StatsRecord, 0, len(order))
	for _, l := range order {
		byLabel[l].UniqueCharacters = len(byLabel[l].charSet)
		recs = append(recs, *byLabel[l])
	}
	return recs, nil
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
	if r.charSet == nil {
		r.charSet = map[rune]struct{}{}
	}
	for c := range o.charSet {
		r.charSet[c] = struct{}{}
	}
	r.UniqueCharacters = len(r.charSet)
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
