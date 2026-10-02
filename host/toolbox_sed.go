package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/model"
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
	// Target names the edition the substitutions edit, a translation a
	// bilingual file holds; empty edits the document's own text.
	Target model.LocaleID
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
	// PrintOps prints the change set the substitutions compile to, one
	// replace_text operation per edited block and substitution, and changes
	// nothing.
	PrintOps bool
}

// RunSed applies a sed program to each file args name. ksed compiles its
// substitutions into replace_text operations, each carrying the revision of
// the edition it read as if_match, and applies them through the change
// service, which writes the file through its format. A file is edited in
// place with -i; otherwise the change is applied to a private copy and the
// edited document is written to standard output, and the file stays as it is.
func (a *App) RunSed(ctx context.Context, cmd Command, args []string, prog sedProgram, opts SedOptions) error {
	hadError := false
	files, err := expandInputs(args, opts.Recursive, func(path string, err error) {
		hadError = true
		fmt.Fprintf(os.Stderr, "ksed: %s: %v\n", path, err)
	})
	if err != nil {
		return err
	}
	a.InitRegistries()
	resolved, err := a.commandActor()
	if err != nil {
		return err
	}
	run := &sedRun{app: a, cmd: cmd, prog: prog, opts: opts, actor: changeActorOf(resolved.Actor)}
	for _, file := range files {
		err := run.file(ctx, file)
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
	if opts.PrintOps {
		set := change.Set{Schema: change.SchemaID, Mode: change.ModeApply, Gate: change.GateEnforce, Ops: run.printed}
		if set.Ops == nil {
			set.Ops = []change.Op{}
		}
		if err := writeChangeSet(os.Stdout, set); err != nil {
			return err
		}
	}
	if hadError {
		// A read/process error occurred (messages already printed per file);
		// exit 2 (trouble), matching the grep-style toolbox contract and kgrep.
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}

// sedRun is one ksed invocation.
type sedRun struct {
	app   *App
	cmd   Command
	prog  sedProgram
	opts  SedOptions
	actor change.Actor
	// dir is the service for files named on the command line, outside any
	// project.
	dir *change.Service
	// printed collects the operations --print-ops prints.
	printed []change.Op
}

// service builds the change service ksed reads and writes files through:
// outside any project, references from the working directory, or from root
// when it is set, with backup keeping a copy of each file it replaces.
func (r *sedRun) service(ctx context.Context, root, backup string) (*change.Service, error) {
	opts := ChangeServiceOptions{Origin: "ksed", Format: r.app.FormatFlag, TargetLocale: r.opts.Target, BackupSuffix: backup}
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		opts.Root, opts.AnyPath = wd, true
	} else {
		opts.Root = root
	}
	return r.app.changeService(ctx, r.cmd, opts)
}

// file runs the program over one file argument: a file, an archive, one
// member of an archive (container!entry), or "-" for standard input.
func (r *sedRun) file(ctx context.Context, file string) error {
	switch {
	case r.opts.PrintOps:
		return r.print(ctx, file)
	case r.opts.InPlace:
		return r.inPlace(ctx, file)
	}
	return r.toStdout(ctx, file)
}

// docs are the documents a file argument names: each eligible member of an
// archive, or the file itself.
func (r *sedRun) docs(file string) ([]string, error) {
	if isContainer(file) {
		return r.app.containerMembers(file)
	}
	return []string{file}, nil
}

// print compiles the operations the program makes on a file and keeps them
// for --print-ops.
func (r *sedRun) print(ctx context.Context, file string) error {
	if file == StdinName {
		path, cleanup, err := stdinDocument(ctx)
		if err != nil {
			return err
		}
		defer cleanup()
		svc, err := r.service(ctx, filepath.Dir(path), "")
		if err != nil {
			return err
		}
		ops, err := r.compile(ctx, svc, filepath.Base(path))
		for i := range ops {
			ops[i].At.Doc = StdinName
		}
		r.printed = append(r.printed, ops...)
		return err
	}
	docs, err := r.docs(file)
	if err != nil {
		return err
	}
	if r.dir == nil {
		if r.dir, err = r.service(ctx, "", ""); err != nil {
			return err
		}
	}
	for _, path := range docs {
		ops, err := r.compile(ctx, r.dir, r.ref(path))
		r.printed = append(r.printed, ops...)
		if err != nil {
			return err
		}
	}
	return nil
}

// ref is the reference of a file argument in the working-directory service.
func (r *sedRun) ref(path string) string {
	file, member := path, ""
	if loc, ok := parseEntryLocator(path); ok {
		file, member = loc.Archive, loc.Entry
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	return withMember(relRef(wd, abs), member)
}

// inPlace edits a file where it lies. An archive is copied once before its
// first member is edited, when a backup is asked for, and each member is
// edited in a change set of its own.
func (r *sedRun) inPlace(ctx context.Context, file string) error {
	if file == StdinName {
		return errors.New("in-place editing requires a file argument")
	}
	if isContainer(file) {
		docs, err := r.docs(file)
		if err != nil {
			return err
		}
		if r.opts.BackupSuffix != "" {
			if err := copyFile(file, file+r.opts.BackupSuffix); err != nil {
				return fmt.Errorf("write backup: %w", err)
			}
		}
		svc, err := r.service(ctx, "", "")
		if err != nil {
			return err
		}
		for _, path := range docs {
			if err := r.apply(ctx, svc, r.ref(path)); err != nil {
				return err
			}
		}
		return nil
	}
	svc, err := r.service(ctx, "", r.opts.BackupSuffix)
	if err != nil {
		return err
	}
	return r.apply(ctx, svc, r.ref(file))
}

// toStdout applies the program to a private copy of the document a file
// argument names (the member itself for container!entry, the whole archive
// for an archive) and writes the edited copy to standard output.
func (r *sedRun) toStdout(ctx context.Context, file string) error {
	dir, err := os.MkdirTemp("", "ksed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var copyPath string
	switch loc, member := parseEntryLocator(file); {
	case file == StdinName:
		data, rerr := readContent(ctx, StdinName)
		if rerr != nil {
			return rerr
		}
		// The copy has no extension, so its format is the one --format names
		// or detection finds in its content, as it is for a pipe.
		copyPath = filepath.Join(dir, "stdin")
		err = os.WriteFile(copyPath, data, 0o600)
	case member:
		content, _, oerr := container.OpenEntry(loc.Archive, loc.Entry)
		if oerr != nil {
			return fmt.Errorf("%s!%s: %w", loc.Archive, loc.Entry, oerr)
		}
		copyPath = filepath.Join(dir, filepath.Base(filepath.FromSlash(loc.Entry)))
		err = os.WriteFile(copyPath, content, 0o600)
	default:
		copyPath = filepath.Join(dir, filepath.Base(file))
		err = copyFile(file, copyPath)
	}
	if err != nil {
		return err
	}
	svc, err := r.service(ctx, dir, "")
	if err != nil {
		return err
	}
	docs, err := r.docs(copyPath)
	if err != nil {
		return err
	}
	for _, path := range docs {
		ref, rerr := filepath.Rel(dir, strings.SplitN(path, "!", 2)[0])
		if rerr != nil {
			return rerr
		}
		if loc, ok := parseEntryLocator(path); ok {
			ref += "!" + loc.Entry
		}
		if err := r.apply(ctx, svc, filepath.ToSlash(ref)); err != nil {
			return err
		}
	}
	// One guard per document: each is judged on its own opening bytes, and a
	// file that streams to the terminal must not license the next one.
	out, flush := io.Writer(os.Stdout), func() error { return nil }
	if !r.opts.Force {
		out, flush = r.app.guardedStdout()
	}
	in, err := os.Open(copyPath)
	if err != nil {
		return err
	}
	defer in.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return flush()
}

// apply compiles the program's operations on doc and applies them as one
// change set. A refused operation is an error naming why; nothing in the
// document is written then.
func (r *sedRun) apply(ctx context.Context, svc *change.Service, doc string) error {
	ops, err := r.compile(ctx, svc, doc)
	if err != nil || len(ops) == 0 {
		return err
	}
	set := change.Set{Schema: change.SchemaID, Mode: change.ModeApply, Gate: change.GateEnforce, Ops: ops}
	res, err := svc.Apply(ctx, set, r.actor)
	if err != nil {
		return err
	}
	switch res.Status {
	case change.SetApplied:
		return nil
	case change.SetPartial:
		return fmt.Errorf("the edit of %s landed in part", doc)
	}
	for _, op := range res.Ops {
		if op.Status == change.OpRefused && op.Error != nil {
			at := ""
			if op.At != nil {
				at = " " + op.At.Block
			}
			return fmt.Errorf("block%s: %w", at, op.Error)
		}
	}
	return fmt.Errorf("the edit of %s was refused", doc)
}

// compile reads doc through svc and returns the operations the program makes
// on it: for each block whose edition (the document's own, or --target's)
// the substitutions change, one replace_text per substitution that matched,
// with the edition's revision as if_match.
func (r *sedRun) compile(ctx context.Context, svc *change.Service, doc string) ([]change.Op, error) {
	var key model.EditionKey
	if r.opts.Target != "" {
		key = model.EditionKey{Locale: r.opts.Target}.Canonical()
	}
	var ops []change.Op
	_, err := svc.ReadEach(ctx, change.ReadRequest{Doc: doc}, func(b *model.Block, read change.BlockRead) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !b.Translatable {
			return nil
		}
		ed, ok := b.Edition(key)
		if !ok {
			return nil
		}
		at := change.Ref{Doc: read.Ref.Doc, Block: read.Ref.Block, Edition: key}
		if b.IsSourceEdition(key) {
			at.Edition = model.EditionKey{}
		}
		blockOps, err := r.prog.ops(at, model.EditionRevision(b, key), ed.Runs)
		if err != nil {
			return fmt.Errorf("block %s: %w", at.Block, err)
		}
		ops = append(ops, blockOps...)
		return nil
	})
	return ops, err
}

// --- sed s/// program ---------------------------------------------------------

// sedCmd is one compiled substitution.
type sedCmd struct {
	re     *regexp.Regexp
	repl   string // Go-style replacement ($1, ${name})
	global bool
}

type sedProgram []sedCmd

// ops compiles the program over an edition's runs into replace_text
// operations at at, guarded by rev. Each substitution that matches gives one
// operation, whose edits are positions in the content the substitutions
// before it left, as the change service applies operations in order.
func (p sedProgram) ops(at change.Ref, rev string, runs []model.Run) ([]change.Op, error) {
	cur := runs
	var out []change.Op
	for i, c := range p {
		first := true
		edits := c.edits(cur, nil, &first)
		if len(edits) == 0 {
			continue
		}
		op := change.Op{Kind: change.KindReplaceText, At: at, IfMatch: rev, Body: &change.ReplaceText{Edits: edits}}
		out = append(out, op)
		if i == len(p)-1 {
			break
		}
		next, err := applySed(cur, op)
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return out, nil
}

// applySed applies one compiled operation to runs, through the applier the
// change service uses, so the next substitution matches the text the
// service will have.
func applySed(runs []model.Run, op change.Op) ([]model.Run, error) {
	b := model.NewRunsBlock("ksed", runs)
	sim := op
	sim.At = change.Ref{}
	sim.IfMatch = change.AnyRevision
	res := change.ApplyBlock(b, []change.Op{sim}, change.BlockEnv{Actor: change.Actor{Kind: change.ActorPerson}})
	if len(res) == 1 && res[0].Error != nil {
		return nil, res[0].Error
	}
	ed, _ := b.Edition(model.EditionKey{})
	return ed.Runs, nil
}

// edits are the text edits the substitution makes in seq, the run sequence
// path reaches, and in the branches of every plural and select it holds. A
// match is found in the sequence's own text, in which inline codes, plurals
// and selects have no width, so a match may span an inline code and the
// codes around a change are kept; a match that would swallow a plural or a
// select is left alone. Without g, only the first match is replaced; first
// says whether it is still to come.
func (c sedCmd) edits(seq []model.Run, path model.RunPath, first *bool) []change.TextEdit {
	if !c.global && !*first {
		return nil
	}
	text := model.SequenceText(seq)
	var matches [][]int
	if c.global {
		matches = c.re.FindAllStringSubmatchIndex(text, -1)
	} else if m := c.re.FindStringSubmatchIndex(text); m != nil {
		matches = [][]int{m}
	}
	var structures []int
	off := 0
	for _, r := range seq {
		switch {
		case r.Text != nil:
			off += utf8.RuneCountInString(r.Text.Text)
		case r.Plural != nil, r.Select != nil:
			structures = append(structures, off)
		}
	}
	src := []byte(text)
	var out []change.TextEdit
	// The matches are byte offsets; a text edit counts code points.
	byteAt, runeAt := 0, 0
	toRunes := func(b int) int {
		runeAt += utf8.RuneCount(src[byteAt:b])
		byteAt = b
		return runeAt
	}
	for _, m := range matches {
		repl := string(c.re.Expand(nil, []byte(c.repl), src, m))
		start, end := toRunes(m[0]), toRunes(m[1])
		if repl == text[m[0]:m[1]] {
			continue
		}
		if slices.ContainsFunc(structures, func(at int) bool { return start < at && at < end }) {
			continue
		}
		s, e := start, end
		out = append(out, change.TextEdit{Path: slices.Clone(path), Start: &s, End: &e, Text: repl})
		*first = false
	}
	for i, r := range seq {
		step := model.RunPathStep{Kind: model.StepIndex, Index: i}
		switch {
		case r.Plural != nil:
			for _, form := range slices.Sorted(maps.Keys(r.Plural.Forms)) {
				branch := append(slices.Clone(path), step, model.RunPathStep{Kind: model.StepPlural, PluralForm: form})
				out = append(out, c.edits(r.Plural.Forms[form], branch, first)...)
			}
		case r.Select != nil:
			for _, value := range slices.Sorted(maps.Keys(r.Select.Cases)) {
				branch := append(slices.Clone(path), step, model.RunPathStep{Kind: model.StepSelect, SelectValue: value})
				out = append(out, c.edits(r.Select.Cases[value], branch, first)...)
			}
		}
	}
	return out
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
