package host

import (
	"bytes"
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
// service, which writes the file through its format. A file of the project
// discovery finds is a document of that project: named by its
// project-relative path, read with the format the recipe binds and locked as
// kapi apply locks it, so the change set --print-ops prints is the one kapi
// apply applies. Any other file is read with the format detection finds, a
// file no format claims as plain text. A file is edited in place with -i;
// otherwise the change is applied to a private copy and the edited document
// is written to standard output, and the file stays as it is.
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
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	base := ChangeServiceOptions{Origin: "ksed", Format: a.FormatFlag, TargetLocale: opts.Target, PlainText: true}
	run := &sedRun{app: a, cmd: cmd, prog: prog, opts: opts, actor: changeActorOf(resolved.Actor), changes: a.newCommandChanges(cmd, recipe, base)}
	run.backup = run.changes
	if opts.BackupSuffix != "" {
		withBackup := base
		withBackup.BackupSuffix = opts.BackupSuffix
		run.backup = a.newCommandChanges(cmd, recipe, withBackup)
	}
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
	// changes holds the services ksed reads and writes files through, built
	// once for the run; backup holds those that keep a copy of each file they
	// replace (-i.bak), and is changes when no backup is asked for.
	changes, backup *commandChanges
	// printed collects the operations --print-ops prints.
	printed []change.Op
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

// stdinService is the service over the private directory a copy of standard
// input is read from.
func (r *sedRun) stdinService(ctx context.Context, dir string) (*change.Service, error) {
	return r.app.changeService(ctx, r.cmd, ChangeServiceOptions{Origin: "ksed", Root: dir, Format: r.app.FormatFlag, TargetLocale: r.opts.Target, PlainText: true})
}

// print compiles the operations the program makes on a file and keeps them
// for --print-ops. A file whose operations do not all compile contributes
// none.
func (r *sedRun) print(ctx context.Context, file string) error {
	if file == StdinName {
		path, cleanup, err := stdinDocument(ctx)
		if err != nil {
			return err
		}
		defer cleanup()
		svc, err := r.stdinService(ctx, filepath.Dir(path))
		if err != nil {
			return err
		}
		ops, err := r.compile(ctx, svc, filepath.Base(path))
		if err != nil {
			return err
		}
		for i := range ops {
			ops[i].At.Doc = StdinName
		}
		r.printed = append(r.printed, ops...)
		return nil
	}
	docs, err := r.docs(file)
	if err != nil {
		return err
	}
	var all []change.Op
	for _, path := range docs {
		svc, ref, err := r.changes.For(ctx, path)
		if err != nil {
			return err
		}
		ops, err := r.compile(ctx, svc, ref)
		if err != nil {
			return err
		}
		all = append(all, ops...)
	}
	r.printed = append(r.printed, all...)
	return nil
}

// sedArchiveCompiled, when set, is called once ksed -i has compiled the
// operations on every member of an archive and before it checks any. A test
// changes a member there, as another writer would.
var sedArchiveCompiled func(archive string)

// sedEdit is the operations the program makes on one document, to apply
// through the service that read it.
type sedEdit struct {
	svc *change.Service
	doc string
	ops []change.Op
}

// inPlace edits a file where it lies. An archive's members are edited each in
// a change set of its own, since one change set writes one archive through one
// document; every member's change is computed and checked before any is
// written, so a member that is refused leaves the archive as it was, and the
// archive is copied once, when a backup is asked for and a member changes.
func (r *sedRun) inPlace(ctx context.Context, file string) error {
	if file == StdinName {
		return errors.New("in-place editing requires a file argument")
	}
	if !isContainer(file) {
		svc, doc, err := r.backup.For(ctx, file)
		if err != nil {
			return err
		}
		ops, err := r.compile(ctx, svc, doc)
		if err != nil || len(ops) == 0 {
			return err
		}
		return r.apply(ctx, sedEdit{svc: svc, doc: doc, ops: ops}, change.ModeApply)
	}
	docs, err := r.docs(file)
	if err != nil {
		return err
	}
	var edits []sedEdit
	for _, path := range docs {
		svc, doc, err := r.changes.For(ctx, path)
		if err != nil {
			return err
		}
		ops, err := r.compile(ctx, svc, doc)
		if err != nil {
			return err
		}
		if len(ops) > 0 {
			edits = append(edits, sedEdit{svc: svc, doc: doc, ops: ops})
		}
	}
	if len(edits) == 0 {
		return nil
	}
	if sedArchiveCompiled != nil {
		sedArchiveCompiled(file)
	}
	for _, e := range edits {
		if err := r.apply(ctx, e, change.ModePreview); err != nil {
			return err
		}
	}
	if r.opts.BackupSuffix != "" {
		if err := copyFile(file, file+r.opts.BackupSuffix); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	}
	for _, e := range edits {
		if err := r.apply(ctx, e, change.ModeApply); err != nil {
			return err
		}
	}
	return nil
}

// toStdout applies the program to a private copy of the file a file argument
// names, the whole archive for an archive or one of its members, read exactly
// as the file itself is read, and writes the edited document to standard
// output: the member itself for container!entry, else the copy. The file
// stays as it is, and nothing is written when an edit is refused.
func (r *sedRun) toStdout(ctx context.Context, file string) error {
	dir, err := os.MkdirTemp("", "ksed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var (
		copyPath, member string
		edits            []sedEdit
	)
	if file == StdinName {
		data, rerr := readContent(ctx, StdinName)
		if rerr != nil {
			return rerr
		}
		// The copy has no extension, so its format is the one --format names
		// or detection finds in its content, as it is for a pipe.
		copyPath = filepath.Join(dir, "stdin")
		if err := os.WriteFile(copyPath, data, 0o600); err != nil {
			return err
		}
		svc, serr := r.stdinService(ctx, dir)
		if serr != nil {
			return serr
		}
		ops, cerr := r.compile(ctx, svc, "stdin")
		if cerr != nil {
			return cerr
		}
		edits = append(edits, sedEdit{svc: svc, doc: "stdin", ops: ops})
	} else {
		var real string
		real, member = splitLocator(file)
		copyPath = filepath.Join(dir, filepath.Base(real))
		if err := copyFile(real, copyPath); err != nil {
			return err
		}
		docs, derr := r.docs(file)
		if derr != nil {
			return derr
		}
		svc, serr := r.changes.copyService(ctx, file, copyPath, filepath.Join(dir, "locks"))
		if serr != nil {
			return serr
		}
		for _, path := range docs {
			_, doc, ferr := r.changes.For(ctx, path)
			if ferr != nil {
				return ferr
			}
			ops, cerr := r.compile(ctx, svc, doc)
			if cerr != nil {
				return cerr
			}
			edits = append(edits, sedEdit{svc: svc, doc: doc, ops: ops})
		}
	}
	for _, e := range edits {
		if len(e.ops) == 0 {
			continue
		}
		if err := r.apply(ctx, e, change.ModeApply); err != nil {
			return err
		}
	}
	var in io.Reader
	if member != "" {
		data, _, oerr := container.OpenEntry(copyPath, member)
		if oerr != nil {
			return fmt.Errorf("%s: %w", file, oerr)
		}
		in = bytes.NewReader(data)
	} else {
		f, oerr := os.Open(copyPath)
		if oerr != nil {
			return oerr
		}
		defer f.Close()
		in = f
	}
	// One guard per document: each is judged on its own opening bytes, and a
	// file that streams to the terminal must not license the next one.
	out, flush := io.Writer(os.Stdout), func() error { return nil }
	if !r.opts.Force {
		out, flush = r.app.guardedStdout()
	}
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return flush()
}

// apply applies one document's operations as one change set in mode. A
// refused operation is an error naming why; nothing in the document is
// written then.
func (r *sedRun) apply(ctx context.Context, e sedEdit, mode change.Mode) error {
	set := change.Set{Schema: change.SchemaID, Mode: mode, Gate: change.GateEnforce, Ops: e.ops}
	res, err := e.svc.Apply(ctx, set, r.actor)
	if err != nil {
		return err
	}
	switch res.Status {
	case change.SetApplied, change.SetPreviewed:
		return nil
	case change.SetPartial:
		return fmt.Errorf("the edit of %s landed in part", e.doc)
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
	return fmt.Errorf("the edit of %s was refused", e.doc)
}

// compile reads doc through svc and returns the operations the program makes
// on it: for each block whose edition the substitutions change, one
// replace_text per substitution that matched, with the edition's revision as
// if_match. The edition is the one --target names, else the one the read was
// opened on: the document's own, or the translation a translation's file
// holds. A block is addressed by the key a read reports, or, where another
// block of the document reports the same key, by the id the reader gave it,
// which the service resolves too and which names one block.
func (r *sedRun) compile(ctx context.Context, svc *change.Service, doc string) ([]change.Op, error) {
	var target model.EditionKey
	if r.opts.Target != "" {
		target = model.EditionKey{Locale: r.opts.Target}.Canonical()
	}
	type blockOps struct {
		key, id string
		ops     []change.Op
	}
	var (
		edited []blockOps
		seen   = map[string]int{}
	)
	_, err := svc.ReadEach(ctx, change.ReadRequest{Doc: doc}, func(b *model.Block, read change.BlockRead) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		seen[read.Ref.Block]++
		if !b.Translatable {
			return nil
		}
		key := read.Ref.Edition
		if r.opts.Target != "" {
			key = target
		}
		ed, ok := b.Edition(key)
		if !ok {
			return nil
		}
		at := change.Ref{Doc: read.Ref.Doc, Block: read.Ref.Block, Edition: key}
		if b.IsSourceEdition(key) {
			at.Edition = model.EditionKey{}
		}
		ops, err := r.prog.ops(at, model.EditionRevision(b, key), ed.Runs)
		if err != nil {
			return fmt.Errorf("block %s: %w", at.Block, err)
		}
		if len(ops) > 0 {
			edited = append(edited, blockOps{key: read.Ref.Block, id: b.ID, ops: ops})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []change.Op
	for _, e := range edited {
		if seen[e.key] > 1 && e.id != "" && seen[e.id] == 0 {
			for i := range e.ops {
				e.ops[i].At.Block = e.id
			}
		}
		out = append(out, e.ops...)
	}
	return out, nil
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
