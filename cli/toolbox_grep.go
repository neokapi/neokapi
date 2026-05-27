package cli

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
	"github.com/spf13/cobra"
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

// newGrepCmd builds the grep command with the full classic option surface.
// It is used as the standalone `kgrep` root and, via newToolboxProxies, behind
// the detached `kapi grep` subcommand — never as a plain child of the kapi root
// (which would shadow -v/-c with kapi's global flags).
func (a *App) newGrepCmd() *cobra.Command {
	var (
		ignoreCase   bool
		invert       bool
		count        bool
		number       bool
		onlyMatching bool
		filesWith    bool
		filesWithout bool
		wordRegexp   bool
		fixedStrings bool
		recursive    bool
		withFilename bool
		noFilename   bool
		patterns     []string
		targetLoc    string
	)

	cmd := &cobra.Command{
		Use:     "grep [flags] PATTERN [FILE...]",
		Short:   "Search the translatable text of files for a pattern",
		GroupID: "content",
		Long: `Search the human-readable text inside any supported format for a regular
expression — the prose of a Word .docx, the values of a JSON catalog, the
segments of an XLIFF file — skipping markup and structure. Output mirrors grep:
one matching block per line, optionally prefixed with the file name and the
block number.

With no FILE, or when FILE is "-", standard input is read. Exit status is 0 if
any block matched, 1 if none did, 2 on error.`,
		Example: `  kgrep "Tervetuloa" report.docx
  kgrep -i todo locales/*.json
  kgrep -r --target fr "déconnexion" ./content
  kgrep -c "©" *.md`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Pattern comes from -e flags, or the first positional argument.
			pats := patterns
			files := args
			if len(pats) == 0 {
				if len(args) == 0 {
					return errors.New("no pattern given")
				}
				pats = []string{args[0]}
				files = args[1:]
			}
			m, err := newMatcher(pats, matcherOpts{
				ignoreCase:   ignoreCase,
				wordRegexp:   wordRegexp,
				fixedStrings: fixedStrings,
				invert:       invert,
			})
			if err != nil {
				return err
			}
			colorMode, _ := cmd.Flags().GetString("color")
			jsonOut, _ := cmd.Flags().GetBool("json")
			quiet, _ := cmd.Flags().GetBool("quiet")
			return a.runGrep(cmd.Context(), files, m, grepOptions{
				count:        count,
				number:       number,
				onlyMatching: onlyMatching,
				filesWith:    filesWith,
				filesWithout: filesWithout,
				withFilename: withFilename,
				noFilename:   noFilename,
				recursive:    recursive,
				quiet:        quiet,
				json:         jsonOut,
				color:        useColor(colorMode),
				targetLoc:    model.LocaleID(targetLoc),
			})
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&ignoreCase, "ignore-case", "i", false, "case-insensitive matching")
	f.BoolVarP(&number, "line-number", "n", false, "prefix each match with its block number")
	f.BoolVarP(&onlyMatching, "only-matching", "o", false, "print only the matched text, not the whole block")
	f.BoolVarP(&filesWith, "files-with-matches", "l", false, "print only the names of files containing matches")
	f.BoolVarP(&filesWithout, "files-without-match", "L", false, "print only the names of files containing no match")
	f.BoolVarP(&wordRegexp, "word-regexp", "w", false, "match only whole words")
	f.BoolVarP(&fixedStrings, "fixed-strings", "F", false, "treat the pattern as a literal string, not a regexp")
	f.BoolVarP(&recursive, "recursive", "r", false, "recurse into directory arguments")
	f.BoolVarP(&withFilename, "with-filename", "H", false, "print the file name for each match")
	f.BoolVar(&noFilename, "no-filename", false, "suppress file-name prefixes on output")
	f.StringArrayVarP(&patterns, "regexp", "e", nil, "pattern to search for (repeatable; PATTERN positional not needed)")
	f.StringVar(&targetLoc, "target", "", "search the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input encoding")

	// Full classic shorthand surface — kapi's persistent flags are never inherited
	// (busybox root, or detached proxy), so -v/-c/-q are ours to define.
	f.BoolVarP(&invert, "invert-match", "v", false, "select blocks that do NOT match")
	f.BoolVarP(&count, "count", "c", false, "print a count of matching blocks per file")
	f.BoolP("quiet", "q", false, "suppress all output; exit status only")
	f.String("color", "auto", "highlight matches: auto, always, never")
	f.Bool("json", false, "emit matches as JSON")
	return cmd
}

type matcherOpts struct {
	ignoreCase   bool
	wordRegexp   bool
	fixedStrings bool
	invert       bool
}

// matcher compiles one or more patterns; a block matches when ANY pattern
// matches (then inverted if requested).
type matcher struct {
	res    []*regexp.Regexp
	invert bool
}

func newMatcher(patterns []string, o matcherOpts) (*matcher, error) {
	m := &matcher{invert: o.invert}
	for _, p := range patterns {
		expr := p
		if o.fixedStrings {
			expr = regexp.QuoteMeta(expr)
		}
		if o.wordRegexp {
			expr = `\b(?:` + expr + `)\b`
		}
		if o.ignoreCase {
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

func (m *matcher) test(s string) bool {
	hit := false
	for _, re := range m.res {
		if re.MatchString(s) {
			hit = true
			break
		}
	}
	return hit != m.invert
}

// findAll returns every matched substring across all patterns (for -o). Invert
// is not applied — there are no "matched substrings" in an inverted search.
func (m *matcher) findAll(s string) []string {
	var out []string
	for _, re := range m.res {
		out = append(out, re.FindAllString(s, -1)...)
	}
	return out
}

// spans returns match ranges for highlighting (non-inverted only).
func (m *matcher) spans(s string) [][]int {
	var spans [][]int
	for _, re := range m.res {
		spans = append(spans, re.FindAllStringIndex(s, -1)...)
	}
	return spans
}

type grepOptions struct {
	count        bool
	number       bool
	onlyMatching bool
	filesWith    bool
	filesWithout bool
	withFilename bool
	noFilename   bool
	recursive    bool
	quiet        bool
	json         bool
	color        bool
	targetLoc    model.LocaleID
}

func (a *App) runGrep(ctx context.Context, args []string, m *matcher, opts grepOptions) error {
	hadError := false
	files, err := expandInputs(args, opts.recursive, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "kgrep: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}

	// Show file-name prefixes when scanning more than one file, or when -H is
	// set; -h always suppresses them.
	showName := (len(files) > 1 || opts.withFilename) && !opts.noFilename

	anyMatch := false
	var jsonMatches []grepMatch

	for _, file := range files {
		fileCount := 0
		_, ferr := a.streamBlocks(ctx, file, func(_ int, b *model.Block) error {
			text, ok := blockScopeText(b, opts.targetLoc)
			if !ok {
				return nil
			}
			if !m.test(text) {
				return nil
			}
			anyMatch = true
			fileCount++

			if opts.quiet || opts.filesWith || opts.filesWithout || opts.count {
				return nil // counting / existence only — no per-line output
			}
			if opts.json {
				jm := grepMatch{File: displayName(file), Number: fileCount, ID: b.ID, Text: text}
				if opts.onlyMatching {
					jm.Matches = m.findAll(text)
				}
				jsonMatches = append(jsonMatches, jm)
				return nil
			}
			a.printGrepMatch(file, fileCount, b.ID, text, showName, m, opts)
			return nil
		})
		if ferr != nil {
			// A cancelled context (Ctrl-C) is a global interrupt, not a per-file
			// error: stop now and let cli.Run map it to exit 130 with no message.
			if errors.Is(ferr, context.Canceled) {
				return ferr
			}
			hadError = true
			fmt.Fprintf(os.Stderr, "kgrep: %s: %v\n", displayName(file), ferr)
			continue
		}

		switch {
		case opts.quiet || opts.json:
			// handled below / streamed above
		case opts.filesWith:
			if fileCount > 0 {
				fmt.Println(displayName(file))
			}
		case opts.filesWithout:
			if fileCount == 0 {
				fmt.Println(displayName(file))
			}
		case opts.count:
			if showName {
				fmt.Printf("%s:%d\n", displayName(file), fileCount)
			} else {
				fmt.Printf("%d\n", fileCount)
			}
		}
	}

	if opts.json && !opts.quiet {
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

func (a *App) printGrepMatch(file string, num int, id, text string, showName bool, m *matcher, opts grepOptions) {
	var b strings.Builder
	if showName {
		if opts.color {
			b.WriteString(colorFilename + displayName(file) + colorReset + colorSep + ":" + colorReset)
		} else {
			b.WriteString(displayName(file) + ":")
		}
	}
	if opts.number {
		if opts.color {
			b.WriteString(colorLineNum + strconv.Itoa(num) + colorReset + colorSep + ":" + colorReset)
		} else {
			b.WriteString(strconv.Itoa(num) + ":")
		}
	}

	if opts.onlyMatching {
		prefix := b.String()
		for _, mt := range m.findAll(text) {
			if opts.color {
				fmt.Printf("%s%s%s%s\n", prefix, colorMatch, mt, colorReset)
			} else {
				fmt.Printf("%s%s\n", prefix, mt)
			}
		}
		return
	}

	if opts.color && !m.invert {
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
