package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/spf13/pflag"
)

// InPlaceFlag is the `-i` shared by `ksed` and `kapi apply`: a switch on its
// own, carrying an optional backup suffix when one is attached (`-i.bak`).
//
// pflag needs a non-empty NoOptDefVal for a flag whose value is optional —
// without one, a bare `-i` swallows the next argument as its value. That marker
// used to be a NUL-prefixed sentinel in a plain string flag, and pflag prints
// NoOptDefVal verbatim into the flag list: `--help` came out with a raw NUL in
// it, so the usage text registered as *binary* and `ksed --help | grep -i
// recursive` matched nothing at all.
//
// Declaring the type as "bool" is what fixes the display: pflag omits both the
// value placeholder and the `[=…]` suffix for a bool whose NoOptDefVal is
// "true", so the flag lists like any other switch and the optional suffix is
// explained in its description — which is how GNU sed documents `-i` too.
type InPlaceFlag struct {
	// Suffix is the backup suffix (`-i.bak` → ".bak"); empty means no backup.
	Suffix string
}

// inPlaceBare is what pflag substitutes for a bare `-i`. Writing it as a suffix
// on purpose (`-itrue`) is read as the bare form; no one names a backup "true",
// and the alternative is putting an unprintable byte back into --help.
const inPlaceBare = "true"

// Set records an attached suffix, or clears it for the bare form.
func (f *InPlaceFlag) Set(s string) error {
	if s == inPlaceBare {
		f.Suffix = ""
		return nil
	}
	f.Suffix = s
	return nil
}

func (f *InPlaceFlag) String() string { return f.Suffix }

// Type is "bool" so pflag renders `-i` as a switch; see InPlaceFlag.
func (f *InPlaceFlag) Type() string { return "bool" }

// RegisterInPlace adds the `-i/--in-place[=SUFFIX]` flag to fs and returns the
// value it writes into. Callers pair it with fs.Changed("in-place"): the flag
// being set is what turns editing on, and Suffix only says whether a backup is
// kept.
func RegisterInPlace(fs *pflag.FlagSet, usage string) *InPlaceFlag {
	v := &InPlaceFlag{}
	fs.VarP(v, "in-place", "i", usage)
	fs.Lookup("in-place").NoOptDefVal = inPlaceBare
	return v
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

// SedOptions carries one `ksed` invocation's flags.
type SedOptions struct {
	// WriteLocale selects the translation the writer emits ("" = source).
	WriteLocale model.LocaleID
	// InPlace rewrites each input rather than streaming to stdout (-i).
	InPlace bool
	// BackupSuffix keeps a copy of each rewritten input (-i.bak).
	BackupSuffix string
	// Recursive walks directory arguments. Spelled -R rather than -r, because
	// sed's own -r is --regexp-extended and quietly repurposing it would edit a
	// tree for someone who only asked for a different regexp dialect.
	Recursive bool
	// Force writes an edited document to a terminal even when it is binary,
	// which the guard in binaryout.go otherwise refuses. gzip's -f, spelled
	// long-only because ksed's -f is already --format.
	Force bool
}

func (a *App) RunSed(ctx context.Context, args []string, t *tool.BaseTool, opts SedOptions) error {
	hadError := false
	files, err := expandInputs(args, opts.Recursive, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "ksed: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}
	for _, file := range files {
		// One guard per document: each is judged on its own opening bytes, and
		// a file that streams to the terminal must not license the next one.
		out, flush := io.Writer(os.Stdout), func() error { return nil }
		if !opts.InPlace && !opts.Force {
			out, flush = a.guardedStdout()
		}
		err := a.EditDocument(ctx, file, t, opts.WriteLocale, opts.InPlace, opts.BackupSuffix, out)
		if err == nil {
			err = flush()
		}
		if err != nil {
			// A cancelled context (Ctrl-C) is a global interrupt, not a per-file
			// error: stop now and let cli.Run map it to exit 130 with no message.
			if errors.Is(err, context.Canceled) {
				return err
			}
			// The refusal replaces the write-path chain it arrived in rather
			// than nesting inside it: the file is already named by the prefix
			// below, and "write x.docx: write x.docx: …" tells no one anything.
			if errors.Is(err, ErrBinaryStdout) {
				err = errBinaryStdout("use -i to edit the file in place, redirect stdout to a file, or pass --force")
			}
			hadError = true
			fmt.Fprintf(os.Stderr, "ksed: %s: %v\n", DisplayName(file), err)
		}
	}
	if hadError {
		// A read/process error occurred (messages already printed per file);
		// exit 2 (trouble), matching the grep-style toolbox contract and kgrep.
		return WithExitCode(ExitUsage, ErrSilentExit)
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

// applyRuns runs the program over a flat run sequence, editing the text while
// preserving the inline codes around the change (model.ApplyTextEdits). It
// returns the rewritten runs and whether the flattened text actually changed;
// when nothing changes the original runs are returned untouched (no needless
// re-chunking). Callers must guard with model.HasStructuredRuns: plural/select
// runs have no linear text mapping and take the whole-text fallback instead.
func (p sedProgram) applyRuns(runs []model.Run) ([]model.Run, bool) {
	before := model.RunsText(runs)
	cur := runs
	for _, c := range p {
		cur = c.editRuns(cur)
	}
	if model.RunsText(cur) == before {
		return runs, false
	}
	return cur, true
}

// editRuns turns one substitution into a set of constraint-aware text edits and
// applies them to the run sequence, so codes around the change are preserved
// and an emptied deletable span (e.g. bold left wrapping nothing) collapses
// rather than leaving an empty tag (see model.ApplyTextEdits). If nothing
// matches, the input is returned unchanged.
func (c sedCmd) editRuns(runs []model.Run) []model.Run {
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
	src := []byte(text)
	edits := make([]model.TextEdit, 0, len(matches))
	for _, m := range matches {
		repl := c.re.Expand(nil, []byte(c.repl), src, m)
		edits = append(edits, model.TextEdit{Start: m[0], End: m[1], Replacement: string(repl)})
	}
	return model.ApplyTextEdits(runs, edits)
}

func ParseSedProgram(scripts []string) (sedProgram, error) {
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

// NewSedTool builds a source-transform tool that applies the sed program to the
// source text (scopeSource) or to the target translation for locale otherwise.
//
// Editing is run-aware: substitutions edit the run sequence so inline codes
// (bold/link spans, placeholders) around the change survive, an emptied
// deletable span collapses, and a non-deletable code (line break, variable) is
// kept — all per the vocabulary constraints (see model.ApplyTextEdits). A match
// may even span a code boundary, because the regex sees the code-free flattening
// of the runs. Sequences with plural/select runs have no linear text mapping, so
// those fall back to whole-text replacement.
func NewSedTool(prog sedProgram, locale model.LocaleID, scopeSource bool) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "ksed",
		ToolDescription: "stream editor for text/content",
	}
	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		var plan tool.EditPlan
		if !v.Translatable() {
			return plan, nil
		}
		if scopeSource {
			runs := v.SourceRuns()
			if !model.HasStructuredRuns(runs) {
				if out, changed := prog.applyRuns(runs); changed {
					plan.NewRuns = out
					plan.Edits = tool.FullSpanEdit(runs, out)
				}
				return plan, nil
			}
			src := v.SourceText()
			if out := prog.apply(src); out != src {
				plan.ReplaceAll = &out
			}
			return plan, nil
		}
		if locale.IsEmpty() || !v.HasTarget(locale) {
			return plan, nil
		}
		runs := v.TargetRuns(locale)
		if !model.HasStructuredRuns(runs) {
			if out, changed := prog.applyRuns(runs); changed {
				plan.SetTarget(locale, out)
			}
			return plan, nil
		}
		tgt := v.TargetText(locale)
		if out := prog.apply(tgt); out != tgt {
			plan.SetTarget(locale, []model.Run{{Text: &model.TextRun{Text: out}}})
		}
		return plan, nil
	}
	return t
}
