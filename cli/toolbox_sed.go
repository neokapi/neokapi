package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/spf13/cobra"
)

// sentinelInPlace marks "-i given with no backup suffix"; cobra's NoOptDefVal
// distinguishes a bare -i from one carrying a SUFFIX (-i.bak).
const sentinelInPlace = "\x00nosuffix"

// newSedCmd builds the sed command. Used as the standalone `ksed` root and,
// via newToolboxProxies, behind the detached `kapi sed` subcommand.
func (a *App) newSedCmd() *cobra.Command {
	var (
		scripts   []string
		targetLoc string
	)

	cmd := &cobra.Command{
		Use:     "sed [flags] SCRIPT [FILE...]",
		Short:   "Stream-edit the translatable text of files (s/regexp/replacement/)",
		GroupID: "content",
		Long: `Apply sed-style substitutions to the human-readable text inside any supported
format, then write the document back in the same format. Only the editable text
changes — a .docx keeps its styles, a JSON catalog keeps its keys and shape.

SCRIPT is a substitution command: s/regexp/replacement/flags. Backreferences
(\1, &), and the g (global) and i (ignore-case) flags are supported. Any
single-byte delimiter works (s|a|b|). Pass several with repeated -e.

By default the edited document is written to standard output (like sed); use -i
to edit files in place, optionally keeping a backup (-i.bak). Edits apply to the
source text unless --target LOCALE selects a translation.

With no FILE, or when FILE is "-", standard input is read.`,
		Example: `  ksed 's/colour/color/g' guide.md
  ksed -i 's/Inc\./LLC/' *.docx
  ksed -i.bak -e 's/v1/v2/g' -e 's/beta//' locales/en.json
  ksed --target fr 's/Bonjour/Salut/g' messages.xliff`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scriptStrs := scripts
			files := args
			if len(scriptStrs) == 0 {
				if len(args) == 0 {
					return errors.New("no script given")
				}
				scriptStrs = []string{args[0]}
				files = args[1:]
			}
			prog, err := parseSedProgram(scriptStrs)
			if err != nil {
				return err
			}

			inPlace := cmd.Flags().Changed("in-place")
			backupSuffix := ""
			if inPlace {
				if v, _ := cmd.Flags().GetString("in-place"); v != sentinelInPlace {
					backupSuffix = v
				}
			}

			loc := model.LocaleID(targetLoc)
			scopeSource := loc == ""
			t := newSedTool(prog, loc, scopeSource)
			writeLocale := loc // "" for source round-trip

			return a.runSed(cmd.Context(), files, t, writeLocale, inPlace, backupSuffix)
		},
	}

	f := cmd.Flags()
	f.StringArrayVarP(&scripts, "expression", "e", nil, "add a substitution script (repeatable; SCRIPT positional not needed)")
	f.StringVar(&targetLoc, "target", "", "edit the target translation for LOCALE instead of the source")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input/output format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input/output encoding")

	// -i takes an OPTIONAL backup suffix: `-i` (no backup) or `-i.bak`.
	f.StringP("in-place", "i", "", "edit files in place; append a backup SUFFIX if given (e.g. -i.bak)")
	f.Lookup("in-place").NoOptDefVal = sentinelInPlace

	return cmd
}

// NormalizeSedInPlaceArgs rewrites sed's attached backup form `-iSUFFIX`
// (e.g. `-i.bak`) into pflag's `--in-place=SUFFIX`, which pflag's optional-value
// shorthand parsing cannot express directly. Bare `-i`, `-i=...`, and any
// `--in-place...` token pass through unchanged. Applied only on the sed path
// (busybox ksed, or `kapi sed`), so it never touches another command's `-i`.
func NormalizeSedInPlaceArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "-i") && !strings.HasPrefix(a, "--") && len(a) > 2 && a[2] != '=' {
			out[i] = "--in-place=" + a[2:]
			continue
		}
		out[i] = a
	}
	return out
}

func (a *App) runSed(ctx context.Context, args []string, t *tool.BaseTool, writeLocale model.LocaleID, inPlace bool, backupSuffix string) error {
	files, err := expandInputs(args, false, func(path string, err error) {
		fmt.Fprintf(os.Stderr, "ksed: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}
	var firstErr error
	for _, file := range files {
		if err := a.editDocument(ctx, file, t, writeLocale, inPlace, backupSuffix, os.Stdout); err != nil {
			// A cancelled context (Ctrl-C) is a global interrupt, not a per-file
			// error: stop now and let cli.Run map it to exit 130 with no message.
			if errors.Is(err, context.Canceled) {
				return err
			}
			fmt.Fprintf(os.Stderr, "ksed: %s: %v\n", displayName(file), err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return ErrSilentExit // message already printed per file
	}
	return nil
}

// --- sed s/// program ---------------------------------------------------------

// sedCmd is one compiled substitution.
type sedCmd struct {
	re     *regexp.Regexp
	repl   string // Go-style replacement ($1, ${name})
	global bool
}

type sedProgram []sedCmd

// apply runs every substitution over text in order.
func (p sedProgram) apply(text string) string {
	for _, c := range p {
		if c.global {
			text = c.re.ReplaceAllString(text, c.repl)
			continue
		}
		// Replace the first match only.
		loc := c.re.FindStringIndex(text)
		if loc == nil {
			continue
		}
		text = text[:loc[0]] + c.re.ReplaceAllString(text[loc[0]:loc[1]], c.repl) + text[loc[1]:]
	}
	return text
}

// applyRuns runs the program over a flat run sequence, preserving the inline
// codes (placeholders and paired codes) that surround the edited text rather
// than flattening the block to a single text run. It returns the rewritten
// runs and whether the flattened text actually changed; when nothing changes
// the original runs are returned untouched (no needless re-chunking).
//
// Only flat sequences are spliceable — see runsSpliceable. Plural/select runs
// carry text inside nested structure that has no linear position in the
// flattened string, so callers fall back to text replacement for those.
func (p sedProgram) applyRuns(runs []model.Run) ([]model.Run, bool) {
	before := model.RunsText(runs)
	cur := runs
	for _, c := range p {
		cur = c.spliceRuns(cur)
	}
	if model.RunsText(cur) == before {
		return runs, false
	}
	return cur, true
}

// codeRunAt is an inline-code run (ph / pcOpen / pcClose / sub) paired with its
// byte position in the flattened text — the number of text bytes that precede
// it. Codes are zero-width, so several may share one position.
type codeRunAt struct {
	pos int
	run model.Run
}

// collectCodeRuns lists every inline-code run with its byte position in the
// flattened text, in document order. Text runs only advance the position.
func collectCodeRuns(runs []model.Run) []codeRunAt {
	var codes []codeRunAt
	pos := 0
	for _, r := range runs {
		if r.Text != nil {
			pos += len(r.Text.Text)
			continue
		}
		codes = append(codes, codeRunAt{pos: pos, run: r})
	}
	return codes
}

// runsSpliceable reports whether a run sequence can be spliced position-wise.
// Plural/select runs draw their flattened text from nested forms, so a byte
// offset into the flattening does not map back to a single run — those are
// edited via whole-text replacement instead.
func runsSpliceable(runs []model.Run) bool {
	for _, r := range runs {
		if r.Plural != nil || r.Select != nil {
			return false
		}
	}
	return true
}

// spliceRuns applies one substitution to a flat run sequence, replacing only
// the matched text spans while weaving every original inline code back into
// the result. Codes on a match boundary stay on their side of the replacement;
// codes strictly inside a replaced span are carried over (in order) just after
// the replacement, so a paired code never loses its partner and overall code
// order is preserved. If nothing matches, the input is returned unchanged.
func (c sedCmd) spliceRuns(runs []model.Run) []model.Run {
	text := model.RunsText(runs)
	var matches [][]int
	if c.global {
		matches = c.re.FindAllStringSubmatchIndex(text, -1)
	} else if m := c.re.FindStringSubmatchIndex(text); m != nil {
		matches = [][]int{m}
	}
	if len(matches) == 0 {
		return runs
	}

	codes := collectCodeRuns(runs)
	src := []byte(text)
	out := make([]model.Run, 0, len(runs)+2*len(matches))
	ci := 0

	emitText := func(lo, hi int) {
		if lo < hi {
			out = append(out, model.Run{Text: &model.TextRun{Text: text[lo:hi]}})
		}
	}
	// emitGap emits text [from,to) interleaved with codes positioned in [from,to).
	emitGap := func(from, to int) {
		seg := from
		for ci < len(codes) && codes[ci].pos < to {
			p := codes[ci].pos
			if p < from { // already consumed by an earlier boundary
				ci++
				continue
			}
			emitText(seg, p)
			for ci < len(codes) && codes[ci].pos == p {
				out = append(out, codes[ci].run)
				ci++
			}
			seg = p
		}
		emitText(seg, to)
	}
	// emitCodesAt emits every code sitting exactly at pos.
	emitCodesAt := func(pos int) {
		for ci < len(codes) && codes[ci].pos == pos {
			out = append(out, codes[ci].run)
			ci++
		}
	}
	// emitCodesBefore emits codes positioned strictly below hi (the interior of
	// a match, once its left boundary has been consumed).
	emitCodesBefore := func(hi int) {
		for ci < len(codes) && codes[ci].pos < hi {
			out = append(out, codes[ci].run)
			ci++
		}
	}

	cursor := 0
	for _, m := range matches {
		s, e := m[0], m[1]
		emitGap(cursor, s) // untouched text + its codes
		emitCodesAt(s)     // left-boundary codes, before the replacement
		repl := c.re.Expand(nil, []byte(c.repl), src, m)
		if len(repl) > 0 {
			out = append(out, model.Run{Text: &model.TextRun{Text: string(repl)}})
		}
		emitCodesBefore(e) // interior codes, carried over after the replacement
		emitCodesAt(e)     // right-boundary codes, after the replacement
		cursor = e
	}
	emitGap(cursor, len(text)) // trailing text + its codes
	emitCodesAt(len(text))     // codes pinned to the very end

	return mergeAdjacentText(out)
}

// mergeAdjacentText coalesces consecutive text runs (which splicing can produce
// where a replacement abuts untouched text) into one, leaving code runs as the
// only thing that separates text. New TextRun values are allocated so the
// caller's input runs are never mutated in place.
func mergeAdjacentText(runs []model.Run) []model.Run {
	out := make([]model.Run, 0, len(runs))
	for _, r := range runs {
		if r.Text != nil && len(out) > 0 && out[len(out)-1].Text != nil {
			out[len(out)-1].Text = &model.TextRun{Text: out[len(out)-1].Text.Text + r.Text.Text}
			continue
		}
		out = append(out, r)
	}
	return out
}

func parseSedProgram(scripts []string) (sedProgram, error) {
	prog := make(sedProgram, 0, len(scripts))
	for _, s := range scripts {
		c, err := parseSedCmd(s)
		if err != nil {
			return nil, err
		}
		prog = append(prog, c)
	}
	if len(prog) == 0 {
		return nil, errors.New("no script given")
	}
	return prog, nil
}

// parseSedCmd parses a single `s<delim>pattern<delim>replacement<delim>flags`
// command with an arbitrary single-byte delimiter.
func parseSedCmd(script string) (sedCmd, error) {
	s := strings.TrimSpace(script)
	if len(s) < 3 || s[0] != 's' {
		return sedCmd{}, fmt.Errorf("unsupported script %q: only s/regexp/replacement/ substitution is supported", script)
	}
	delim := s[1]
	if delim == '\\' || delim == '\n' {
		return sedCmd{}, fmt.Errorf("invalid delimiter in %q", script)
	}
	pat, repl, flags, err := splitSed(s[2:], delim)
	if err != nil {
		return sedCmd{}, fmt.Errorf("%w in %q", err, script)
	}

	global := strings.ContainsRune(flags, 'g')
	var prefix string
	if strings.ContainsAny(flags, "iI") {
		prefix += "(?i)"
	}
	if strings.ContainsRune(flags, 'm') {
		prefix += "(?m)"
	}
	if strings.ContainsRune(flags, 's') {
		prefix += "(?s)"
	}
	re, err := regexp.Compile(prefix + pat)
	if err != nil {
		return sedCmd{}, fmt.Errorf("invalid regexp %q: %w", pat, err)
	}
	return sedCmd{re: re, repl: sedReplToGo(repl, delim), global: global}, nil
}

// splitSed splits "pattern<delim>replacement<delim>flags" honouring
// backslash-escaped delimiters; the pattern and replacement keep their escapes
// for later interpretation.
func splitSed(s string, delim byte) (pat, repl, flags string, err error) {
	fields := make([]string, 0, 2)
	var cur strings.Builder
	i := 0
	for i < len(s) && len(fields) < 2 {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			cur.WriteByte(c)
			cur.WriteByte(s[i+1])
			i += 2
			continue
		}
		if c == delim {
			fields = append(fields, cur.String())
			cur.Reset()
			i++
			continue
		}
		cur.WriteByte(c)
		i++
	}
	if len(fields) < 2 {
		return "", "", "", errors.New("unterminated `s` command")
	}
	flags = strings.TrimRight(s[i:], " \t\n;")
	return fields[0], fields[1], flags, nil
}

// sedReplToGo converts a sed replacement into Go regexp.ReplaceAllString form:
// \1 → ${1}, & → ${0}, escaped delimiter/&/backslash become literals, \n / \t
// expand, and literal $ is escaped to $$.
func sedReplToGo(s string, delim byte) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			if i+1 >= len(s) {
				b.WriteByte('\\')
				continue
			}
			n := s[i+1]
			i++
			switch {
			case n >= '0' && n <= '9':
				b.WriteString("${")
				b.WriteByte(n)
				b.WriteByte('}')
			case n == 'n':
				b.WriteByte('\n')
			case n == 't':
				b.WriteByte('\t')
			case n == '&':
				b.WriteByte('&')
			case n == '\\':
				b.WriteByte('\\')
			case n == delim:
				b.WriteByte(delim)
			default:
				b.WriteByte(n) // sed: \x is literal x
			}
		case '&':
			b.WriteString("${0}")
		case '$':
			b.WriteString("$$")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// newSedTool builds a source-transform tool that applies the sed program to the
// source text (scopeSource) or to the target translation for locale otherwise.
//
// Editing is run-aware: substitutions splice into the run sequence so the inline
// codes (bold/link spans, placeholders) around the edited text survive — a match
// may even span a code boundary, because the regex sees the code-free flattening
// of the runs (see sedProgram.applyRuns). Sequences with plural/select runs have
// no linear text mapping, so those fall back to whole-text replacement.
func newSedTool(prog sedProgram, locale model.LocaleID, scopeSource bool) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "ksed",
		ToolDescription: "stream editor for translatable text",
	}
	t.Transform = func(v tool.SourceView) error {
		if !v.Translatable() {
			return nil
		}
		if scopeSource {
			runs := v.SourceRuns()
			if runsSpliceable(runs) {
				if out, changed := prog.applyRuns(runs); changed {
					v.SetSourceRuns(out)
				}
				return nil
			}
			src := v.SourceText()
			if out := prog.apply(src); out != src {
				v.SetSourceText(out)
			}
			return nil
		}
		if locale.IsEmpty() || !v.HasTarget(locale) {
			return nil
		}
		runs := v.TargetRuns(locale)
		if runsSpliceable(runs) {
			if out, changed := prog.applyRuns(runs); changed {
				v.SetTargetRuns(locale, out)
			}
			return nil
		}
		tgt := v.TargetText(locale)
		if out := prog.apply(tgt); out != tgt {
			v.SetTargetText(locale, out)
		}
		return nil
	}
	return t
}
