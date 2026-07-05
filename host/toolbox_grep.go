package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// ANSI colours matching grep's default GREP_COLORS scheme.
const (
	colorMatch    = "\x1b[01;31m" // bold red
	colorFilename = "\x1b[35m"    // magenta
	colorLineNum  = "\x1b[32m"    // green
	colorSep      = "\x1b[36m"    // cyan
	colorReset    = "\x1b[0m"
)

// grepMatch is one matching block in --json output.
type grepMatch struct {
	File    string   `json:"file"`
	Number  int      `json:"number"`
	ID      string   `json:"id,omitempty"`
	Text    string   `json:"text"`
	Matches []string `json:"matches,omitempty"`
}

type MatcherOpts struct {
	IgnoreCase   bool
	WordRegexp   bool
	FixedStrings bool
	Invert       bool
}

// Matcher compiles one or more patterns; a block matches when ANY pattern
// matches (then inverted if requested).
type Matcher struct {
	res    []*regexp.Regexp
	Invert bool
}

func NewMatcher(patterns []string, o MatcherOpts) (*Matcher, error) {
	m := &Matcher{Invert: o.Invert}
	for _, p := range patterns {
		expr := p
		if o.FixedStrings {
			expr = regexp.QuoteMeta(expr)
		}
		if o.WordRegexp {
			expr = `\b(?:` + expr + `)\b`
		}
		if o.IgnoreCase {
			expr = `(?i)` + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
		}
		m.res = append(m.res, re)
	}
	if len(m.res) == 0 {
		return nil, errors.New("no pattern given")
	}
	return m, nil
}

func (m *Matcher) Test(s string) bool {
	hit := false
	for _, re := range m.res {
		if re.MatchString(s) {
			hit = true
			break
		}
	}
	return hit != m.Invert
}

// findAll returns every matched substring across all patterns (for -o). Invert
// is not applied — there are no "matched substrings" in an inverted search.
func (m *Matcher) findAll(s string) []string {
	var out []string
	for _, re := range m.res {
		out = append(out, re.FindAllString(s, -1)...)
	}
	return out
}

// spans returns match ranges for highlighting (non-inverted only).
func (m *Matcher) spans(s string) [][]int {
	var spans [][]int
	for _, re := range m.res {
		spans = append(spans, re.FindAllStringIndex(s, -1)...)
	}
	return spans
}

type GrepOptions struct {
	Count        bool
	Number       bool
	OnlyMatching bool
	FilesWith    bool
	FilesWithout bool
	WithFilename bool
	NoFilename   bool
	Recursive    bool
	Quiet        bool
	JSON         bool
	Color        bool
	TargetLocale model.LocaleID
}

func (a *App) RunGrep(ctx context.Context, args []string, m *Matcher, opts GrepOptions) error {
	hadError := false
	files, err := expandInputs(args, opts.Recursive, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "kgrep: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}

	// Show file-name prefixes when scanning more than one file, or when -H is
	// set; -h always suppresses them.
	// A single archive argument still expands to many inner files, so show the
	// per-entry filename for containers/locators just as for multiple inputs.
	showName := (len(files) > 1 || opts.WithFilename || anyContainerInput(files)) && !opts.NoFilename

	anyMatch := false
	var jsonMatches []grepMatch

	for _, file := range files {
		fileCount := 0
		_, ferr := a.StreamBlocks(ctx, file, func(_ int, b *model.Block) error {
			text, ok := blockScopeText(b, opts.TargetLocale)
			if !ok {
				return nil
			}
			if !m.Test(text) {
				return nil
			}
			anyMatch = true
			fileCount++

			if opts.Quiet || opts.FilesWith || opts.FilesWithout || opts.Count {
				return nil // counting / existence only — no per-line output
			}
			// Attribute matches inside an archive to `<archive>!<entry>`.
			label := entryLabel(DisplayName(file), b)
			if opts.JSON {
				jm := grepMatch{File: label, Number: fileCount, ID: b.ID, Text: text}
				if opts.OnlyMatching {
					jm.Matches = m.findAll(text)
				}
				jsonMatches = append(jsonMatches, jm)
				return nil
			}
			a.printGrepMatch(label, fileCount, b.ID, text, showName, m, opts)
			return nil
		})
		if ferr != nil {
			// A cancelled context (Ctrl-C) is a global interrupt, not a per-file
			// error: stop now and let cli.Run map it to exit 130 with no message.
			if errors.Is(ferr, context.Canceled) {
				return ferr
			}
			hadError = true
			fmt.Fprintf(os.Stderr, "kgrep: %s: %v\n", DisplayName(file), ferr)
			continue
		}

		switch {
		case opts.Quiet || opts.JSON:
			// handled below / streamed above
		case opts.FilesWith:
			if fileCount > 0 {
				fmt.Println(DisplayName(file))
			}
		case opts.FilesWithout:
			if fileCount == 0 {
				fmt.Println(DisplayName(file))
			}
		case opts.Count:
			if showName {
				fmt.Printf("%s:%d\n", DisplayName(file), fileCount)
			} else {
				fmt.Printf("%d\n", fileCount)
			}
		}
	}

	if opts.JSON && !opts.Quiet {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(jsonMatches); err != nil {
			return err
		}
	}

	if hadError {
		// A read/access error occurred (the per-file warnings above were already
		// printed). grep's convention is exit 2 (trouble), which takes precedence
		// over the match/no-match status. WithExitCode(ExitUsage, ErrSilentExit)
		// yields exit 2 with no extra summary line (ErrSilentExit suppresses it).
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	if !anyMatch {
		return ErrSilentExit // exit status 1, no message (grep convention)
	}
	return nil
}

func (a *App) printGrepMatch(file string, num int, id, text string, showName bool, m *Matcher, opts GrepOptions) {
	var b strings.Builder
	if showName {
		if opts.Color {
			b.WriteString(colorFilename + DisplayName(file) + colorReset + colorSep + ":" + colorReset)
		} else {
			b.WriteString(DisplayName(file) + ":")
		}
	}
	if opts.Number {
		if opts.Color {
			b.WriteString(colorLineNum + strconv.Itoa(num) + colorReset + colorSep + ":" + colorReset)
		} else {
			b.WriteString(strconv.Itoa(num) + ":")
		}
	}

	if opts.OnlyMatching {
		prefix := b.String()
		for _, mt := range m.findAll(text) {
			if opts.Color {
				fmt.Printf("%s%s%s%s\n", prefix, colorMatch, mt, colorReset)
			} else {
				fmt.Printf("%s%s\n", prefix, mt)
			}
		}
		return
	}

	if opts.Color && !m.Invert {
		b.WriteString(highlight(text, m.spans(text)))
	} else {
		b.WriteString(text)
	}
	fmt.Println(b.String())
}

// highlight wraps each (possibly overlapping) match span in the match colour.
func highlight(s string, spans [][]int) string {
	if len(spans) == 0 {
		return s
	}
	// Merge overlapping/adjacent spans, in order.
	merged := mergeSpans(spans)
	var b strings.Builder
	last := 0
	for _, sp := range merged {
		if sp[0] < last {
			continue
		}
		b.WriteString(s[last:sp[0]])
		b.WriteString(colorMatch)
		b.WriteString(s[sp[0]:sp[1]])
		b.WriteString(colorReset)
		last = sp[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func mergeSpans(spans [][]int) [][]int {
	if len(spans) <= 1 {
		return spans
	}
	// Insertion sort by start (span counts are tiny).
	sorted := make([][]int, len(spans))
	copy(sorted, spans)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j][0] < sorted[j-1][0]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	out := [][]int{sorted[0]}
	for _, sp := range sorted[1:] {
		last := out[len(out)-1]
		if sp[0] <= last[1] {
			if sp[1] > last[1] {
				last[1] = sp[1]
			}
			continue
		}
		out = append(out, sp)
	}
	return out
}
